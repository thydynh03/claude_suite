package services

import (
	"agent_center/backend/textutil"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"agent_center/backend/cli"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

type BrowserActionResult struct {
	URL              string   `json:"url"`
	Title            string   `json:"title"`
	HTMLSnippet      string   `json:"html_snippet"`
	TextContent      string   `json:"text_content"`
	ScreenshotBase64 string   `json:"screenshot_base64"`
	ConsoleErrors    []string `json:"console_errors"`
	Assertions       []string `json:"assertions"`
	Passed           bool     `json:"passed"`
	Logs             []string `json:"logs"`
	Success          bool     `json:"success"`
	Error            string   `json:"error"`
	AIResponse       string   `json:"ai_response"`
	Status           string   `json:"status"`            // "completed", "in_progress", "need_user_intervention", "stopped", "failed"
	CurrentStep      int      `json:"current_step"`      // Current step count
	MaxSteps         int      `json:"max_steps"`         // Max step count
	UserIntervention string   `json:"user_intervention"` // Reason if user intervention is required
}

type BrowserAgentService struct {
	mu         sync.Mutex
	cancelFunc context.CancelFunc

	askMu    sync.Mutex
	askChans map[string]chan string
	emit     BrowserEventEmitter
}

func NewBrowserAgentService() *BrowserAgentService {
	return &BrowserAgentService{}
}

func (s *BrowserAgentService) StopBrowserTask() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
	}
	s.cancelPendingAsks()
}

func isPortOpen(host string, port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 1*time.Second)
	if err != nil {
		return false
	}
	if conn != nil {
		_ = conn.Close()
		return true
	}
	return false
}

// IsPortOpen is the exported version of isPortOpen for use in other packages.
func IsPortOpen(host string, port int) bool {
	return isPortOpen(host, port)
}

func getChromeExecOptions(headless bool) []chromedp.ExecAllocatorOption {
	return append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("ignore-certificate-errors", true),
	)
}

func createCDPAContext(parentCtx context.Context, targetURL string, headless bool, usePersistentProfile bool, logf func(string)) (context.Context, context.CancelFunc, context.CancelFunc, error) {
	if usePersistentProfile {
		port, err := EnsureAgentChromeSession(targetURL, logf)
		if err != nil {
			return nil, nil, nil, err
		}
		wsURL, err := chromeWebSocketURL(port)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("không đọc được webSocketDebuggerUrl trên port %d: %w", port, err)
		}
		if logf != nil {
			logf("🔑 Đã attach vào Chrome Agent qua CDP — dùng lại các phiên đăng nhập đã lưu trong profile này.")
		}
		allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(parentCtx, wsURL)
		ctx, cancelCtx := chromedp.NewContext(allocCtx)
		return ctx, cancelCtx, cancelAlloc, nil
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parentCtx, getChromeExecOptions(headless)...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	return ctx, cancelCtx, cancelAlloc, nil
}

// jsString escapes a value for safe interpolation into an evaluated JS expression.
func jsString(v string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `""`
	}
	return string(b)
}

func truncateHTML(html string) string {
	return textutil.Truncate(html, 8000, "\n... [HTML Snippet Truncated]")
}

func truncateText(text string) string {
	return textutil.Truncate(text, 4000, "\n... [Text Content Truncated]")
}

// normalizeNavURL makes a user-typed target navigable over CDP. Page.navigate
// takes its URL literally: "google.com" fails with "Cannot navigate to
// invalid URL (-32000)" — even though the same text works in the omnibox and
// as a Chrome launch argument, which is exactly why it looks like it should
// work. Anything without an explicit scheme gets https://.
func normalizeNavURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return u
	}
	lower := strings.ToLower(u)
	for _, scheme := range []string{"http://", "https://", "about:", "file://", "data:", "chrome://", "ws://", "wss://"} {
		if strings.HasPrefix(lower, scheme) {
			return u
		}
	}
	return "https://" + u
}

// RunBrowserTask performs browser navigation, DOM inspection & full-page screenshot via Chrome CDP
func (s *BrowserAgentService) RunBrowserTask(targetURL string, prompt string, takeScreenshot bool, headless bool) (*BrowserActionResult, error) {
	return s.RunE2ETestWithProfile(targetURL, prompt, nil, takeScreenshot, headless, false)
}

// RunE2ETest navigates to a URL, captures console errors, and verifies that each
// expected text fragment is present on the page — a real assertion-based E2E pass.
func (s *BrowserAgentService) RunE2ETest(targetURL string, prompt string, expectTexts []string, takeScreenshot bool, headless bool) (*BrowserActionResult, error) {
	return s.RunE2ETestWithProfile(targetURL, prompt, expectTexts, takeScreenshot, headless, false)
}

func (s *BrowserAgentService) RunE2ETestWithProfile(targetURL string, prompt string, expectTexts []string, takeScreenshot bool, headless bool, useRealProfile bool) (*BrowserActionResult, error) {
	if targetURL == "" {
		return nil, fmt.Errorf("target URL cannot be empty")
	}
	targetURL = normalizeNavURL(targetURL)

	var logs []string
	logf := func(msg string) { logs = append(logs, msg) }

	ctx, cancelCtx, cancelAlloc, err := createCDPAContext(context.Background(), targetURL, headless, useRealProfile, logf)
	if err != nil {
		logs = append(logs, fmt.Sprintf("❌ Không kết nối được Chrome: %v", err))
		return &BrowserActionResult{
			URL:     targetURL,
			Success: false,
			Error:   err.Error(),
			Logs:    logs,
			Status:  "failed",
		}, err
	}
	defer cancelAlloc()
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, 35*time.Second)
	defer cancelTimeout()

	var title, html, text string
	var buf []byte

	var consoleMu sync.Mutex
	var consoleErrors []string
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			if e.Type == "error" {
				parts := make([]string, 0, len(e.Args))
				for _, a := range e.Args {
					if a.Value != nil {
						parts = append(parts, strings.Trim(string(a.Value), "\""))
					}
				}
				consoleMu.Lock()
				consoleErrors = append(consoleErrors, strings.Join(parts, " "))
				consoleMu.Unlock()
			}
		case *runtime.EventExceptionThrown:
			if e.ExceptionDetails != nil {
				consoleMu.Lock()
				consoleErrors = append(consoleErrors, e.ExceptionDetails.Text)
				consoleMu.Unlock()
			}
		}
	})

	logs = append(logs, fmt.Sprintf("🎯 Điều hướng tới URL: %s", targetURL))
	if prompt != "" {
		logs = append(logs, fmt.Sprintf("🎯 Nhận lệnh từ người dùng: \"%s\"", prompt))
	}

	tasks := chromedp.Tasks{
		chromedp.Navigate(targetURL),
		chromedp.Sleep(2 * time.Second),
		chromedp.Title(&title),
		chromedp.ActionFunc(func(ctx context.Context) error {
			logs = append(logs, "🔍 Đang trích xuất cấu trúc DOM & nội dung văn bản...")
			return nil
		}),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
		chromedp.Text("body", &text, chromedp.ByQuery),
	}

	if takeScreenshot {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			logs = append(logs, "📸 Chụp ảnh màn hình trang web (Capture Full-Page Screenshot)...")
			return nil
		}), chromedp.FullScreenshot(&buf, 90))
	}

	err = chromedp.Run(ctx, tasks)
	if err != nil {
		logs = append(logs, fmt.Sprintf("❌ Lỗi khi điều khiển Chrome: %v", err))
		return &BrowserActionResult{
			URL:     targetURL,
			Success: false,
			Error:   err.Error(),
			Logs:    logs,
			Status:  "failed",
		}, err
	}

	logs = append(logs, fmt.Sprintf("✅ Tải trang thành công! Tiêu đề: '%s'", title))

	passed := true
	var assertions []string
	for _, want := range expectTexts {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		if strings.Contains(text, want) {
			assertions = append(assertions, "✅ Tìm thấy: \""+want+"\"")
		} else {
			assertions = append(assertions, "❌ KHÔNG tìm thấy: \""+want+"\"")
			passed = false
		}
	}

	consoleMu.Lock()
	errs := append([]string(nil), consoleErrors...)
	consoleMu.Unlock()
	if len(errs) > 0 {
		passed = false
		logs = append(logs, fmt.Sprintf("⚠️ Phát hiện %d lỗi console trên trang.", len(errs)))
	}

	screenshotB64 := ""
	if len(buf) > 0 {
		screenshotB64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf)
	}

	return &BrowserActionResult{
		URL:              targetURL,
		Title:            title,
		HTMLSnippet:      truncateHTML(html),
		TextContent:      truncateText(text),
		ScreenshotBase64: screenshotB64,
		ConsoleErrors:    errs,
		Assertions:       assertions,
		Passed:           passed,
		Logs:             logs,
		Success:          true,
		Status:           "in_progress",
	}, nil
}

// RunAutonomousBrowserTask performs multi-step ReAct loop using a SINGLE Chrome CDP context
func (s *BrowserAgentService) RunAutonomousBrowserTask(
	targetURL string,
	prompt string,
	systemPrompt string,
	model string,
	takeScreenshot bool,
	headless bool,
	usePersistentProfile bool,
	maxSteps int,
	runner cli.CLIRunner,
	onLog func(msg string),
	// onFrame receives a screenshot of the page at the start of each step, as a
	// data URL. Without it the only visible evidence of a multi-step run was the
	// log text and a single screenshot at the end, so a headless agent doing five
	// steps was something the user had to take on trust. May be nil.
	onFrame func(step int, dataURL string),
) (*BrowserActionResult, error) {
	if maxSteps <= 0 {
		maxSteps = 5
	}

	result := &BrowserActionResult{
		URL:         targetURL,
		MaxSteps:    maxSteps,
		CurrentStep: 1,
		Status:      "in_progress",
		Success:     true,
	}

	logMsg := func(msg string) {
		result.Logs = append(result.Logs, msg)
		if onLog != nil {
			onLog(msg)
		}
	}

	if targetURL == "" {
		result.Success = false
		result.Error = "Target URL cannot be empty"
		result.Status = "failed"
		return result, fmt.Errorf("target URL cannot be empty")
	}
	targetURL = normalizeNavURL(targetURL)
	result.URL = targetURL

	// Cancellable task context
	ctx, cancelTask := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancelFunc = cancelTask
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancelFunc = nil
		s.mu.Unlock()
		cancelTask()
	}()

	// Allocate SINGLE CDP Context for the entire multi-step loop
	if usePersistentProfile && headless {
		logMsg("ℹ️ Chế độ Profile bền vững luôn chạy Chrome hiển thị (bỏ qua tuỳ chọn Headless) để bạn đăng nhập được khi cần.")
	}
	cdpCtx, cancelCtx, cancelAlloc, err := createCDPAContext(ctx, targetURL, headless, usePersistentProfile, logMsg)
	if err != nil {
		logMsg(fmt.Sprintf("❌ Không kết nối được Chrome: %v", err))
		result.Success = false
		result.Error = err.Error()
		result.Status = "failed"
		return result, err
	}
	defer cancelAlloc()
	defer cancelCtx()

	// Initial Navigation & DOM Extraction
	var title, html, text, landedURL string
	var buf []byte

	logMsg(fmt.Sprintf("🌐 Chrome CDP Agent: Kết nối & Điều hướng tới: %s", targetURL))
	if prompt != "" {
		logMsg(fmt.Sprintf("🎯 Nhận lệnh từ người dùng: \"%s\"", prompt))
	}

	initTasks := chromedp.Tasks{
		chromedp.Navigate(targetURL),
		chromedp.Sleep(2 * time.Second),
		chromedp.Title(&title),
		chromedp.ActionFunc(func(c context.Context) error {
			logMsg("🔍 Đang trích xuất cấu trúc DOM & nội dung văn bản...")
			return nil
		}),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
		chromedp.Text("body", &text, chromedp.ByQuery),
		chromedp.Location(&landedURL),
	}
	if takeScreenshot {
		initTasks = append(initTasks, chromedp.ActionFunc(func(c context.Context) error {
			logMsg("📸 Chụp ảnh màn hình trang web (Full-Page Screenshot)...")
			return nil
		}), chromedp.FullScreenshot(&buf, 90))
	}

	initCtx, cancelInit := context.WithTimeout(cdpCtx, 60*time.Second)
	defer cancelInit()
	if err := chromedp.Run(initCtx, initTasks); err != nil {
		logMsg(fmt.Sprintf("❌ Lỗi khi điều khiển Chrome: %v", err))
		result.Success = false
		result.Error = err.Error()
		result.Status = "failed"
		return result, err
	}

	result.Title = title
	result.HTMLSnippet = truncateHTML(html)
	result.TextContent = truncateText(text)
	if len(buf) > 0 {
		result.ScreenshotBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf)
	}
	if landedURL != "" {
		result.URL = landedURL
	}
	logMsg(fmt.Sprintf("✅ Tải trang thành công! Tiêu đề: '%s'", title))

	// Free-text answer to the login prompt below; seeded into the AI's history.
	userGuidance := ""

	// A sign-in wall would otherwise eat the entire step budget on a form the AI
	// cannot fill. Park the run on a choice form instead of killing it.
	if usePersistentProfile && LooksLikeLoginWall(result.URL, title) {
		logMsg(fmt.Sprintf("🔐 Trang đang yêu cầu đăng nhập: %s", result.URL))
		answer, askErr := s.AskUser(ctx,
			fmt.Sprintf("Trang này cần đăng nhập (%s).\nHãy đăng nhập trong cửa sổ Chrome Agent vừa mở — profile sẽ ghi nhớ cho các lần sau — rồi chọn bên dưới.", result.URL),
			[]BrowserAskOption{
				{Label: "✅ Tôi đã đăng nhập xong — tải lại trang", Value: "reload"},
				{Label: "➡️ Cứ tiếp tục, không cần đăng nhập", Value: "continue"},
				{Label: "🛑 Dừng agent", Value: "stop"},
			})
		switch {
		case askErr != nil:
			logMsg(fmt.Sprintf("⚠️ Không hỏi được người dùng (%v) — dừng để chờ can thiệp.", askErr))
			result.Status = "need_user_intervention"
			result.UserIntervention = fmt.Sprintf("Cần đăng nhập tại %s", result.URL)
			return result, nil
		case answer == "stop":
			logMsg("🛑 Người dùng chọn dừng agent tại bước đăng nhập.")
			result.Status = "stopped"
			return result, nil
		case answer == "continue":
			logMsg("➡️ Người dùng chọn tiếp tục dù chưa đăng nhập.")
		case answer == "reload":
			logMsg("🔄 Người dùng xác nhận đã đăng nhập — tải lại trang đích...")
			if err := s.executeActionOnCDPContext(cdpCtx, "reload", "", targetURL, "", 0, takeScreenshot, result, logMsg); err != nil {
				logMsg(fmt.Sprintf("⚠️ Tải lại trang thất bại: %v", err))
			}
			title = result.Title
		default:
			// Free text is an instruction, not a "reload" — hand it to the AI verbatim.
			logMsg(fmt.Sprintf("💬 Chỉ dẫn của người dùng: \"%s\" — tiếp tục theo hướng đó.", answer))
			userGuidance = answer
		}
	}

	if strings.TrimSpace(prompt) == "" || runner == nil {
		result.Status = "completed"
		return result, nil
	}

	logMsg(fmt.Sprintf("🧠 Bộ não AI (%s): Bắt đầu vòng lặp ReAct Đa bước (Tối đa %d bước)...", model, maxSteps))
	actionHistory := []string{}
	if userGuidance != "" {
		actionHistory = append(actionHistory,
			fmt.Sprintf("Chỉ dẫn bổ sung từ người dùng: \"%s\". Hãy tuân theo chỉ dẫn này.", userGuidance))
	}

	for step := 1; step <= maxSteps; step++ {
		select {
		case <-ctx.Done():
			logMsg("🛑 Đã dừng Browser Agent theo yêu cầu của người dùng.")
			result.Status = "stopped"
			return result, nil
		default:
		}

		result.CurrentStep = step
		logMsg(fmt.Sprintf("🔄 [Bước %d/%d]: Đang phân tích DOM & lịch sử thao tác...", step, maxSteps))

		// Best effort: a frame that fails to capture is not worth failing a step
		// over, and the log already says what the agent is doing.
		// cdpCtx, not ctx: chromedp.Run on the plain task context returns
		// ErrInvalidContext unconditionally, which this guard then swallowed —
		// the live-frame feature was wired end to end and never fired once.
		if onFrame != nil {
			var frame []byte
			if err := chromedp.Run(cdpCtx, chromedp.CaptureScreenshot(&frame)); err == nil && len(frame) > 0 {
				onFrame(step, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(frame))
			}
		}

		historyStr := "Chưa có thao tác nào."
		if len(actionHistory) > 0 {
			historyStr = strings.Join(actionHistory, "\n")
		}

		fullPrompt := fmt.Sprintf(`SYSTEM ROLE & GUIDELINES:
%s

You are an Autonomous Web Browser Agent operating in a Multi-Step ReAct Loop.
Your goal is to complete the user instruction on the webpage step by step.

CURRENT STEP: %d / %d
PAGE TARGET URL: %s
PAGE TITLE: %s

USER INSTRUCTION PROMPT:
%s

LỊCH SỬ CÁC THAO TÁC ĐÃ THỰC HIỆN TRƯỚC ĐÓ:
%s

WEBPAGE TEXT CONTENT (BODY TEXT):
%s

WEBPAGE HTML SNIPPET (DOM HIỆN TẠI):
%s

HƯỚNG DẪN XỬ LÝ QUAN TRỌNG (BẮT BỘC TUÂN THỦ DẠNG JSON Ở CUỐI PHẢN HỒI):
1. Phân tích trạng thái hiện tại của trang web và đối chiếu với yêu cầu của người dùng.
2. Nếu bạn cần thực hiện một hành động (Click, Gõ văn bản, Scroll):
   BẮT BỘC xuất mã ở cuối câu trả lời dạng:
   ACTION_JSON: {"action": "click", "selector": "..."}
   hoặc ACTION_JSON: {"action": "type", "selector": "...", "text": "..."}
   hoặc ACTION_JSON: {"action": "scroll", "direction": "down"}
   hoặc ACTION_JSON: {"action": "wait", "seconds": 2}
   hoặc ACTION_JSON: {"action": "navigate", "text": "https://..."}

3. Nếu bạn ĐÃ HOÀN THÀNH MỤC TIÊU của người dùng (ví dụ: đã comment xong, hoặc đã tổng hợp thông tin xong):
   BẮT BỘC xuất mã ở cuối câu trả lời dạng:
   FINISH_JSON: {"status": "success", "summary": "Nội dung tóm tắt kết quả thành công..."}

4. Nếu bạn CẦN NGƯỜI DÙNG QUYẾT ĐỊNH hoặc CUNG CẤP THÔNG TIN (ví dụ: chọn 1 trong nhiều nút,
   cần nội dung comment cụ thể, cần xác nhận trước khi bấm Submit, không chắc chọn phần tử nào):
   ĐỪNG dừng lại. BẮT BUỘC xuất mã ở cuối câu trả lời dạng:
   ASK_JSON: {"question": "Câu hỏi rõ ràng cho người dùng...", "options": ["Lựa chọn 1", "Lựa chọn 2"]}
   Người dùng sẽ thấy form lựa chọn ngay trên Live Log (luôn kèm ô "Khác" để tự nhập).
   Câu trả lời sẽ được đưa lại cho bạn ở LỊCH SỬ THAO TÁC, và bạn TIẾP TỤC làm việc bình thường.

5. CHỈ dùng STOP_JSON khi thực sự BẾ TẮC và người dùng cũng không giúp được gì thêm:
   STOP_JSON: {"status": "need_user_intervention", "reason": "Lý do cụ thể..."}
`, systemPrompt, step, maxSteps, result.URL, result.Title, prompt, historyStr, result.TextContent, result.HTMLSnippet)

		runRes := runner.RunOnce(fullPrompt, model, systemPrompt, nil, "")
		if runRes == nil || !runRes.Success || runRes.Output == "" {
			errMsg := "Lỗi gọi AI Model"
			if runRes != nil && runRes.Error != "" {
				errMsg = runRes.Error
			}
			logMsg(fmt.Sprintf("⚠️ [Bước %d/%d] AI Model phản hồi lỗi: %s", step, maxSteps, errMsg))
			result.Status = "failed"
			break
		}

		result.AIResponse = runRes.Output

		// Check FINISH_JSON
		if strings.Contains(runRes.Output, "FINISH_JSON:") {
			finishPart := runRes.Output[strings.Index(runRes.Output, "FINISH_JSON:")+len("FINISH_JSON:"):]
			finishPart = strings.TrimSpace(finishPart)
			if idx := strings.Index(finishPart, "}"); idx != -1 {
				var fin struct {
					Status  string `json:"status"`
					Summary string `json:"summary"`
				}
				_ = json.Unmarshal([]byte(finishPart[:idx+1]), &fin)
				result.Status = "completed"
				logMsg(fmt.Sprintf("🎉 [Bước %d/%d] AI xác nhận HOÀN THÀNH TÁC VỤ! Summary: %s", step, maxSteps, fin.Summary))
				break
			}
		}

		// Check ASK_JSON — park the loop on a form instead of tearing the agent down.
		if strings.Contains(runRes.Output, "ASK_JSON:") {
			askPart := strings.TrimSpace(runRes.Output[strings.Index(runRes.Output, "ASK_JSON:")+len("ASK_JSON:"):])
			if idx := strings.Index(askPart, "}"); idx != -1 {
				var ask struct {
					Question string   `json:"question"`
					Options  []string `json:"options"`
				}
				if json.Unmarshal([]byte(askPart[:idx+1]), &ask) == nil && strings.TrimSpace(ask.Question) != "" {
					opts := make([]BrowserAskOption, 0, len(ask.Options))
					for _, o := range ask.Options {
						if strings.TrimSpace(o) != "" {
							opts = append(opts, BrowserAskOption{Label: o, Value: o})
						}
					}
					logMsg(fmt.Sprintf("❓ [Bước %d/%d] AI cần bạn quyết định: %s", step, maxSteps, ask.Question))

					answer, askErr := s.AskUser(ctx, ask.Question, opts)
					if askErr != nil {
						logMsg(fmt.Sprintf("⚠️ [Bước %d/%d] Không nhận được phản hồi (%v).", step, maxSteps, askErr))
						result.Status = "need_user_intervention"
						result.UserIntervention = ask.Question
						break
					}

					logMsg(fmt.Sprintf("💬 [Bước %d/%d] Người dùng trả lời: \"%s\"", step, maxSteps, answer))
					actionHistory = append(actionHistory,
						fmt.Sprintf("Bước %d: Đã hỏi người dùng \"%s\" -> Người dùng trả lời: \"%s\"", step, ask.Question, answer))
					continue
				}
			}
		}

		// Check STOP_JSON
		if strings.Contains(runRes.Output, "STOP_JSON:") {
			stopPart := runRes.Output[strings.Index(runRes.Output, "STOP_JSON:")+len("STOP_JSON:"):]
			stopPart = strings.TrimSpace(stopPart)
			if idx := strings.Index(stopPart, "}"); idx != -1 {
				var stp struct {
					Status string `json:"status"`
					Reason string `json:"reason"`
				}
				_ = json.Unmarshal([]byte(stopPart[:idx+1]), &stp)
				logMsg(fmt.Sprintf("🛑 [Bước %d/%d] AI báo bế tắc: %s", step, maxSteps, stp.Reason))

				// Offer a way out before giving up — the run stays alive either way.
				answer, askErr := s.AskUser(ctx,
					fmt.Sprintf("Agent đang bế tắc:\n%s\n\nBạn muốn xử lý thế nào?", stp.Reason),
					[]BrowserAskOption{
						{Label: "🔄 Tôi đã xử lý xong (đăng nhập / CAPTCHA / OTP) — thử lại", Value: "retry"},
						{Label: "➡️ Bỏ qua, để AI tự tìm cách khác", Value: "skip"},
						{Label: "🛑 Dừng agent tại đây", Value: "stop"},
					})

				if askErr != nil || answer == "stop" {
					result.Status = stp.Status
					if result.Status == "" || answer == "stop" {
						result.Status = "need_user_intervention"
					}
					result.UserIntervention = stp.Reason
					if askErr != nil {
						logMsg(fmt.Sprintf("⚠️ [Bước %d/%d] Không nhận được phản hồi (%v) — dừng vòng lặp.", step, maxSteps, askErr))
					} else {
						logMsg(fmt.Sprintf("🛑 [Bước %d/%d] Người dùng chọn dừng agent.", step, maxSteps))
					}
					break
				}

				logMsg(fmt.Sprintf("💬 [Bước %d/%d] Người dùng trả lời: \"%s\"", step, maxSteps, answer))
				if answer == "retry" {
					if err := s.executeActionOnCDPContext(cdpCtx, "reload", "", result.URL, "", 0, takeScreenshot, result, logMsg); err != nil {
						logMsg(fmt.Sprintf("⚠️ Tải lại trang thất bại: %v", err))
					}
					actionHistory = append(actionHistory,
						fmt.Sprintf("Bước %d: Bế tắc (%s) -> Người dùng đã xử lý thủ công, trang đã được tải lại. Hãy thử lại.", step, stp.Reason))
				} else {
					actionHistory = append(actionHistory,
						fmt.Sprintf("Bước %d: Bế tắc (%s) -> Chỉ dẫn của người dùng: \"%s\". Hãy làm theo.", step, stp.Reason, answer))
				}
				continue
			}
		}

		// Check ACTION_JSON
		if strings.Contains(runRes.Output, "ACTION_JSON:") {
			actionPart := runRes.Output[strings.Index(runRes.Output, "ACTION_JSON:")+len("ACTION_JSON:"):]
			actionPart = strings.TrimSpace(actionPart)
			if idx := strings.Index(actionPart, "}"); idx != -1 {
				actionJSONStr := actionPart[:idx+1]
				var act struct {
					Action    string `json:"action"`
					Selector  string `json:"selector"`
					Text      string `json:"text"`
					Direction string `json:"direction"`
					Seconds   int    `json:"seconds"`
				}
				if json.Unmarshal([]byte(actionJSONStr), &act) == nil && act.Action != "" {
					actionMsg := fmt.Sprintf("Bước %d: Action=%s, Selector=%s, Text=%s", step, act.Action, act.Selector, act.Text)
					actionHistory = append(actionHistory, actionMsg)
					logMsg(fmt.Sprintf("⚡ [Bước %d/%d] Đang thực thi CDP Action trên cùng 1 tab: %s", step, maxSteps, actionMsg))

					if err := s.executeActionOnCDPContext(cdpCtx, act.Action, act.Selector, act.Text, act.Direction, act.Seconds, takeScreenshot, result, logMsg); err != nil {
						logMsg(fmt.Sprintf("⚠️ [Bước %d/%d] Thao tác CDP bị lỗi: %v", step, maxSteps, err))
					}
				}
			}
		} else {
			logMsg(fmt.Sprintf("ℹ️ [Bước %d/%d] AI không tạo thêm hành động mới, kết thúc chu kỳ.", step, maxSteps))
			result.Status = "completed"
			break
		}
	}

	if result.Status == "in_progress" {
		result.Status = "completed"
	}

	return result, nil
}

func (s *BrowserAgentService) executeActionOnCDPContext(
	ctx context.Context,
	action, selector, text, direction string,
	seconds int,
	takeScreenshot bool,
	result *BrowserActionResult,
	logMsg func(string),
) error {
	var tasks chromedp.Tasks

	switch strings.ToLower(action) {
	case "click":
		if selector != "" {
			tasks = append(tasks,
				chromedp.ActionFunc(func(c context.Context) error {
					logMsg(fmt.Sprintf("🖱️ Auto-scroll Into View & Click phần tử: '%s'", selector))
					return nil
				}),
				chromedp.Evaluate(fmt.Sprintf("document.querySelector(%s)?.scrollIntoView({behavior: 'smooth', block: 'center'})", jsString(selector)), nil),
				chromedp.Sleep(1*time.Second),
				chromedp.Evaluate(fmt.Sprintf("document.querySelector(%s)?.click()", jsString(selector)), nil),
				chromedp.Sleep(2*time.Second),
			)
		}
	case "type":
		if selector != "" {
			tasks = append(tasks,
				chromedp.ActionFunc(func(c context.Context) error {
					logMsg(fmt.Sprintf("⌨️ Auto-scroll Into View, Focus & Type '%s' vào '%s'", text, selector))
					return nil
				}),
				chromedp.Evaluate(fmt.Sprintf("document.querySelector(%s)?.scrollIntoView({behavior: 'smooth', block: 'center'})", jsString(selector)), nil),
				chromedp.Sleep(1*time.Second),
				chromedp.SendKeys(selector, text, chromedp.ByQuery),
				chromedp.Sleep(1*time.Second),
			)
		}
	case "scroll":
		scrollAmount := 800
		if strings.ToLower(direction) == "up" {
			scrollAmount = -800
		}
		tasks = append(tasks,
			chromedp.ActionFunc(func(c context.Context) error {
				logMsg(fmt.Sprintf("📜 Scroll trang web (%d px)", scrollAmount))
				return nil
			}),
			chromedp.Evaluate(fmt.Sprintf("window.scrollBy(0, %d)", scrollAmount), nil),
			chromedp.Sleep(2*time.Second),
		)
	case "wait":
		if seconds <= 0 {
			seconds = 2
		}
		tasks = append(tasks, chromedp.Sleep(time.Duration(seconds)*time.Second))
	case "reload", "navigate":
		// The AI writes this target too, and it drops the scheme as readily
		// as a person does.
		target := normalizeNavURL(text)
		if target == "" {
			target = result.URL
		}
		tasks = append(tasks,
			chromedp.ActionFunc(func(c context.Context) error {
				logMsg(fmt.Sprintf("🔄 Điều hướng tới: %s", target))
				return nil
			}),
			chromedp.Navigate(target),
			chromedp.Sleep(2*time.Second),
		)
	}

	var newTitle, newHTML, newText, newURL string
	var buf []byte
	tasks = append(tasks,
		chromedp.Title(&newTitle),
		chromedp.OuterHTML("html", &newHTML, chromedp.ByQuery),
		chromedp.Text("body", &newText, chromedp.ByQuery),
		chromedp.Location(&newURL),
	)
	if takeScreenshot {
		tasks = append(tasks, chromedp.FullScreenshot(&buf, 90))
	}

	runCtx, cancelRun := context.WithTimeout(ctx, 60*time.Second)
	defer cancelRun()
	if err := chromedp.Run(runCtx, tasks); err != nil {
		return err
	}

	if newURL != "" {
		result.URL = newURL
	}
	if newTitle != "" {
		result.Title = newTitle
	}
	if newHTML != "" {
		result.HTMLSnippet = truncateHTML(newHTML)
	}
	if newText != "" {
		result.TextContent = truncateText(newText)
	}
	if len(buf) > 0 {
		result.ScreenshotBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf)
	}
	logMsg("✅ Cập nhật DOM, Text & Screenshot mới nhất sau thao tác!")
	return nil
}
