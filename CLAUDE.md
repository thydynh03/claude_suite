# CLAUDE.md

Guidance for AI agents working in this repository.

## What this is

Claude Suite / Antigravity Manager — a Wails v2 desktop app (Go backend + Svelte 5
frontend) that orchestrates AI coding sub-agents. Users describe a project → an AI
decomposes it into Kanban tasks → the orchestrator dispatches tasks to sub-agents
(Claude CLI or Antigravity/Gemini CLI) that run against a chosen workspace folder.

## Build & test

```bash
go build ./...            # backend
go vet ./...
go test ./backend/...     # backend tests
cd frontend && npm ci && npm run build   # frontend
wails dev                 # full app hot-reload (needs Wails CLI)
wails build -platform windows/amd64 -clean
```

CI runs the first block on every push/PR (`.github/workflows/ci.yml`).

## Architecture map

- `app.go` — Wails-bound `App` methods (the frontend API surface).
- `backend/orchestrator/` — parallel worker pool. `orchestrator.go` dispatches
  dependency-satisfied tasks up to `maxConcurrency`, each in its own goroutine with
  a cancellable context (Stop/Retry per task). `dispatcher.go` matches tasks→agents.
- `backend/cli/` — CLI runners. `claude.go` uses `--output-format stream-json` to
  parse REAL token usage/cost and surface tool calls; `antigravity.go` handles
  Gemini with key-pool rotation on 429. Both honor `TaskTimeout()`.
- `backend/database/` — SQLite repos (agents, tasks, memory).
- `frontend/src/components/pages/` — main UI; `KanbanView.svelte` is the board +
  Task Inspector drawer; `TaskBoardPage.svelte` is the container + AI Plan Builder.
- `frontend/src/lib/stores/appState.ts` — global stores (`tasksStore`, `agentsStore`,
  `taskLogsStore`).

## Critical conventions & gotchas

- **Wails bindings are NOT auto-generated here.** When you add/change an `App`
  method in `app.go`, you MUST manually mirror it in BOTH
  `frontend/wailsjs/go/main/App.js` and `App.d.ts`, or the frontend call fails at
  runtime. (`wails generate module` regenerates them if the CLI is available.)
- **Event bus** (Wails `EventsEmit`/`EventsOn`): `board_updated`, `agent_updated`,
  `log_entry`, `task_log` (per-task, carries `task_id`), `ask_approval` (carries
  `taskId`). `board_updated` is handled centrally in `App.svelte` (single source
  refreshing `tasksStore`) — do not re-add per-component board fetch listeners.
- **Secrets**: never hardcode credentials. GCP OAuth is read from env
  (`CLAUDE_SUITE_GCP_CLIENT_ID` / `_CLIENT_SECRET`) or `gcp_oauth.json` in the data
  dir (both gitignored).
- Sub-agents run with `--dangerously-skip-permissions` against the workspace — they
  can create/modify files directly. `AutoSnapshot` git-commits before each task so
  the current task's changes stay as an uncommitted diff (`GetWorkspaceDiff`).
- Keyword→agent matching uses `keywordMatch` (word boundaries) — never plain
  `strings.Contains` for short tags, or "DATABASE" matches "BA".
