import { describe, expect, it } from 'vitest'

// Svelte reads a function call in the markup *untracked*: `{#each rows() as r}`
// renders once and then never again, however reactive the state inside `rows`
// is. Nothing warns — the page just quietly stops following its own data, which
// is how the Cockpit metrics and the Code Studio diff both froze while the rest
// of the screen kept moving. Derive the value with `$:` and iterate over that.
const OFFENDER = /\{#(?:each|if)\s+[A-Za-z_$][\w$]*\(\)/

// Sourced through Vite rather than fs so the check needs no node typings.
const components = import.meta.glob('/src/**/*.svelte', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

// Only the markup can carry the mistake; prose about it in a script block or an
// HTML comment must not fail the build.
function markupOf(source: string): string {
  const lastScript = source.lastIndexOf('</script>')
  let markup = lastScript === -1 ? source : source.slice(lastScript + '</script>'.length)
  // To a fixpoint, not one pass: removing `<!-- -->` can splice a new `<!--`
  // together out of the surrounding text, and the comment it opens would then
  // hide (or expose) markup this check is supposed to see.
  // (CodeQL js/incomplete-multi-character-sanitization)
  let before: string
  do {
    before = markup
    markup = markup.replace(/<!--[\s\S]*?-->/g, '')
  } while (markup !== before)
  return markup
}

describe('no untracked function calls drive the markup', () => {
  const files = Object.keys(components)

  it('finds components to check', () => {
    expect(files.length).toBeGreaterThan(0)
  })

  it('recognises the shape it is meant to catch', () => {
    expect(markupOf('</script>{#each metricCards() as m}')).toMatch(OFFENDER)
    expect(markupOf('</script>{#each metricCards as m}')).not.toMatch(OFFENDER)
    expect(markupOf('<script>// {#each rows() as r}</script>')).not.toMatch(OFFENDER)
    // A comment spliced together by the first removal pass is still a comment:
    // one-pass stripping would leave `<!--{#each rows() as r}-->` behind and
    // flag its contents as live markup.
    expect(markupOf('</script><!<!-- x -->--{#each rows() as r}-->')).not.toMatch(OFFENDER)
  })

  it.each(files)('%s', (file) => {
    const offending = markupOf(components[file])
      .split('\n')
      .map((line, i) => ({ line: line.trim(), no: i + 1 }))
      .filter(({ line }) => OFFENDER.test(line))

    expect(
      offending.map(({ line, no }) => `line ${no}: ${line}`),
      'a block driven by a function call never re-renders; derive it with `$:` instead',
    ).toEqual([])
  })
})
