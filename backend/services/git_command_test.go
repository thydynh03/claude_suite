package services

import (
	"os/exec"
	"strings"
	"testing"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	return dir
}

func TestRunCommand_AllowsStatus(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGitService()
	out, err := g.RunCommand(dir, []string{"status"})
	if err != nil {
		t.Fatalf("unexpected error: %v (%s)", err, out)
	}
}

func TestRunCommand_BlocksPush(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGitService()
	if _, err := g.RunCommand(dir, []string{"push"}); err == nil {
		t.Error("expected push to be blocked")
	}
}

func TestRunCommand_BlocksForceFlag(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGitService()
	if _, err := g.RunCommand(dir, []string{"commit", "--force", "-m", "x"}); err == nil {
		t.Error("expected --force to be blocked regardless of subcommand")
	}
}

func TestRunCommand_BlocksDisallowedSubcommand(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGitService()
	if _, err := g.RunCommand(dir, []string{"filter-branch"}); err == nil {
		t.Error("expected filter-branch to be blocked (not in allowlist)")
	}
}

func TestRunCommand_RejectsEmptyWorkspace(t *testing.T) {
	g := NewGitService()
	if _, err := g.RunCommand("", []string{"status"}); err == nil {
		t.Error("expected an error when cwd is empty")
	}
}

func TestRunCommand_StripsLeadingGitToken(t *testing.T) {
	dir := initTestRepo(t)
	g := NewGitService()
	out, err := g.RunCommand(dir, []string{"git", "status"})
	if err != nil {
		t.Fatalf("unexpected error: %v (%s)", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "branch") && !strings.Contains(strings.ToLower(out), "no commits") {
		t.Errorf("unexpected git status output: %s", out)
	}
}
