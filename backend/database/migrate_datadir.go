package database

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"agent_center/backend/paths"
)

// MigrationReport describes what a legacy adoption moved.
type MigrationReport struct {
	From       string
	Files      []string
	DatabaseOK bool
}

// AdoptLegacyDataDir copies state from an older location into target the first
// time the new location is used. It returns a report, or nil when there was
// nothing to adopt.
//
// The legacy copy is never deleted. If anything here is wrong, the user's
// original state is still sitting where it always was.
func AdoptLegacyDataDir(target string) (*MigrationReport, error) {
	for _, legacy := range paths.LegacyDataDirs() {
		report, err := adoptFrom(legacy, target)
		if err != nil {
			return nil, err
		}
		if report != nil {
			return report, nil
		}
	}
	return nil, nil
}

// adoptFrom copies one specific legacy directory into target. It reports nil
// when there is nothing to do: no legacy database, or a target that already
// holds one.
func adoptFrom(legacy, target string) (*MigrationReport, error) {
	targetDB := filepath.Join(target, paths.DatabaseFile)
	if _, err := os.Stat(targetDB); err == nil {
		// Already populated: adopting now would overwrite live data.
		return nil, nil
	}

	legacyDB := filepath.Join(legacy, paths.DatabaseFile)
	if _, err := os.Stat(legacyDB); err != nil {
		return nil, nil
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir %s: %w", target, err)
	}

	report := &MigrationReport{From: legacy}
	if err := copyDatabase(legacyDB, targetDB); err != nil {
		return nil, fmt.Errorf("copy database from %s: %w", legacy, err)
	}
	report.DatabaseOK = true

	for _, name := range paths.StateFiles {
		if copyIfMissing(filepath.Join(legacy, name), filepath.Join(target, name)) {
			report.Files = append(report.Files, name)
		}
	}
	for _, name := range paths.StateDirs {
		if copyTreeIfMissing(filepath.Join(legacy, name), filepath.Join(target, name)) {
			report.Files = append(report.Files, name+string(filepath.Separator))
		}
	}
	return report, nil
}

// copyDatabase writes a single consistent database file at dst.
//
// Copying the .db file on its own is not enough and not a partial loss either:
// in WAL mode the recent pages live in the -wal sidecar, and a bare copy can
// come out with no tables at all. VACUUM INTO asks SQLite to write one complete
// file including whatever is still in the WAL, and it refuses to overwrite an
// existing file — which is exactly the guard wanted here.
func copyDatabase(src, dst string) error {
	db, err := sql.Open("sqlite", src)
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.Exec(`VACUUM INTO ?`, dst); err != nil {
		return err
	}
	return nil
}

func copyIfMissing(src, dst string) bool {
	if _, err := os.Stat(dst); err == nil {
		return false
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return false
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return false
	}
	return true
}

func copyTreeIfMissing(src, dst string) bool {
	if _, err := os.Stat(dst); err == nil {
		return false
	}
	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		return false
	}
	if err := copyTree(src, dst); err != nil {
		return false
	}
	return true
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		w, err := os.Create(out)
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = io.Copy(w, in)
		return err
	})
}
