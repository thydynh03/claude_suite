import { render, screen, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import KanbanView from './KanbanView.svelte'
import { agentsStore, tasksStore, taskLogsStore } from '../../lib/stores/appState'

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

// The Task Inspector — and with it the live sub-agent log — used to be
// reachable only by hitting the card's title, a single `truncate`d line. Every
// other pixel of the card ignored the click, which is indistinguishable from a
// board that has stopped responding.
describe('opening the Task Inspector from a card', () => {
  beforeEach(() => {
    tasksStore.set([])
    agentsStore.set([])
    taskLogsStore.set({})
  })

  it('opens when the title is clicked', async () => {
    tasksStore.set([task('a', 'running', { prompt: 'PROMPT BODY TEXT' })] as never)
    render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()

    await fireEvent.click(screen.getByText('Task a'))
    await tick()

    expect(screen.queryByText('Task Inspector')).not.toBeNull()
  })

  it('opens when the prompt line is clicked', async () => {
    tasksStore.set([task('a', 'running', { prompt: 'PROMPT BODY TEXT' })] as never)
    render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()

    await fireEvent.click(screen.getByText('PROMPT BODY TEXT'))
    await tick()

    expect(screen.queryByText('Task Inspector')).not.toBeNull()
  })

  it('opens when the card body is clicked', async () => {
    tasksStore.set([task('a', 'running', { prompt: 'PROMPT BODY TEXT' })] as never)
    const { container } = render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()

    await fireEvent.click(container.querySelector('[draggable="true"]') as Element)
    await tick()

    expect(screen.queryByText('Task Inspector')).not.toBeNull()
  })

  // Selecting a task for bulk delete, or changing its agent, must not also
  // throw the drawer open over the board.
  it('does not open when the select checkbox is clicked', async () => {
    tasksStore.set([task('a', 'running')] as never)
    render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()

    await fireEvent.click(screen.getByLabelText('Chọn task Task a'))
    await tick()

    expect(screen.queryByText('Task Inspector')).toBeNull()
  })

  it('does not open when a dropdown on the card is clicked', async () => {
    tasksStore.set([task('a', 'running')] as never)
    const { container } = render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()

    const card = container.querySelector('[draggable="true"]') as Element
    const trigger = card.querySelector('button')
    expect(trigger).not.toBeNull()
    await fireEvent.click(trigger as Element)
    await tick()

    expect(screen.queryByText('Task Inspector')).toBeNull()
  })

  // Dropping outside a column fires neither drop nor dragleave.
  it('clears the drop-target highlight when a drag is abandoned', async () => {
    tasksStore.set([task('a', 'backlog')] as never)
    const { container } = render(KanbanView, { props: { onRefresh: vi.fn() } })
    await tick()

    const card = container.querySelector('[draggable="true"]') as Element
    const column = card.parentElement as Element

    await fireEvent.dragStart(card)
    await fireEvent.dragOver(column)
    await tick()
    expect(container.querySelectorAll('.border-primary').length).toBeGreaterThan(0)

    await fireEvent.dragEnd(card)
    await tick()

    expect(container.querySelectorAll('.border-primary').length).toBe(0)
  })
})
