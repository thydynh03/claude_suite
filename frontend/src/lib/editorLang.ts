import { javascript } from '@codemirror/lang-javascript'
import { json } from '@codemirror/lang-json'
import { go } from '@codemirror/lang-go'
import { html } from '@codemirror/lang-html'
import { css } from '@codemirror/lang-css'
import { markdown } from '@codemirror/lang-markdown'
import { python } from '@codemirror/lang-python'
import type { Extension } from '@codemirror/state'

// One place to map a file path to a CodeMirror language. The editor and the
// diff view must agree on this, or the same file highlights differently
// depending on which pane it is in.
export function languageFor(p: string): Extension {
  const ext = (p.split('.').pop() || '').toLowerCase()
  switch (ext) {
    case 'ts':
    case 'tsx':
      return javascript({ typescript: true, jsx: ext === 'tsx' })
    case 'js':
    case 'jsx':
    case 'mjs':
    case 'cjs':
      return javascript({ jsx: ext === 'jsx' })
    case 'json':
      return json()
    case 'go':
      return go()
    // Svelte has no dedicated package here; its markup is close enough to HTML
    // that highlighting it as HTML is better than not highlighting it at all.
    case 'html':
    case 'svelte':
    case 'vue':
      return html()
    case 'css':
    case 'scss':
      return css()
    case 'md':
    case 'markdown':
      return markdown()
    case 'py':
      return python()
    default:
      return []
  }
}
