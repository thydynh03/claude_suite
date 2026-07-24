import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// Custom plugin to strip crossorigin attribute for Wails compatibility
function removeCrossorigin() {
  return {
    name: 'remove-crossorigin',
    transformIndexHtml(html: string) {
      return html.replace(/ crossorigin/g, '')
    }
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    svelte(),
    tailwindcss(),
    removeCrossorigin()
  ],
  server: {
    port: 5173,
    strictPort: true,
    host: '127.0.0.1'
  },
  build: {
    modulePreload: false,
  }
})
