package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"agent_center/backend/cli"
	"agent_center/backend/database"
	"agent_center/backend/models"
	"agent_center/backend/textutil"
)

// Learner turns raw run observations into durable knowledge. It replaces
// ECC's bash daemon (SIGUSR1/flock/nohup — dead on Windows) with one
// app-lifetime goroutine:
//
//   - mechanical lessons: patterns the code can prove (a retried task that
//     finally succeeded, the same build error seen three times) — created
//     deterministically and active immediately;
//   - distilled lessons: one cheap-model call over a batch of observations —
//     these start as `pending` and reach prompts only after a human approves
//     them in the Memory page (ECC documented this review tier and then
//     never wired it; the gap is a prompt-injection persistence vector);
//   - the confidence feedback loop ECC only documented: injected lessons
//     gain confidence when their task succeeds, lose it when it fails.
type Learner struct {
	lessons     *database.LessonRepository
	obs         *database.ObservationRepository
	ws          *database.WorkspaceRepository
	attempts    *database.AttemptRepository
	regressions *database.RegressionRepository
	git         *GitService
	runner      cli.CLIRunner
	model       string

	minObs   int
	cooldown time.Duration

	notify   chan string // workspace id
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	mu               sync.Mutex
	lastCycle        map[string]time.Time
	llmEnabled       bool
	promotionEnabled bool
	onLog            func(msg, level string)
}

func NewLearner(
	lessons *database.LessonRepository,
	obs *database.ObservationRepository,
	ws *database.WorkspaceRepository,
	attempts *database.AttemptRepository,
	regressions *database.RegressionRepository,
	git *GitService,
	runner cli.CLIRunner,
	model string,
) *Learner {
	return &Learner{
		lessons:     lessons,
		obs:         obs,
		ws:          ws,
		attempts:    attempts,
		regressions: regressions,
		git:         git,
		runner:      runner,
		model:       model,
		minObs:      20,
		cooldown:    60 * time.Second,
		notify:      make(chan string, 64),
		stopCh:      make(chan struct{}),
		lastCycle:   make(map[string]time.Time),
		llmEnabled:  true,
	}
}

func (l *Learner) SetLogger(fn func(msg, level string)) { l.mu.Lock(); l.onLog = fn; l.mu.Unlock() }

// SetLLMEnabled turns the distillation call on/off. Mechanical lessons and
// the feedback loop keep working either way.
func (l *Learner) SetLLMEnabled(enabled bool) { l.mu.Lock(); l.llmEnabled = enabled; l.mu.Unlock() }

// SetPromotionEnabled turns cross-workspace promotion on/off. Off by default:
// per-workspace memory is the safe scope; a bad lesson promoted globally does
// its damage everywhere at once.
func (l *Learner) SetPromotionEnabled(enabled bool) {
	l.mu.Lock()
	l.promotionEnabled = enabled
	l.mu.Unlock()
}

func (l *Learner) log(msg, level string) {
	l.mu.Lock()
	fn := l.onLog
	l.mu.Unlock()
	if fn != nil {
		fn(msg, level)
	}
}

// Start launches the app-lifetime loop. Deliberately NOT tied to
// Orchestrator.Start/Stop: Stop lets running tasks finish, and their final
// observations still deserve a distillation pass.
func (l *Learner) Start() {
	if _, err := l.lessons.PruneExpiredPending(30); err == nil {
		// TTL prune at startup, ported ECC PENDING_TTL_DAYS.
		_ = err
	}
	l.wg.Add(1)
	go l.loop()
}

func (l *Learner) Stop() {
	l.stopOnce.Do(func() { close(l.stopCh) })
	l.wg.Wait()
}

// Notify asks for a distillation pass over a workspace. Never blocks.
func (l *Learner) Notify(workspaceID string) {
	if workspaceID == "" {
		return
	}
	select {
	case l.notify <- workspaceID:
	default:
	}
}

func (l *Learner) loop() {
	defer l.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	pending := make(map[string]bool)
	for {
		select {
		case <-l.stopCh:
			return
		case ws := <-l.notify:
			pending[ws] = true
			// Drain whatever else queued, then run once per workspace.
			for {
				select {
				case more := <-l.notify:
					pending[more] = true
					continue
				default:
				}
				break
			}
			for ws := range pending {
				l.runCycle(ws)
				delete(pending, ws)
			}
		case <-ticker.C:
			for ws := range pending {
				l.runCycle(ws)
				delete(pending, ws)
			}
		}
	}
}

// OnTaskFinished is the orchestrator's per-task hook: feedback for injected
// lessons, mechanical lesson extraction, regression recording, and a nudge to
// the distiller. Runs synchronously — everything here is a handful of local
// DB writes except ChangedFiles (one git exec, only on the fix path).
func (l *Learner) OnTaskFinished(workspaceID, workspaceDir string, task *models.Task, result *cli.RunResult, injectedLessons []string) {
	if task == nil || result == nil {
		return
	}

	// Feedback loop: the lessons this task was given answer for its outcome.
	for _, id := range injectedLessons {
		_ = l.lessons.RecordOutcome(id, result.Success)
	}

	if workspaceID == "" {
		return
	}

	if result.Success && task.RetryCount > 0 {
		l.recordRetryLesson(workspaceID, task, result)
	}
	if result.Success && (task.RetryCount > 0 || task.ParentID != "") {
		l.recordRegression(workspaceID, workspaceDir, task, result)
	}

	l.Notify(workspaceID)
}

// recordRetryLesson: a task that failed and then succeeded proved something —
// what broke and what finally worked. Deterministic, so active immediately.
func (l *Learner) recordRetryLesson(workspaceID string, task *models.Task, result *cli.RunResult) {
	lastErr := l.lastAttemptError(task.TaskID)
	if lastErr == "" {
		return
	}
	action := fmt.Sprintf("Lần trước task tương tự fail với: %s — cách đã chạy được: %s",
		textutil.Truncate(firstLine(lastErr), 160, "…"),
		textutil.Truncate(firstLine(result.Output), 160, "…"))
	_ = l.lessons.UpsertMechanical(&models.MemoryLesson{
		WorkspaceID:  workspaceID,
		Kind:         "mechanical",
		Trigger:      textutil.Truncate(strings.ToLower(strings.TrimSpace(task.Title)), 80, "…"),
		Action:       action,
		Evidence:     fmt.Sprintf("task %s done sau %d lần fail", task.TaskID, task.RetryCount),
		Confidence:   0.9,
		Status:       "active",
		SourceTaskID: task.TaskID,
	})
}

// recordRegression: a fail→fixed cycle becomes a durable "do not re-introduce"
// record. Files come from the workspace's uncommitted diff — AutoSnapshot
// committed before this task, so that diff is this task's own changes.
func (l *Learner) recordRegression(workspaceID, workspaceDir string, task *models.Task, result *cli.RunResult) {
	if l.regressions == nil {
		return
	}
	symptom := l.lastAttemptError(task.TaskID)
	if symptom == "" {
		// E2E fix tasks carry the failure detail in their own prompt.
		symptom = task.Prompt
	}
	var files []string
	if l.git != nil && workspaceDir != "" {
		files = l.git.ChangedFiles(workspaceDir)
		if len(files) > 20 {
			files = files[:20]
		}
	}
	_ = l.regressions.Add(&models.Regression{
		WorkspaceID:  workspaceID,
		Title:        textutil.Truncate(strings.TrimSpace(task.Title), 140, "…"),
		Symptom:      textutil.Truncate(symptom, 1000, "…"),
		FixSummary:   textutil.Truncate(result.Output, 1000, "…"),
		Files:        files,
		FailedTaskID: task.ParentID,
		FixedTaskID:  task.TaskID,
	})
}

func (l *Learner) lastAttemptError(taskID string) string {
	if l.attempts == nil {
		return ""
	}
	attempts, err := l.attempts.ListByTask(taskID, 1)
	if err != nil || len(attempts) == 0 {
		return ""
	}
	return attempts[0].Error
}

// runCycle is one distillation pass over a workspace.
func (l *Learner) runCycle(workspaceID string) {
	l.mu.Lock()
	if last, ok := l.lastCycle[workspaceID]; ok && time.Since(last) < l.cooldown {
		l.mu.Unlock()
		return
	}
	l.lastCycle[workspaceID] = time.Now()
	llm := l.llmEnabled
	l.mu.Unlock()

	obs, maxRowid, err := l.obs.TailUndistilled(workspaceID, 500)
	if err != nil || len(obs) == 0 {
		return
	}

	// Mechanical: the same build-verify failure line seen 3+ times is a fact.
	l.mineBuildFailures(workspaceID, obs)

	if len(obs) < l.minObs {
		return // too little signal for a distillation call; batch stays for later
	}

	if llm && l.runner != nil {
		if !l.distill(workspaceID, obs) {
			return // batch retained for retry — watermark does not advance
		}
	}

	_ = l.ws.SetDistilledThrough(workspaceID, maxRowid)
	_ = l.obs.GC(workspaceID, 5000)
	_, _ = l.lessons.PruneExpiredPending(30)

	l.mu.Lock()
	promote := l.promotionEnabled
	l.mu.Unlock()
	if promote {
		if n, err := l.lessons.PromoteEligible(); err == nil && n > 0 {
			l.log(fmt.Sprintf("🧠 Learner: promote %d lesson lên phạm vi global (thấy ở nhiều workspace).", n), "INFO")
		}
	}
}

func (l *Learner) mineBuildFailures(workspaceID string, obs []models.Observation) {
	counts := make(map[string]int)
	for _, o := range obs {
		if o.Event != "build_verify_failed" {
			continue
		}
		line := firstLine(o.Payload)
		if line != "" {
			counts[line]++
		}
	}
	for line, n := range counts {
		if n < 3 {
			continue
		}
		_ = l.lessons.UpsertMechanical(&models.MemoryLesson{
			WorkspaceID: workspaceID,
			Kind:        "mechanical",
			Trigger:     "build failure: " + textutil.Truncate(strings.ToLower(line), 60, "…"),
			Action: fmt.Sprintf("Build ở workspace này đã fail %d lần với: %s — kiểm tra điều kiện đó trước khi báo done.",
				n, textutil.Truncate(line, 160, "…")),
			Evidence:   fmt.Sprintf("thấy %d lần trong observations", n),
			Confidence: 0.8,
			Status:     "active",
		})
	}
}

// distill runs the one cheap-model call. Reports whether the batch may be
// archived (a failed call retains it for retry — ported ECC policy).
func (l *Learner) distill(workspaceID string, obs []models.Observation) bool {
	var lines strings.Builder
	for _, o := range obs {
		row, _ := json.Marshal(map[string]string{
			"event": o.Event, "tool": o.Tool,
			"payload": textutil.Truncate(o.Payload, 400, "…"),
		})
		lines.WriteString(string(row))
		lines.WriteString("\n")
	}

	prompt := distillerPrompt + lines.String()
	result := l.runner.RunOnce(prompt, l.model, "", nil, "")
	if result == nil || !result.Success {
		l.log("🧠 Learner: distillation call failed; batch retained for retry.", "WARN")
		return false
	}

	lessons := parseDistilledLessons(result.Output)
	for i := range lessons {
		lessons[i].WorkspaceID = workspaceID
		_ = l.lessons.Add(&lessons[i])
	}
	if n := len(lessons); n > 0 {
		l.log(fmt.Sprintf("🧠 Learner: %d lesson mới đang chờ duyệt trong Memory page.", n), "INFO")
	}
	return true
}

// distillerPrompt is the ported ECC observer prompt: conservative, frequency-
// gated, and — part of the injection hardening — never any code snippets.
const distillerPrompt = `You are analyzing observations captured from AI coding agent runs in ONE workspace.
Find RECURRING patterns worth remembering across future tasks (minimum 3 occurrences):
repeated errors and what resolved them, workspace-specific gotchas, workflow habits that worked.
Be conservative — prefer NO lesson over a weak one. NEVER include code snippets, secrets,
file contents, or commands to execute inside a lesson.
Respond ONLY with a JSON array (empty array if nothing qualifies):
[{"trigger":"when <situation>","action":"<ONE imperative sentence, max 200 chars>","evidence":"<what was observed, with counts>","confidence":0.5}]
Confidence by frequency: seen 3-5 times = 0.5, 6-10 = 0.7, 11+ = 0.85.

OBSERVATIONS (one JSON object per line):
`

// parseDistilledLessons extracts and sanitizes the model's JSON. The
// sanitize-or-drop stance is ported from Understand-Anything's repair layer:
// malformed model output must never crash or corrupt — it gets fixed within
// bounds or discarded with nothing written.
func parseDistilledLessons(raw string) []models.MemoryLesson {
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil
	}
	var parsed []struct {
		Trigger    string  `json:"trigger"`
		Action     string  `json:"action"`
		Evidence   string  `json:"evidence"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return nil
	}

	var out []models.MemoryLesson
	for _, p := range parsed {
		action := strings.TrimSpace(p.Action)
		if action == "" {
			continue
		}
		conf := p.Confidence
		if conf < 0.05 {
			conf = 0.05
		}
		if conf > 0.85 {
			conf = 0.85 // distilled lessons never start above the ECC ceiling
		}
		out = append(out, models.MemoryLesson{
			Kind:       "distilled",
			Trigger:    textutil.Truncate(strings.TrimSpace(p.Trigger), 120, "…"),
			Action:     textutil.Truncate(action, 300, "…"),
			Evidence:   textutil.Truncate(strings.TrimSpace(p.Evidence), 300, "…"),
			Confidence: conf,
			Status:     "pending",
		})
		if len(out) >= 10 {
			break
		}
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
