package claims

import (
	"fmt"
	"net"
	"strings"
)

// JoinTarget is one address a teammate can be told to connect to, with the case
// it is meant for.
type JoinTarget struct {
	Host  string `json:"host"`  // ws://…:port
	Label string `json:"label"` // shown in the UI next to the address
	Scope string `json:"scope"` // "local" | "lan"
}

// JoinTargets turns the listener address into the addresses a teammate can
// actually reach, most useful first.
//
// The listener binds every interface, which prints as "[::]", and the app used to
// rewrite that to "localhost". That is correct for a second agent in another IDE
// on this same machine, and wrong for everybody else: on the recipient's computer
// "localhost" is their computer, where no session is listening. The command was
// copied, pasted, and failed to connect with no hint as to why.
//
// So the LAN addresses are enumerated and offered explicitly, labelled with who
// each one is for.
func JoinTargets(addr string) []JoinTarget {
	port := portOf(addr)
	if port == "" {
		return nil
	}

	targets := []JoinTarget{{
		Host:  "ws://localhost:" + port,
		Label: "Agent trên chính máy này (IDE khác)",
		Scope: "local",
	}}

	for _, ip := range lanAddresses() {
		targets = append(targets, JoinTarget{
			Host:  "ws://" + net.JoinHostPort(ip, port),
			Label: "Người khác trong cùng mạng LAN",
			Scope: "lan",
		})
	}

	return targets
}

// portOf pulls the port out of a listener address such as "[::]:9111".
func portOf(addr string) string {
	if addr == "" {
		return ""
	}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return port
	}
	// Some listeners report a bare port.
	if !strings.ContainsAny(addr, ":[]") {
		return addr
	}
	return ""
}

// lanAddresses lists this machine's routable IPv4 addresses, skipping loopback,
// interfaces that are down, and the link-local range Windows invents when DHCP
// fails — handing a teammate a 169.254.x.x address is worse than handing them
// nothing, because it looks like it should work.
func lanAddresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	return out
}

// JoinCommand builds the line a teammate's agent runs.
//
// The tool is named without a path on purpose. The previous version embedded the
// absolute path of the machine that opened the session — resolved here, then
// pasted into a chat window and run somewhere else entirely. It happened to work
// only while both sides accepted the same default install directory. The
// installer puts the tool on PATH instead, so a bare name is correct on every
// machine that has the app.
func JoinCommand(host, sessionID, token string) string {
	return fmt.Sprintf(
		"claude-suite-claim --host %s --session %s --token %s "+
			`--provider claude --subject "file.go:12" `+
			`--assert "what is wrong" --falsify "check-name"`,
		host, sessionID, token)
}
