package services

import (
	"path/filepath"
	"strings"
	"testing"

	"agent_center/backend/cli"
	"agent_center/backend/database"
	"agent_center/backend/models"
	"agent_center/backend/testsupport"
)

type learnerHarness struct {
	learner  *Learner
	lessons  *database.LessonRepository
	obs      *database.ObservationRepository
	ws       *database.WorkspaceRepository
	attempts *database.AttemptRepository
	regress  *database.RegressionRepository
	runner   *testsupport.FakeRunner
	wsID     string
}

func newLearnerHarness(t *testing.T) *learnerHarness {
	t.Helper()
	db, err := database.OpenAt(filepath.Join(t.TempDir(), "learner_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	h := &learnerHarness{
		lessons:  database.NewLessonRepository(db),
		obs:      database.NewObservationRepository(db),
		ws:       database.NewWorkspaceRepository(db),
		attempts: database.NewAttemptRepository(db),
		regress:  database.NewRegressionRepository(db),
		runner:   &testsupport.FakeRunner{},
	}
	row, err := h.ws.EnsureWorkspace(`C:\fixture\project`, "")
	if err != nil {
		t.Fatal(err)
	}
	h.wsID = row.WorkspaceID
	h.learner = NewLearner(h.lessons, h.obs, h.ws, h.attempts, h.regress, nil, h.runner, "claude-haiku-4-5")
	h.learner.cooldown = 0
	return h
}

// A task that failed and then succeeded must leave an ACTIVE mechanical
// lesson carrying both the failure and the working approach.
func TestRetryThenSuccessCreatesAnActiveMechanicalLesson(t *testing.T) {
	h := newLearnerHarness(t)
	task := &models.Task{TaskID: "t1", Title: "[CODE] sửa updater", RetryCount: 2}
	_ = h.attempts.Add(task.TaskID, "npm ERR! peer dep conflict")

	h.learner.OnTaskFinished(h.wsID, "", task, &cli.RunResult{Success: true, Output: "Dùng npm install --legacy-peer-deps."}, nil)

	active, err := h.lessons.ListActive(h.wsID, 0.7, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("active lessons = %d, want 1", len(active))
	}
	l := active[0]
	if l.Kind != "mechanical" || l.Confidence != 0.9 {
		t.Fatalf("lesson shape wrong: %+v", l)
	}
	if !strings.Contains(l.Action, "peer dep conflict") || !strings.Contains(l.Action, "legacy-peer-deps") {
		t.Fatalf("lesson action missing failure or fix: %q", l.Action)
	}
}

// The confidence feedback loop: success raises, failure lowers, and a lesson
// that keeps failing archives itself.
func TestInjectedLessonConfidenceFollowsTaskOutcomes(t *testing.T) {
	h := newLearnerHarness(t)
	lesson := &models.MemoryLesson{WorkspaceID: h.wsID, Action: "do X", Confidence: 0.7, Status: "active", Trigger: "x"}
	if err := h.lessons.Add(lesson); err != nil {
		t.Fatal(err)
	}
	task := &models.Task{TaskID: "t1", Title: "task"}

	h.learner.OnTaskFinished(h.wsID, "", task, &cli.RunResult{Success: true}, []string{lesson.LessonID})
	after, _ := h.lessons.ListByStatus(h.wsID, "active", 10)
	if len(after) != 1 || after[0].Confidence <= 0.7 || after[0].UseCount != 1 {
		t.Fatalf("success must raise confidence and count use: %+v", after)
	}

	// Seven failures drive 0.72 below the 0.4 archive line.
	for i := 0; i < 7; i++ {
		h.learner.OnTaskFinished(h.wsID, "", task, &cli.RunResult{Success: false, Error: "x"}, []string{lesson.LessonID})
	}
	archived, _ := h.lessons.ListByStatus(h.wsID, "archived", 10)
	if len(archived) != 1 {
		t.Fatalf("a repeatedly failing lesson must archive itself, got %+v", archived)
	}
}

// Distilled lessons must arrive as PENDING (human review before injection),
// and the watermark advances only when the model call succeeds.
func TestDistillationProducesPendingLessonsAndAdvancesTheWatermark(t *testing.T) {
	h := newLearnerHarness(t)
	for i := 0; i < 25; i++ {
		_ = h.obs.Add(&models.Observation{WorkspaceID: h.wsID, TaskID: "t", Event: "tool_call", Payload: "🔧 Tool: Bash → npm test"})
	}

	h.runner.OnceBehaviour = func(prompt, model, system string) *cli.RunResult {
		if !strings.Contains(prompt, "OBSERVATIONS") {
			return &cli.RunResult{Success: false, Error: "unexpected prompt"}
		}
		return &cli.RunResult{Success: true, Output: `Here you go:
[{"trigger":"when running tests","action":"Chạy npm test trước khi báo done.","evidence":"seen 25 times","confidence":0.7}]`}
	}

	h.learner.runCycle(h.wsID)

	pending, err := h.lessons.ListByStatus(h.wsID, "pending", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending lessons = %d, want 1 (distilled output must NOT be active)", len(pending))
	}
	if pending[0].Kind != "distilled" {
		t.Fatalf("kind = %q, want distilled", pending[0].Kind)
	}

	row, _ := h.ws.Get(h.wsID)
	if row.DistilledThrough == 0 {
		t.Fatal("watermark did not advance after a successful distillation")
	}

	// Approved → appears in the injectable set.
	if err := h.lessons.SetStatus(pending[0].LessonID, "active"); err != nil {
		t.Fatal(err)
	}
	active, _ := h.lessons.ListActive(h.wsID, 0.7, 8)
	if len(active) != 1 {
		t.Fatalf("approved lesson must become injectable, got %d", len(active))
	}
}

func TestFailedDistillationRetainsTheBatch(t *testing.T) {
	h := newLearnerHarness(t)
	for i := 0; i < 25; i++ {
		_ = h.obs.Add(&models.Observation{WorkspaceID: h.wsID, Event: "tool_call", Payload: "x"})
	}
	h.runner.OnceBehaviour = func(prompt, model, system string) *cli.RunResult {
		return &cli.RunResult{Success: false, Error: "quota"}
	}

	h.learner.runCycle(h.wsID)

	row, _ := h.ws.Get(h.wsID)
	if row.DistilledThrough != 0 {
		t.Fatal("watermark advanced although distillation failed — batch would be lost")
	}
}

// The same build-verify failure line seen three times becomes a fact-backed
// mechanical lesson without any LLM involvement.
func TestRepeatedBuildFailuresBecomeAMechanicalLesson(t *testing.T) {
	h := newLearnerHarness(t)
	h.learner.SetLLMEnabled(false)
	for i := 0; i < 3; i++ {
		_ = h.obs.Add(&models.Observation{
			WorkspaceID: h.wsID, Event: "build_verify_failed",
			Payload: "go build failed: cannot find package agent_center/backend/foo",
		})
	}

	h.learner.runCycle(h.wsID)

	active, err := h.lessons.ListActive(h.wsID, 0.7, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("active lessons = %d, want 1", len(active))
	}
	if !strings.Contains(active[0].Action, "cannot find package") {
		t.Fatalf("lesson does not carry the build error: %q", active[0].Action)
	}
}

// A fixed-after-failing task leaves a regression row with the real symptom.
func TestFixedCycleRecordsARegression(t *testing.T) {
	h := newLearnerHarness(t)
	task := &models.Task{TaskID: "t9", Title: "[CODE] sửa lỗi login", RetryCount: 1}
	_ = h.attempts.Add(task.TaskID, "TypeError: cannot read properties of undefined (reading 'user')")

	h.learner.OnTaskFinished(h.wsID, "", task, &cli.RunResult{Success: true, Output: "Thêm null check trong auth store."}, nil)

	regs, err := h.regress.List(h.wsID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(regs) != 1 {
		t.Fatalf("regressions = %d, want 1", len(regs))
	}
	if !strings.Contains(regs[0].Symptom, "cannot read properties") {
		t.Fatalf("regression symptom wrong: %q", regs[0].Symptom)
	}

	// And a related task must get told about it.
	matched, err := h.regress.Match(h.wsID, "Refactor login flow trong auth store", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 {
		t.Fatalf("a login-related task should match the regression, got %d", len(matched))
	}
	if matched2, _ := h.regress.Match(h.wsID, "Cập nhật màu nút footer", 3); len(matched2) != 0 {
		t.Fatalf("an unrelated task must not match, got %+v", matched2)
	}
}
