//go:build !windows

package cli

// Non-Windows builds exist for CI; there is no console to open. The log file
// is still written, which is what the viewer tests exercise.
func spawnViewerConsole(logPath, title string) {}
