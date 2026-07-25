import { beforeEach, describe, expect, it, vi } from 'vitest'
import { get } from 'svelte/store'
import {
  addLog,
  addTaskLog,
  addToast,
  clearTaskLog,
  dismissToast,
  logs,
  setTaskScreenshot,
  taskLogsStore,
  taskScreenshotsStore,
  toasts,
} from './appState'

beforeEach(() => {
  logs.set([])
  taskLogsStore.set({})
  taskScreenshotsStore.set({})
  toasts.set([])
})

describe('addLog', () => {
  it('records the message and level', () => {
    addLog('orchestrator started', 'SUCCESS')

    const entries = get(logs)
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ message: 'orchestrator started', level: 'SUCCESS' })
  })

  it('defaults the level to INFO', () => {
    addLog('plain message')
    expect(get(logs)[0].level).toBe('INFO')
  })

  it('uses the timestamp the backend sent, when there is one', () => {
    addLog('from the backend', 'INFO', '09:41:00')
    expect(get(logs)[0].time).toBe('09:41:00')
  })

  it('stamps a time itself when the backend did not send one', () => {
    addLog('local')
    expect(get(logs)[0].time).toMatch(/^\d{2}:\d{2}:\d{2}$/)
  })

  // A long orchestrator run streams thousands of lines; the store is bounded so
  // the UI does not grow without limit.
  it('keeps the newest entries and drops the oldest', () => {
    for (let i = 0; i < 1200; i++) addLog(`line ${i}`)

    const entries = get(logs)
    expect(entries.length).toBeLessThanOrEqual(1001)
    expect(entries[entries.length - 1].message).toBe('line 1199')
    expect(entries.some((e) => e.message === 'line 0')).toBe(false)
  })
})

describe('addTaskLog', () => {
  it('keeps each task’s output separate', () => {
    addTaskLog('task-a', 'building')
    addTaskLog('task-b', 'testing')

    const byTask = get(taskLogsStore)
    expect(byTask['task-a'].map((e) => e.message)).toEqual(['building'])
    expect(byTask['task-b'].map((e) => e.message)).toEqual(['testing'])
  })

  // The backend emits task_log for every line an agent writes. Without an id
  // there is no inspector to show it in, so it is dropped rather than pooled
  // under an empty key.
  it('ignores an entry with no task id', () => {
    addTaskLog('', 'orphan line')
    expect(get(taskLogsStore)).toEqual({})
  })

  it('bounds a single task’s log', () => {
    for (let i = 0; i < 700; i++) addTaskLog('task-a', `line ${i}`)

    const entries = get(taskLogsStore)['task-a']
    expect(entries.length).toBeLessThanOrEqual(501)
    expect(entries[entries.length - 1].message).toBe('line 699')
  })

  it('does not disturb other tasks when one is cleared', () => {
    addTaskLog('task-a', 'a')
    addTaskLog('task-b', 'b')

    clearTaskLog('task-a')

    const byTask = get(taskLogsStore)
    expect(byTask['task-a']).toBeUndefined()
    expect(byTask['task-b']).toHaveLength(1)
  })
})

describe('setTaskScreenshot', () => {
  it('stores the capture against its task', () => {
    setTaskScreenshot('task-a', 'data:image/png;base64,AAAA')
    expect(get(taskScreenshotsStore)['task-a']).toBe('data:image/png;base64,AAAA')
  })

  it('ignores an empty id or payload', () => {
    setTaskScreenshot('', 'data:image/png;base64,AAAA')
    setTaskScreenshot('task-a', '')
    expect(get(taskScreenshotsStore)).toEqual({})
  })

  it('replaces the previous capture for the same task', () => {
    setTaskScreenshot('task-a', 'first')
    setTaskScreenshot('task-a', 'second')
    expect(get(taskScreenshotsStore)['task-a']).toBe('second')
  })
})

describe('toasts', () => {
  it('disappears on its own after the time to live', () => {
    vi.useFakeTimers()
    try {
      addToast('task done', 'SUCCESS', 4000)
      expect(get(toasts)).toHaveLength(1)

      vi.advanceTimersByTime(3999)
      expect(get(toasts)).toHaveLength(1)

      vi.advanceTimersByTime(1)
      expect(get(toasts)).toHaveLength(0)
    } finally {
      vi.useRealTimers()
    }
  })

  it('dismisses only the toast asked for', () => {
    vi.useFakeTimers()
    try {
      addToast('first')
      addToast('second')

      const [first] = get(toasts)
      dismissToast(first.id)

      expect(get(toasts).map((t) => t.message)).toEqual(['second'])
    } finally {
      vi.useRealTimers()
    }
  })

  // Each toast expires on its own timer, so an id has to stay unique or one
  // expiring would take another with it.
  it('gives every toast a distinct id', () => {
    vi.useFakeTimers()
    try {
      addToast('a')
      addToast('b')
      addToast('c')

      const ids = get(toasts).map((t) => t.id)
      expect(new Set(ids).size).toBe(ids.length)
    } finally {
      vi.useRealTimers()
    }
  })

  it('shrugs off dismissing an id that is not there', () => {
    vi.useFakeTimers()
    try {
      addToast('kept')
      dismissToast(999999)
      expect(get(toasts)).toHaveLength(1)
    } finally {
      vi.useRealTimers()
    }
  })
})
