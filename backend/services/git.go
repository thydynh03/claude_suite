package services

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"claude_suite/backend/sysproc"
)

// logFieldSep separates the fields of one git-log line. A "|" was used before
// and real commit subjects contain it, so those lines split into more fields
// than expected and the subject arrived truncated. ASCII unit separator cannot
// appear in any of these fields.
const logFieldSep = ""

type GitCommitInfo struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	// Parents is what makes a graph drawable: one parent is a normal commit, two
	// or more is a merge, and none is the root. Without it the UI could only ever
	// render a flat list, which is what it did.
	Parents []string `json:"parents"`
	// Refs are the branch and tag names pointing at this commit, so the graph can
	// show where each branch head actually is.
	Refs []string `json:"refs"`
}

type GitBranchInfo struct {
	Current  string   `json:"current"`
	Branches []string `json:"branches"`
}

type GitService struct{}

func NewGitService() *GitService {
	return &GitService{}
}

func (g *GitService) AutoSnapshot(cwd string) error {
	if cwd == "" {
		return nil
	}

	cmdCheck := sysproc.Command("git", "rev-parse", "--is-inside-work-tree")
	cmdCheck.Dir = cwd
	if err := cmdCheck.Run(); err != nil {
		return nil
	}

	cmdAdd := sysproc.Command("git", "add", ".")
	cmdAdd.Dir = cwd
	_ = cmdAdd.Run()

	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf("Auto-snapshot git tự động tại %s", ts)
	cmdCommit := sysproc.Command("git", "commit", "-m", msg)
	cmdCommit.Dir = cwd

	return cmdCommit.Run()
}

// allowedGitSubcommands are the git subcommands the in-app command box may run.
// Destructive history rewrites and force pushes are intentionally excluded.
var allowedGitSubcommands = map[string]bool{
	"status": true, "add": true, "commit": true, "log": true, "diff": true,
	"branch": true, "checkout": true, "switch": true, "restore": true, "reset": true,
	"revert": true, "stash": true, "show": true, "remote": true, "fetch": true,
	"pull": true, "tag": true, "rm": true, "mv": true, "config": true,
}

// RunCommand runs a whitelisted git command inside the workspace and returns its
// combined output. Rejects non-git input and disallowed/destructive subcommands.
func (g *GitService) RunCommand(cwd string, args []string) (string, error) {
	if cwd == "" {
		return "", fmt.Errorf("chưa chọn workspace")
	}
	// Accept an optional leading "git" token for convenience.
	if len(args) > 0 && args[0] == "git" {
		args = args[1:]
	}
	if len(args) == 0 {
		return "", fmt.Errorf("lệnh git rỗng")
	}
	sub := args[0]
	if !allowedGitSubcommands[sub] {
		return "", fmt.Errorf("lệnh git '%s' không được phép trong panel này", sub)
	}
	if sub == "push" {
		return "", fmt.Errorf("push bị chặn trong panel vì an toàn")
	}
	// Destructive flags are matched per argument, exactly. The old check
	// substring-scanned the joined string: `checkout -f` slipped through (the
	// pattern needed a trailing space), `reset --hard` was never looked at —
	// each able to destroy the current task's uncommitted diff, the one thing
	// AutoSnapshot exists to preserve — while a commit message mentioning
	// "-f " was blocked with a force-push error.
	for _, a := range args[1:] {
		switch a {
		case "-f", "--force", "--force-with-lease", "-D", "--hard", "--delete-force":
			return "", fmt.Errorf("cờ '%s' bị chặn trong panel vì có thể phá dữ liệu chưa commit", a)
		}
		// Flag arguments end at "--"; everything after is pathspec/message
		// territory where these tokens are data, not switches.
		if a == "--" {
			break
		}
	}

	cmd := sysproc.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %v", sub, err)
	}
	return string(out), nil
}

// Workspace identity resolution lives in projectmap.WorkspaceRemoteIdentity —
// one resolver, shared by the mapper and the orchestrator's capture path.

// ChangedFiles lists the paths touched since HEAD — tracked changes plus
// untracked files. Because AutoSnapshot commits before each task, right after
// a task this is exactly that task's changes; the incremental map update runs
// on it after every run, so this must stay on sysproc (a bare exec.Command
// here would strobe a console window per task on Windows).
func (g *GitService) ChangedFiles(cwd string) []string {
	if cwd == "" {
		return nil
	}
	check := sysproc.Command("git", "rev-parse", "--is-inside-work-tree")
	check.Dir = cwd
	if err := check.Run(); err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var files []string
	collect := func(args ...string) {
		cmd := sysproc.Command("git", args...)
		cmd.Dir = cwd
		var out bytes.Buffer
		cmd.Stdout = &out
		_ = cmd.Run()
		for _, line := range strings.Split(out.String(), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || seen[line] {
				continue
			}
			seen[line] = true
			files = append(files, line)
		}
	}
	collect("diff", "--name-only", "HEAD")
	collect("ls-files", "--others", "--exclude-standard")
	return files
}

// GetWorkspaceDiff returns the current uncommitted diff (tracked changes plus a
// listing of new untracked files) so the UI can show what an agent just changed.
func (g *GitService) GetWorkspaceDiff(cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	check := sysproc.Command("git", "rev-parse", "--is-inside-work-tree")
	check.Dir = cwd
	if err := check.Run(); err != nil {
		return "", nil // not a git repo — nothing to diff
	}

	var out bytes.Buffer
	diff := sysproc.Command("git", "diff", "HEAD", "--stat", "--patch")
	diff.Dir = cwd
	diff.Stdout = &out
	_ = diff.Run()

	// Append untracked (new) files, which don't appear in `git diff HEAD`.
	untracked := sysproc.Command("git", "ls-files", "--others", "--exclude-standard")
	untracked.Dir = cwd
	var uout bytes.Buffer
	untracked.Stdout = &uout
	_ = untracked.Run()
	if files := strings.TrimSpace(uout.String()); files != "" {
		out.WriteString("\n# Untracked (new) files:\n")
		for _, f := range strings.Split(files, "\n") {
			out.WriteString("+ " + f + "\n")
		}
	}

	// A clean tree returns "", not a sentence. The old in-band sentinel
	// ("Không có thay đổi...") was detected downstream with strings.Contains,
	// so a REAL diff touching any file containing that phrase — this repo's
	// own UI files — read as "no changes": the commit-message button errored
	// and the nightly commit job skipped, both silently. Callers render
	// their own empty-state text.
	if strings.TrimSpace(out.String()) == "" {
		return "", nil
	}
	return out.String(), nil
}

func (g *GitService) GetStatus(cwd string) (map[string]interface{}, error) {
	if cwd == "" {
		return map[string]interface{}{"is_repo": false}, nil
	}

	cmdBranch := sysproc.Command("git", "branch", "--show-current")
	cmdBranch.Dir = cwd
	var outBranch bytes.Buffer
	cmdBranch.Stdout = &outBranch
	if err := cmdBranch.Run(); err != nil {
		// "git is not installed" and "this folder is not a repo" both landed on
		// is_repo:false, so every git feature went quietly inert on a machine
		// without git and nothing ever said why.
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("chưa cài git trên máy này — cài Git for Windows rồi thử lại")
		}
		return map[string]interface{}{"is_repo": false}, nil
	}
	currentBranch := strings.TrimSpace(outBranch.String())

	cmdStatus := sysproc.Command("git", "status", "--porcelain")
	cmdStatus.Dir = cwd
	var outStatus bytes.Buffer
	cmdStatus.Stdout = &outStatus
	_ = cmdStatus.Run()

	lines := strings.Split(strings.TrimSpace(outStatus.String()), "\n")
	changedFiles := 0
	if len(lines) > 0 && lines[0] != "" {
		changedFiles = len(lines)
	}

	return map[string]interface{}{
		"is_repo":       true,
		"branch":        currentBranch,
		"changed_files": changedFiles,
		"clean":         changedFiles == 0,
	}, nil
}

func (g *GitService) GetBranches(cwd string) (*GitBranchInfo, error) {
	if cwd == "" {
		return &GitBranchInfo{Current: "main", Branches: []string{"main"}}, nil
	}

	cmd := sysproc.Command("git", "branch", "-a")
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	lines := strings.Split(out.String(), "\n")
	var current string
	var branches []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "* ") {
			bName := strings.TrimPrefix(l, "* ")
			current = bName
			branches = append(branches, bName)
		} else if !strings.Contains(l, "->") {
			branches = append(branches, l)
		}
	}
	return &GitBranchInfo{Current: current, Branches: branches}, nil
}

func (g *GitService) GetLog(cwd string, limit int) ([]GitCommitInfo, error) {
	if cwd == "" {
		return []GitCommitInfo{}, nil
	}
	if limit <= 0 {
		limit = 10
	}

	// --all so branches other than the current one appear; without it the "graph"
	// could only ever be a straight line. %p gives the parents and %D the refs.
	// The separator is a unit separator rather than "|", which occurs in real
	// commit subjects and used to split them into fragments.
	cmd := sysproc.Command("git", "log", "--all", fmt.Sprintf("-n%d", limit),
		"--date-order", "--pretty=format:%h"+logFieldSep+"%an"+logFieldSep+"%ar"+logFieldSep+"%p"+logFieldSep+"%D"+logFieldSep+"%s")
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return []GitCommitInfo{}, nil
	}

	lines := strings.Split(out.String(), "\n")
	commits := make([]GitCommitInfo, 0, len(lines))
	for _, l := range lines {
		parts := strings.Split(l, logFieldSep)
		if len(parts) < 6 {
			continue
		}

		c := GitCommitInfo{
			Hash:    parts[0],
			Author:  parts[1],
			Date:    parts[2],
			Message: parts[5],
		}
		if p := strings.TrimSpace(parts[3]); p != "" {
			c.Parents = strings.Fields(p)
		}
		for _, ref := range strings.Split(parts[4], ",") {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			// "HEAD -> master" names the branch the working tree is on; keeping
			// the arrow form would print the pointer rather than the branch name.
			ref = strings.TrimPrefix(ref, "HEAD -> ")
			c.Refs = append(c.Refs, ref)
		}
		commits = append(commits, c)
	}

	return commits, nil
}

func (g *GitService) CreateBranch(cwd string, branchName string) error {
	if cwd == "" || branchName == "" {
		return fmt.Errorf("invalid path or branch name")
	}
	cmd := sysproc.Command("git", "checkout", "-b", branchName)
	cmd.Dir = cwd
	return cmd.Run()
}

func (g *GitService) CheckoutBranch(cwd string, branchName string) error {
	if cwd == "" || branchName == "" {
		return fmt.Errorf("invalid path or branch name")
	}
	cmd := sysproc.Command("git", "checkout", branchName)
	cmd.Dir = cwd
	return cmd.Run()
}

func (g *GitService) CreateCommit(cwd string, message string) error {
	if cwd == "" || message == "" {
		return fmt.Errorf("invalid path or message")
	}
	cmdAdd := sysproc.Command("git", "add", ".")
	cmdAdd.Dir = cwd
	if err := cmdAdd.Run(); err != nil {
		return err
	}
	cmdCommit := sysproc.Command("git", "commit", "-m", message)
	cmdCommit.Dir = cwd
	return cmdCommit.Run()
}

func (g *GitService) RevertCommit(cwd string, commitHash string) error {
	if cwd == "" || commitHash == "" {
		return fmt.Errorf("invalid path or hash")
	}
	cmd := sysproc.Command("git", "revert", "--no-edit", commitHash)
	cmd.Dir = cwd
	return cmd.Run()
}
