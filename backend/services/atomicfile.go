package services

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WriteFileAtomic writes data to path without leaving a half-written file behind.
//
// os.WriteFile truncates first and then writes. Lose power, run out of disk, or
// crash between the two and the file is empty or cut short — which for these
// stores means every saved account, every schedule, or the day's spending is
// gone, and the app reads back a parse error on next launch. Writing to a
// temporary file in the same directory and renaming it over the target makes the
// swap atomic: a reader sees either the old contents or the new ones.
//
// The temporary file is created alongside the target rather than in the system
// temp directory, because rename is only atomic within a filesystem.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temp file next to %s: %w", path, err)
	}
	tempName := temp.Name()

	// Any failure from here leaves the original untouched; the temporary file is
	// removed rather than left as litter.
	cleanup := func() { _ = temp.Close(); _ = os.Remove(tempName) }

	if _, err := temp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write %s: %w", tempName, err)
	}
	// Flush to disk before the rename, or a crash can leave the renamed file
	// present but empty.
	if err := temp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync %s: %w", tempName, err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("close %s: %w", tempName, err)
	}
	if err := os.Chmod(tempName, perm); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("set permissions on %s: %w", tempName, err)
	}

	// Rename straight over the target: one step, so a reader sees the old file or
	// the new one and never a half-written mix. Deleting first would be worse, not
	// safer, for the same reason the retry below exists.
	//
	// Windows refuses to replace a file another handle has open, and something
	// always has one — the UI reading this store, a backup tool, an antivirus
	// scan. Those handles live for microseconds, so a few quick attempts turn a
	// lost save into a brief wait. Measured: a test polling the file every 20ms
	// silently lost every update until this loop was added.
	var renameErr error
	for attempt := 0; attempt < 20; attempt++ {
		if renameErr = os.Rename(tempName, path); renameErr == nil {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}

	_ = os.Remove(tempName)
	return fmt.Errorf("rename %s to %s: %w", tempName, path, renameErr)
}
