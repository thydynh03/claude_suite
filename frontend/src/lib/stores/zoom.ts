import { writable } from 'svelte/store';

// Zoom is applied with the CSS `zoom` property on <html> rather than through
// WebView2's own zoom control (windows.Options.IsZoomControlEnabled).
//
// The native control is one line in main.go and was rejected on purpose: Wails
// exposes no way to read the level back or be told when it changes, so the app
// could neither show what the zoom is nor restore it on the next launch. A
// zoom that silently resets every restart is worse than none, because the user
// has to rediscover it each time.
//
// It is applied to documentElement rather than body so that the app's
// `position: fixed` overlays (the Task Inspector, the confirm dialogs, the
// toasts) are inside the scaled subtree along with everything else — their
// containing block is the viewport, so anything mounted below body is the wrong
// place to scale from.

const STORAGE_KEY = 'zoom';

// The steps Chromium itself uses, trimmed at both ends. A free-running
// multiplier lands on values like 1.0700000000000003 and shows "107%" after two
// scroll clicks; discrete steps keep every stop a round number a user can
// describe. Below 50% the 240px sidebar rail stops being readable and above
// 200% the Kanban columns no longer fit the 1024px minimum width.
export const ZOOM_STEPS = [0.5, 0.67, 0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2] as const;

export const ZOOM_MIN = ZOOM_STEPS[0];
export const ZOOM_MAX = ZOOM_STEPS[ZOOM_STEPS.length - 1];
export const ZOOM_DEFAULT = 1;

/** Nearest allowed step to an arbitrary value — used when reading back storage. */
export function snapZoom(value: number): number {
  if (!Number.isFinite(value)) return ZOOM_DEFAULT;
  let best: number = ZOOM_STEPS[0];
  for (const step of ZOOM_STEPS) {
    if (Math.abs(step - value) < Math.abs(best - value)) best = step;
  }
  return best;
}

/**
 * One step in `direction` from `current`. At either end it returns the end
 * itself, so holding the wheel down at 200% is a no-op rather than an error.
 */
export function stepZoom(current: number, direction: 1 | -1): number {
  const index = ZOOM_STEPS.indexOf(snapZoom(current) as (typeof ZOOM_STEPS)[number]);
  const next = index + direction;
  if (next < 0) return ZOOM_MIN;
  if (next >= ZOOM_STEPS.length) return ZOOM_MAX;
  return ZOOM_STEPS[next];
}

function readStored(): number {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return ZOOM_DEFAULT;
    return snapZoom(Number(raw));
  } catch {
    // Storage can be unavailable; a missing preference is not worth a crash.
    return ZOOM_DEFAULT;
  }
}

export const zoom = writable<number>(readStored());

zoom.subscribe((value) => {
  try {
    localStorage.setItem(STORAGE_KEY, String(value));
  } catch {
    /* nothing to do — the zoom still applies for this session */
  }
  if (typeof document !== 'undefined') {
    // setProperty rather than style.zoom: `zoom` is not in every TS DOM lib,
    // and 1 is written as "1" so the attribute does not accumulate noise.
    document.documentElement.style.setProperty('zoom', String(value));
  }
});

export function zoomIn() {
  zoom.update((v) => stepZoom(v, 1));
}

export function zoomOut() {
  zoom.update((v) => stepZoom(v, -1));
}

export function zoomReset() {
  zoom.set(ZOOM_DEFAULT);
}

/**
 * Ctrl+wheel and Ctrl+plus/minus, registered on window. Returns the cleanup.
 *
 * Ctrl+0 is deliberately NOT the reset: App.svelte already binds Ctrl+0..9 to
 * the navigation tabs, and taking one back would break a shortcut that is
 * already documented. Reset is Ctrl+Shift+0, which that handler ignores because
 * it returns early on shiftKey.
 */
export function attachZoomControls(target: Window = window): () => void {
  const onWheel = (e: WheelEvent) => {
    if (!e.ctrlKey && !e.metaKey) return;
    // Without { passive: false } below, Chromium ignores this and zooms the
    // page itself as well — the browser assumes a wheel listener never cancels.
    e.preventDefault();
    if (e.deltaY === 0) return;
    zoom.update((v) => stepZoom(v, e.deltaY < 0 ? 1 : -1));
  };

  const onKeydown = (e: KeyboardEvent) => {
    if (!e.ctrlKey && !e.metaKey) return;
    if (e.altKey) return;

    // Ctrl+Shift+0 resets. Checked before the shift-less shortcuts so the
    // combination is not swallowed by them.
    if (e.shiftKey) {
      if (e.key === '0' || e.code === 'Digit0') {
        e.preventDefault();
        zoomReset();
      }
      return;
    }

    // "+" needs shift on most layouts, so "=" and the numpad keys are accepted
    // too — binding only "+" left Ctrl+= doing nothing on a US keyboard.
    if (e.key === '+' || e.key === '=' || e.code === 'NumpadAdd') {
      e.preventDefault();
      zoomIn();
      return;
    }
    if (e.key === '-' || e.key === '_' || e.code === 'NumpadSubtract') {
      e.preventDefault();
      zoomOut();
    }
  };

  target.addEventListener('wheel', onWheel as EventListener, { passive: false });
  target.addEventListener('keydown', onKeydown as EventListener);

  return () => {
    target.removeEventListener('wheel', onWheel as EventListener);
    target.removeEventListener('keydown', onKeydown as EventListener);
  };
}
