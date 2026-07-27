<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view';
  import { EditorState, Compartment } from '@codemirror/state';
  import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
  import { searchKeymap, highlightSelectionMatches, search } from '@codemirror/search';
  import { bracketMatching, foldGutter, foldKeymap, indentOnInput, syntaxHighlighting, defaultHighlightStyle } from '@codemirror/language';
  import { closeBrackets, closeBracketsKeymap, autocompletion, completionKeymap } from '@codemirror/autocomplete';
  import { oneDark } from '@codemirror/theme-one-dark';
  import { languageFor } from '../../lib/editorLang';

  /**
   * The code editor for Code Studio.
   *
   * It replaced a <textarea> with a hand-drawn line-number column: no syntax
   * highlighting, no folding, no real search, and Tab handled by rewriting the
   * whole string and moving the caret manually. That is why the page read as a
   * text box rather than an IDE.
   */

  export let value = '';
  /** Used only to pick the language — the parent still owns loading and saving. */
  export let path = '';
  export let dark = true;
  export let readOnly = false;
  /** Editor font size in px. The toolbar stepper drives it; it used to be
      hardcoded here, so the stepper changed a number and nothing else. */
  export let fontSize = 13;

  const dispatch = createEventDispatcher<{ change: string; save: void; cursor: { line: number; col: number } }>();

  let host: HTMLDivElement;
  let view: EditorView | null = null;

  const language = new Compartment();
  const theme = new Compartment();
  const editable = new Compartment();
  const metrics = new Compartment();

  function metricsTheme(px: number) {
    return EditorView.theme({
      '&': { height: '100%', fontSize: `${px}px` },
      '.cm-scroller': { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' },
    });
  }

  // Language selection lives in lib/editorLang.ts, shared with DiffView so
  // the same file highlights identically in both panes.

  function buildExtensions() {
    return [
      lineNumbers(),
      highlightActiveLineGutter(),
      highlightActiveLine(),
      foldGutter(),
      history(),
      indentOnInput(),
      bracketMatching(),
      closeBrackets(),
      autocompletion(),
      highlightSelectionMatches(),
      search({ top: true }),
      syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
      keymap.of([
        // Save first so it wins over anything else bound to Ctrl+S.
        {
          key: 'Mod-s',
          preventDefault: true,
          run: () => {
            dispatch('save');
            return true;
          },
        },
        ...closeBracketsKeymap,
        ...defaultKeymap,
        ...searchKeymap,
        ...historyKeymap,
        ...foldKeymap,
        ...completionKeymap,
        // Tab indents rather than moving focus. Placed last so autocomplete and
        // snippet navigation get first refusal on the key.
        indentWithTab,
      ]),
      language.of(languageFor(path)),
      theme.of(dark ? oneDark : []),
      editable.of(EditorView.editable.of(!readOnly)),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          dispatch('change', update.state.doc.toString());
        }
        if (update.selectionSet || update.docChanged) {
          const pos = update.state.selection.main.head;
          const line = update.state.doc.lineAt(pos);
          dispatch('cursor', { line: line.number, col: pos - line.from + 1 });
        }
      }),
      metrics.of(metricsTheme(fontSize)),
    ];
  }

  onMount(() => {
    view = new EditorView({
      state: EditorState.create({ doc: value, extensions: buildExtensions() }),
      parent: host,
    });
  });

  onDestroy(() => {
    view?.destroy();
    view = null;
  });

  // Push external changes in (switching tabs, reloading a file) without
  // disturbing the cursor when the text is already what the editor holds —
  // replacing the document on every keystroke would fight the user's typing.
  $: if (view && value !== view.state.doc.toString()) {
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: value },
    });
  }

  $: if (view) {
    view.dispatch({ effects: language.reconfigure(languageFor(path)) });
  }

  $: if (view) {
    view.dispatch({ effects: theme.reconfigure(dark ? oneDark : []) });
  }

  $: if (view) {
    view.dispatch({ effects: editable.reconfigure(EditorView.editable.of(!readOnly)) });
  }

  $: if (view) {
    view.dispatch({ effects: metrics.reconfigure(metricsTheme(fontSize)) });
  }
</script>

<div bind:this={host} class="h-full w-full overflow-hidden"></div>
