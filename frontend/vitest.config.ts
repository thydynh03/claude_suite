import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Kept separate from vite.config.ts so the production build is untouched by test
// tooling: that config also carries the Wails crossorigin workaround and the
// Three.js chunking, neither of which should differ between build and test.
export default defineConfig({
  plugins: [svelte({ hot: false })],
  test: {
    // jsdom is only needed by the component tests; the store and utility tests
    // are plain TypeScript and would run without it.
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,js}'],
    restoreMocks: true,
  },
  resolve: {
    // Component tests import the browser build of Svelte, not the SSR one.
    conditions: ['browser'],
  },
})
