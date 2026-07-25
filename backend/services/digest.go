package services

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"claude_suite/backend/models"
	"claude_suite/backend/textutil"
)

// DigestInput is everything a digest is built from. Taking plain slices rather
// than repositories keeps this testable without a database, and keeps the
// summary logic out of the Wails layer, which CI cannot compile.
type DigestInput struct {
	Tasks  []models.Task
	Agents []models.Agent
	Since  time.Time
}

// BuildDigest summarises what the agents did, for the daily message sent to the
// configured webhook. It answers the question someone actually has in the
// morning — did anything finish, did anything break, and what did it cost —
// rather than listing every row.
func BuildDigest(in DigestInput) string {
	var done, failed, running, waiting []models.Task
	for _, task := range in.Tasks {
		switch task.Status {
		case "done":
			if in.Since.IsZero() || task.FinishedAt.After(in.Since) {
				done = append(done, task)
			}
		case "failed":
			failed = append(failed, task)
		case "running":
			running = append(running, task)
		case "backlog", "queued":
			waiting = append(waiting, task)
		}
	}

	var tokens int64
	busiest := ""
	best := 0
	for _, agent := range in.Agents {
		tokens += agent.TokensUsed
		if agent.TasksDone > best {
			best, busiest = agent.TasksDone, agent.Name
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "🗒️ Tổng kết Claude Suite — %s\n\n", time.Now().Format("02/01/2006"))
	fmt.Fprintf(&b, "✅ Hoàn thành: %d   ❌ Thất bại: %d   ⏳ Đang chạy: %d   📋 Chờ: %d\n",
		len(done), len(failed), len(running), len(waiting))

	if tokens > 0 {
		fmt.Fprintf(&b, "🔢 Token đã dùng: %s\n", humaniseCount(tokens))
	}
	if busiest != "" {
		fmt.Fprintf(&b, "🏆 Agent làm nhiều nhất: %s (%d task)\n", busiest, best)
	}

	// Failures first: they are the only part that needs someone to act.
	if len(failed) > 0 {
		b.WriteString("\n❌ Cần xem lại:\n")
		for _, task := range topN(failed, 5) {
			fmt.Fprintf(&b, "  • %s\n", oneLineTitle(task))
		}
		if len(failed) > 5 {
			fmt.Fprintf(&b, "  … và %d task khác\n", len(failed)-5)
		}
	}

	if len(done) > 0 {
		b.WriteString("\n✅ Đã xong:\n")
		for _, task := range topN(done, 5) {
			fmt.Fprintf(&b, "  • %s\n", oneLineTitle(task))
		}
		if len(done) > 5 {
			fmt.Fprintf(&b, "  … và %d task khác\n", len(done)-5)
		}
	}

	if len(done) == 0 && len(failed) == 0 && len(running) == 0 {
		b.WriteString("\nKhông có hoạt động nào trong kỳ này.\n")
	}

	return b.String()
}

func topN(tasks []models.Task, n int) []models.Task {
	sorted := append([]models.Task(nil), tasks...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].FinishedAt.After(sorted[j].FinishedAt) })
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}

func oneLineTitle(task models.Task) string {
	// textutil, not a byte slice: task titles are routinely Vietnamese, and
	// cutting one mid-character is how the digest would arrive full of U+FFFD.
	return textutil.Truncate(strings.Join(strings.Fields(task.Title), " "), 80, "…")
}

func humaniseCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
