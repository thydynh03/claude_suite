package sysproc

import (
	"context"
	"runtime"
	"testing"
)

// The whole point of this package is the SysProcAttr it attaches. Dropping back
// to a plain exec.Command compiles and runs fine — it just makes a console
// window blink on every git poll and every build verification, which is how the
// flicker got shipped in the first place.
func TestCommandSuppressesTheConsoleWindow(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows allocates a console for child processes")
	}

	if got := Command("cmd", "/c", "echo"); got.SysProcAttr == nil {
		t.Fatal("Command returned a process with no SysProcAttr; the console will flash")
	}
	if got := CommandContext(context.Background(), "cmd", "/c", "echo"); got.SysProcAttr == nil {
		t.Fatal("CommandContext returned a process with no SysProcAttr; the console will flash")
	}
}

func TestCommandStillRuns(t *testing.T) {
	name, args := "echo", []string{"ok"}
	if runtime.GOOS == "windows" {
		name, args = "cmd", []string{"/c", "echo ok"}
	}
	out, err := Command(name, args...).Output()
	if err != nil {
		t.Fatalf("Command(%s): %v", name, err)
	}
	if len(out) == 0 {
		t.Fatal("command produced no output")
	}
}
