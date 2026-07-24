import { writable } from 'svelte/store';

const initialTheme = localStorage.getItem('theme') || 'light';
export const theme = writable<string>(initialTheme);

theme.subscribe((val) => {
  localStorage.setItem('theme', val);
  if (typeof document !== 'undefined') {
    if (val === 'dark') {
      document.documentElement.classList.add('dark');
      document.documentElement.classList.remove('light');
    } else {
      document.documentElement.classList.remove('dark');
      document.documentElement.classList.add('light');
    }
  }
});

export function toggleTheme() {
  theme.update((t) => (t === 'light' ? 'dark' : 'light'));
}
