import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/svelte'
import { afterEach } from 'vitest'

// jsdom has no Web Animations API. Svelte's animate:/transition: directives
// call element.getAnimations() and element.animate() whenever a keyed list
// item moves; without these stubs that throws mid-flush, the rest of the DOM
// patch is abandoned, and a test then asserts against a half-updated board —
// which looks exactly like a reactivity bug in the component.
if (!Element.prototype.getAnimations) {
  Element.prototype.getAnimations = () => []
}
if (!Element.prototype.animate) {
  Element.prototype.animate = function () {
    const anim = {
      cancel() {},
      finish() {},
      finished: Promise.resolve(),
    } as Record<string, unknown>
    // Svelte assigns onfinish and waits for it; fire it on the next tick so
    // the "animation" completes instead of leaving the element in flight.
    Object.defineProperty(anim, 'onfinish', {
      set(fn: () => void) {
        setTimeout(fn, 0)
      },
      get() {
        return null
      },
    })
    return anim as unknown as Animation
  }
}


// Components mounted by a test are torn down before the next one runs, so a
// leftover component cannot make an unrelated test pass or fail.
afterEach(() => {
  cleanup()
})
