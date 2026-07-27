<script lang="ts">
  import { onMount } from 'svelte';
  import { activeTab, orchestratorRunning, addToast, onboardingOpen, commandPaletteOpen } from '../../lib/stores/appState';
  import * as AppBindings from '../../../wailsjs/go/main/App';

  let open = false;
  let query = '';
  let inputEl: HTMLInputElement | null = null;
  let selected = 0;

  type Cmd = { label: string; icon: string; run: () => void; keywords?: string };

  const go = (tab: string) => { activeTab.set(tab); close(); };

  const commands: Cmd[] = [
    { label: 'Đi tới: Cockpit', icon: 'rocket_launch', run: () => go('cockpit'), keywords: 'home dashboard' },
    { label: 'Đi tới: Task Board (Kanban)', icon: 'view_kanban', run: () => go('kanban'), keywords: 'task board' },
    { label: 'Đi tới: Code Studio', icon: 'code', run: () => go('editor') },
    { label: 'Đi tới: Virtual Office 3D', icon: 'meeting_room', run: () => go('office') },
    { label: 'Đi tới: Scheduler', icon: 'schedule', run: () => go('scheduler') },
    { label: 'Đi tới: Browser Agent', icon: 'public', run: () => go('browser') },
    { label: 'Đi tới: Source Control (Git)', icon: 'account_tree', run: () => go('git'), keywords: 'git commit source control' },
    { label: 'Đi tới: Memory & Project Map', icon: 'psychology', run: () => go('memory'), keywords: 'memory lessons map regressions bug' },
    { label: 'Đi tới: Settings', icon: 'settings', run: () => go('settings') },
    { label: 'Orchestrator: Bắt đầu', icon: 'play_arrow', run: async () => { await (AppBindings as any).StartOrchestrator(); orchestratorRunning.set(true); addToast('Orchestrator đã bắt đầu.', 'SUCCESS'); close(); }, keywords: 'start run' },
    { label: 'Orchestrator: Dừng', icon: 'stop', run: async () => { await (AppBindings as any).StopOrchestrator(); orchestratorRunning.set(false); addToast('Orchestrator đã dừng.', 'WARN'); close(); }, keywords: 'stop pause' },
    { label: 'Xuất báo cáo Kanban (Markdown)', icon: 'description', run: async () => { const f = await (AppBindings as any).ExportKanbanReport(); addToast('Đã xuất báo cáo: ' + f, 'SUCCESS'); close(); }, keywords: 'export report' },
    { label: 'Chọn thư mục Workspace', icon: 'folder_open', run: async () => { await (AppBindings as any).SelectWorkspaceFolder(); close(); }, keywords: 'workspace folder open' },
    { label: 'Xem lại hướng dẫn sử dụng', icon: 'auto_awesome', run: () => { onboardingOpen.set(true); close(); }, keywords: 'onboarding tour huong dan help guide' },
  ];

  $: filtered = commands.filter((c) => {
    const q = query.toLowerCase().trim();
    if (!q) return true;
    return (c.label + ' ' + (c.keywords || '')).toLowerCase().includes(q);
  });
  $: if (selected >= filtered.length) selected = 0;

  function openPalette() {
    open = true;
    query = '';
    selected = 0;
    setTimeout(() => inputEl?.focus(), 0);
  }
  function close() { open = false; commandPaletteOpen.set(false); }

  // The menu bar asks for the palette through this store rather than reaching
  // into the component.
  $: if ($commandPaletteOpen && !open) openPalette();

  function onKeydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      open ? close() : openPalette();
      return;
    }
    if (!open) return;
    if (e.key === 'Escape') { close(); return; }
    if (e.key === 'ArrowDown') { e.preventDefault(); selected = Math.min(selected + 1, filtered.length - 1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); selected = Math.max(selected - 1, 0); }
    else if (e.key === 'Enter') { e.preventDefault(); filtered[selected]?.run(); }
  }

  onMount(() => {
    window.addEventListener('keydown', onKeydown);
    return () => window.removeEventListener('keydown', onKeydown);
  });
</script>

{#if open}
<div class="fixed inset-0 z-[300] flex items-start justify-center pt-[15vh] bg-black/50 backdrop-blur-sm" on:click|self={close} role="presentation">
  <div class="bg-surface border border-outline-variant rounded-xl shadow-lg w-[560px] max-w-[92vw] overflow-hidden" role="dialog" aria-modal="true" tabindex="-1">
    <div class="flex items-center gap-2 px-4 py-3 border-b border-outline-variant">
      <span class="material-symbols-outlined text-base text-on-surface-variant">search</span>
      <input
        bind:this={inputEl}
        bind:value={query}
        placeholder="Gõ lệnh... (Ctrl+K để mở/đóng, ↑↓ chọn, Enter chạy)"
        class="flex-1 bg-transparent text-sm text-on-surface placeholder:text-on-surface-variant"
      />
      <kbd class="text-[10px] font-mono bg-surface-container-high text-on-surface-variant px-1.5 py-0.5 rounded-lg border border-outline-variant">ESC</kbd>
    </div>
    <div class="max-h-[50vh] overflow-y-auto py-1">
      {#each filtered as c, i}
        <button
          type="button"
          on:click={c.run}
          on:mouseenter={() => selected = i}
          class="w-full flex items-center gap-3 px-4 py-2.5 text-left text-sm transition-colors cursor-pointer
          {i === selected ? 'bg-primary/10 text-primary' : 'text-on-surface hover:bg-surface-container-high'}"
        >
          <span class="material-symbols-outlined text-base">{c.icon}</span>
          <span class="flex-1">{c.label}</span>
        </button>
      {:else}
        <div class="px-4 py-6 text-center text-xs text-on-surface-variant">Không có lệnh nào khớp.</div>
      {/each}
    </div>
  </div>
</div>
{/if}
