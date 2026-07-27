<script lang="ts">
  import { onDestroy } from 'svelte';
  import { zoom, ZOOM_DEFAULT, zoomReset } from '../../lib/stores/zoom';

  // A zoom that changes with no feedback leaves the user unsure whether the
  // gesture registered or the window merely re-laid-out. The badge says the
  // level, offers the way back, and gets out of the way.
  let visible = false;
  let timer: ReturnType<typeof setTimeout> | null = null;

  // Skip the first run: the store fires on subscribe with the restored value,
  // which would flash a badge on every launch for a zoom nobody just changed.
  let seenInitial = false;

  const unsubscribe = zoom.subscribe(() => {
    if (!seenInitial) {
      seenInitial = true;
      return;
    }
    visible = true;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => (visible = false), 1400);
  });

  onDestroy(() => {
    unsubscribe();
    if (timer) clearTimeout(timer);
  });
</script>

{#if visible}
  <!-- Bottom centre, above every overlay. Not top-right: that is where the
       toasts land, and two things appearing in one corner read as one thing. -->
  <div
    class="fixed bottom-6 left-1/2 -translate-x-1/2 z-[200] flex items-center gap-2
           bg-surface-container-highest border border-outline-variant rounded-xl
           px-3 py-1.5 shadow-lg text-xs text-on-surface"
    role="status"
    aria-live="polite"
  >
    <span class="material-symbols-outlined text-sm text-on-surface-variant">search</span>
    <span class="font-mono font-medium tabular-nums">{Math.round($zoom * 100)}%</span>
    {#if $zoom !== ZOOM_DEFAULT}
      <button
        type="button"
        on:click={zoomReset}
        class="text-on-surface-variant hover:text-on-surface underline underline-offset-2 cursor-pointer"
      >
        Về 100%
      </button>
    {/if}
  </div>
{/if}
