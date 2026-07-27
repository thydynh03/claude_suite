package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agent_center/backend/sysproc"
)

// ── Why a dedicated profile instead of the user's real one ────────────────
//
// Chrome >= 136 refuses --remote-debugging-port (and --remote-debugging-pipe)
// whenever the profile is the DEFAULT "User Data" directory — whether that
// directory is passed explicitly or left implicit. Google shipped this because
// remote debugging had become the standard way infostealers dump cookies once
// App-Bound Encryption blocked reading the cookie DB directly.
//
// Two workarounds were measured on Chrome 150 and both FAIL:
//   - copying the real profile  -> App-Bound Encryption cannot unseal the copied
//                                  cookies, every site falls back to logged out.
//   - junction to the real dir  -> the debug port opens, but Chrome treats the
//                                  mismatched path as tampering and WIPES the
//                                  cookie store of the real profile.
//
// So the agent gets its own persistent profile. The user signs in once per site
// inside the agent window; the directory survives app and machine restarts, so
// it is a one-time cost rather than a per-run one.

// AgentUserDataDir is the agent's own Chrome profile root.
//
// The result can contain spaces, because LOCALAPPDATA carries the account name.
// That is safe: the directory is only ever passed as a single element of an
// exec.Command argument slice, which Go quotes when it builds the Windows
// command line, and agentChromePIDs compares it in Go rather than pasting it
// into a PowerShell string. Keep it that way — building a command line by
// concatenating this path is what would break it.
func AgentUserDataDir() string {
	if base := os.Getenv("LOCALAPPDATA"); base != "" {
		return filepath.Join(base, "AgentCenter", "ChromeAgent")
	}
	return filepath.Join(os.TempDir(), "AgentCenterChromeAgent")
}

// ChromeExecutablePath resolves chrome.exe from the App Paths registry key,
// falling back to the usual install locations.
func ChromeExecutablePath() string {
	out, err := sysproc.Command("reg", "query",
		`HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe`,
		"/ve").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if idx := strings.Index(line, "REG_SZ"); idx >= 0 {
				if val := strings.TrimSpace(line[idx+len("REG_SZ"):]); val != "" {
					return val
				}
			}
		}
	}

	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
		if base == "" {
			continue
		}
		p := filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "chrome.exe"
}

// devToolsActivePort reads the port Chrome picked for itself. Chrome writes this
// file into the user-data-dir on startup, which is how a --remote-debugging-port=0
// (OS-assigned) instance can be found again without hardcoding 9222.
func devToolsActivePort(userDataDir string) int {
	data, err := os.ReadFile(filepath.Join(userDataDir, "DevToolsActivePort"))
	if err != nil {
		return 0
	}
	firstLine := strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)[0])
	port, err := strconv.Atoi(firstLine)
	if err != nil {
		return 0
	}
	return port
}

// chromeWebSocketURL fetches the browser-level CDP endpoint on a given port.
func chromeWebSocketURL(port int) (string, error) {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var data struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	if data.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("empty webSocketDebuggerUrl")
	}
	return data.WebSocketDebuggerURL, nil
}

// liveAgentPort returns the port of an already-running agent Chrome, or 0.
// DevToolsActivePort is left behind when Chrome exits, so the port is probed
// rather than trusted.
func liveAgentPort(userDataDir string) int {
	port := devToolsActivePort(userDataDir)
	if port <= 0 || !isPortOpen("127.0.0.1", port) {
		return 0
	}
	if _, err := chromeWebSocketURL(port); err != nil {
		return 0
	}
	return port
}

// ChromeAgentStatus reports whether the agent's Chrome is up and attachable.
func ChromeAgentStatus() (ready bool, port int, profileDir string) {
	dir := AgentUserDataDir()
	p := liveAgentPort(dir)
	return p > 0, p, dir
}

// agentChromePIDs lists chrome.exe processes running on the agent's profile.
// Matching on the profile directory is what keeps this from ever touching the
// user's own Chrome.
func agentChromePIDs() []int {
	dir := AgentUserDataDir()
	if dir == "" {
		return nil
	}
	script := `Get-CimInstance Win32_Process -Filter "Name='chrome.exe'" | ForEach-Object { "$($_.ProcessId)|$($_.CommandLine)" }`
	out, err := sysproc.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		sep := strings.Index(line, "|")
		if sep < 0 {
			continue
		}
		if !strings.Contains(line[sep+1:], dir) {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(line[:sep]))
		if err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

// closeAgentChrome shuts the agent's Chrome down, preferring a graceful close so
// that cookies written during a sign-in are flushed to disk before exit.
func closeAgentChrome(logf func(string)) {
	pids := agentChromePIDs()
	if len(pids) == 0 {
		return
	}
	if logf != nil {
		logf("🔻 Đóng cửa sổ Chrome Agent đang mở để chuyển chế độ...")
	}
	for _, pid := range pids {
		// No /F: let Chrome exit on its own so the cookie store is committed.
		_ = sysproc.Command("taskkill", "/PID", strconv.Itoa(pid), "/T").Run()
	}
	for i := 0; i < 24; i++ {
		time.Sleep(250 * time.Millisecond)
		if len(agentChromePIDs()) == 0 {
			return
		}
	}
	// Graceful close did not take; fall back to a forced kill.
	for _, pid := range agentChromePIDs() {
		_ = sysproc.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
	}
	time.Sleep(time.Second)
}

// OpenAgentChromeForLogin opens the agent profile as an ORDINARY browser, with no
// debug port at all.
//
// Google refuses to sign in to a browser it can tell is driven over CDP
// ("This browser or app may not be secure"), which blocks both Google accounts
// and every "Sign in with Google" button. Launching the same profile without the
// debug port removes the signal, and the session written during that visit is
// still there when the agent reattaches later — the cookie store survives the
// switch because the profile path never changes.
func OpenAgentChromeForLogin(targetURL string, logf func(string)) error {
	dir := AgentUserDataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("không tạo được thư mục profile agent %s: %w", dir, err)
	}

	// A second launch on the same user-data-dir just opens a window inside the
	// existing process, so an attached instance has to go first or the new window
	// would still be CDP-controlled.
	closeAgentChrome(logf)

	args := []string{
		"--user-data-dir=" + dir,
		"--no-first-run",
		"--no-default-browser-check",
	}
	// Deliberately NOT passing targetURL here. chromedp opens its own target and
	// navigates it, so handing the URL to the launch as well produced a second
	// window per run showing the same page — which is where the stack of Chrome
	// windows came from. The parameter is kept because callers pass it and the
	// signature is used elsewhere; it no longer opens anything.
	_ = targetURL

	if logf != nil {
		logf("🔓 Mở Chrome Agent ở chế độ ĐĂNG NHẬP (không bật debug port) — Google sẽ không chặn.")
	}
	if err := sysproc.Command(ChromeExecutablePath(), args...).Start(); err != nil {
		return fmt.Errorf("không khởi động được Chrome tại %s: %w", ChromeExecutablePath(), err)
	}
	if logf != nil {
		logf("👉 Đăng nhập xong thì ĐÓNG cửa sổ này lại, rồi bấm 'Kích hoạt Agent'.")
	}
	return nil
}

// EnsureAgentChromeSession makes the agent's persistent Chrome profile reachable
// over CDP and returns the port it is listening on. It reuses an already-running
// instance and never touches — or shuts down — the user's own Chrome.
func EnsureAgentChromeSession(targetURL string, logf func(string)) (int, error) {
	log := func(format string, args ...interface{}) {
		if logf != nil {
			logf(fmt.Sprintf(format, args...))
		}
	}

	userDataDir := AgentUserDataDir()
	if err := os.MkdirAll(userDataDir, 0o755); err != nil {
		return 0, fmt.Errorf("không tạo được thư mục profile agent %s: %w", userDataDir, err)
	}

	if port := liveAgentPort(userDataDir); port > 0 {
		log("🌐 Chrome Agent đã chạy sẵn trên port %d — kết nối lại (giữ nguyên các phiên đã đăng nhập).", port)
		return port, nil
	}

	// The profile may still be open from login mode (no debug port). Launching
	// again would just add a window to that portless process and the poll below
	// would time out, so close it first — matched by profile dir, so the user's
	// own Chrome is never involved.
	closeAgentChrome(logf)

	// A stale file from a previous run would be read back as a live port.
	_ = os.Remove(filepath.Join(userDataDir, "DevToolsActivePort"))

	args := []string{
		// Port 0 = let the OS assign a free one; Chrome reports it back via
		// DevToolsActivePort. Avoids clashing with anything already on 9222.
		"--remote-debugging-port=0",
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
	}
	// Deliberately NOT passing targetURL here. chromedp opens its own target and
	// navigates it, so handing the URL to the launch as well produced a second
	// window per run showing the same page — which is where the stack of Chrome
	// windows came from. The parameter is kept because callers pass it and the
	// signature is used elsewhere; it no longer opens anything.
	_ = targetURL

	log("🚀 Mở Chrome Agent (profile riêng: %s)...", userDataDir)
	if err := sysproc.Command(ChromeExecutablePath(), args...).Start(); err != nil {
		return 0, fmt.Errorf("không khởi động được Chrome tại %s: %w", ChromeExecutablePath(), err)
	}

	for i := 0; i < 40; i++ {
		time.Sleep(500 * time.Millisecond)
		if port := liveAgentPort(userDataDir); port > 0 {
			log("✅ Chrome Agent sẵn sàng trên port %d.", port)
			return port, nil
		}
	}

	return 0, fmt.Errorf("Chrome không mở được debug port sau 20 giây (thử chạy thủ công: chrome.exe --remote-debugging-port=0 --user-data-dir=\"%s\")", userDataDir)
}

// loginWallURLHints are path fragments that almost always mean "you are not
// signed in" rather than being part of normal content.
var loginWallURLHints = []string{
	"/login", "/signin", "/sign_in", "/sign-in", "/session/new",
	"accounts.google.com", "/oauth/authorize", "/auth/realms",
}

var loginWallTitleHints = []string{
	"sign in", "log in", "login", "đăng nhập", "sign up",
}

// LooksLikeLoginWall guesses whether the agent landed on a sign-in page instead
// of the page it asked for. Used to prompt the user rather than letting the AI
// burn its whole step budget on a login form it cannot fill.
func LooksLikeLoginWall(pageURL, title string) bool {
	u := strings.ToLower(pageURL)
	for _, hint := range loginWallURLHints {
		if strings.Contains(u, hint) {
			return true
		}
	}
	t := strings.ToLower(title)
	for _, hint := range loginWallTitleHints {
		if strings.HasPrefix(t, hint) {
			return true
		}
	}
	return false
}
