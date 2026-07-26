package services

import (
	"claude_suite/backend/textutil"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"os/exec"

	"claude_suite/backend/sysproc"
)

// VerifyService runs build checks in a workspace so an agent's changes are
// validated (compile / build) before a task is reported as done.
type VerifyService struct{}

func NewVerifyService() *VerifyService {
	return &VerifyService{}
}

// VerifyResult reports the outcome of the workspace build checks.
type VerifyResult struct {
	Ran    bool   `json:"ran"`    // whether any check ran
	Passed bool   `json:"passed"` // true if all checks that ran succeeded
	Report string `json:"report"` // combined command output
}

// Verify detects the project type(s) in cwd and runs the relevant build:
//   - Go:       `go build ./...` when go.mod is present
//   - Frontend: `npm run build` when a package.json with a build script exists
//
// It returns Ran=false when nothing applicable is found.
func (v *VerifyService) Verify(cwd string) VerifyResult {
	res := VerifyResult{Passed: true}
	if cwd == "" {
		return res
	}

	if fileExists(filepath.Join(cwd, "go.mod")) {
		ok, out := runCmd(cwd, 4*time.Minute, "go", "build", "./...")
		res.Ran = true
		res.Report += "$ go build ./...\n" + out + "\n"
		if !ok {
			res.Passed = false
		}
	}

	// Prefer a frontend/ subdir, else the root, when it has a build script.
	feDir := cwd
	if fileExists(filepath.Join(cwd, "frontend", "package.json")) {
		feDir = filepath.Join(cwd, "frontend")
	}
	if hasNpmBuildScript(filepath.Join(feDir, "package.json")) {
		ok, out := runCmd(feDir, 6*time.Minute, npmExe(), "run", "build")
		res.Ran = true
		res.Report += "$ npm run build\n" + out + "\n"
		if !ok {
			res.Passed = false
		}
	}

	return res
}

func runCmd(dir string, timeout time.Duration, name string, args ...string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := sysproc.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	report := string(out)
	// A command that never started produces no output at all, and the whole
	// explanation lives in err: on a machine with no Node or Go toolchain the
	// report was empty, so every otherwise-successful task failed with
	// "Build verification FAILED:" and nothing after it. An ExitError already
	// spoke through the output, so only its bare status is redundant.
	if err != nil {
		if _, isExit := err.(*exec.ExitError); !isExit {
			if strings.TrimSpace(report) == "" {
				report = err.Error()
			} else {
				report += "\n" + err.Error()
			}
		}
	}
	// Keep the tail: a compiler or bundler puts its error summary at the bottom.
	return err == nil, textutil.TruncateTail(report, 6000)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// hasNpmBuildScript parses the manifest rather than searching it for the word
// "build": a dependency literally named build ("build": "^0.1.4") matched the
// substring test, and `npm run build` then failed with "Missing script:
// build", failing every completed task in that workspace.
func hasNpmBuildScript(pkgPath string) bool {
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	return strings.TrimSpace(pkg.Scripts["build"]) != ""
}

func npmExe() string {
	if _, err := exec.LookPath("npm.cmd"); err == nil {
		return "npm.cmd"
	}
	return "npm"
}
