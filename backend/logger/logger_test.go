package logger

import (
	"testing"
)

func TestLogger_Log(t *testing.T) {
	l := GetLogger()
	if l == nil {
		t.Fatalf("expected non-nil logger")
	}

	Info("Test log entry INFO")
	Warn("Test log entry WARN")
	Error("Test log entry ERROR")
	Success("Test log entry SUCCESS")
}
