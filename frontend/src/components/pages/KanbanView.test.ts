import { render, screen, waitFor } from '@testing-library/svelte'
import { tick } from 'svelte'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import KanbanView from './KanbanView.svelte'
import { agentsStore, tasksStore } from '../../lib/stores/appState'

// The board never fetches for itself — App.svelte's central board_updated
// handler refreshes tasksStore and this view reads it. The bindings it does
// call on mount (concurrency, verify toggle) are stubbed here.
vi.mock('../../../wailsjs/go/main/App', () => ({
  GetMaxConcurrency: vi.fn(async () => 3),
  GetVerifyBuild: vi.fn(async () => true),
  GetTasks: vi.fn(async () => []),
  RetryTask: vi.fn(async () => undefined),
  CreateTask: vi.fn(async () => undefined),
  UpdateTaskStatus: vi.fn(async () => undefined),
}))

function task(id: string, status: string, extra: Record<string, unknown> = {}) {
  return {
    task_id: id,
    title: 'Task ' + id,
    status,
    priority: 'normal',
    depends_on: [],
    ...extra,
  }
}

function columnHeader(title: string): string {
  // Headers render as "Backlog (2)" — the count is the assertion target.
  const el = screen.getByText((content) => content.trim().startsWith(title + ' ('))
  return el.textContent?.trim() ?? ''
}

describe('KanbanView columns', () => {
  beforeEach(() => {
    tasksStore.set([])
    agentsStore.set([])
  })

  it('sorts tasks into the five status columns with counts', async () => {
    tasksStore.set([
      task('a', 'backlog'),
      task('b', 'backlog'),
      task('c', 'queued'),
      task('d', 'running'),
      task('e', 'done'),
      task('f', 'failed'),
    ] as never)
    render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()

    expect(columnHeader('Backlog')).toBe('Backlog (2)')
    expect(columnHeader('Queued')).toBe('Queued (1)')
    expect(columnHeader('Running')).toBe('Running (1)')
    expect(columnHeader('Done')).toBe('Done (1)')
    expect(columnHeader('Failed')).toBe('Failed (1)')
  })

  it('follows the store when a card moves — the board is a reader, not an owner', async () => {
    tasksStore.set([task('a', 'backlog')] as never)
    render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()
    expect(columnHeader('Backlog')).toBe('Backlog (1)')

    // The orchestrator dispatches it: board_updated refreshes the store.
    tasksStore.set([task('a', 'running')] as never)

    await waitFor(() => expect(columnHeader('Backlog')).toBe('Backlog (0)'))
    expect(columnHeader('Running')).toBe('Running (1)')
  })
})

// The blocked badge is why a full board that dispatches nothing stopped
// looking like a dead Execute Plan button: a waiting task and a stuck task
// used to render identically.
describe('KanbanView blocked badges', () => {
  beforeEach(() => {
    tasksStore.set([])
    agentsStore.set([])
  })

  it('marks a task waiting on an unfinished dependency', async () => {
    tasksStore.set([
      task('dep', 'running'),
      task('blocked', 'backlog', { depends_on: ['dep'] }),
    ] as never)
    render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()

    expect(screen.getByText('Đang chờ task khác')).toBeInTheDocument()
  })

  it('marks a task dead when its dependency failed — waiting will not fix it', async () => {
    tasksStore.set([
      task('dep', 'failed'),
      task('blocked', 'backlog', { depends_on: ['dep'] }),
    ] as never)
    render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()

    expect(screen.getByText('Bị chặn bởi task lỗi')).toBeInTheDocument()
  })

  it('marks a task dead when its dependency no longer exists', async () => {
    tasksStore.set([task('blocked', 'backlog', { depends_on: ['vanished'] })] as never)
    render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()

    expect(screen.getByText('Bị chặn bởi task lỗi')).toBeInTheDocument()
  })

  it('drops the badge the moment the dependency is done', async () => {
    tasksStore.set([
      task('dep', 'running'),
      task('blocked', 'backlog', { depends_on: ['dep'] }),
    ] as never)
    render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()
    expect(screen.getByText('Đang chờ task khác')).toBeInTheDocument()

    tasksStore.set([
      task('dep', 'done'),
      task('blocked', 'backlog', { depends_on: ['dep'] }),
    ] as never)
    await tick()

    expect(screen.queryByText('Đang chờ task khác')).not.toBeInTheDocument()
    expect(screen.queryByText('Bị chặn bởi task lỗi')).not.toBeInTheDocument()
  })
})
