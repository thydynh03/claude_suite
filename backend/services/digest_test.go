package services

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"agent_center/backend/models"
)

func TestBuildDigestCountsEachStatus(t *testing.T) {
	digest := BuildDigest(DigestInput{
		Tasks: []models.Task{
			{Title: "one", Status: "done"},
			{Title: "two", Status: "done"},
			{Title: "three", Status: "failed"},
			{Title: "four", Status: "running"},
			{Title: "five", Status: "backlog"},
			{Title: "six", Status: "queued"},
		},
	})

	for _, want := range []string{"Hoàn thành: 2", "Thất bại: 1", "Đang chạy: 1", "Chờ: 2"} {
		if !strings.Contains(digest, want) {
			t.Errorf("digest is missing %q:\n%s", want, digest)
		}
	}
}

// Failures are the only part someone has to act on, so they come before the
// successes rather than after a wall of them.
func TestBuildDigestPutsFailuresFirst(t *testing.T) {
	digest := BuildDigest(DigestInput{
		Tasks: []models.Task{
			{Title: "worked", Status: "done"},
			{Title: "broke", Status: "failed"},
		},
	})

	failed := strings.Index(digest, "Cần xem lại")
	done := strings.Index(digest, "Đã xong")
	if failed < 0 || done < 0 {
		t.Fatalf("digest is missing a section:\n%s", digest)
	}
	if failed > done {
		t.Errorf("failures are listed after the successes:\n%s", digest)
	}
}

func TestBuildDigestSummarisesAgents(t *testing.T) {
	digest := BuildDigest(DigestInput{
		Tasks: []models.Task{{Title: "one", Status: "done"}},
		Agents: []models.Agent{
			{Name: "Back-end Developer", TasksDone: 7, TokensUsed: 1_500_000},
			{Name: "QA/QC Specialist", TasksDone: 2, TokensUsed: 250_000},
		},
	})

	if !strings.Contains(digest, "Back-end Developer") {
		t.Errorf("the busiest agent is not named:\n%s", digest)
	}
	if !strings.Contains(digest, "1.8M") {
		t.Errorf("token total is missing or unrounded:\n%s", digest)
	}
}

// A long list is trimmed, but the digest has to say how many it left out rather
// than quietly showing five.
func TestBuildDigestSaysHowManyItLeftOut(t *testing.T) {
	var tasks []models.Task
	for i := 0; i < 9; i++ {
		tasks = append(tasks, models.Task{Title: "done task", Status: "done"})
	}

	digest := BuildDigest(DigestInput{Tasks: tasks})
	if !strings.Contains(digest, "và 4 task khác") {
		t.Errorf("the digest does not say what it omitted:\n%s", digest)
	}
}

func TestBuildDigestHandlesAQuietDay(t *testing.T) {
	digest := BuildDigest(DigestInput{})
	if !strings.Contains(digest, "Không có hoạt động") {
		t.Errorf("an empty digest should say so:\n%s", digest)
	}
}

// Task titles are routinely Vietnamese, and this used to trim with a byte slice.
func TestBuildDigestKeepsTitlesValid(t *testing.T) {
	long := strings.Repeat("kiểm thử ", 40)
	digest := BuildDigest(DigestInput{
		Tasks: []models.Task{{Title: long, Status: "failed"}},
	})

	if !utf8.ValidString(digest) {
		t.Error("the digest contains a broken character")
	}
}

// Only work finished after Since belongs in the digest; the rest already went
// out in an earlier one.
func TestBuildDigestRespectsTheWindow(t *testing.T) {
	now := time.Now()
	digest := BuildDigest(DigestInput{
		Since: now.Add(-time.Hour),
		Tasks: []models.Task{
			{Title: "recent", Status: "done", FinishedAt: now.Add(-time.Minute)},
			{Title: "yesterday", Status: "done", FinishedAt: now.Add(-48 * time.Hour)},
		},
	})

	if !strings.Contains(digest, "recent") {
		t.Errorf("recent work is missing:\n%s", digest)
	}
	if strings.Contains(digest, "yesterday") {
		t.Errorf("work from before the window was included:\n%s", digest)
	}
}
