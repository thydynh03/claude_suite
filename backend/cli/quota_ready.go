package cli

import (
	"fmt"
	"time"
)

// QuotaCooldown is how long after a MEASURED 429 an account is assumed usable
// again. Gemini's free-tier quotas reset daily, so this errs on the long side
// of that cycle. It is the one assumption in an otherwise measured system —
// the pool records when an account was turned away (Usage.LastRateLimitAt),
// but Google never tells us when it will be welcome back.
const QuotaCooldown = 24 * time.Hour

// QuotaReady reports whether the pool has an account worth spending a run on:
// one that is active, or one whose last measured rate-limit is older than
// cooldown. Disabled accounts never count — switching one off was deliberate.
//
// An empty pool is ready: the CLI then authenticates from the environment,
// about which this app measures nothing and therefore claims nothing.
func (p *AccountKeyPool) QuotaReady(cooldown time.Duration) (bool, string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.keys) == 0 {
		return true, "pool trống — CLI tự xác thực từ môi trường"
	}

	now := time.Now()
	usable := 0
	var soonest time.Time
	for i := range p.keys {
		k := &p.keys[i]
		switch k.Status {
		case statusDisabled:
			continue
		case "rate_limited_429":
			// A mark with no measured time cannot schedule anything; treat
			// the account as usable rather than holding a job forever on a
			// timestamp nobody recorded.
			if k.Usage.LastRateLimitAt.IsZero() {
				usable++
				continue
			}
			readyAt := k.Usage.LastRateLimitAt.Add(cooldown)
			if now.After(readyAt) {
				usable++
			} else if soonest.IsZero() || readyAt.Before(soonest) {
				soonest = readyAt
			}
		default:
			usable++
		}
	}

	if usable > 0 {
		return true, fmt.Sprintf("%d tài khoản sẵn sàng", usable)
	}
	if soonest.IsZero() {
		return false, "mọi tài khoản trong pool đều đang tắt"
	}
	return false, "quota sớm nhất dự kiến hồi lúc " + soonest.Format("15:04 02/01")
}
