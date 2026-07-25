package services

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Manual: screenshots a page of the running `wails dev` UI.
// Run with UI_SHOT=<output.png> and UI_SHOT_NAV=<sidebar button title>.
func TestScreenshotDevUI(t *testing.T) {
	out := os.Getenv("UI_SHOT")
	if out == "" {
		t.Skip("set UI_SHOT=<path.png> to run")
	}
	nav := os.Getenv("UI_SHOT_NAV")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cdpCtx, cancelCtx, cancelAlloc, err := createCDPAContext(ctx, "", false, true, func(m string) { t.Log(m) })
	if err != nil {
		t.Fatalf("cdp: %v", err)
	}
	defer cancelAlloc()
	defer cancelCtx()

	tasks := chromedp.Tasks{
		chromedp.EmulateViewport(1280, 900),
		chromedp.Navigate("http://localhost:34115"),
		chromedp.Sleep(5 * time.Second),
	}
	if nav != "" {
		tasks = append(tasks,
			chromedp.Evaluate(fmt.Sprintf(`(() => {
				const b = [...document.querySelectorAll('button')].find(x => (x.title||'').includes(%q));
				if (b) { b.click(); return 'clicked'; }
				return 'not found';
			})()`, nav), nil),
			// WebGL needs a few frames after mount before it has drawn anything.
			chromedp.Sleep(8*time.Second),
		)
	}

	var buf []byte
	tasks = append(tasks, chromedp.CaptureScreenshot(&buf))
	if err := chromedp.Run(cdpCtx, tasks); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := os.WriteFile(out, buf, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("wrote %s (%d bytes)", out, len(buf))
}
