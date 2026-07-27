# Contributing to Agent Center

**English** · [Tiếng Việt](CONTRIBUTING.vi.md)

Thanks for wanting to help. This page covers how to get the project running, the few
rules that are not obvious from the code, and what CI will check.

Before changing something that looks redundant, read
[`docs/ARCHITECTURE_DECISIONS.md`](docs/ARCHITECTURE_DECISIONS.md) — it lists the
decisions that already cost someone a debugging session.

By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

---

## Ways to help

| You want to… | Start here |
|---|---|
| Report a bug | [Open a bug report](https://github.com/thydynh03/Agent_Center/issues/new?template=bug_report.yml) — include the version from the title bar and the log file from the data directory |
| Suggest a feature | [Open a feature request](https://github.com/thydynh03/Agent_Center/issues/new?template=feature_request.yml) — describe the problem before the solution |
| Fix something small | Anything labelled [`good first issue`](https://github.com/thydynh03/Agent_Center/labels/good%20first%20issue) |
| Translate | `frontend/src/lib/stores/i18n.ts` holds every string. Keep labels short — the sidebar rail is 240px and there is a test that fails when a nav label no longer fits |
| Improve the docs | Every fix is welcome, including typos. Both language versions should stay in step |

You do not need to ask permission before opening a pull request. For anything large
enough that you would be upset to have it rejected, open an issue first so the
approach can be agreed cheaply.

---

## Prerequisites

| Tool | Version | Notes |
|---|---|---|
| Go | as pinned in `go.mod` | the toolchain line is authoritative |
| Node.js | 20.x | for the Svelte frontend |
| Wails CLI | v2.13.0 | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0` |

SQLite is `modernc.org/sqlite`, a pure-Go driver, so **no gcc is required** to build
or test — only the race detector needs CGO, and that runs on Linux in CI.

---

## Running the project

There are two frontends over one backend. Both read the same database.

```bash
# Desktop app (Wails + Svelte), hot reload
wails dev

# Terminal UI — read-only by default: no migrations, no writes
go run ./cmd/agent-center-tui --db /path/to/agent_manager.db

# Terminal UI with task mutations and orchestration enabled
go run ./cmd/agent-center-tui --db /path/to/agent_manager.db --write
```

Production build:

```bash
wails build -platform windows/amd64 -clean   # -> build/bin/AgentCenter.exe
```

Point the app at a **throwaway workspace** while you develop. Sub-agents run with
`--dangerously-skip-permissions` and will edit files for real.

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

Frontend tests run on Vitest. `npm run test:watch` reruns them as you edit. Stores
and utilities are plain TypeScript and need no DOM; component tests use
`@testing-library/svelte` against jsdom, configured in `vitest.config.ts` —
deliberately separate from `vite.config.ts`, so test tooling cannot change what the
production build does.

Two things about that command list are deliberate:

- **The repository root is excluded.** `main.go` has
  `//go:embed all:frontend/dist`, which does not exist until the frontend has been
  built. `./cmd/...` *is* included — the TUI binary went uncompiled by CI until
  someone noticed. The root is covered instead by the separate **Desktop app build**
  job, which runs a real `wails build`; that job is the only thing standing between a
  broken `app.go` and release day, and it also fails if `frontend/wailsjs` has
  drifted from the Go source.
- **The race detector runs on Linux in CI**, not on your machine. `-race` needs CGO,
  and Windows checkouts usually have no gcc. If you touch the orchestrator's
  goroutines, let CI check it.

`codeql.yml` runs static analysis on every push, and `release.yml` builds the
installer when a `v*` tag is pushed.

---

## The rules that are not obvious

### 1. A capability must have the same method name on both frontends

`app.go` (Wails) and `backend/tui/task_actions.go` (TUI) are parallel adapters over
the same services. Eight capabilities once existed under two names —
`GetGitStatus`/`GitStatus`, `ExportKanbanReport`/`ExportReport`, and six more — and
review caught none of them.

`backend/contract` now fails the build when the TUI declares a capability the Wails
app does not have under the same name. If a method genuinely belongs to one frontend
only, add it to `tuiOnlyMethods` **with a reason**.

Signatures may differ: Wails returns values to JavaScript, the TUI mutates its Bubble
Tea model. Only the names are enforced.

### 2. The Wails bindings are generated — never hand-edit them

After changing an `App` method, or any struct that crosses the boundary:

```bash
wails generate module
```

This rewrites `frontend/wailsjs/`. Hand-maintained copies have drifted from `app.go`
twice and broken the build both times. Note that a running `wails dev` may delete
these files while it rebuilds — regenerate rather than restoring an old copy.

### 3. Never match short keywords with `strings.Contains`

Agent routing has been broken twice by substring matching: `"DATABASE"` matched the
`BA` role, and `"latest"` matched the `test` keyword and sent the task to the QA
agent. Use word boundaries — see `orchestrator.keywordMatch` and
`models.keywordTags`.

### 4. Secrets never go in the source

GCP OAuth credentials are read from `AGENT_CENTER_GCP_CLIENT_ID` /
`AGENT_CENTER_GCP_CLIENT_SECRET`, or from `gcp_oauth.json` in the data directory.
Both are gitignored. Never paste a token into an issue — see [SECURITY.md](SECURITY.md).

### 5. Sub-agents write to the workspace directly

They run with `--dangerously-skip-permissions`. The orchestrator takes a git snapshot
before each task so the task's own changes stay as an uncommitted diff. Any new path
that reads or writes user files must go through `services.EnsureWithinWorkspace`.

### 6. Shared state crosses goroutines more often than it looks

The CLI runners are single instances and every task runs in its own goroutine. A
plain field write in a runner is a data race even when it looks like a one-line
cache — see `backend/cli/resolved_path.go` for the shape the fix should take.

### 7. Vendor CSS imported from JS is unlayered, and beats every utility class

An unlayered declaration wins over a layered one at equal specificity, so a package
stylesheet imported in `main.ts` silently outranks Tailwind. Import it from
`app.css` with `layer(base)` instead, and verify with `getComputedStyle` on the
running app rather than by eye — the failure is invisible in screenshots.

---

## Where things live

| Path | Contents |
|---|---|
| `app.go` | Wails-bound methods — the desktop app's API surface |
| `backend/orchestrator/` | worker pool, dispatch, per-task approval and retry |
| `backend/cli/` | Claude and Antigravity CLI runners |
| `backend/services/` | git, browser agent, webhook, exporter, scheduler |
| `backend/claims/` | the adjudication host, falsifier runner and MCP server |
| `backend/database/` | SQLite repositories and migrations |
| `backend/tui/` | terminal UI (`model` / `update` / `commands` / `view` files) |
| `backend/contract/` | cross-frontend consistency checks |
| `frontend/src/` | Svelte 5 desktop UI (`*.test.ts` next to what they cover) |
| `cmd/agent-center-tui/` | TUI entry point |
| `cmd/agent-center-claim/` | the claim-filing CLI |

---

## Commits and pull requests

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):
`fix:`, `feat:`, `refactor:`, `test:`, `docs:`, `chore:`, with an optional scope such
as `fix(ui):`. The subject says what changed; the body says **why**, and what went
wrong before. A message that only restates the diff is not worth the line.

The app itself can draft one for you from the diff, in the Source Control panel.

### Pull request checklist

- [ ] `go build`, `go vet` and `go test` pass for `./backend/... ./cmd/...`
- [ ] `npm run check`, `npm run test` and `npm run build` pass in `frontend/`
- [ ] `wails generate module` was run if an `App` method or shared struct changed
- [ ] New capability uses the same method name on both frontends
- [ ] Behaviour changes come with a test that fails without the change
- [ ] User-facing strings exist in both `vi` and `en`
- [ ] Docs updated in both languages if the change alters how the app is used
- [ ] No credentials, tokens or absolute local paths in the diff

---

## Reporting a defect you found while reviewing

If you are an agent — or a person using one — this repository has a way to state a
finding so it can be proven wrong, instead of writing it straight into a PR comment:

```bash
agent-center-claim --checks                    # what you may point at
agent-center-claim --host ws://HOST:9111 --session ID --token T \
  --author you/your-agent --provider claude \
  --subject "backend/cli/process_windows.go:17" \
  --assert  "cmd.Wait() never returns when the console is visible" \
  --falsify "console-window-hidden"
```

`--falsify` names an entry in `.agent-center/checks.json`, and **a falsifier passes
when your claim is wrong** — so a failing check confirms the defect. Omit it and the
claim is kept as an opinion that cannot block a merge. Full details, including the
MCP transport, are in [CLAUDE.md](CLAUDE.md).
