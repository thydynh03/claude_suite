package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type BrowserActionResult struct {
	URL              string   `json:"url"`
	Title            string   `json:"title"`
	HTMLSnippet      string   `json:"html_snippet"`
	TextContent      string   `json:"text_content"`
	ScreenshotBase64 string   `json:"screenshot_base64"`
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
		Logs:             logs,
		Success:          true,
	}, nil
}
