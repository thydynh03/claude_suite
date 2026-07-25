package services

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type GitCommitInfo struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
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

	cmdCheck := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmdCheck.Dir = cwd
	if err := cmdCheck.Run(); err != nil {
		return nil
	}

	cmdAdd := exec.Command("git", "add", ".")
	cmdAdd.Dir = cwd
	_ = cmdAdd.Run()

	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf("Auto-snapshot git tự động tại %s", ts)
	cmdCommit := exec.Command("git", "commit", "-m", msg)
	cmdCommit.Dir = cwd

	return cmdCommit.Run()
}

// GetWorkspaceDiff returns the current uncommitted diff (tracked changes plus a
// listing of new untracked files) so the UI can show what an agent just changed.
func (g *GitService) GetWorkspaceDiff(cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	check := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	check.Dir = cwd
	if err := check.Run(); err != nil {
		return "", nil // not a git repo — nothing to diff
	}

	var out bytes.Buffer
	diff := exec.Command("git", "diff", "HEAD", "--stat", "--patch")
	diff.Dir = cwd
	diff.Stdout = &out
	_ = diff.Run()

	// Append untracked (new) files, which don't appear in `git diff HEAD`.
	untracked := exec.Command("git", "ls-files", "--others", "--exclude-standard")
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

	if strings.TrimSpace(out.String()) == "" {
		return "Không có thay đổi nào trong workspace.", nil
	}
	return out.String(), nil
}

func (g *GitService) GetStatus(cwd string) (map[string]interface{}, error) {
	if cwd == "" {
		return map[string]interface{}{"is_repo": false}, nil
	}

	cmdBranch := exec.Command("git", "branch", "--show-current")
	cmdBranch.Dir = cwd
	var outBranch bytes.Buffer
	cmdBranch.Stdout = &outBranch
	if err := cmdBranch.Run(); err != nil {
		return map[string]interface{}{"is_repo": false}, nil
	}
	currentBranch := strings.TrimSpace(outBranch.String())

	cmdStatus := exec.Command("git", "status", "--porcelain")
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

	cmd := exec.Command("git", "branch", "-a")
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

	cmd := exec.Command("git", "log", fmt.Sprintf("-n%d", limit), "--pretty=format:%h|%an|%ar|%s")
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return []GitCommitInfo{}, nil
	}

	lines := strings.Split(out.String(), "\n")
	var commits []GitCommitInfo
	for _, l := range lines {
		parts := strings.Split(l, "|")
		if len(parts) >= 4 {
			commits = append(commits, GitCommitInfo{
				Hash:    parts[0],
				Author:  parts[1],
				Date:    parts[2],
				Message: strings.Join(parts[3:], "|"),
			})
		}
	}
	return commits, nil
}

func (g *GitService) CreateBranch(cwd string, branchName string) error {
	if cwd == "" || branchName == "" {
		return fmt.Errorf("invalid path or branch name")
	}
	cmd := exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = cwd
	return cmd.Run()
}

func (g *GitService) CheckoutBranch(cwd string, branchName string) error {
	if cwd == "" || branchName == "" {
		return fmt.Errorf("invalid path or branch name")
	}
	cmd := exec.Command("git", "checkout", branchName)
	cmd.Dir = cwd
	return cmd.Run()
}

func (g *GitService) CreateCommit(cwd string, message string) error {
	if cwd == "" || message == "" {
		return fmt.Errorf("invalid path or message")
	}
	cmdAdd := exec.Command("git", "add", ".")
	cmdAdd.Dir = cwd
	if err := cmdAdd.Run(); err != nil {
		return err
	}
	cmdCommit := exec.Command("git", "commit", "-m", message)
	cmdCommit.Dir = cwd
	return cmdCommit.Run()
}

func (g *GitService) RevertCommit(cwd string, commitHash string) error {
	if cwd == "" || commitHash == "" {
		return fmt.Errorf("invalid path or hash")
	}
	cmd := exec.Command("git", "revert", "--no-edit", commitHash)
	cmd.Dir = cwd
	return cmd.Run()
}

