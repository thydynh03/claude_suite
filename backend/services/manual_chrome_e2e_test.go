package services

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Manual smoke test: launches a real browser. Run with CHROME_E2E=1.
func TestAgentChromeSessionE2E(t *testing.T) {
	if os.Getenv("CHROME_E2E") != "1" {
		t.Skip("set CHROME_E2E=1 to run")
	}

	logs := []string{}
	logf := func(m string) { logs = append(logs, m); t.Log(m) }

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cdpCtx, cancelCtx, cancelAlloc, err := createCDPAContext(ctx, "https://github.com/", false, true, logf)
	if err != nil {
		t.Fatalf("createCDPAContext: %v", err)
	}
	defer cancelAlloc()
	defer cancelCtx()

	var title, landed string
	if err := chromedp.Run(cdpCtx,
		chromedp.Navigate("https://github.com/"),
		chromedp.Sleep(4*time.Second),
		chromedp.Title(&title),
		chromedp.Location(&landed),
	); err != nil {
		t.Fatalf("chromedp.Run: %v", err)
	}

	t.Logf("TITLE=%q", title)
	t.Logf("URL=%q", landed)
	t.Logf("LooksLikeLoginWall=%v", LooksLikeLoginWall(landed, title))
	t.Logf("AgentUserDataDir=%s", AgentUserDataDir())

	ready, port, dir := ChromeAgentStatus()
	t.Logf("status ready=%v port=%d dir=%s", ready, port, dir)
	if !ready {
		t.Fatal("agent chrome should be reachable after createCDPAContext")
	}

	// Reconnect must reuse the running instance, not spawn a second Chrome.
	port2, err := EnsureAgentChromeSession("", logf)
	if err != nil {
		t.Fatalf("re-ensure: %v", err)
	}
	if port2 != port {
		t.Fatalf("expected reuse of port %d, got %d", port, port2)
	}
	t.Logf("REUSED port=%d ✓", port2)

	// A protected page must trip the login-wall detector so the agent asks the
	// user instead of burning its step budget.
	var pTitle, pURL string
	if err := chromedp.Run(cdpCtx,
		chromedp.Navigate("https://github.com/settings/profile"),
		chromedp.Sleep(4*time.Second),
		chromedp.Title(&pTitle),
		chromedp.Location(&pURL),
	); err != nil {
		t.Fatalf("protected nav: %v", err)
	}
	t.Logf("protected TITLE=%q", pTitle)
	t.Logf("protected URL=%q", pURL)
	t.Logf("protected LooksLikeLoginWall=%v", LooksLikeLoginWall(pURL, pTitle))
}
