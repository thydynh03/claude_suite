package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"claude_suite/backend/cli"

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
	Status           string   `json:"status"`            // "completed", "in_progress", "need_user_intervention", "failed"
	CurrentStep      int      `json:"current_step"`      // Current step count
	MaxSteps         int      `json:"max_steps"`         // Max step count
	UserIntervention string   `json:"user_intervention"` // Reason if user intervention is required
}

type BrowserAgentService struct{}

func NewBrowserAgentService() *BrowserAgentService {
	return &BrowserAgentService{}
}

func getChromeExecOptions(headless bool, useRealProfile bool) []chromedp.ExecAllocatorOption {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("ignore-certificate-errors", true),
	)

	if useRealProfile {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			userDataDir := filepath.Join(localAppData, "Google", "Chrome", "User Data")
			if _, err := os.Stat(userDataDir); err == nil {
				opts = append(opts, chromedp.UserDataDir(userDataDir))
			}
		}
	}
	return opts
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

	opts := getChromeExecOptions(headless, useRealProfile)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, 35*time.Second)
	defer cancelTimeout()

	var title, html, text string
	var buf []byte
	var logs []string

	// Capture browser console errors and uncaught exceptions.
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

	profileInfo := "Temp Profile"
	if useRealProfile {
		profileInfo = "Real Chrome Profile (Logged-in Sessions)"
	}
	logs = append(logs, fmt.Sprintf("🌐 Chrome CDP Agent: Kết nối (%s) & Điều hướng tới: %s", profileInfo, targetURL))
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

	err := chromedp.Run(ctx, tasks)
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

	// Run assertions: each expected text must appear in the page body.
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

	if len(html) > 8000 {
		html = html[:8000] + "\n... [HTML Snippet Truncated]"
	}
	if len(text) > 4000 {
		text = text[:4000] + "\n... [Text Content Truncated]"
	}

	return &BrowserActionResult{
		URL:              targetURL,
		Title:            title,
		HTMLSnippet:      html,
		TextContent:      text,
		ScreenshotBase64: screenshotB64,
		ConsoleErrors:    errs,
		Assertions:       assertions,
		Passed:           passed,
		Logs:             logs,
		Success:          true,
		Status:           "in_progress",
	}, nil
}

// RunAutonomousBrowserTask performs multi-step ReAct loop for web automation
func (s *BrowserAgentService) RunAutonomousBrowserTask(
	targetURL string,
	prompt string,
	systemPrompt string,
	model string,
	takeScreenshot bool,
	headless bool,
	useRealProfile bool,
	maxSteps int,
	runner cli.CLIRunner,
) (*BrowserActionResult, error) {
	if maxSteps <= 0 {
		maxSteps = 5
	}

	result, err := s.RunE2ETestWithProfile(targetURL, prompt, nil, takeScreenshot, headless, useRealProfile)
	if err != nil || result == nil || !result.Success {
		return result, err
	}

	result.MaxSteps = maxSteps
	result.CurrentStep = 1

	if strings.TrimSpace(prompt) == "" || runner == nil {
		result.Status = "completed"
		return result, nil
	}

	result.Logs = append(result.Logs, fmt.Sprintf("🧠 Bộ não AI (%s): Bắt đầu vòng lặp ReAct Đa bước (Tối đa %d bước)...", model, maxSteps))

	actionHistory := []string{}

	for step := 1; step <= maxSteps; step++ {
		result.CurrentStep = step
		result.Logs = append(result.Logs, fmt.Sprintf("🔄 [Bước %d/%d]: Đang phân tích trạng thái DOM & lịch sử hành động...", step, maxSteps))

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

3. Nếu bạn ĐÃ HOÀN THÀNH MỤC TIÊU của người dùng (ví dụ: đã comment xong, hoặc đã tổng hợp thông tin xong):
   BẮT BỘC xuất mã ở cuối câu trả lời dạng:
   FINISH_JSON: {"status": "success", "summary": "Nội dung tóm tắt kết quả thành công..."}

4. Nếu gặp LỖI KHÔNG THỂ PHỤC HỒI hoặc CẦN NGUỜI DÙNG CAN THIỆP (ví dụ: dính CAPTCHA, chưa đăng nhập, nút 2FA OTP, hoặc không thấy phần tử):
   BẮT BỘC xuất mã ở cuối câu trả lời dạng:
   STOP_JSON: {"status": "need_user_intervention", "reason": "Lý do cụ thể cần người dùng can thiệp..."}
`, systemPrompt, step, maxSteps, result.URL, result.Title, prompt, historyStr, result.TextContent, result.HTMLSnippet)

		runRes := runner.RunOnce(fullPrompt, model, systemPrompt, nil, "")
		if runRes == nil || !runRes.Success || runRes.Output == "" {
			errMsg := "Lỗi gọi AI Model"
			if runRes != nil && runRes.Error != "" {
				errMsg = runRes.Error
			}
			result.Logs = append(result.Logs, fmt.Sprintf("⚠️ [Bước %d/%d] AI Model phản hồi lỗi: %s", step, maxSteps, errMsg))
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
				result.Logs = append(result.Logs, fmt.Sprintf("🎉 [Bước %d/%d] AI xác nhận HOÀN THÀNH TÁC VỤ! Summary: %s", step, maxSteps, fin.Summary))
				break
			}
		}

		// Check STOP_JSON (User intervention)
		if strings.Contains(runRes.Output, "STOP_JSON:") {
			stopPart := runRes.Output[strings.Index(runRes.Output, "STOP_JSON:")+len("STOP_JSON:"):]
			stopPart = strings.TrimSpace(stopPart)
			if idx := strings.Index(stopPart, "}"); idx != -1 {
				var stp struct {
					Status string `json:"status"`
					Reason string `json:"reason"`
				}
				_ = json.Unmarshal([]byte(stopPart[:idx+1]), &stp)
				result.Status = stp.Status
				if result.Status == "" {
					result.Status = "need_user_intervention"
				}
				result.UserIntervention = stp.Reason
				result.Logs = append(result.Logs, fmt.Sprintf("🛑 [Bước %d/%d] Dừng vòng lặp: Cần người dùng can thiệp! Lý do: %s", step, maxSteps, stp.Reason))
				break
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
					result.Logs = append(result.Logs, fmt.Sprintf("⚡ [Bước %d/%d] Đang thực thi CDP Action: %s", step, maxSteps, actionMsg))

					s.executeCDPActionWithProfile(result.URL, act.Action, act.Selector, act.Text, act.Direction, act.Seconds, headless, useRealProfile, result)
				}
			}
		} else {
			result.Logs = append(result.Logs, fmt.Sprintf("ℹ️ [Bước %d/%d] AI không tạo thêm hành động mới, kết thúc chu kỳ.", step, maxSteps))
			result.Status = "completed"
			break
		}
	}

	if result.Status == "in_progress" {
		result.Status = "completed"
	}

	return result, nil
}

func (s *BrowserAgentService) executeCDPActionWithProfile(targetURL, action, selector, text, direction string, seconds int, headless bool, useRealProfile bool, result *BrowserActionResult) {
	opts := getChromeExecOptions(headless, useRealProfile)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()

	var tasks chromedp.Tasks

	switch strings.ToLower(action) {
	case "click":
		if selector != "" {
			tasks = append(tasks,
				chromedp.ActionFunc(func(ctx context.Context) error {
					result.Logs = append(result.Logs, fmt.Sprintf("🖱️ Auto-scroll Into View & Click: '%s'", selector))
					return nil
				}),
				chromedp.Evaluate(fmt.Sprintf("document.querySelector('%s')?.scrollIntoView({behavior: 'smooth', block: 'center'})", selector), nil),
				chromedp.Sleep(1*time.Second),
				chromedp.Evaluate(fmt.Sprintf("document.querySelector('%s')?.click()", selector), nil),
				chromedp.Sleep(2*time.Second),
			)
		}
	case "type":
		if selector != "" {
			tasks = append(tasks,
				chromedp.ActionFunc(func(ctx context.Context) error {
					result.Logs = append(result.Logs, fmt.Sprintf("⌨️ Auto-scroll Into View, Focus & Type '%s' vào '%s'", text, selector))
					return nil
				}),
				chromedp.Evaluate(fmt.Sprintf("document.querySelector('%s')?.scrollIntoView({behavior: 'smooth', block: 'center'})", selector), nil),
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
			chromedp.ActionFunc(func(ctx context.Context) error {
				result.Logs = append(result.Logs, fmt.Sprintf("📜 Scroll trang web (%d px)", scrollAmount))
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
	}

	var newTitle, newHTML, newText string
	var buf []byte
	tasks = append(tasks,
		chromedp.Title(&newTitle),
		chromedp.OuterHTML("html", &newHTML, chromedp.ByQuery),
		chromedp.Text("body", &newText, chromedp.ByQuery),
		chromedp.FullScreenshot(&buf, 90),
	)

	if err := chromedp.Run(ctx, tasks); err == nil {
		if newTitle != "" {
			result.Title = newTitle
		}
		if newHTML != "" {
			if len(newHTML) > 8000 {
				newHTML = newHTML[:8000] + "\n... [HTML Snippet Truncated]"
			}
			result.HTMLSnippet = newHTML
		}
		if newText != "" {
			if len(newText) > 4000 {
				newText = newText[:4000] + "\n... [Text Content Truncated]"
			}
			result.TextContent = newText
		}
		if len(buf) > 0 {
			result.ScreenshotBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf)
		}
		result.Logs = append(result.Logs, "✅ Cập nhật DOM, Text & Screenshot mới nhất sau thao tác!")
	} else {
		result.Logs = append(result.Logs, fmt.Sprintf("⚠️ Thao tác CDP Action thất bại: %v", err))
	}
}
