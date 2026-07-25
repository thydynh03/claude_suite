import { beforeEach, describe, expect, it } from 'vitest'
import { get } from 'svelte/store'
import { currentLang, t } from './i18n'

beforeEach(() => {
  currentLang.set('vi')
})

describe('translations', () => {
  it('returns Vietnamese by default', () => {
    expect(get(t)('cockpit')).toBe('Bàn Điều Khiển (Cockpit)')
  })

  // The sidebar renders through this store, so switching the language has to
  // change what those labels resolve to — a toggle that only sets a variable is
  // the bug this replaced.
  it('re-resolves every label when the language changes', () => {
    const before = get(t)('kanban')

    currentLang.set('en')
    const after = get(t)('kanban')

    expect(before).toBe('Bảng Kanban Kế Hoạch')
    expect(after).toBe('Kanban Board')
  })

  it('falls back to the key itself when there is no translation', () => {
    expect(get(t)('not_a_real_key' as never)).toBe('not_a_real_key')
  })

  it('covers the same keys in both languages', () => {
    currentLang.set('vi')
    const translate = get(t)

    const keys = [
      'cockpit',
      'kanban',
      'virtual_office',
      'browser_agent',
      'scheduler',
      'settings',
      'code_studio',
      'support',
    ] as const

    for (const key of keys) {
      currentLang.set('vi')
      const vi = get(t)(key)
      currentLang.set('en')
      const en = get(t)(key)

      expect(vi, `missing Vietnamese for ${key}`).not.toBe(key)
      expect(en, `missing English for ${key}`).not.toBe(key)
    }

    expect(translate).toBeTypeOf('function')
  })
})
