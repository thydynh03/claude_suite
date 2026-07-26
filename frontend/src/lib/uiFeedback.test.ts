import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';

/**
 * Guards the defect that made six separate features look broken: a button calls
 * the backend, the backend succeeds, and the only acknowledgement goes to
 * addLog() — the Live Log panel, which sits on another tab and is usually
 * collapsed. From where the user is looking, nothing happened.
 *
 * The rule this enforces is narrow on purpose: a function that both calls a
 * binding AND reports with addLog must also report with addToast. It does not
 * demand a toast from every backend call — plenty are page loads and refreshes
 * whose result is the rendered data itself.
 */

function svelteFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) return svelteFiles(full);
    return full.endsWith('.svelte') ? [full] : [];
  });
}

/** Function bodies, matched by brace balance from each `function` keyword. */
function functionBodies(source: string): { name: string; body: string }[] {
  const out: { name: string; body: string }[] = [];
  const re = /function\s+([A-Za-z0-9_$]+)\s*\([^)]*\)\s*\{/g;

  let match: RegExpExecArray | null;
  while ((match = re.exec(source)) !== null) {
    let depth = 1;
    let i = re.lastIndex;
    while (i < source.length && depth > 0) {
      if (source[i] === '{') depth++;
      else if (source[i] === '}') depth--;
      i++;
    }
    out.push({ name: match[1], body: source.slice(re.lastIndex, i) });
  }
  return out;
}

describe('user-visible feedback', () => {
  it('never acknowledges a backend call with addLog alone', () => {
    const offenders: string[] = [];

    for (const file of svelteFiles(join(process.cwd(), 'src', 'components'))) {
      const source = readFileSync(file, 'utf8');
      if (!source.includes('AppBindings')) continue;

      for (const fn of functionBodies(source)) {
        const callsBinding = /AppBindings[.\s)\]]*\s*(as any\))?\s*[.[]/.test(fn.body);
        const logs = fn.body.includes('addLog(');
        const toasts = fn.body.includes('addToast(');

        if (callsBinding && logs && !toasts) {
          offenders.push(`${file.split(/[\/]/).slice(-2).join('/')}:${fn.name}`);
        }
      }
    }

    expect(
      offenders,
      'These call the backend and report only into the Live Log, so the action ' +
        'looks like it did nothing. Add addToast() beside the addLog().'
    ).toEqual([]);
  });
});
