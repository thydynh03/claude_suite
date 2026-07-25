# Architecture decisions that look wrong until you change them

Every entry below is a decision that already cost someone a debugging session.
Each one reads like an unnecessary detour, which is exactly why it keeps getting
"cleaned up" — and then the bug comes back.

If you are about to simplify something on this list, read the entry first. If you
still think it should change, change it *and* delete the entry in the same
commit, so the next person is not misled.

---

## 1. The two frontends must use the same method names

Claude Suite ships two frontends over one backend:

| | Adapter |
|---|---|
| Wails desktop app | methods on `*App` in `app.go` |
| Terminal UI | methods on `*tui.RepositoryTaskActions` in `backend/tui/task_actions.go` |

Both are hand-written adapters over the same `backend/services` and
`backend/database`. Eight capabilities once existed under two different names —
`GetGitStatus`/`GitStatus`, `ExportKanbanReport`/`ExportReport`,
`CreateGitCommit`/`GitCommit`, and five more. Code review caught none of them.

`backend/contract` now reads both adapters from source and fails when the TUI
declares a capability the Wails app does not have under the same name.
Genuinely TUI-only methods go in `tuiOnlyMethods` **with a written reason**.

**Why a shared interface was rejected.** `var _ core.Facade = (*App)(nil)` on
both adapters is the obvious idea, and it does not fit: six methods differ in
signature for real reasons. Wails returns values to JavaScript
(`StartOrchestrator() bool`, `RunQuickCLI(...) (*cli.RunResult, error)`), while
the TUI mutates its Bubble Tea model and publishes events
(`StartOrchestrator()`, `RunQuickCLI(...) (string, error)`). Forcing one shape
would damage both frontends to satisfy the compiler.

Note also that an interface would only have locked the *names*. It would not
have noticed that the TUI's `RunQuickCLI` silently drops the `system` prompt —
which it still does.

**Why the check parses source instead of using reflection.** It would need to
import package `main`, and the repository root cannot be compiled in CI: `main.go`
has `//go:embed all:frontend/dist`, which does not exist until the frontend job
has built it. A test that cannot run in CI guards nothing.

---

### The TUI is deliberately the smaller frontend

Name parity is enforced; *depth* parity is not. The desktop app's
`RunBrowserTask` takes eight parameters and drives a multi-step ReAct loop with a
persistent Chrome profile and interactive forms. The TUI's takes two — a URL and
whether to screenshot — and that is on purpose: the TUI is a keyboard inspector,
not a second full client.

If you port that loop to the TUI, it needs its own input flow for the prompt,
model and step limit, plus somewhere to render each step. Treat it as a feature,
not as a consistency fix.

---

## 2. `sqliteURI` looks like string-fiddling around `url.URL`. It is not.

`backend/tui/loader.go`:

```go
p := filepath.ToSlash(absolute)
if !strings.HasPrefix(p, "/") {
    p = "/" + p
}
return (&url.URL{Scheme: "file", Path: p, RawQuery: query}).String()
```

A Windows path is `E:\data\agent.db`. Without the leading slash, the `file:` URL
becomes `file://E:/data/agent.db` — and `E:` is then parsed as the *host*, not
the drive. The path normalisation is not decoration; it is the difference
between opening the database and failing to find it.

---

## 3. Spawning a visible console uses `/c`, never `/k`

`backend/cli/process_windows.go` already says it, and it is worth repeating
because `/k` is what most examples on the internet use:

> `/k` leaves the shell alive after the tool exits, so `cmd.Wait()` would block
> until the user closes the console by hand and the task would never complete.

The new console still shows the output with `/c`.

---

## 4. The browser agent gets its own Chrome profile, at a path without spaces

Two separate constraints, both learned the hard way:

- **A dedicated profile, not the user's real one.** Driving the real profile
  made Chrome wipe its cookies, which signed the user out of everything. The
  agent profile is persistent, so signing in is a one-time cost, not per-run.
- **No spaces in the profile path** (`AgentUserDataDir`). A space silently breaks
  `--user-data-dir` quoting and Chrome opens the fragments as URLs instead.

---

## 5. Approval channels are keyed by task ID

`backend/orchestrator` keeps `approvalChans[task.TaskID]`, not a single shared
channel. With the worker pool running several agents at once, one shared channel
means the wrong task consumes the answer — the user approves task A and task B
starts running. The `ask_approval` event carries `taskId` for the same reason.

---

## 6. The webhook server binds before `Start` returns

`backend/services/webhook.go` calls `net.Listen` synchronously and only then
hands the listener to a goroutine. The obvious version — `go
srv.ListenAndServe()` — always returns `nil`, so a port collision was reported to
the user as "webhook running" while nothing was listening.

The service also holds a mutex around `running`/`server`: they are written by the
serve goroutine and read by the UI thread.

---

## 7. Workspace containment lives in one function

`services.EnsureWithinWorkspace` is called by *both* adapters' `ReadFileContent`.
It previously existed only on the TUI path, which is worse than having no check
at all: the code reads as protected on both paths while only one is.

---

## 8. CI is split on purpose

`.github/workflows/ci.yml`:

- **The Go job runs on Windows and excludes the repository root.** The CLI
  runners set Windows-only `syscall.SysProcAttr` fields; the root cannot build
  before the frontend job has produced `frontend/dist` for `//go:embed`.
  `./cmd/...` *is* included — the TUI binary went uncompiled by CI until it was.
- **The race detector runs on Linux**, because `-race` needs CGO and Windows
  developer machines rarely have gcc. `backend/cli/process_other.go` is what
  lets the backend compile there.

---

## 9. Regenerate the Wails bindings; do not hand-edit them

Covered in `CLAUDE.md`. Repeated here because it has broken the build twice:
`frontend/wailsjs/**` is generated. Run `wails generate module` after changing an
`App` method or any struct that crosses the boundary. Hand-maintained copies
drift, and a running `wails dev` may delete these files mid-edit.
