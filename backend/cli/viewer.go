package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// runViewer gives "Hiện cửa sổ CMD" a window that actually shows something
// and does not vanish when the run ends.
//
// The old approach — CREATE_NEW_CONSOLE on the agent process — showed an
// EMPTY console: stdout/stderr are piped to the app either way, and the
// window died with the process, before a human could read anything. Keeping
// the process console alive is the `/k` trap (cmd.Wait() never returns), so
// instead the agent always runs hidden and the visible window is a separate
// PowerShell `Get-Content -Wait` tailing this run's log file. That viewer is
// never Wait()ed on: it survives the run until the user closes it, and it
// cannot block a worker slot.
type runViewer struct {
	mu sync.Mutex
	f  *os.File
}

// runLogDir holds one log file per viewed run, under the OS temp dir.
func runLogDir() string {
	return filepath.Join(os.TempDir(), "claude-suite-runs")
}

// startRunViewer opens the log file and the console tailing it. Returns nil
// (a safe no-op receiver) when the console toggle is off or setup fails —
// a broken viewer must never break the run itself.
func startRunViewer(title string) *runViewer {
	if !ShowCLIConsole {
		return nil
	}
	dir := runLogDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	cleanOldRunLogs(dir)

	f, err := os.CreateTemp(dir, "run-*.log")
	if err != nil {
		return nil
	}
	// UTF-8 BOM: PowerShell 5.1's Get-Content decodes by BOM sniffing, and
	// these logs carry Vietnamese text.
	_, _ = f.Write([]byte{0xEF, 0xBB, 0xBF})

	v := &runViewer{f: f}
	v.Line("=== " + title + " ===")
	v.Line(fmt.Sprintf("Bắt đầu: %s", time.Now().Format("15:04:05 02/01/2006")))
	v.Line("Cửa sổ này theo dõi run và KHÔNG tự đóng khi run kết thúc — đóng tay khi xem xong.")
	v.Line("")

	spawnViewerConsole(f.Name(), title)
	return v
}

// Line appends one line. Nil-safe: callers never have to guard.
func (v *runViewer) Line(s string) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.f != nil {
		_, _ = v.f.WriteString(s + "\r\n")
	}
}

// Finish writes the closing banner and releases the file handle. The file
// itself stays on disk — the PowerShell tail is still reading it, and the
// whole point is that the window outlives the run.
func (v *runViewer) Finish(summary string) {
	if v == nil {
		return
	}
	v.Line("")
	v.Line("=== " + summary + " ===")
	v.Line("(Run đã kết thúc. Cửa sổ này giữ nguyên để bạn đọc lại — đóng tay khi xong.)")
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.f != nil {
		_ = v.f.Close()
		v.f = nil
	}
}

// logSink merges the viewer into an existing log callback. Either side may be
// absent; the result is always callable.
func (v *runViewer) logSink(onLog LogCallback) LogCallback {
	if v == nil {
		return onLog
	}
	return func(msg, level string) {
		v.Line("[" + level + "] " + msg)
		if onLog != nil {
			onLog(msg, level)
		}
	}
}

// cleanOldRunLogs drops viewer logs older than a day. Best-effort: a file a
// viewer still holds open simply stays for the next sweep.
func cleanOldRunLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || info.IsDir() || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
}
