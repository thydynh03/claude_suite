package claims

import (
	"strings"
	"testing"
)

// The address handed to a teammate must not be localhost only: on their machine
// that resolves to their machine, where nothing is listening. This is the defect
// the whole file exists to fix, so it is asserted directly.
func TestJoinTargetsOfferMoreThanLocalhostWhenTheMachineIsOnANetwork(t *testing.T) {
	got := JoinTargets("[::]:9111")

	if len(got) == 0 {
		t.Fatal("no join targets at all")
	}
	if got[0].Scope != "local" || !strings.Contains(got[0].Host, "localhost:9111") {
		t.Errorf("first target = %+v, want the localhost one first", got[0])
	}

	// A machine with no network is legitimate (offline laptop), so the LAN
	// entries are checked for shape rather than for existence.
	for _, tgt := range got[1:] {
		if tgt.Scope != "lan" {
			t.Errorf("target %+v: scope should be lan", tgt)
		}
		if !strings.HasPrefix(tgt.Host, "ws://") || !strings.HasSuffix(tgt.Host, ":9111") {
			t.Errorf("target %+v: not a ws address on the listener port", tgt)
		}
		if strings.Contains(tgt.Host, "127.0.0.1") || strings.Contains(tgt.Host, "169.254.") {
			t.Errorf("target %+v: loopback and link-local are not reachable from another machine", tgt)
		}
	}
}

func TestJoinTargetsRefuseAnAddressWithNoPort(t *testing.T) {
	if got := JoinTargets(""); got != nil {
		t.Errorf("JoinTargets(\"\") = %+v, want nil rather than a broken command", got)
	}
}

// The command is copied from one machine and run on another, so it must not
// contain anything that only exists on the machine that produced it.
func TestJoinCommandCarriesNoMachineSpecificPath(t *testing.T) {
	cmd := JoinCommand("ws://192.168.1.5:9111", "abc123", "tok")

	// A Windows drive letter or a backslash means the command describes where the
	// app sits on the machine that generated it. ws:// legitimately contains a
	// colon and a slash, so the check has to be narrower than "://".
	if strings.Contains(cmd, `\`) || strings.Contains(cmd, "Program Files") || strings.Contains(cmd, "C:") {
		t.Errorf("command = %q, still embeds a local install path", cmd)
	}
	if strings.Contains(cmd, "YOU/your-agent") {
		t.Errorf("command = %q, still carries the placeholder author", cmd)
	}
	if !strings.HasPrefix(cmd, "claude-suite-claim ") {
		t.Errorf("command = %q, want the tool named plainly so PATH resolves it", cmd)
	}
	for _, want := range []string{"--host ws://192.168.1.5:9111", "--session abc123", "--token tok"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command = %q, missing %q", cmd, want)
		}
	}
}

// The bootstrap line is for a machine that never installed the app, so it may
// not assume the tool: it must fetch the published exe itself, from the
// stable latest-release URL, and then pass the same join parameters.
func TestBootstrapJoinCommandFetchesTheToolItself(t *testing.T) {
	cmd := BootstrapJoinCommand("ws://192.168.1.5:9111", "abc123", "tok")

	for _, want := range []string{
		"releases/latest/download/claude-suite-claim.exe",
		"Test-Path",
		"--host ws://192.168.1.5:9111", "--session abc123", "--token tok",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("bootstrap = %q, missing %q", cmd, want)
		}
	}
	// $env:TEMP is the recipient's; a path from the generating machine
	// (drive letter, Program Files) must not appear.
	if strings.Contains(cmd, "C:") || strings.Contains(cmd, "Program Files") {
		t.Errorf("bootstrap = %q, embeds a path from the generating machine", cmd)
	}
	if strings.Contains(cmd, "\n") {
		t.Errorf("bootstrap must be one pasteable line, got %q", cmd)
	}
}

// The prompt is pasted into an arbitrary AI agent, so it has to carry every
// fact the agent needs: connection parameters, where to get the tool on each
// OS, how to verify the connection, and how a claim differs from an opinion.
func TestAgentJoinPromptIsSelfContained(t *testing.T) {
	p := AgentJoinPrompt("ws://192.168.1.5:9111", "abc123", "tok", "backend/cli/process_windows.go:17")

	for _, want := range []string{
		"backend/cli/process_windows.go:17",
		"--host ws://192.168.1.5:9111", "--session abc123", "--token tok",
		"releases/latest/download/claude-suite-claim.exe",
		"go run ./cmd/claude-suite-claim",
		"https://github.com/thydynh03/claude_suite",
		"--ping",
		"--checks",
		"--falsify",
		"session-abc123/verdict.json",
		// The conversation loop: an agent handed this prompt must know how to
		// listen and how to answer, on both doors.
		"--listen",
		"--say",
		"wait_for_chat",
		"after_seq",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
