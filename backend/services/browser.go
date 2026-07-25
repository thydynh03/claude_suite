package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
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
}

type BrowserAgentService struct{}

func NewBrowserAgentService() *BrowserAgentService {
	return &BrowserAgentService{}
}

// RunBrowserTask performs headless browser navigation, DOM inspection & full-page screenshot via Chrome CDP
func (s *BrowserAgentService) RunBrowserTask(targetURL string, takeScreenshot bool) (*BrowserActionResult, error) {
	return s.RunE2ETest(targetURL, nil, takeScreenshot)
}

// RunE2ETest navigates to a URL, captures console errors, and verifies that each
// expected text fragment is present on the page — a real assertion-based E2E pass.
// The test passes only when navigation succeeds, no console errors occur, and all
// expected texts are found.
func (s *BrowserAgentService) RunE2ETest(targetURL string, expectTexts []string, takeScreenshot bool) (*BrowserActionResult, error) {
	if targetURL == "" {
		return nil, fmt.Errorf("target URL cannot be empty")
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("ignore-certificate-errors", true),
	)

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

	logs = append(logs, fmt.Sprintf("🌐 Native Chrome CDP Agent: Bắt đầu kết nối & điều hướng tới: %s", targetURL))

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
			var err error
			buf, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatPng).Do(ctx)
			return err
		}))
	}

	err := chromedp.Run(ctx, tasks)
	if err != nil {
		logs = append(logs, fmt.Sprintf("❌ Lỗi khi điều khiển Chrome: %v", err))
		return &BrowserActionResult{
			URL:     targetURL,
			Success: false,
			Error:   err.Error(),
			Logs:    logs,
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

	// Truncate if too long for IPC
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
	}, nil
}
