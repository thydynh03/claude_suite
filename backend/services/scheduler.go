package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type ScheduledJob struct {
	ID         string    `json:"id"`
	Prompt     string    `json:"prompt"`
	TargetTime time.Time `json:"target_time"`
	Repeat     bool      `json:"repeat"`

	// Provider and Model pick who runs the job. Empty means the caller's default;
	// scheduling overnight work against a specific model is the whole point of
	// having two providers with separate quotas.
	Provider string `json:"provider"`
	Model    string `json:"model"`

	// Kind selects what the job does. Blank means a plain prompt.
	Kind string `json:"kind"`

	// Enabled lets a repeating job be paused without losing its schedule.
	Enabled bool `json:"enabled"`

	// WaitForQuota holds a due job until the account pool reports usable
	// quota, instead of burning the run into a pool that is entirely
	// rate-limited. The job stays due; every tick re-asks the gate, so it
	// fires the moment quota is back. Scheduling heavy overnight work for
	// when a provider's quota resets is the reason this flag exists.
	WaitForQuota bool `json:"wait_for_quota"`

	// The outcome of the last run, so the UI can show whether a nightly job has
	// been quietly failing.
	LastRunAt  time.Time `json:"last_run_at"`
	LastStatus string    `json:"last_status"` // "ok" | "failed"
	LastError  string    `json:"last_error"`
	RunCount   int       `json:"run_count"`
}

// Job kinds. The scheduler does not know what any of these do — it passes the
// kind to the application, which owns the capability. Keeping the mapping out of
// here is what lets a job run a whole plan or a browser suite without the
// scheduler growing a dependency on the orchestrator.
const (
	JobKindPrompt    = "prompt"     // send a prompt to an agent
	JobKindPlan      = "plan"       // decompose the prompt into tasks and run them
	JobKindE2E       = "e2e"        // drive the browser suite, repairing what fails
	JobKindGitCommit = "git-commit" // let the AI commit whatever is uncommitted
	JobKindDigest    = "digest"     // summarise the day and send it to the webhook
)

// KnownJobKinds lists every kind the scheduler accepts.
var KnownJobKinds = []string{JobKindPrompt, JobKindPlan, JobKindE2E, JobKindGitCommit, JobKindDigest}

// normaliseKind defaults a blank kind to a plain prompt — which is what every
// job scheduled before kinds existed is — and rejects anything unrecognised
// rather than silently running it as a prompt.
func normaliseKind(kind string) (string, error) {
	if kind == "" {
		return JobKindPrompt, nil
	}
	for _, known := range KnownJobKinds {
		if kind == known {
			return kind, nil
		}
	}
	return "", fmt.Errorf("unknown job kind %q (expected one of %v)", kind, KnownJobKinds)
}

// JobRequest is what the scheduler hands the application when a job comes due.
type JobRequest struct {
	Kind     string
	Prompt   string
	Provider string
	Model    string
}

type SchedulerService struct {
	ctx       context.Context
	jobs      map[string]*ScheduledJob
	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	onTrigger func(JobRequest) error

	// quotaGate answers "is it worth firing a run for this provider right
	// now?". The scheduler owns no pool knowledge; the app wires this to the
	// account pool. Nil means never hold.
	quotaGate func(provider string) (ok bool, reason string)
	// quotaHeldAt throttles the "still waiting" log per job — one line every
	// few minutes, not one per one-second tick.
	quotaHeldAt map[string]time.Time

	// storePath is where jobs are persisted. Without it a scheduled job lived
	// only in memory: setting up a nightly run and closing the app lost it, with
	// nothing to say so.
	storePath string
}

func NewSchedulerService() *SchedulerService {
	return &SchedulerService{
		jobs:        make(map[string]*ScheduledJob),
		quotaHeldAt: make(map[string]time.Time),
	}
}

// SetQuotaGate installs the pool check WaitForQuota jobs consult. Call before
// Start, like the other wiring.
func (s *SchedulerService) SetQuotaGate(gate func(provider string) (bool, string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quotaGate = gate
}


// SetContext and SetTriggerCallback are currently called before Start, where the
// `go` statement itself orders the write ahead of the loop's reads. They take the
// lock anyway: checkJobs reads both fields from the loop goroutine, so a caller
// that ever sets one afterwards would be racing.
// UseStore points the scheduler at a file to keep its jobs in and loads whatever
// is already there. Call it before Start. Jobs survive a restart from then on;
// a due job whose time passed while the app was closed runs on the next tick.
func (s *SchedulerService) UseStore(path string) error {
	s.mu.Lock()
	s.storePath = path
	s.mu.Unlock()
	return s.load()
}

func (s *SchedulerService) load() error {
	s.mu.Lock()
	path := s.storePath
	s.mu.Unlock()
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil // nothing scheduled yet
	}
	if err != nil {
		return fmt.Errorf("read scheduled jobs: %w", err)
	}

	var stored []ScheduledJob
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("parse scheduled jobs: %w", err)
	}

	// Migration for files written before the Enabled field existed, decided by
	// whether the key is present at all — not by inferring from values. The
	// old value-based heuristic ran on every tick and matched a just-fired
	// one-shot job (Enabled freshly false, run not yet recorded), re-arming
	// and re-firing it once per second until the first run finished; it also
	// silently un-paused any never-run job the user had switched off.
	var keyPresence []struct {
		Enabled *bool `json:"enabled"`
	}
	_ = json.Unmarshal(data, &keyPresence)

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range stored {
		job := stored[i]
		if job.ID == "" {
			continue
		}
		if i < len(keyPresence) && keyPresence[i].Enabled == nil {
			// Legacy entry with no enabled key: treat as on rather than
			// silently never running it.
			job.Enabled = true
		}
		s.jobs[job.ID] = &job
	}
	return nil
}

// save writes the jobs out. Callers hold the lock; the write itself is
// best-effort and reported rather than returned, because losing a schedule
// matters more than the caller's return value.
func (s *SchedulerService) saveLocked() {
	if s.storePath == "" {
		return
	}

	jobs := make([]ScheduledJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, *job)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].TargetTime.Before(jobs[j].TargetTime) })

	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		s.logLocked(fmt.Sprintf("Không lưu được lịch: %v", err), "ERROR")
		return
	}
	if err := WriteFileAtomic(s.storePath, data, 0o600); err != nil {
		s.logLocked(fmt.Sprintf("Không lưu được lịch vào %s: %v", s.storePath, err), "ERROR")
	}
}

// logLocked emits a scheduler log line. Callers hold the lock.
func (s *SchedulerService) logLocked(message, level string) {
	if s.ctx == nil {
		return
	}
	runtime.EventsEmit(s.ctx, "scheduler_log", map[string]string{"msg": message, "level": level})
}

func (s *SchedulerService) SetContext(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
}

// SetTriggerCallback installs what runs when a job comes due. Returning an error
// records the run as failed on the job, so a nightly task that has been broken
// for a week says so instead of looking untouched.
func (s *SchedulerService) SetTriggerCallback(cb func(JobRequest) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTrigger = cb
}

func (s *SchedulerService) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	// Hand this run its own channel: a later Start replaces the field while the
	// old loop is still reading it. Same defect the orchestrator had.
	stop := s.stopCh
	s.mu.Unlock()

	go s.loop(stop)
}

func (s *SchedulerService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()
}

func (s *SchedulerService) loop(stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.checkJobs()
		}
	}
}

func (s *SchedulerService) checkJobs() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	fired := false

	for id, job := range s.jobs {
		// No migration here: legacy files are handled once, in load(), by key
		// presence. Doing it per-tick re-armed one-shot jobs mid-run.
		if !job.Enabled || !now.After(job.TargetTime) {
			continue
		}

		// A due job that asked to wait for quota stays due until the gate
		// opens — it neither fires into an exhausted pool nor loses its
		// turn: the next tick asks again.
		if job.WaitForQuota && s.quotaGate != nil {
			if ok, reason := s.quotaGate(job.Provider); !ok {
				if now.Sub(s.quotaHeldAt[id]) > 5*time.Minute {
					s.quotaHeldAt[id] = now
					s.logLocked(fmt.Sprintf("Lịch %s đang chờ quota: %s", id, reason), "INFO")
				}
				continue
			}
			delete(s.quotaHeldAt, id)
		}
		fired = true

		if s.onTrigger != nil {
			request := JobRequest{Kind: job.Kind, Prompt: job.Prompt, Provider: job.Provider, Model: job.Model}
			// The callback runs an agent, which takes minutes. Doing it inline would
			// hold the lock and stall every other job.
			go s.runJob(id, request)
		}

		if job.Repeat {
			// Skip forward past any occurrences missed while the app was closed, so
			// a daily job does not fire once per missed day on the next launch.
			for !job.TargetTime.After(now) {
				job.TargetTime = job.TargetTime.Add(24 * time.Hour)
			}
			s.logLocked(fmt.Sprintf("Lịch lặp %s: lần chạy kế tiếp %s", id, job.TargetTime.Format("02/01 15:04")), "INFO")
		} else {
			// A one-shot job is switched off rather than deleted, so its outcome can
			// still be read afterwards. Deleting it here also raced the run itself:
			// runJob had nothing left to record the failure against, and a job that
			// died at 2am looked exactly like one that never existed. Cancel removes
			// it for good.
			job.Enabled = false
		}
	}

	if fired {
		s.saveLocked()
		if s.ctx != nil {
			runtime.EventsEmit(s.ctx, "scheduler_updated", nil)
		}
	}
}

// runJob executes one due job outside the lock and records how it went.
func (s *SchedulerService) runJob(id string, request JobRequest) {
	s.mu.Lock()
	trigger := s.onTrigger
	s.mu.Unlock()
	if trigger == nil {
		return
	}

	err := trigger(request)

	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if ok {
		job.LastRunAt = time.Now()
		job.RunCount++
		if err != nil {
			job.LastStatus, job.LastError = "failed", err.Error()
		} else {
			job.LastStatus, job.LastError = "ok", ""
		}
		s.saveLocked()
	}

	if err != nil {
		s.logLocked(fmt.Sprintf("Lịch %s chạy thất bại: %v", id, err), "ERROR")
	} else {
		s.logLocked(fmt.Sprintf("Lịch %s đã chạy xong", id), "SUCCESS")
	}
	if s.ctx != nil {
		runtime.EventsEmit(s.ctx, "scheduler_updated", nil)
	}
}

// SchedulePrompt keeps the original signature so existing callers still work;
// the run uses whatever provider and model the caller's trigger defaults to.
func (s *SchedulerService) SchedulePrompt(prompt string, targetTimeStr string, repeat bool) (string, error) {
	return s.ScheduleJob(prompt, targetTimeStr, repeat, "", "")
}

// ScheduleKind adds a job of a given kind. An unknown kind is rejected here
// rather than at midnight, when nobody is watching.
func (s *SchedulerService) ScheduleKind(kind, prompt, targetTimeStr string, repeat bool, provider, model string) (string, error) {
	return s.ScheduleKindJob(kind, prompt, targetTimeStr, repeat, provider, model, false)
}

// ScheduleJob adds a job that runs against a chosen provider and model. Pointing
// heavy overnight work at the provider whose quota resets first is the reason
// this app carries two of them.
func (s *SchedulerService) ScheduleJob(prompt, targetTimeStr string, repeat bool, provider, model string) (string, error) {
	return s.ScheduleKindJob("", prompt, targetTimeStr, repeat, provider, model, false)
}

// ScheduleKindJob adds a complete job in ONE critical section. Kind and the
// quota flag land with the row: the one-second scan ticker contends on the
// same lock, so a due-now job created bare and patched afterwards could fire
// in the gap — unheld, and without its kind.
func (s *SchedulerService) ScheduleKindJob(kind, prompt, targetTimeStr string, repeat bool, provider, model string, waitForQuota bool) (string, error) {
	resolved, err := normaliseKind(kind)
	if err != nil {
		return "", err
	}
	targetTime, err := time.Parse(time.RFC3339, targetTimeStr)
	if err != nil {
		return "", fmt.Errorf("invalid time format: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.New().String()
	s.jobs[id] = &ScheduledJob{
		ID:           id,
		Kind:         resolved,
		Prompt:       prompt,
		TargetTime:   targetTime,
		Repeat:       repeat,
		Provider:     provider,
		Model:        model,
		Enabled:      true,
		WaitForQuota: waitForQuota,
	}
	s.saveLocked()

	if s.ctx != nil {
		runtime.EventsEmit(s.ctx, "scheduler_updated", nil)
		runtime.EventsEmit(s.ctx, "scheduler_log", map[string]string{
			"msg":   fmt.Sprintf("Job scheduled for %s", targetTime.Format("15:04:05")),
			"level": "SUCCESS",
		})
	}

	return id, nil
}

func (s *SchedulerService) CancelJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	// The quota-hold clock only cleared when the gate opened, so a job
	// cancelled while held left its entry behind for the process lifetime.
	delete(s.quotaHeldAt, id)
	s.saveLocked()

	if s.ctx != nil {
		runtime.EventsEmit(s.ctx, "scheduler_updated", nil)
		runtime.EventsEmit(s.ctx, "scheduler_log", map[string]string{
			"msg":   fmt.Sprintf("Job %s cancelled", id),
			"level": "WARN",
		})
	}
}

func (s *SchedulerService) GetJobs() []ScheduledJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	var list []ScheduledJob
	for _, j := range s.jobs {
		list = append(list, *j)
	}
	return list
}

// SetJobEnabled pauses or resumes a repeating job without losing its schedule.
func (s *SchedulerService) SetJobEnabled(id string, enabled bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return false
	}
	job.Enabled = enabled
	if !enabled {
		// A disabled job is skipped before the gate is consulted, so its hold
		// clock would never be cleared otherwise.
		delete(s.quotaHeldAt, id)
	}
	s.saveLocked()

	state := "tạm dừng"
	if enabled {
		state = "bật lại"
	}
	s.logLocked(fmt.Sprintf("Lịch %s đã %s", id, state), "INFO")
	if s.ctx != nil {
		runtime.EventsEmit(s.ctx, "scheduler_updated", nil)
	}
	return true
}
