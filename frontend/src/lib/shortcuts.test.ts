import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { allShortcuts, SHORTCUT_GROUPS } from './shortcuts'

const src = (rel: string) => readFileSync(join(__dirname, '..', rel), 'utf8')

describe('the shortcut sheet', () => {
  it('says which file binds each key, and that file exists', () => {
    for (const s of allShortcuts()) {
      expect(s.boundIn, `${s.keys.join('+')} has no source`).toBeTruthy()
      expect(() => src(s.boundIn), `${s.boundIn} (for ${s.keys.join('+')})`).not.toThrow()
    }
  })

  it('has both languages for every entry', () => {
    for (const s of allShortcuts()) {
      expect(s.vi.length, `${s.keys.join('+')} vi`).toBeGreaterThan(0)
      expect(s.en.length, `${s.keys.join('+')} en`).toBeGreaterThan(0)
    }
    for (const g of SHORTCUT_GROUPS) {
      expect(g.vi.length).toBeGreaterThan(0)
      expect(g.en.length).toBeGreaterThan(0)
    }
  })

  it('lists no key twice', () => {
    const seen = new Set<string>()
    for (const s of allShortcuts()) {
      const key = s.keys.join('+')
      expect(seen.has(key), `${key} is listed more than once`).toBe(false)
      seen.add(key)
    }
  })
})

// The previous version of this information was three lists that disagreed. The
// File menu advertised Ctrl+Shift+O and Ctrl+Shift+P, which were bound nowhere,
// and the View menu called its reset Ctrl+0 while Ctrl+0 opened the tenth tab.
// These check the claims that can be checked against the source mechanically.
describe('the sheet matches what the code binds', () => {
  it('Ctrl+K really is the command palette', () => {
    const palette = src('components/ui/CommandPalette.svelte')
    expect(palette).toContain("e.key.toLowerCase() === 'k'")
  })

  it('the numeric tab shortcuts really are bound in App.svelte', () => {
    const app = src('App.svelte')
    expect(app).toContain('TAB_SHORTCUTS')
    expect(app).toMatch(/\/\^\[0-9\]\$\/\.test\(e\.key\)/)
  })

  it('the zoom keys really are bound in the zoom store', () => {
    const zoomSrc = src('lib/stores/zoom.ts')
    expect(zoomSrc).toContain("e.key === '+'")
    expect(zoomSrc).toContain("e.key === '-'")
    expect(zoomSrc).toContain('e.shiftKey')
  })

  // The one that already went wrong once, in the opposite direction.
  it('does not claim Ctrl+0 resets the zoom', () => {
    const reset = allShortcuts().find((s) => s.vi.includes('Về 100%'))
    expect(reset?.keys).toEqual(['Ctrl', 'Shift', '0'])

    const ctrl0 = allShortcuts().find((s) => s.keys.join('+') === 'Ctrl+0')
    expect(ctrl0, 'Ctrl+0 should still be documented as a tab shortcut').toBeTruthy()
    expect(ctrl0!.en.toLowerCase()).not.toContain('zoom')
  })

  it('F1 opens the sheet, and the sheet is what binds it', () => {
    const sheet = src('components/ui/ShortcutSheet.svelte')
    expect(sheet).toContain("e.key === 'F1'")
    const f1 = allShortcuts().find((s) => s.keys.join('+') === 'F1')
    expect(f1?.boundIn).toBe('components/ui/ShortcutSheet.svelte')
  })
})

// A Help menu whose entries lead nowhere is worse than none: it is the first
// place a stuck user looks.
describe('the Help menu', () => {
  const nav = () => src('components/layout/TopNavBar.svelte')

  it('exists and offers the guide, the shortcuts, the docs and support', () => {
    const bar = nav()
    expect(bar).toContain("toggleMenu('help')")
    expect(bar).toContain("openHelp('guide')")
    expect(bar).toContain("openHelp('shortcuts')")
    expect(bar).toContain("goTab('docs')")
    expect(bar).toContain("goTab('support')")
  })

  // Two zoom systems that multiplied: the View menu kept its own level and
  // wrote it to body.style.zoom, while Ctrl+wheel scales documentElement — and
  // body sits inside it, so 150% here on top of 200% there was 300%.
  //
  // Comments are stripped first. The first version of this matched the comment
  // explaining the fix and failed on the fixed file, which is the same class of
  // mistake as the bug: reading text that looks like the thing instead of the
  // thing.
  it('does not keep a second zoom of its own', () => {
    const code = nav().replace(/\/\/[^\n]*/g, '')
    expect(code).not.toMatch(/body\.style\.zoom\s*=/)
    expect(code).not.toMatch(/\blet\s+zoomLevel\b/)
    expect(code).toContain("from '../../lib/stores/zoom'")
  })
})
