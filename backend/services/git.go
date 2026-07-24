package services

import (
	"fmt"
	"os/exec"
	"time"
)

type GitService struct{}

func NewGitService() *GitService {
	return &GitService{}
}

func (g *GitService) AutoSnapshot(cwd string) error {
	if cwd == "" {
		return nil
	}

	// Check if git repository
	cmdCheck := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmdCheck.Dir = cwd
	if err := cmdCheck.Run(); err != nil {
		// Not a git repo, skip
		return nil
	}

	// Stage all changes
	cmdAdd := exec.Command("git", "add", ".")
	cmdAdd.Dir = cwd
	_ = cmdAdd.Run()

	// Create commit
	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf("Auto-snapshot git tự động tại %s", ts)
	cmdCommit := exec.Command("git", "commit", "-m", msg)
	cmdCommit.Dir = cwd

	return cmdCommit.Run()
}
