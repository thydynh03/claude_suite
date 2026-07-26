<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { MergeView } from '@codemirror/merge';
  import { EditorView, lineNumbers } from '@codemirror/view';
  import { EditorState } from '@codemirror/state';
  import { syntaxHighlighting, defaultHighlightStyle } from '@codemirror/language';
  import { oneDark } from '@codemirror/theme-one-dark';
  import { languageFor } from '../../lib/editorLang';

  /**
   * The side-by-side diff for Code Studio.
   *
   * It replaced two hand-rendered columns that compared line i against line i:
   * one inserted line shifted everything below it, so identical code showed as
   * "changed" while the actual edit drowned un-highlighted, and the panes had
   * no syntax colouring at all. CodeMirror's MergeView aligns real chunks,
   * highlights them down to the character, and runs the same language + theme
   * extensions as the editor next door.
   */

  export let original = '';
  export let modified = '';
  /** Used only to pick the language, exactly like CodeEditor. */
  export let path = '';
  export let dark = true;

  let host: HTMLDivElement;
  let view: MergeView | null = null;

  // CodeMirror stores documents with \n regardless of input, so raw CRLF
  // props (this is a Windows app; most workspace files are CRLF) can never
  // compare equal to a pane's contents — an equality-guarded sync loops
  // forever on them. Normalising at the boundary keeps every comparison in
  // one universe.
  $: normOriginal = original.replace(/\r\n/g, '\n');
  $: normModified = modified.replace(/\r\n/g, '\n');

  function paneExtensions() {
    return [
      lineNumbers(),
      EditorView.editable.of(false),
      EditorState.readOnly.of(true),
      EditorView.lineWrapping,
      syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
      languageFor(path),
      ...(dark ? [oneDark] : []),
      EditorView.theme({
        '.cm-scroller': { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: '13px' },
      }),
    ];
  }

  // The view is rebuilt whenever ANY input changes, not patched: this pane is
  // a read-only viewer that changes docs at most once per tab switch or AI
  // turn, and dispatching a whole-document replacement into a live MergeView
  // silently destroyed every collapseUnchanged fold — the field computes its
  // ranges only at state creation and never re-adds them.
  let builtOriginal = '';
  let builtModified = '';
  let builtPath = '';
  let builtDark = true;

  function build() {
    if (!host) return;
    builtOriginal = normOriginal;
    builtModified = normModified;
    builtPath = path;
    builtDark = dark;
    view?.destroy();
    view = new MergeView({
      a: { doc: normOriginal, extensions: paneExtensions() },
      b: { doc: normModified, extensions: paneExtensions() },
      parent: host,
      // Chunk backgrounds plus per-character emphasis inside each chunk.
      highlightChanges: true,
      gutter: true,
      // Long files are mostly unchanged; fold the quiet stretches behind an
      // expandable bar so the edits are what the eye lands on.
      collapseUnchanged: { margin: 3, minSize: 6 },
    });
  }

  onMount(() => {
    build();
  });

  onDestroy(() => {
    view?.destroy();
    view = null;
  });

  $: if (
    view &&
    host &&
    (normOriginal !== builtOriginal || normModified !== builtModified || path !== builtPath || dark !== builtDark)
  ) {
    build();
  }
</script>

<div bind:this={host} class="h-full w-full overflow-hidden diff-host"></div>

<style>
  /* MergeView is its own single scroll container: the package forces the
     editors inside it to height:auto !important, so the ONLY correct sizing
     is a height on .cm-mergeView itself. Pinning the inner wrappers to 100%
     (the obvious-looking move) clips the auto-height editors and leaves a
     long diff unscrollable. */
  .diff-host :global(.cm-mergeView) {
    height: 100%;
  }
</style>
