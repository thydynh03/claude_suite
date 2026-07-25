# Contributing to Claude Suite

Thanks for wanting to help. This page covers how to get the project running, the
few rules that are not obvious from the code, and what CI will check.

Before changing something that looks redundant, read
[`docs/ARCHITECTURE_DECISIONS.md`](docs/ARCHITECTURE_DECISIONS.md) — it lists the
decisions that already cost someone a debugging session.

---

## Prerequisites

| Tool | Version | Notes |
|---|---|---|
| Go | as pinned in `go.mod` | the toolchain line is authoritative |
| Node.js | 20.x | for the Svelte frontend |
| Wails CLI | v2.13.0 | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0` |

SQLite is `modernc.org/sqlite`, a pure-Go driver, so **no gcc is required** to
build or test — only the race detector needs CGO, and that runs on Linux in CI.

---

## Running the project

There are two frontends over one backend. Both read the same database.

```bash
# Desktop app (Wails + Svelte), hot reload
wails dev

# Terminal UI — read-only by default: no migrations, no writes
go run ./cmd/claude-suite-tui --db /path/to/agent_manager.db

# Terminal UI with task mutations and orchestration enabled
go run ./cmd/claude-suite-tui --db /path/to/agent_manager.db --write
```

Production build:

```bash
wails build -platform windows/amd64 -clean   # -> build/bin/ClaudeSuite.exe
```

---

## What CI runs

Run this locally before opening a pull request — it is what
`.github/workflows/ci.yml` does:

```bash
go build ./backend/... ./cmd/...
go vet ./backend/... ./cmd/...
go test ./backend/... ./cmd/...
cd frontend && npm ci && npm run check && npm run test && npm run build
```

Frontend tests run on Vitest. `npm run test:watch` reruns them as you edit.
Stores and utilities are plain TypeScript and need no DOM; component tests use
`@testing-library/svelte` against jsdom, configured in `vitest.config.ts` —
deliberately separate from `vite.config.ts`, so test tooling cannot change what
the production build does.

Two things about that command list are deliberate:

- **The repository root is excluded.** `main.go` has
  `//go:embed all:frontend/dist`, which does not exist until the frontend has
  been built. `./cmd/...` *is* included — the TUI binary went uncompiled by CI
  until someone noticed. The root is covered instead by the separate
  **Desktop app build** job, which runs a real `wails build`; that job is the
  only thing standing between a broken `app.go` and release day, and it also
  fails if `frontend/wailsjs` has drifted from the Go source.
- **The race detector runs on Linux in CI**, not on your machine. `-race`
  needs CGO, and Windows checkouts usually have no gcc. If you touch the
  orchestrator's goroutines, let CI check it.

---

## The rules that are not obvious

### 1. A capability must have the same method name on both frontends

`app.go` (Wails) and `backend/tui/task_actions.go` (TUI) are parallel adapters
over the same services. Eight capabilities once existed under two names —
`GetGitStatus`/`GitStatus`, `ExportKanbanReport`/`ExportReport`, and six more —
and review caught none of them.

`backend/contract` now fails the build when the TUI declares a capability the
Wails app does not have under the same name. If a method genuinely belongs to
one frontend only, add it to `tuiOnlyMethods` **with a reason**.

Signatures may differ: Wails returns values to JavaScript, the TUI mutates its
Bubble Tea model. Only the names are enforced.

### 2. The Wails bindings are generated — never hand-edit them

After changing an `App` method, or any struct that crosses the boundary:

```bash
wails generate module
```

This rewrites `frontend/wailsjs/`. Hand-maintained copies have drifted from
`app.go` twice and broken the build both times. Note that a running `wails dev`
may delete these files while it rebuilds — regenerate rather than restoring an
old copy.

### 3. Never match short keywords with `strings.Contains`

Agent routing has been broken twice by substring matching: `"DATABASE"` matched
the `BA` role, and `"latest"` matched the `test` keyword and sent the task to the
QA agent. Use word boundaries — see `orchestrator.keywordMatch` and
`models.keywordTags`.

### 4. Secrets never go in the source

GCP OAuth credentials are read from `CLAUDE_SUITE_GCP_CLIENT_ID` /
`CLAUDE_SUITE_GCP_CLIENT_SECRET`, or from `gcp_oauth.json` in the data
directory. Both are gitignored.

### 5. Sub-agents write to the workspace directly

They run with `--dangerously-skip-permissions`. The orchestrator takes a git
snapshot before each task so the task's own changes stay as an uncommitted diff.
Any new path that reads or writes user files must go through
`services.EnsureWithinWorkspace`.

---

## Where things live

| Path | Contents |
|---|---|
| `app.go` | Wails-bound methods — the desktop app's API surface |
| `backend/orchestrator/` | worker pool, dispatch, per-task approval and retry |
| `backend/cli/` | Claude and Antigravity CLI runners |
| `backend/services/` | git, browser agent, webhook, exporter, scheduler |
| `backend/database/` | SQLite repositories and migrations |
| `backend/tui/` | terminal UI (`model` / `update` / `commands` / `view` files) |
| `backend/contract/` | cross-frontend consistency checks |
| `frontend/src/` | Svelte 5 desktop UI (`*.test.ts` next to what they cover) |
| `cmd/claude-suite-tui/` | TUI entry point |

---

## Pull request checklist

- [ ] `go build`, `go vet` and `go test` pass for `./backend/... ./cmd/...`
- [ ] `npm run check` and `npm run build` pass in `frontend/`
- [ ] `wails generate module` was run if an `App` method or shared struct changed
- [ ] New capability uses the same method name on both frontends
- [ ] Behaviour changes come with a test that fails without the change
- [ ] No credentials, tokens or absolute local paths in the diff
