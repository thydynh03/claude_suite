package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The viewer file writer is tested directly — startRunViewer spawns a real
// console window, which no test may do. Nil-safety matters most: every
// call site runs with viewer == nil whenever the console toggle is off.
func TestRunViewerWritesLinesAndSurvivesNil(t *testing.T) {
	var nilViewer *runViewer
	nilViewer.Line("must not panic")
	nilViewer.Finish("must not panic")
	if sink := nilViewer.logSink(nil); sink != nil {
		t.Fatal("nil viewer with nil onLog must yield a nil sink")
	}

	f, err := os.CreateTemp(t.TempDir(), "run-*.log")
	if err != nil {
		t.Fatal(err)
	}
	v := &runViewer{f: f}

	var forwarded []string
	sink := v.logSink(func(msg, level string) { forwarded = append(forwarded, level+":"+msg) })
	sink("dòng tiếng Việt có dấu", "INFO")
	v.Finish("RUN KẾT THÚC")

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "[INFO] dòng tiếng Việt có dấu") {
		t.Fatalf("log line missing from viewer file:\n%s", content)
	}
	if !strings.Contains(content, "RUN KẾT THÚC") {
		t.Fatalf("finish banner missing:\n%s", content)
	}
	if len(forwarded) != 1 || forwarded[0] != "INFO:dòng tiếng Việt có dấu" {
		t.Fatalf("original onLog must still receive the line: %v", forwarded)
	}

	// Finish closed the handle; further lines must be silently dropped.
	v.Line("after close — must not panic")
}

func TestCleanOldRunLogsDropsOnlyStaleFiles(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "run-old.log")
	newFile := filepath.Join(dir, "run-new.log")
	if err := os.WriteFile(oldFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, stale, stale); err != nil {
		t.Fatal(err)
	}

	cleanOldRunLogs(dir)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("stale log must be removed")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatal("fresh log must survive")
	}
}
