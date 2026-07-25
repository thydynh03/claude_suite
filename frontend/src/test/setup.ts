import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/svelte'
import { afterEach } from 'vitest'

// Components mounted by a test are torn down before the next one runs, so a
// leftover component cannot make an unrelated test pass or fail.
afterEach(() => {
  cleanup()
})
