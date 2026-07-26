import { render, screen } from '@testing-library/svelte'
import { tick } from 'svelte'
import { beforeEach, describe, expect, it } from 'vitest'
import CockpitPage from './CockpitPage.svelte'
import { agentsStore, tasksStore } from '../../lib/stores/appState'

// The Cockpit and the Kanban board read the same store. What used to differ was
// whether they re-rendered: a metric computed by a function called from the
// template is read untracked, so the numbers froze at whatever they were when
// the page mounted while the board next door kept moving.
function metric(label: string): string {
  const cell = screen.getByText(label).closest('div')?.parentElement
  const value = cell?.querySelector('span.text-2xl')
  return value?.textContent?.trim() ?? ''
}

function task(id: string, status: string) {
  return { task_id: id, title: id, status, priority: 'normal' }
}

describe('CockpitPage metrics', () => {
  beforeEach(() => {
    tasksStore.set([])
    agentsStore.set([])
  })

  it('counts the tasks present when the page mounts', async () => {
    tasksStore.set([task('a', 'running'), task('b', 'done')] as never)
    render(CockpitPage)
    await tick()

    expect(metric('Running')).toBe('1')
    expect(metric('Done')).toBe('1')
  })

  it('follows the board when a task changes status afterwards', async () => {
    tasksStore.set([task('a', 'queued')] as never)
    render(CockpitPage)
    await tick()
    expect(metric('Queue')).toBe('1')

    // The orchestrator picks the task up: the board moves the card to Running,
    // and the Cockpit has to agree with it.
    tasksStore.set([task('a', 'running')] as never)
    await tick()

    expect(metric('Queue')).toBe('0')
    expect(metric('Running')).toBe('1')
  })

  it('follows tasks that appear after mount, as a decomposed plan does', async () => {
    render(CockpitPage)
    await tick()
    expect(metric('Running')).toBe('0')

    tasksStore.set([task('a', 'backlog'), task('b', 'backlog'), task('c', 'failed')] as never)
    await tick()

    expect(metric('Queue')).toBe('2')
    expect(metric('Failed')).toBe('1')
  })

  it('follows agent activity and token totals', async () => {
    render(CockpitPage)
    await tick()
    expect(metric('Agents Active')).toBe('0')

    agentsStore.set([
      { id: '1', status: 'running', tokens_used: 1500 },
      { id: '2', status: 'idle', tokens_used: 500 },
    ] as never)
    await tick()

    expect(metric('Agents Active')).toBe('1')
    expect(metric('Tokens')).toBe('2.0k')
  })
})
