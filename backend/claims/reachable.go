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

	// VPN addresses go before plain LAN ones: they are the only listed
	// addresses a teammate in another city can actually reach. Addresses that
	// belong to a virtual adapter go last — they look exactly like LAN
	// addresses and reach nobody.
	var lan, virtual []JoinTarget
	for _, a := range lanAddresses() {
		scope, label := classifyAddr(a.ip)
		t := JoinTarget{Host: "ws://" + net.JoinHostPort(a.ip, port), Label: label, Scope: scope}
		switch {
		case scope == "vpn":
			targets = append(targets, t)
		case isVirtualAdapter(a.iface):
			t.Label = "Có thể là adapter ảo (" + a.iface + ") — thử sau cùng"
			virtual = append(virtual, t)
		default:
			lan = append(lan, t)
		}
	}
	return append(append(targets, lan...), virtual...)
}

// isVirtualAdapter spots the interfaces a Windows dev machine collects —
// Hyper-V switches, WSL's NAT, VirtualBox/VMware host-only networks, VPN TAP
// devices. Their addresses are up, private and non-loopback, so they were
// offered to a teammate as ordinary LAN addresses; picking one gives a
// connect timeout with no hint that the address itself was the problem. They
// are demoted rather than dropped: a bridged vEthernet can be the real NIC.
func isVirtualAdapter(name string) bool {
	lower := strings.ToLower(name)
	for _, marker := range []string{
		"vethernet", "wsl", "hyper-v", "default switch",
		"virtualbox", "vmware", "tap-", "npcap", "loopback",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// classifyAddr separates addresses a remote teammate can use from ones only
// the local network can. Tailscale hands every node an address in the CGNAT
// range 100.64.0.0/10; labelling that "same LAN" told a teammate in another
// city the address was useless to them, when it is precisely the one that
// works from anywhere in the tailnet.
func classifyAddr(ip string) (scope, label string) {
	parsed := net.ParseIP(ip)
	if parsed != nil {
		if _, cgnat, _ := net.ParseCIDR("100.64.0.0/10"); cgnat != nil && cgnat.Contains(parsed) {
			return "vpn", "Qua VPN (Tailscale) — người ở XA dùng được nếu cùng tailnet"
		}
	}
	return "lan", "Người khác trong cùng mạng LAN"
}

// NormalizeJoinHost turns whatever a person pastes for a remote member — a
// tunnel URL, a domain behind a reverse proxy, an ip:port from a port
// forward — into the ws(s) host the join commands need. https becomes wss
// (the CLI dials TLS; the MCP builder maps it back to https); a bare
// host:port is a direct socket, a bare domain is assumed to sit on 443.
func NormalizeJoinHost(raw string) (string, error) {
	h := strings.TrimRight(strings.TrimSpace(raw), "/")
	if h == "" {
		return "", fmt.Errorf("địa chỉ trống")
	}

	scheme := ""
	rest := h
	lower := strings.ToLower(h)
	for _, s := range []struct{ in, out string }{
		{"wss://", "wss://"}, {"ws://", "ws://"},
		{"https://", "wss://"}, {"http://", "ws://"},
	} {
		if strings.HasPrefix(lower, s.in) {
			scheme, rest = s.out, h[len(s.in):]
			break
		}
	}

	// Whatever the scheme, the commands need an origin: a pasted URL with a
	// path (someone copying the MCP endpoint back in) would silently produce
	// commands that dial the wrong place.
	if strings.Contains(rest, "/") {
		return "", fmt.Errorf("chỉ cần origin, không kèm đường dẫn — ví dụ https://abc.trycloudflare.com")
	}
	if rest == "" {
		return "", fmt.Errorf("địa chỉ trống")
	}

	if scheme != "" {
		return scheme + rest, nil
	}
	if _, _, err := net.SplitHostPort(rest); err == nil {
		return "ws://" + rest, nil
	}
	return "wss://" + rest, nil
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
// The interface name travels with the address so JoinTargets can tell a real
// NIC from a virtual one.
type ifaceAddr struct {
	ip    string
	iface string
}

func lanAddresses() []ifaceAddr {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var out []ifaceAddr
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
			out = append(out, ifaceAddr{ip: ip.String(), iface: iface.Name})
		}
	}
	return out
}

// claimToolURL is where the latest published claim tool always lives. GitHub
// keeps this path stable across releases, which is what lets a machine that
// never installed the app fetch the tool by itself.
const claimToolURL = "https://github.com/thydynh03/Agent_Center/releases/latest/download/agent-center-claim.exe"

// projectRepoURL is the fallback for machines the prebuilt exe cannot serve
// (macOS, Linux): clone and `go run` the tool from source.
const projectRepoURL = "https://github.com/thydynh03/Agent_Center"

// claimArgs is the part of a join line that is identical however the tool got
// onto the recipient's machine. The placeholders describe themselves in the
// reader's language: "what is wrong" looked like literal input to paste, and
// was pasted.
func claimArgs(host, sessionID, token string) string {
	return fmt.Sprintf("--host %s --session %s --token %s "+
		`--provider claude --subject "duong-dan/file.go:12" `+
		`--assert "loi la gi, mot cau" --falsify "ten-check"`,
		host, sessionID, token)
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
	return "agent-center-claim " + claimArgs(host, sessionID, token)
}

// BootstrapJoinCommand is JoinCommand for a machine that has never installed
// the app: one PowerShell line that fetches the prebuilt tool into the
// recipient's temp dir and joins from there. curl.exe rather than
// Invoke-RestMethod because it ships with Windows 10 1803+ and does not
// inherit PowerShell 5.1's .NET TLS defaults, which on older machines refuse
// GitHub. The $env:TEMP in it is the recipient's, so the line stays valid
// wherever it is pasted.
func BootstrapJoinCommand(host, sessionID, token string) string {
	return `$c = "$env:TEMP\agent-center-claim.exe"; ` +
		`if (!(Test-Path $c)) { curl.exe -L -o $c ` + claimToolURL + ` }; ` +
		`& $c ` + claimArgs(host, sessionID, token)
}

// MCPJoinURL is the session's MCP endpoint — the door that needs no download
// at all. The token travels in the URL because that URL is itself the thing
// being handed over; it is as secret as the join command that carries the
// same token.
func MCPJoinURL(host, sessionID, token string) string {
	base := host
	// wss first: a plain ws-replace would turn "wss://" into "http://s...".
	if strings.HasPrefix(base, "wss://") {
		base = "https://" + strings.TrimPrefix(base, "wss://")
	} else {
		base = strings.Replace(base, "ws://", "http://", 1)
	}
	return base + "/mcp/" + sessionID + "?token=" + token
}

// MCPJoinCommand registers the session as an MCP server with a teammate's
// agent. One command, zero downloads: the agent then has join/submit/finish
// as tools.
func MCPJoinCommand(host, sessionID, token string) string {
	return fmt.Sprintf(`claude mcp add --transport http agent-center-debate "%s"`, MCPJoinURL(host, sessionID, token))
}

// AgentJoinPrompt is a self-contained instruction block the session owner
// hands to a teammate, who pastes it into whatever AI agent they use.
//
// It encodes the decision tree, in order of least friction: a machine with the
// app uses the tool it already has; without the tool the agent must ASK its
// user whether downloading is acceptable; if not, the MCP endpoint joins with
// no download at all. The asking is deliberate — downloading and running an
// exe on someone's machine is their call, not their agent's.
func AgentJoinPrompt(host, sessionID, token, subject string) string {
	conn := fmt.Sprintf("--host %s --session %s --token %s --provider claude", host, sessionID, token)
	return fmt.Sprintf(`Bạn được mời làm reviewer trong một phiên phân xử (adjudication session) của Agent Center.

Chủ đề: %s

Cách phiên hoạt động: mỗi agent tự điều tra rồi nộp phát hiện dưới dạng claim kèm cách
kiểm chứng (falsifier). Host chạy check — kết quả check quyết định đúng sai, không ai
thắng nhờ lý lẽ. Falsifier PASS khi claim SAI, nên check FAIL nghĩa là lỗi có thật.
Claim không kèm falsifier chỉ là ý kiến: được ghi lại nhưng không chặn được merge.

=== BƯỚC 1 — KẾT NỐI (chọn một, theo thứ tự ưu tiên) ===

A. Máy đã cài app Agent Center: công cụ agent-center-claim có sẵn trên PATH — sang BƯỚC 2A.

B. Chưa có công cụ: HỎI người dùng của bạn trước, đừng tự quyết.
   - Họ đồng ý tải (Windows, PowerShell):
     $c = "$env:TEMP\agent-center-claim.exe"; if (!(Test-Path $c)) { curl.exe -L -o $c %s }
     rồi dùng $c thay cho "agent-center-claim" trong mọi lệnh bên dưới. Sang BƯỚC 2A.
     (macOS/Linux: git clone %s rồi chạy "go run ./cmd/agent-center-claim" — cần Go.)
   - Họ KHÔNG muốn tải: kết nối MCP, không tải gì cả —
     claude mcp add --transport http agent-center-debate "%s"
     (Gemini CLI: gemini mcp add --transport http agent-center-debate "%s")
     Khởi động lại agent nếu chưa thấy tool mới, rồi làm BƯỚC 2B và 3B; bỏ qua 2A/3A.

=== BƯỚC 2A — NỘP PHÁT HIỆN, bằng công cụ CLI ===

1. Thử kết nối trước khi bỏ công review:
   agent-center-claim %s --ping
2. Xem danh sách check mà claim được phép trỏ tới (chạy trong thư mục dự án đang xét):
   agent-center-claim --checks
3. Tự đọc mã nguồn liên quan chủ đề, rồi nộp mỗi phát hiện một bộ ba:
   agent-center-claim %s --subject "duong-dan/file.go:12" --assert "loi la gi, mot cau" --falsify "ten-check"
   (ba cờ này lặp lại được để nộp nhiều claim trong một lần chạy)
4. Lệnh trên tự chờ các agent khác nộp xong và chờ check chạy; kết quả đầy đủ nằm ở
   .agent-center/session-%s/verdict.json

=== BƯỚC 3A — THẢO LUẬN, bằng công cụ CLI ===

Phiên có một kênh chat chung giữa các agent và người trọng tài (chủ phiên).
- NGHE (chạy nền, in mỗi tin nhắn thành một dòng JSON có seq/author/text):
  agent-center-claim %s --listen 15m
- NÓI:
  agent-center-claim %s --say "noi dung"
Luật hội thoại: chỉ trả lời khi tin nhắn gọi tên bạn, hỏi bạn, hoặc phản bác claim của
bạn. Trả lời ngắn, dẫn file:dòng. Không trả lời tin của chính mình. Chat không phải
bằng chứng — chỉ falsifier mới kết luận được một claim.

=== BƯỚC 2B — NỘP PHÁT HIỆN, qua MCP (tool trên server agent-center-debate) ===

1. join_session với author dạng "ten-ban@ten-may/agent". Nó trả về participant_key —
   LƯU LẠI: mọi tool sau đều phải gửi kèm key này cùng đúng author đó.
2. list_checks — xem claim được trỏ tới những check nào.
3. Tự đọc mã nguồn rồi submit_claim cho từng phát hiện; bỏ trống falsifier nếu chỉ là ý kiến.
4. finish_reporting khi không còn gì để nộp.

=== BƯỚC 3B — THẢO LUẬN, qua MCP ===

Sau finish_reporting, vào vòng hội thoại và lặp cho đến khi phase là "record":
1. wait_for_chat với after_seq = seq lớn nhất bạn đã thấy (lần đầu: 0). Tool tự block
   chờ tin mới; hết giờ thì gọi lại y nguyên — không cần ngủ giữa các lần gọi.
2. Tin mới gọi tên bạn / hỏi bạn / phản bác claim của bạn thì dùng say trả lời ngắn, dẫn
   file:dòng. Tin khác thì chỉ cập nhật after_seq rồi wait_for_chat tiếp.
3. Không trả lời tin của chính mình. Khi phase thành "record" thì dừng và đọc kết quả
   cuối bằng get_session_state.
Gọi tool đều đặn cũng là nhịp tim của bạn: im lặng quá lâu bị coi là đã rời phiên.`,
		subject, claimToolURL, projectRepoURL,
		MCPJoinURL(host, sessionID, token), MCPJoinURL(host, sessionID, token),
		conn, conn, sessionID, conn, conn)
}
