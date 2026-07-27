package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"agent_center/backend/models"

	"github.com/google/uuid"
)

// LessonRepository stores distilled cross-task knowledge. Unlike agent_memory
// (a raw ring buffer), lessons have no count-based GC: what survives is
// decided by review status, the confidence feedback loop, and the pending TTL.
type LessonRepository struct {
	db *sql.DB
}

func NewLessonRepository(db *sql.DB) *LessonRepository {
	return &LessonRepository{db: db}
}

const lessonColumns = `lesson_id, workspace_id, kind, trigger_text, action, evidence, confidence, status, source_task_id, use_count, created_at, updated_at, last_used_at`

// ReviewDecisionStatus maps a UI review decision onto a lesson status. Shared
// by both frontends so "approve" cannot quietly mean two different things.
func ReviewDecisionStatus(decision string) (string, error) {
	switch decision {
	case "approve":
		return "active", nil
	case "reject", "archive":
		return "archived", nil
	default:
		return "", fmt.Errorf("quyết định không hợp lệ: %q (approve|reject|archive)", decision)
	}
}

func (r *LessonRepository) Add(l *models.MemoryLesson) error {
	if l.LessonID == "" {
		l.LessonID = uuid.New().String()
	}
	if l.Kind == "" {
		l.Kind = "distilled"
	}
	if l.Status == "" {
		l.Status = "pending"
	}
	now := time.Now()
	_, err := r.db.Exec(
		`INSERT INTO memory_lessons (lesson_id, workspace_id, kind, trigger_text, action, evidence, confidence, status, source_task_id, use_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		l.LessonID, l.WorkspaceID, l.Kind, l.Trigger, l.Action, l.Evidence, l.Confidence, l.Status, l.SourceTaskID, now, now,
	)
	return err
}

// UpsertMechanical inserts a machine-derived lesson or refreshes the existing
// one with the same (workspace, kind, trigger) — repeated observations update
// evidence and confidence instead of piling up duplicates.
func (r *LessonRepository) UpsertMechanical(l *models.MemoryLesson) error {
	var existing string
	err := r.db.QueryRow(
		`SELECT lesson_id FROM memory_lessons WHERE workspace_id = ? AND kind = 'mechanical' AND trigger_text = ?`,
		l.WorkspaceID, l.Trigger,
	).Scan(&existing)
	if err == sql.ErrNoRows {
		l.Kind = "mechanical"
		if l.Status == "" {
			l.Status = "active"
		}
		return r.Add(l)
	}
	if err != nil {
		return err
	}
	_, err = r.db.Exec(
		`UPDATE memory_lessons SET action = ?, evidence = ?, confidence = MAX(confidence, ?), updated_at = ? WHERE lesson_id = ?`,
		l.Action, l.Evidence, l.Confidence, time.Now(), existing,
	)
	return err
}

// ListActive returns injectable lessons: the workspace's own first, then
// global ones, deduped by trigger (workspace wins), confidence-gated and
// ranked — the ported ECC selection (summarizeActiveInstincts).
func (r *LessonRepository) ListActive(workspaceID string, minConfidence float64, limit int) ([]models.MemoryLesson, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := r.db.Query(
		`SELECT `+lessonColumns+` FROM memory_lessons
		 WHERE status = 'active' AND confidence >= ? AND (workspace_id = ? OR workspace_id = '')
		 ORDER BY CASE WHEN workspace_id = '' THEN 1 ELSE 0 END, confidence DESC, lesson_id`,
		minConfidence, workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	all, err := scanLessons(rows)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var out []models.MemoryLesson
	for _, l := range all {
		key := strings.ToLower(strings.TrimSpace(l.Trigger))
		if key != "" && seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, l)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *LessonRepository) ListByStatus(workspaceID, status string, limit int) ([]models.MemoryLesson, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(
		`SELECT `+lessonColumns+` FROM memory_lessons
		 WHERE status = ? AND (workspace_id = ? OR workspace_id = '')
		 ORDER BY updated_at DESC LIMIT ?`,
		status, workspaceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLessons(rows)
}

func (r *LessonRepository) SetStatus(lessonID, status string) error {
	_, err := r.db.Exec(
		`UPDATE memory_lessons SET status = ?, updated_at = ? WHERE lesson_id = ?`,
		status, time.Now(), lessonID,
	)
	return err
}

// RecordOutcome is the feedback loop ECC documented but never built: an
// injected lesson whose task succeeds gains confidence; one whose task fails
// loses it, and below 0.4 it archives itself.
func (r *LessonRepository) RecordOutcome(lessonID string, success bool) error {
	now := time.Now()
	var err error
	if success {
		_, err = r.db.Exec(
			`UPDATE memory_lessons SET use_count = use_count + 1,
			   confidence = MIN(0.95, confidence + 0.02), last_used_at = ?, updated_at = ? WHERE lesson_id = ?`,
			now, now, lessonID,
		)
	} else {
		_, err = r.db.Exec(
			`UPDATE memory_lessons SET use_count = use_count + 1,
			   confidence = confidence - 0.05, last_used_at = ?, updated_at = ? WHERE lesson_id = ?`,
			now, now, lessonID,
		)
	}
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`UPDATE memory_lessons SET status = 'archived', updated_at = ? WHERE lesson_id = ? AND confidence < 0.4 AND status = 'active'`, now, lessonID)
	return err
}

// PromoteEligible copies lessons proven in several workspaces to the global
// scope (workspace_id = ''): the same trigger active in ≥2 workspaces with
// average confidence ≥0.8 — the ported ECC promotion rule. Idempotent: an
// existing global row with the same trigger blocks re-promotion.
func (r *LessonRepository) PromoteEligible() (int, error) {
	rows, err := r.db.Query(
		`SELECT trigger_text FROM memory_lessons
		 WHERE status = 'active' AND workspace_id != '' AND trigger_text != ''
		 GROUP BY trigger_text
		 HAVING COUNT(DISTINCT workspace_id) >= 2 AND AVG(confidence) >= 0.8`,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var triggers []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return 0, err
		}
		triggers = append(triggers, t)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	promoted := 0
	for _, trigger := range triggers {
		var existing int
		if err := r.db.QueryRow(
			`SELECT COUNT(*) FROM memory_lessons WHERE workspace_id = '' AND trigger_text = ?`, trigger,
		).Scan(&existing); err != nil {
			return promoted, err
		}
		if existing > 0 {
			continue
		}
		// Copy the strongest instance into the global scope.
		now := time.Now()
		_, err := r.db.Exec(
			`INSERT INTO memory_lessons (lesson_id, workspace_id, kind, trigger_text, action, evidence, confidence, status, source_task_id, use_count, created_at, updated_at)
			 SELECT ?, '', kind, trigger_text, action, evidence || ' (promoted: thấy ở nhiều workspace)', confidence, 'active', source_task_id, 0, ?, ?
			 FROM memory_lessons WHERE status = 'active' AND workspace_id != '' AND trigger_text = ?
			 ORDER BY confidence DESC LIMIT 1`,
			uuid.New().String(), now, now, trigger,
		)
		if err != nil {
			return promoted, err
		}
		promoted++
	}
	return promoted, nil
}

// PruneExpiredPending deletes pending lessons nobody reviewed within the TTL
// (ported ECC PENDING_TTL_DAYS).
func (r *LessonRepository) PruneExpiredPending(ttlDays int) (int64, error) {
	if ttlDays <= 0 {
		ttlDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -ttlDays)
	res, err := r.db.Exec(`DELETE FROM memory_lessons WHERE status = 'pending' AND created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func scanLessons(rows *sql.Rows) ([]models.MemoryLesson, error) {
	var out []models.MemoryLesson
	for rows.Next() {
		var l models.MemoryLesson
		var rawCreated, rawUpdated, rawUsed interface{}
		if err := rows.Scan(&l.LessonID, &l.WorkspaceID, &l.Kind, &l.Trigger, &l.Action, &l.Evidence,
			&l.Confidence, &l.Status, &l.SourceTaskID, &l.UseCount, &rawCreated, &rawUpdated, &rawUsed); err != nil {
			return nil, err
		}
		l.CreatedAt = parseTimeValue(rawCreated)
		l.UpdatedAt = parseTimeValue(rawUpdated)
		l.LastUsedAt = parseTimeValue(rawUsed)
		out = append(out, l)
	}
	return out, rows.Err()
}
