package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"agent_center/backend/models"
)

// WorkspaceRepository keys every piece of project memory to a stable workspace
// identity. The caller supplies the git remote URL (fetched by GitService);
// this package never shells out — git execution stays in services.
type WorkspaceRepository struct {
	db *sql.DB
}

func NewWorkspaceRepository(db *sql.DB) *WorkspaceRepository {
	return &WorkspaceRepository{db: db}
}

// credentialRe strips user:password@ / token@ from remote URLs before hashing
// or storing them — the identity must never embed a secret.
var credentialRe = regexp.MustCompile(`^([a-z+]+://)[^/@]+@`)

// NormalizeRemoteURL reduces the remote spellings of one repository to one
// canonical form: credentials stripped, scp syntax rewritten, ".git" dropped,
// lowercased. git@github.com:User/Repo.git and https://github.com/user/repo
// hash to the same workspace.
func NormalizeRemoteURL(remote string) string {
	r := strings.TrimSpace(remote)
	if r == "" {
		return ""
	}
	r = strings.ToLower(r)
	// scp-like syntax: git@host:path → host/path
	if !strings.Contains(r, "://") {
		if at := strings.Index(r, "@"); at >= 0 {
			r = r[at+1:]
		}
		r = strings.Replace(r, ":", "/", 1)
	} else {
		r = credentialRe.ReplaceAllString(r, "$1")
		r = strings.TrimPrefix(r, "https://")
		r = strings.TrimPrefix(r, "http://")
		r = strings.TrimPrefix(r, "ssh://")
		r = strings.TrimPrefix(r, "git://")
	}
	r = strings.TrimSuffix(r, "/")
	r = strings.TrimSuffix(r, ".git")
	return r
}

// WorkspaceIDFor derives the 12-hex identity: the normalized remote when there
// is one, else the cleaned lowercased absolute path (Windows filesystems are
// case-insensitive, so the same folder typed two ways must not fork memory).
func WorkspaceIDFor(rootPath, remoteURL string) string {
	source := NormalizeRemoteURL(remoteURL)
	if source == "" {
		source = strings.ToLower(filepath.Clean(rootPath))
	}
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])[:12]
}

// EnsureWorkspace registers (or refreshes) the workspace for rootPath and
// returns its row. When a previously remote-less workspace shows up with a
// remote, its identity would change — the old rows are re-keyed so the
// accumulated memory follows the project instead of silently forking.
func (r *WorkspaceRepository) EnsureWorkspace(rootPath, remoteURL string) (*models.Workspace, error) {
	id := WorkspaceIDFor(rootPath, remoteURL)

	// A remote appeared on a path we already track under a path-derived id.
	if remoteURL != "" {
		pathID := WorkspaceIDFor(rootPath, "")
		if pathID != id {
			var exists int
			_ = r.db.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE workspace_id = ? AND remote_url = ''`, pathID).Scan(&exists)
			if exists > 0 {
				if err := r.RekeyWorkspace(pathID, id); err != nil {
					return nil, err
				}
			}
		}
	}

	now := time.Now()
	_, err := r.db.Exec(
		`INSERT INTO workspaces (workspace_id, root_path, remote_url, created_at, last_seen)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(workspace_id) DO UPDATE SET root_path = excluded.root_path, remote_url = excluded.remote_url, last_seen = excluded.last_seen`,
		id, rootPath, NormalizeRemoteURL(remoteURL), now, now,
	)
	if err != nil {
		return nil, err
	}
	return r.Get(id)
}

// workspaceKeyedTables lists every table carrying a workspace_id, so RekeyWorkspace
// cannot silently miss one added later — extend this list with each new table.
var workspaceKeyedTables = []string{
	"observations",
	"code_nodes",
	"code_edges",
	"file_fingerprints",
	"project_meta",
	"memory_lessons",
	"regressions",
}

// RekeyWorkspace moves a workspace and everything keyed to it onto a new
// identity, in one transaction.
func (r *WorkspaceRepository) RekeyWorkspace(oldID, newID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, table := range workspaceKeyedTables {
		if _, err := tx.Exec(`UPDATE `+table+` SET workspace_id = ? WHERE workspace_id = ?`, newID, oldID); err != nil {
			return err
		}
	}
	// The new id may already have a row (fresh EnsureWorkspace raced the rekey);
	// keep whichever exists and drop the old identity row.
	if _, err := tx.Exec(`UPDATE OR IGNORE workspaces SET workspace_id = ? WHERE workspace_id = ?`, newID, oldID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM workspaces WHERE workspace_id = ?`, oldID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *WorkspaceRepository) Get(id string) (*models.Workspace, error) {
	var w models.Workspace
	var rawCreated, rawSeen interface{}
	err := r.db.QueryRow(
		`SELECT workspace_id, root_path, remote_url, distilled_through, created_at, last_seen FROM workspaces WHERE workspace_id = ?`, id,
	).Scan(&w.WorkspaceID, &w.RootPath, &w.RemoteURL, &w.DistilledThrough, &rawCreated, &rawSeen)
	if err != nil {
		return nil, err
	}
	w.CreatedAt = parseTimeValue(rawCreated)
	w.LastSeen = parseTimeValue(rawSeen)
	return &w, nil
}

func (r *WorkspaceRepository) List() ([]models.Workspace, error) {
	rows, err := r.db.Query(`SELECT workspace_id, root_path, remote_url, distilled_through, created_at, last_seen FROM workspaces ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Workspace
	for rows.Next() {
		var w models.Workspace
		var rawCreated, rawSeen interface{}
		if err := rows.Scan(&w.WorkspaceID, &w.RootPath, &w.RemoteURL, &w.DistilledThrough, &rawCreated, &rawSeen); err != nil {
			return nil, err
		}
		w.CreatedAt = parseTimeValue(rawCreated)
		w.LastSeen = parseTimeValue(rawSeen)
		out = append(out, w)
	}
	return out, rows.Err()
}

// SetDistilledThrough advances the learner's watermark.
func (r *WorkspaceRepository) SetDistilledThrough(workspaceID string, rowid int64) error {
	_, err := r.db.Exec(`UPDATE workspaces SET distilled_through = ? WHERE workspace_id = ?`, rowid, workspaceID)
	return err
}
