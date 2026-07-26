<script lang="ts">
  // Test stand-in for the CodeMirror editor: jsdom lacks the layout APIs
  // CodeMirror measures with, and the component tests assert the page around
  // the editor, not the editor itself. A plain textarea keeps the value and
  // change event observable.
  import { createEventDispatcher } from 'svelte';
  export let value = '';
  export let path = '';
  export let dark = true;
  const dispatch = createEventDispatcher();
  function onInput(e: Event) {
    dispatch('change', (e.currentTarget as HTMLTextAreaElement).value);
  }
</script>

<textarea data-testid="editor-stub" data-path={path} data-dark={dark} {value} on:input={onInput}></textarea>
