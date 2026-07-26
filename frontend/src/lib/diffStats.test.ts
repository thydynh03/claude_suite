import { describe, expect, it } from 'vitest'
import { countDiffChunks } from './diffStats'

const lines = (n: number, prefix = 'line') =>
  Array.from({ length: n }, (_, i) => `${prefix} ${i + 1}`).join('\n')

describe('countDiffChunks', () => {
  it('reports zero for identical text', () => {
    expect(countDiffChunks(lines(50), lines(50))).toBe(0)
  })

  // The defect the old counter had: one line inserted near the top shifts
  // every following line, and comparing line i against line i counted the
  // whole file as changed — the badge said hundreds for a one-line edit.
  it('counts one insertion as one change however long the file is', () => {
    const original = lines(100)
    const modified = 'inserted at top\n' + original
    expect(countDiffChunks(original, modified)).toBe(1)
  })

  it('counts separated edits separately', () => {
    const original = lines(60)
    const modified = original
      .replace('line 5', 'line 5 edited')
      .replace('line 50', 'line 50 edited')
    expect(countDiffChunks(original, modified)).toBe(2)
  })

  it('sees a change inside a single line', () => {
    expect(countDiffChunks('const a = 1;', 'const a = 2;')).toBe(1)
  })

  it('sees a pure deletion', () => {
    const original = lines(20)
    const modified = original.split('\n').filter((l) => l !== 'line 10').join('\n')
    expect(countDiffChunks(original, modified)).toBe(1)
  })
})
