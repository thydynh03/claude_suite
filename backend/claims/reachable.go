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

// claimToolURL is where the latest published claim tool always lives. GitHub
// keeps this path stable across releases, which is what lets a machine that
// never installed the app fetch the tool by itself.
const claimToolURL = "https://github.com/thydynh03/claude_suite/releases/latest/download/claude-suite-claim.exe"

// projectRepoURL is the fallback for machines the prebuilt exe cannot serve
// (macOS, Linux): clone and `go run` the tool from source.
const projectRepoURL = "https://github.com/thydynh03/claude_suite"

// claimArgs is the part of a join line that is identical however the tool got
// onto the recipient's machine.
func claimArgs(host, sessionID, token string) string {
	return fmt.Sprintf("--host %s --session %s --token %s "+
		`--provider claude --subject "file.go:12" `+
		`--assert "what is wrong" --falsify "check-name"`,
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
	return "claude-suite-claim " + claimArgs(host, sessionID, token)
}

// BootstrapJoinCommand is JoinCommand for a machine that has never installed
// the app: one PowerShell line that fetches the prebuilt tool into the
// recipient's temp dir and joins from there. curl.exe rather than
// Invoke-RestMethod because it ships with Windows 10 1803+ and does not
// inherit PowerShell 5.1's .NET TLS defaults, which on older machines refuse
// GitHub. The $env:TEMP in it is the recipient's, so the line stays valid
// wherever it is pasted.
func BootstrapJoinCommand(host, sessionID, token string) string {
	return `$c = "$env:TEMP\claude-suite-claim.exe"; ` +
		`if (!(Test-Path $c)) { curl.exe -L -o $c ` + claimToolURL + ` }; ` +
		`& $c ` + claimArgs(host, sessionID, token)
}

// MCPJoinURL is the session's MCP endpoint — the door that needs no download
// at all. The token travels in the URL because that URL is itself the thing
// being handed over; it is as secret as the join command that carries the
// same token.
func MCPJoinURL(host, sessionID, token string) string {
	return strings.Replace(host, "ws://", "http://", 1) + "/mcp/" + sessionID + "?token=" + token
}

// MCPJoinCommand registers the session as an MCP server with a teammate's
// agent. One command, zero downloads: the agent then has join/submit/finish
// as tools.
func MCPJoinCommand(host, sessionID, token string) string {
	return fmt.Sprintf(`claude mcp add --transport http claude-suite-debate "%s"`, MCPJoinURL(host, sessionID, token))
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
	return fmt.Sprintf(`Bạn được mời tham gia một phiên tranh luận (adjudication session) của Claude Suite với vai trò reviewer.

Chủ đề phiên: %s

BƯỚC 1 — chọn cách kết nối theo đúng thứ tự sau:

A. Máy đã có claude-suite-claim trên PATH (đã cài app Claude Suite bằng installer):
   dùng luôn, sang bước 2.

B. Chưa có công cụ: HỎI người dùng của bạn muốn tải công cụ về hay không (đừng tự quyết).
   - Đồng ý tải — Windows (PowerShell):
     $c = "$env:TEMP\claude-suite-claim.exe"; if (!(Test-Path $c)) { curl.exe -L -o $c %s }
     rồi thay "claude-suite-claim" trong các lệnh dưới bằng $c.
     (macOS/Linux hoặc tải thất bại: git clone %s rồi chạy "go run ./cmd/claude-suite-claim" — cần Go.)
   - KHÔNG muốn tải — kết nối MCP thẳng vào app của người mở phiên:
     claude mcp add --transport http claude-suite-debate "%s"
     (Gemini CLI: gemini mcp add --transport http claude-suite-debate "%s")
     Khởi động lại agent nếu cần để nhận tool mới, rồi làm theo BƯỚC 2-MCP bên dưới; bỏ qua phần CLI.

BƯỚC 2 — với công cụ CLI:
1. Kiểm tra kết nối trước khi bỏ công review: claude-suite-claim %s --ping
2. Xem các check mà claim được phép trỏ tới (chạy trong thư mục dự án đang xét): claude-suite-claim --checks
   Falsifier PASS khi claim SAI, nên check FAIL là xác nhận lỗi có thật. Claim không có --falsify chỉ là ý kiến, không chặn được merge.
3. Tự xem xét mã nguồn liên quan chủ đề, rồi nộp nhận định:
   claude-suite-claim %s --subject "duong/dan/file.go:12" --assert "lỗi là gì, trong một câu" --falsify "ten-check"
   (--subject/--assert/--falsify lặp lại được cho nhiều claim; --say "..." để thảo luận thêm.)
4. Lệnh nộp claim tự chờ kết quả; verdict đầy đủ nằm ở .claude-suite/session-%s/verdict.json.

BƯỚC 2-MCP — với kết nối MCP (tool trên server claude-suite-debate):
1. join_session với author dạng "ten-ban@may/agent". Kết quả trả về một participant_key —
   GIỮ LẠI và gửi kèm (cùng author) trong MỌI tool sau; thiếu nó call sẽ bị từ chối.
2. list_checks để biết claim được trỏ tới check nào (quy tắc falsifier như trên).
3. Tự xem xét mã nguồn, rồi submit_claim cho từng nhận định; bỏ trống falsifier nếu chỉ là ý kiến.
4. finish_reporting khi hết claim, sau đó gọi get_session_state định kỳ để xem verdict và kết quả phiên.
   Đừng im lặng quá lâu giữa các call: participant vắng mặt quá cửa sổ thu thập bị coi là đã rời phiên.`,
		subject, claimToolURL, projectRepoURL,
		MCPJoinURL(host, sessionID, token), MCPJoinURL(host, sessionID, token),
		conn, conn, sessionID)
}
