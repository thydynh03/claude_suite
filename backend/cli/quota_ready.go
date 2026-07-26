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

// QuotaReady reports whether the pool has an account worth spending a run on.
//
// The judgment is the measured timestamp, never Status: rotation sets
// whatever account it lands on to "active" while that account's minutes-old
// 429 mark persists, so a fully exhausted pool always shows exactly one
// "active" account — a status-based gate read that as open and fired the job
// into the wall. An account is usable when it has no measured rate-limit at
// all, or when its last one is older than cooldown. Disabled accounts never
// count — switching one off was deliberate.
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
		if k.Status == statusDisabled {
			continue
		}
		last := k.Usage.LastRateLimitAt
		if last.IsZero() || now.After(last.Add(cooldown)) {
			usable++
			continue
		}
		readyAt := last.Add(cooldown)
		if soonest.IsZero() || readyAt.Before(soonest) {
			soonest = readyAt
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
