import { get } from 'svelte/store'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  attachZoomControls,
  snapZoom,
  stepZoom,
  zoom,
  zoomIn,
  zoomOut,
  zoomReset,
  ZOOM_MAX,
  ZOOM_MIN,
} from './zoom'

beforeEach(() => {
  zoomReset()
})

describe('zoom steps', () => {
  // A free-running multiplier lands on 1.0700000000000003 and renders "107%".
  it('only ever lands on a round step', () => {
    let v = 1
    for (let i = 0; i < 6; i++) v = stepZoom(v, 1)
    expect(Math.round(v * 100) / 100).toBe(v)
    expect(String(v)).not.toContain('000000')
  })

  it('stops at the ends instead of running away', () => {
    let v = 1
    for (let i = 0; i < 40; i++) v = stepZoom(v, 1)
    expect(v).toBe(ZOOM_MAX)

    for (let i = 0; i < 40; i++) v = stepZoom(v, -1)
    expect(v).toBe(ZOOM_MIN)
  })

  it('snaps a stored value that is not a step', () => {
    expect(snapZoom(1.07)).toBe(1.1)
    expect(snapZoom(99)).toBe(ZOOM_MAX)
    expect(snapZoom(NaN)).toBe(1)
  })

  it('is reversible — in then out returns to where it started', () => {
    zoomIn()
    zoomIn()
    const zoomed = get(zoom)
    zoomOut()
    zoomOut()
    expect(get(zoom)).toBe(1)
    expect(zoomed).toBeGreaterThan(1)
  })
})

describe('applying the zoom', () => {
  it('writes the CSS zoom onto the document root', () => {
    zoomIn()
    expect(document.documentElement.style.getPropertyValue('zoom')).toBe(String(get(zoom)))
  })

  it('persists across a reload', () => {
    zoomIn()
    expect(localStorage.getItem('zoom')).toBe(String(get(zoom)))
  })
})

describe('ctrl+wheel', () => {
  function wheel(target: Window, init: Partial<WheelEvent>) {
    const e = new WheelEvent('wheel', { cancelable: true, ...(init as WheelEventInit) })
    target.dispatchEvent(e)
    return e
  }

  it('zooms in on ctrl+scroll up and out on scroll down', () => {
    const detach = attachZoomControls(window)

    wheel(window, { ctrlKey: true, deltaY: -100 })
    expect(get(zoom)).toBeGreaterThan(1)

    wheel(window, { ctrlKey: true, deltaY: 100 })
    expect(get(zoom)).toBe(1)

    detach()
  })

  // Without preventDefault the WebView zooms the page itself on top of this,
  // which compounds: two zooms for one gesture.
  it('cancels the event so the webview does not also zoom', () => {
    const detach = attachZoomControls(window)
    const e = wheel(window, { ctrlKey: true, deltaY: -100 })
    expect(e.defaultPrevented).toBe(true)
    detach()
  })

  it('leaves a plain scroll alone', () => {
    const detach = attachZoomControls(window)
    const e = wheel(window, { ctrlKey: false, deltaY: -100 })
    expect(e.defaultPrevented).toBe(false)
    expect(get(zoom)).toBe(1)
    detach()
  })

  it('stops listening once detached', () => {
    const detach = attachZoomControls(window)
    detach()
    wheel(window, { ctrlKey: true, deltaY: -100 })
    expect(get(zoom)).toBe(1)
  })
})

describe('keyboard', () => {
  function key(init: KeyboardEventInit) {
    const e = new KeyboardEvent('keydown', { cancelable: true, ...init })
    window.dispatchEvent(e)
    return e
  }

  it('ctrl+= zooms in and ctrl+- zooms out', () => {
    const detach = attachZoomControls(window)

    key({ ctrlKey: true, key: '=' })
    expect(get(zoom)).toBeGreaterThan(1)

    key({ ctrlKey: true, key: '-' })
    expect(get(zoom)).toBe(1)

    detach()
  })

  // App.svelte binds Ctrl+0..9 to the navigation tabs. Taking Ctrl+0 for the
  // zoom reset would break a shortcut that already exists and is documented.
  it('does not touch ctrl+0, which belongs to the tab shortcuts', () => {
    const detach = attachZoomControls(window)
    zoomIn()
    const before = get(zoom)

    const e = key({ ctrlKey: true, key: '0' })

    expect(e.defaultPrevented).toBe(false)
    expect(get(zoom)).toBe(before)
    detach()
  })

  it('ctrl+shift+0 resets', () => {
    const detach = attachZoomControls(window)
    zoomIn()
    expect(get(zoom)).not.toBe(1)

    key({ ctrlKey: true, shiftKey: true, key: '0' })
    expect(get(zoom)).toBe(1)

    detach()
  })

  it('ignores the shortcut keys without ctrl', () => {
    const detach = attachZoomControls(window)
    key({ key: '=' })
    key({ key: '-' })
    expect(get(zoom)).toBe(1)
    detach()
  })
})
