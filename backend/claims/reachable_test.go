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
	// entries are checked for shape rather than for existence. A Tailscale
	// interface classifies as vpn — the address a remote teammate can use.
	for _, tgt := range got[1:] {
		if tgt.Scope != "lan" && tgt.Scope != "vpn" {
			t.Errorf("target %+v: scope should be lan or vpn", tgt)
		}
		if !strings.HasPrefix(tgt.Host, "ws://") || !strings.HasSuffix(tgt.Host, ":9111") {
			t.Errorf("target %+v: not a ws address on the listener port", tgt)
		}
		if strings.Contains(tgt.Host, "127.0.0.1") || strings.Contains(tgt.Host, "169.254.") {
			t.Errorf("target %+v: loopback and link-local are not reachable from another machine", tgt)
		}
	}
}

// A Windows dev machine collects virtual adapters — Hyper-V, WSL, VirtualBox,
// VPN TAP — whose addresses are up, private and utterly unreachable from
// another machine. Offering them as plain LAN addresses gave a teammate
// several commands to try and no way to tell which one could work.
func TestVirtualAdaptersAreRecognised(t *testing.T) {
	for _, name := range []string{
		"vEthernet (Default Switch)", "vEthernet (WSL)", "VirtualBox Host-Only Network",
		"VMware Network Adapter VMnet1", "TAP-Windows Adapter V9",
	} {
		if !isVirtualAdapter(name) {
			t.Errorf("%q was not recognised as a virtual adapter", name)
		}
	}
	for _, name := range []string{"Wi-Fi", "Ethernet", "Ethernet 2", "eth0", "wlan0"} {
		if isVirtualAdapter(name) {
			t.Errorf("%q — a real NIC — was demoted as virtual", name)
		}
	}
}

func TestJoinTargetsRefuseAnAddressWithNoPort(t *testing.T) {
	if got := JoinTargets(""); got != nil {
		t.Errorf("JoinTargets(\"\") = %+v, want nil rather than a broken command", got)
	}
}

// Tailscale's CGNAT range is the one listed address a teammate in another
// city can actually reach; calling it "same LAN" told them it was useless.
func TestClassifyAddrSeparatesVPNFromLAN(t *testing.T) {
	if scope, _ := classifyAddr("100.84.122.101"); scope != "vpn" {
		t.Errorf("a 100.64.0.0/10 address classified %q, want vpn", scope)
	}
	if scope, _ := classifyAddr("100.63.255.255"); scope != "lan" {
		t.Errorf("an address just below the CGNAT range classified %q, want lan", scope)
	}
	if scope, _ := classifyAddr("192.168.1.92"); scope != "lan" {
		t.Errorf("a private LAN address classified %q, want lan", scope)
	}
}

// Whatever a person pastes for a remote member — tunnel URL, domain, ip:port
// — must come out as the ws(s) host the join commands need, or be refused
// with a reason.
func TestNormalizeJoinHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://abc.trycloudflare.com", "wss://abc.trycloudflare.com"},
		{"https://abc.trycloudflare.com/", "wss://abc.trycloudflare.com"},
		{"http://203.0.113.7:9111", "ws://203.0.113.7:9111"},
		{"wss://already.fine", "wss://already.fine"},
		{"ws://192.168.1.5:9111", "ws://192.168.1.5:9111"},
		{"203.0.113.7:9111", "ws://203.0.113.7:9111"},
		{"suite.example.com", "wss://suite.example.com"},
		{"  https://padded.example  ", "wss://padded.example"},
	}
	for _, c := range cases {
		got, err := NormalizeJoinHost(c.in)
		if err != nil {
			t.Errorf("NormalizeJoinHost(%q) errored: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeJoinHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	for _, bad := range []string{"", "https://abc.example/mcp/x", "abc.example/path"} {
		if _, err := NormalizeJoinHost(bad); err == nil {
			t.Errorf("NormalizeJoinHost(%q) accepted input it should refuse", bad)
		}
	}
}

// A wss host must become an https MCP endpoint — the naive ws-replace turned
// "wss://" into "http://s…".
func TestMCPJoinURLHandlesTLSHosts(t *testing.T) {
	got := MCPJoinURL("wss://abc.trycloudflare.com", "s1", "t1")
	if got != "https://abc.trycloudflare.com/mcp/s1?token=t1" {
		t.Errorf("MCPJoinURL over wss = %q", got)
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
	if !strings.HasPrefix(cmd, "agent-center-claim ") {
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
		"releases/latest/download/agent-center-claim.exe",
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
		"releases/latest/download/agent-center-claim.exe",
		"go run ./cmd/agent-center-claim",
		"https://github.com/thydynh03/agent_center",
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
