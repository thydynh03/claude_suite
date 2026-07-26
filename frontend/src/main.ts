import { mount } from 'svelte'
// Fonts and icons ship inside the bundle. Loading them from Google Fonts made
// every icon render as its ligature text ("account_tree", "psychology") on any
// machine that was offline — or merely slow — when the app started.
import '@fontsource/geist-sans/400.css'
import '@fontsource/geist-sans/500.css'
import '@fontsource/geist-sans/600.css'
import '@fontsource/geist-sans/700.css'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/500.css'
import 'material-symbols/outlined.css'
import './app.css'
import App from './App.svelte'

const app = mount(App, {
  target: document.getElementById('app')!
})

export default app
