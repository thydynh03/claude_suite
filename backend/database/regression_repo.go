package database

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"agent_center/backend/models"
	"agent_center/backend/textutil"

	"github.com/google/uuid"
)

// RegressionRepository stores fixed bugs so a later task touching the same
// ground is told about them instead of re-introducing them.
type RegressionRepository struct {
	db *sql.DB
}

func NewRegressionRepository(db *sql.DB) *RegressionRepository {
	return &RegressionRepository{db: db}
}

const regressionColumns = `regression_id, workspace_id, title, symptom, root_cause, fix_summary, files, guard_check_id, guard_status, failed_task_id, fixed_task_id, status, created_at`

func (r *RegressionRepository) Add(reg *models.Regression) error {
	if reg.RegressionID == "" {
		reg.RegressionID = uuid.New().String()
	}
	if reg.Status == "" {
		reg.Status = "active"
	}
	if reg.GuardStatus == "" {
		reg.GuardStatus = "none"
	}
	files, _ := json.Marshal(reg.Files)
	_, err := r.db.Exec(
		`INSERT INTO regressions (regression_id, workspace_id, title, symptom, root_cause, fix_summary, files, guard_check_id, guard_status, failed_task_id, fixed_task_id, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		reg.RegressionID, reg.WorkspaceID, reg.Title, reg.Symptom, reg.RootCause, reg.FixSummary,
		string(files), reg.GuardCheckID, reg.GuardStatus, reg.FailedTaskID, reg.FixedTaskID, reg.Status, time.Now(),
	)
	return err
}

func (r *RegressionRepository) List(workspaceID string, limit int) ([]models.Regression, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(
		`SELECT `+regressionColumns+` FROM regressions WHERE workspace_id = ? ORDER BY created_at DESC LIMIT ?`,
		workspaceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRegressions(rows)
}

// Match returns the active regressions a task's text is talking about, ranked
// by whole-word keyword overlap (textutil.KeywordMatch — never a bare
// substring check) over title, symptom and touched files.
func (r *RegressionRepository) Match(workspaceID, taskText string, limit int) ([]models.Regression, error) {
	if limit <= 0 {
		limit = 3
	}
	rows, err := r.db.Query(
		`SELECT `+regressionColumns+` FROM regressions WHERE workspace_id = ? AND status = 'active' ORDER BY created_at DESC LIMIT 200`,
		workspaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanRegressions(rows)
	if err != nil {
		return nil, err
	}

	tokens := regressionTokens(taskText)
	if len(tokens) == 0 {
		return nil, nil
	}

	type scored struct {
		reg   models.Regression
		score int
	}
	var ranked []scored
	for _, reg := range all {
		haystack := reg.Title + " " + reg.Symptom + " " + strings.Join(reg.Files, " ")
		score := 0
		for _, tok := range tokens {
			if textutil.KeywordMatch(haystack, tok) {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, scored{reg, score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	var out []models.Regression
	for _, s := range ranked {
		out = append(out, s.reg)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// regressionStopwords keeps generic words from matching every regression.
var regressionStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "fix": true, "bug": true, "lỗi": true,
	"sửa": true, "task": true, "code": true, "test": true, "with": true, "file": true,
}

func regressionTokens(text string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, tok := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r == '_' || r == '.' || r == '/' || r == '-' ||
			(r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r >= 0x80)
	}) {
		if len([]rune(tok)) < 3 || regressionStopwords[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
		if len(out) >= 16 {
			break
		}
	}
	return out
}

// Get returns one regression, or (nil, nil) when it does not exist.
func (r *RegressionRepository) Get(regressionID string) (*models.Regression, error) {
	rows, err := r.db.Query(`SELECT `+regressionColumns+` FROM regressions WHERE regression_id = ?`, regressionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	regs, err := scanRegressions(rows)
	if err != nil || len(regs) == 0 {
		return nil, err
	}
	return &regs[0], nil
}

func (r *RegressionRepository) SetGuard(regressionID, checkID, guardStatus string) error {
	_, err := r.db.Exec(
		`UPDATE regressions SET guard_check_id = ?, guard_status = ? WHERE regression_id = ?`,
		checkID, guardStatus, regressionID,
	)
	return err
}

func (r *RegressionRepository) SetStatus(regressionID, status string) error {
	_, err := r.db.Exec(`UPDATE regressions SET status = ? WHERE regression_id = ?`, status, regressionID)
	return err
}

func scanRegressions(rows *sql.Rows) ([]models.Regression, error) {
	var out []models.Regression
	for rows.Next() {
		var reg models.Regression
		var filesJSON string
		var rawCreated interface{}
		if err := rows.Scan(&reg.RegressionID, &reg.WorkspaceID, &reg.Title, &reg.Symptom, &reg.RootCause,
			&reg.FixSummary, &filesJSON, &reg.GuardCheckID, &reg.GuardStatus, &reg.FailedTaskID,
			&reg.FixedTaskID, &reg.Status, &rawCreated); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(filesJSON), &reg.Files)
		reg.CreatedAt = parseTimeValue(rawCreated)
		out = append(out, reg)
	}
	return out, rows.Err()
}
