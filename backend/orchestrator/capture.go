package orchestrator

import (
	"sync"
	"sync/atomic"
	"time"

	"agent_center/backend/database"
	"agent_center/backend/models"
	"agent_center/backend/services/projectmap"
	"agent_center/backend/textutil"
)

// obsPayloadCap bounds one observation payload (chars), after scrubbing.
const obsPayloadCap = 5000

// obsWriter batches observation rows off the hot log path. onLog fires for
// every line an agent streams; a synchronous INSERT there can stall log
// streaming for seconds when the worker pool has the database busy. The
// channel never blocks — when full, rows are dropped and counted, because
// losing an observation is better than freezing a live task's output.
type obsWriter struct {
	repo    *database.ObservationRepository
	ch      chan models.Observation
	dropped atomic.Int64
	once    sync.Once
}

func newObsWriter(repo *database.ObservationRepository) *obsWriter {
	return &obsWriter{repo: repo, ch: make(chan models.Observation, 1024)}
}

func (w *obsWriter) start() {
	w.once.Do(func() { go w.loop() })
}

func (w *obsWriter) loop() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	batch := make([]models.Observation, 0, 64)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		_ = w.repo.AddBatch(batch)
		batch = batch[:0]
	}
	for {
		select {
		case o := <-w.ch:
			batch = append(batch, o)
			if len(batch) >= 50 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// offer enqueues without ever blocking.
func (w *obsWriter) offer(o models.Observation) {
	select {
	case w.ch <- o:
	default:
		w.dropped.Add(1)
	}
}

// resolveWorkspaceID resolves (and caches) the workspace identity for a
// directory. One git exec per directory per app lifetime instead of one per
// captured line. Identity resolution is shared with the mapper —
// projectmap.WorkspaceRemoteIdentity is THE resolver.
func (o *Orchestrator) resolveWorkspaceID(workspaceDir string) string {
	if o.wsRepo == nil || workspaceDir == "" {
		return ""
	}
	o.wsIDMu.Lock()
	if id, ok := o.wsIDCache[workspaceDir]; ok {
		o.wsIDMu.Unlock()
		return id
	}
	o.wsIDMu.Unlock()

	ws, err := o.wsRepo.EnsureWorkspace(workspaceDir, projectmap.WorkspaceRemoteIdentity(workspaceDir))
	if err != nil {
		return ""
	}
	o.wsIDMu.Lock()
	o.wsIDCache[workspaceDir] = ws.WorkspaceID
	o.wsIDMu.Unlock()
	return ws.WorkspaceID
}

// observe captures one event. Payloads are scrubbed BEFORE truncation and
// storage — memory must never be where a token survives. Internal LLM runs
// (summarizer, learner) never pass through runTask, so they are structurally
// invisible here; no self-loop guard flags needed.
func (o *Orchestrator) observe(workspaceID, taskID, agentID, event, tool, payload string) {
	if o.obsW == nil || workspaceID == "" {
		return
	}
	o.obsW.start()
	o.obsW.offer(models.Observation{
		WorkspaceID: workspaceID,
		TaskID:      taskID,
		AgentID:     agentID,
		Event:       event,
		Tool:        tool,
		Payload:     textutil.Truncate(textutil.Scrub(payload), obsPayloadCap, "…"),
	})
}
