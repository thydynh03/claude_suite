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

## 3. The visible console is a detached tail viewer, never the agent's own window

The "show console" toggle does NOT give the agent process a console. Two dead
ends prove why, both lived:

- A `cmd /k` wrapper keeps the shell alive after the tool exits, so
  `cmd.Wait()` blocks until the user closes the window by hand and the task
  never completes.
- `CREATE_NEW_CONSOLE` on the agent process itself opened an EMPTY window:
  stdout/stderr are piped to the app either way, and the window died with the
  process — before a human could read anything.

So the agent always runs hidden (`process_windows.go`), and
`backend/cli/viewer.go` opens a separate PowerShell `Get-Content -Wait`
console tailing that run's log file. The viewer is started and then
**released, never Wait()ed on** — its lifetime belongs to the user's hand,
which is exactly the property that made `/k` poison for the agent process.
If you attach any app-side wait to the viewer, you have rebuilt the `/k` bug
with extra steps.

---

## 4. The browser agent gets its own Chrome profile, at a path without spaces

**A dedicated profile, not the user's real one.** Two reasons, both learned the
hard way: Chrome 136 and later refuse `--remote-debugging-port` when the profile
is the default *User Data* directory, so the agent silently fell back to a
throwaway profile and every site appeared logged out; and driving the real
profile made Chrome wipe its cookies, signing the user out of everything. The
agent profile is persistent, so signing in is a one-time cost.

**The path may contain spaces, and that is fine.** `AgentUserDataDir` is rooted
at `LOCALAPPDATA`, which carries the account name, so it cannot promise
otherwise. It stays safe because the directory is passed as a single element of
an `exec.Command` argument slice — Go quotes it — and `agentChromePIDs` compares
it in Go instead of pasting it into a PowerShell string. Building a command line
by concatenating this path is what would break it.

> An earlier comment here claimed the path "deliberately contains no spaces".
> It never did: set an account name with a space and the test in
> `chrome_debug_test.go` shows the space surviving. Corrected rather than copied
> forward.

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

## 10. `AppendCheck` may write the falsifier catalogue — only from the guard-approval flow

`backend/claims/falsifier.go` documents the catalogue's trust model: it
"changes through review like any other code; an agent may only name an entry".
`claims.AppendCheck` deliberately relaxes that, under conditions that must not
erode:

- It is reachable only from the Memory page's **Approve guard** flow
  (`ApproveRegressionGuard` on both adapters), where a human has just been
  shown the **exact argv** — the UI renders the array, never a summary — and
  clicked approve.
- The command stays an argv slice end to end. Nothing on this path accepts a
  shell string, so there is still nothing for `;` or backticks to do.
- AutoSnapshot commits `.claude-suite/checks.json` afterwards, so the change
  still lands in git history where review can see it.

If you are adding another caller of `AppendCheck` — especially one where the
command text originates from an LLM without a human approving the argv — stop:
that is the exact hole the argv-only design exists to close.

## 11. Workspace identity has one resolver, and it checks the work tree first

`projectmap.WorkspaceRemoteIdentity` is the only way a workspace directory
becomes a memory identity, and both quirks inside it were observed live:

- Outside a repo, `git config --get remote.origin.url` falls back to the
  user's **global** config. One developer machine had a remote stored there —
  without the `rev-parse --show-toplevel` guard, every non-repo folder on that
  machine would share one identity and merge their memories.
- A stray `git init` in the user's home directory swallows everything under
  it (including `%TEMP%`). A workspace deeper than the repo toplevel therefore
  gets `remote#subpath`, so sibling folders under an umbrella repo do not
  collapse into one workspace.

The orchestrator's capture path and the mapper must call this same function;
resolving identity twice in two ways splits one folder's memory across two
workspace rows, and `workspace_repo.go`'s re-keying (`RekeyWorkspace`,
`workspaceKeyedTables`) only heals the path-gains-a-remote case.

---

## 12. The inbound webhook binds loopback, and that is the whole security model

`WebhookService.Start` listens on `127.0.0.1:<port>`, not `:<port>`. Binding
every interface is the obvious "make it useful on the LAN" change, and it is
the one thing this endpoint must never do.

The handler takes an unauthenticated POST and turns its body into a backlog
task. The orchestrator later hands that task's prompt to a sub-agent running
with `--dangerously-skip-permissions` inside the user's workspace. Bound to
`:9090`, anyone on the same café Wi-Fi could make this app write files.

There is no token to add "later" that fixes it retroactively: the whole point
is that reaching this port has to be a deliberate act by the person who owns
the machine — an SSH forward or a tunnel they started — not the default state
of a checkbox in Settings.

---

## 13. The updater's .bat switches codepage before it names a path

`buildUpdaterBat` starts with `chcp 65001` and doubles every literal `%` in the
paths it embeds. Both look like superstition; both were failures.

`cmd.exe` decodes a batch file in the console's OEM codepage, not UTF-8. This
app's users are Vietnamese, so `%TEMP%` routinely sits under `C:\Users\Trần\`
and portable copies live in folders like `D:\Phần mềm\`. Written as UTF-8 and
read as CP1258, those paths arrive as mojibake: all sixty `copy` attempts fail,
and the app — which already called `os.Exit(0)` to release the exe — never
comes back. The preamble is pure ASCII, so it decodes identically in any
codepage and switches the interpreter before the first path-bearing line.

Batch also expands `%VAR%` while reading a line, so a single `%` in a folder
name swallows the rest of the path.

If you rewrite this script, keep both properties, and keep the give-up branch
writing its marker: a swap that silently failed re-offered the same update
forever, and the user only saw an app that restarted unchanged.
