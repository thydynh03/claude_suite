<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { helpOpen } from '../../lib/stores/appState';
  import { currentLang } from '../../lib/stores/i18n';
  import { SHORTCUT_GROUPS } from '../../lib/shortcuts';

  $: vi = $currentLang !== 'en';

  function close() {
    helpOpen.set(false);
  }

  function onKeydown(e: KeyboardEvent) {
    // F1 opens it from anywhere, and closes it again — a help key that only
    // works one way leaves the user hunting for the way out.
    if (e.key === 'F1') {
      e.preventDefault();
      helpOpen.update((v) => !v);
      return;
    }
    if (e.key === 'Escape' && $helpOpen) {
      e.preventDefault();
      close();
    }
  }

  onMount(() => window.addEventListener('keydown', onKeydown));
  onDestroy(() => window.removeEventListener('keydown', onKeydown));
</script>

{#if $helpOpen}
  <div class="fixed inset-0 z-[150] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
    <!-- tabindex="-1": a full-viewport invisible button is otherwise the first
         tab stop inside the dialog, and Enter on it silently dismisses. -->
    <button type="button" tabindex="-1" aria-hidden="true" class="absolute inset-0 cursor-default" on:click={close}></button>

    <div
      class="relative w-full max-w-2xl max-h-[85vh] flex flex-col bg-surface border border-outline-variant rounded-xl shadow-lg"
      role="dialog"
      aria-modal="true"
      aria-label={vi ? 'Phím tắt' : 'Keyboard shortcuts'}
    >
      <div class="flex items-start justify-between p-5 pb-3 border-b border-outline-variant">
        <div>
          <h2 class="text-sm font-semibold text-on-surface flex items-center gap-2">
            <span class="material-symbols-outlined text-base text-on-surface-variant">keyboard</span>
            {vi ? 'Phím tắt' : 'Keyboard shortcuts'}
          </h2>
          <p class="text-xs text-on-surface-variant mt-1">
            {vi
              ? 'Chỉ liệt kê những phím thực sự được gán. Nếu một phím có ở đây, nó chạy.'
              : 'Only keys that are actually bound. If it is listed here, it works.'}
          </p>
        </div>
        <button
          type="button"
          on:click={close}
          aria-label={vi ? 'Đóng' : 'Close'}
          class="text-on-surface-variant hover:text-on-surface p-1 rounded-lg hover:bg-surface-container-high transition-colors cursor-pointer"
        >
          <span class="material-symbols-outlined text-base">close</span>
        </button>
      </div>

      <div class="flex-1 overflow-y-auto p-5 pt-4 space-y-5">
        {#each SHORTCUT_GROUPS as group (group.en)}
          <section class="space-y-1.5">
            <h3 class="text-xs font-semibold text-on-surface">{vi ? group.vi : group.en}</h3>
            <ul class="divide-y divide-outline-variant/60 border border-outline-variant rounded-lg overflow-hidden">
              {#each group.items as item (item.keys.join('+') + item.en)}
                <li class="flex items-start justify-between gap-4 px-3 py-2 bg-surface-container-lowest">
                  <span class="text-xs text-on-surface-variant leading-relaxed">{vi ? item.vi : item.en}</span>
                  <span class="flex items-center gap-1 shrink-0 pt-0.5">
                    {#each item.keys as k}
                      <kbd
                        class="px-1.5 py-0.5 rounded border border-outline-variant bg-surface-container-high
                               text-[11px] font-mono text-on-surface whitespace-nowrap"
                        >{k}</kbd
                      >
                    {/each}
                  </span>
                </li>
              {/each}
            </ul>
          </section>
        {/each}
      </div>

      <div class="px-5 py-3 border-t border-outline-variant text-xs text-on-surface-variant">
        {vi
          ? 'Chưa quen? Help → Hướng dẫn bắt đầu sẽ dẫn bạn đi một vòng.'
          : 'New here? Help → Getting started walks you through it.'}
      </div>
    </div>
  </div>
{/if}
