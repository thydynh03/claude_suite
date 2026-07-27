<script lang="ts">
  import { fade } from 'svelte/transition';
  import { toasts, dismissToast } from '../../lib/stores/appState';

  const styles: Record<string, { icon: string; cls: string }> = {
    SUCCESS: { icon: 'check_circle', cls: 'border-success/40 text-success' },
    ERROR: { icon: 'error', cls: 'border-error/40 text-error' },
    WARN: { icon: 'warning', cls: 'border-warning/40 text-warning' },
    INFO: { icon: 'info', cls: 'border-primary/40 text-primary' },
  };
  const styleFor = (lvl: string) => styles[lvl] || styles.INFO;
</script>

<div class="fixed bottom-5 right-5 z-[200] flex flex-col gap-2 w-[340px] max-w-[90vw]">
  {#each $toasts as t (t.id)}
    <!-- The entry animation used to be `animate-in slide-in-from-right-4
         fade-in`, classes from a plugin this project does not install: they
         compiled to nothing, so the toast simply popped in. -->
    <div
      class="flex items-start gap-3 bg-surface border {styleFor(t.level).cls} rounded-xl shadow-lg p-3"
      transition:fade={{ duration: 150 }}
    >
      <span class="material-symbols-outlined text-base {styleFor(t.level).cls}">{styleFor(t.level).icon}</span>
      <p class="flex-1 text-xs text-on-surface leading-relaxed break-words">{t.message}</p>
      <button
        type="button"
        on:click={() => dismissToast(t.id)}
        class="text-on-surface-variant hover:text-on-surface cursor-pointer">
        <span class="material-symbols-outlined text-base">close</span>
      </button>
    </div>
  {/each}
</div>
