// Package testsupport provides fakes shared by tests in several backend
// packages. It is only referenced from _test.go files, so nothing here reaches
// the shipped binary.
package testsupport

import (
	"context"
	"sync"

	"claude_suite/backend/cli"
	"claude_suite/backend/models"
)

// FakeRunner stands in for cli.CLIRunner so tests never spawn a real CLI.
//
// It records every call and tracks how many ran at once, which is what makes
// concurrency limits observable: assert on PeakInFlight rather than trying to
// catch a moment in time.
type FakeRunner struct {
	mu           sync.Mutex
	calls        []FakeCall
	inFlight     int
	peakInFlight int

	// Behaviour, if set, decides the result of each agent run. It is called
	// concurrently. Block inside it to hold a task "running".
	Behaviour func(ctx context.Context, agent *models.Agent, prompt string) *cli.RunResult
}

// FakeCall is one recorded invocation.
type FakeCall struct {
	Method    string
	AgentName string
	Prompt    string
	Model     string
	Cwd       string
}

func (f *FakeRunner) record(c FakeCall) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
}

func (f *FakeRunner) enter() {
	f.mu.Lock()
	f.inFlight++
	if f.inFlight > f.peakInFlight {
		f.peakInFlight = f.inFlight
	}
	f.mu.Unlock()
}

func (f *FakeRunner) leave() {
	f.mu.Lock()
	f.inFlight--
	f.mu.Unlock()
}

// Calls returns a copy of everything recorded so far.
func (f *FakeRunner) Calls() []FakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeCall(nil), f.calls...)
}

// CallCount is the number of invocations recorded.
func (f *FakeRunner) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// AgentCallCount counts invocations that ran a specific agent.
func (f *FakeRunner) AgentCallCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.AgentName == name {
			n++
		}
	}
	return n
}

// PeakInFlight is the highest number of runs that overlapped.
func (f *FakeRunner) PeakInFlight() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.peakInFlight
}

func okResult(output string) *cli.RunResult {
	return &cli.RunResult{Success: true, Output: output, SessionID: "fake-session"}
}

func (f *FakeRunner) RunAgent(agent *models.Agent, prompt string, onLog cli.LogCallback, cwd string) *cli.RunResult {
	return f.RunAgentCtx(context.Background(), agent, prompt, onLog, cwd)
}

func (f *FakeRunner) RunAgentCtx(ctx context.Context, agent *models.Agent, prompt string, onLog cli.LogCallback, cwd string) *cli.RunResult {
	name := ""
	if agent != nil {
		name = agent.Name
	}
	f.record(FakeCall{Method: "RunAgentCtx", AgentName: name, Prompt: prompt, Cwd: cwd})
	f.enter()
	defer f.leave()

	if f.Behaviour != nil {
		return f.Behaviour(ctx, agent, prompt)
	}
	return okResult("fake agent output")
}

func (f *FakeRunner) RunOnce(prompt, model, system string, onLog cli.LogCallback, cwd string) *cli.RunResult {
	f.record(FakeCall{Method: "RunOnce", Prompt: prompt, Model: model, Cwd: cwd})
	f.enter()
	defer f.leave()
	return okResult("fake once output")
}

func (f *FakeRunner) RunSession(prompt, model, system, sessionID string, onLog cli.LogCallback, cwd string) *cli.RunResult {
	f.record(FakeCall{Method: "RunSession", Prompt: prompt, Model: model, Cwd: cwd})
	f.enter()
	defer f.leave()
	return okResult("fake session output")
}

// Compile-time proof the fake keeps up with the interface. If CLIRunner gains a
// method, this breaks here rather than in every test that uses the fake.
var _ cli.CLIRunner = (*FakeRunner)(nil)
