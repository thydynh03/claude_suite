<script lang="ts">
  import { toasts, dismissToast } from '../../lib/stores/appState';

  const styles: Record<string, { icon: string; cls: string }> = {
    SUCCESS: { icon: 'check_circle', cls: 'border-emerald-500/40 text-emerald-500' },
    ERROR: { icon: 'error', cls: 'border-rose-500/40 text-rose-500' },
    WARN: { icon: 'warning', cls: 'border-amber-500/40 text-amber-500' },
    INFO: { icon: 'info', cls: 'border-primary/40 text-primary' },
  };
  const styleFor = (lvl: string) => styles[lvl] || styles.INFO;
</script>

<div class="fixed bottom-5 right-5 z-[200] flex flex-col gap-2 w-[340px] max-w-[90vw]">
  {#each $toasts as t (t.id)}
    <div
      class="flex items-start gap-3 bg-surface border {styleFor(t.level).cls} rounded-xl shadow-2xl p-3 animate-in slide-in-from-right-4 fade-in duration-200"
    >
      <span class="material-symbols-outlined text-lg {styleFor(t.level).cls}">{styleFor(t.level).icon}</span>
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
