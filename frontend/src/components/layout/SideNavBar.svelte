<script lang="ts">
  import { onMount } from 'svelte';
  import { activeTab, orchestratorRunning, sidebarCollapsed } from '../../lib/stores/appState';
  import { t } from '../../lib/stores/i18n';
  import * as AppBindings from '../../../wailsjs/go/main/App';

  // The real version, from the backend. A hardcoded label here drifted to
  // "v4.8" while the app shipped v2.x — worse than showing nothing.
  let appVersion = '';
  onMount(async () => {
    try {
      appVersion = await AppBindings.GetAppVersion();
    } catch {
      appVersion = '';
    }
  });

  // Ctrl+1..9 across the whole list, in the order shown. Announced in each
  // button's tooltip so the shortcut is discoverable rather than documented.
  const navGroups = [
    {
      title: 'Làm việc',
      items: [
        { id: 'cockpit', label: $t('cockpit'), title: 'AI Cockpit', icon: 'rocket_launch', shortcut: 'Ctrl+1' },
        { id: 'kanban', label: $t('kanban'), title: 'Task Board & Planning', icon: 'assignment', shortcut: 'Ctrl+2' },
        { id: 'editor', label: $t('code_studio'), title: 'Agent IDE & Code Studio', icon: 'code', shortcut: 'Ctrl+3' },
        { id: 'git', label: 'Source Control', title: 'Source Control', icon: 'account_tree', shortcut: 'Ctrl+4' },
      ],
    },
    {
      title: 'Tự động hoá',
      items: [
        { id: 'scheduler', label: 'Scheduler', title: 'Scheduler & Jobs', icon: 'schedule', shortcut: 'Ctrl+5' },
        { id: 'browser', label: 'Browser Agent', title: 'Browser Agent', icon: 'travel_explore', shortcut: 'Ctrl+6' },
        { id: 'claims', label: 'Phân xử', title: 'Claims & Adjudication', icon: 'gavel', shortcut: 'Ctrl+7' },
      ],
    },
    {
      title: 'Giám sát',
      items: [
        { id: 'office', label: 'Virtual Office', title: 'Virtual Office 3D', icon: 'meeting_room', shortcut: 'Ctrl+8' },
        // No shortcut: all ten Ctrl+digit slots are taken, and renumbering
        // them would break everyone's muscle memory (see App.svelte).
        { id: 'memory', label: 'Memory', title: 'Memory & Project Map', icon: 'psychology', shortcut: '' },
      ],
    },
    {
      title: 'Cấu hình',
      items: [
        { id: 'roles', label: 'Agent Roles', title: 'Agent Roles', icon: 'group', shortcut: 'Ctrl+9' },
        { id: 'settings', label: $t('settings'), title: 'Studio & Settings', icon: 'settings', shortcut: 'Ctrl+0' },
      ],
    },
  ];

  async function handleExecutePlan() {
    // StartOrchestrator() returns whether THIS call started it — false also
    // means "already running". Either way the orchestrator is running now;
    // writing the return value into the store flipped every indicator to
    // 'stopped' while the pool kept dispatching, and made the TopNavBar
    // toggle call Start again instead of Stop.
    await AppBindings.StartOrchestrator();
    orchestratorRunning.set(true);
  }
</script>

<aside class="fixed left-0 top-[60px] h-[calc(100vh-60px)] {$sidebarCollapsed ? 'w-[72px]' : 'w-[240px]'} flex flex-col py-4 bg-surface-container-low border-r border-outline-variant z-40 transition-all duration-300">
  <!-- Brand Header & Collapse Toggle -->
  <div class="px-4 mb-4 flex items-center justify-between">
    <div class="flex items-center gap-2 overflow-hidden">
      {#if !$sidebarCollapsed}
        <div class="truncate">
          <h2 class="font-semibold text-on-surface leading-tight text-sm truncate">Agent Center</h2>
          {#if appVersion}
            <p class="text-[10px] text-on-surface-variant">{appVersion}</p>
          {/if}
        </div>
      {/if}
    </div>
    <button
      type="button"
      on:click|preventDefault={() => sidebarCollapsed.update(c => !c)}
      class="p-1 rounded-lg hover:bg-surface-container-high text-on-surface-variant transition-colors flex items-center justify-center cursor-pointer"
      title={$sidebarCollapsed ? 'Mở rộng sidebar' : 'Thu gọn sidebar'}
    >
      <span class="material-symbols-outlined text-base">{$sidebarCollapsed ? 'chevron_right' : 'chevron_left'}</span>
    </button>
  </div>

  <!-- Navigation Items
       Grouped by what the user is doing, because a flat list of eleven items put
       Settings fourth — between Code Studio and the 3D office — and left Agent
       Roles at the bottom, far from the Settings page that configures the same
       agents. Order within a group is by how often it is opened. -->
  <nav class="flex-1 overflow-y-auto">
    {#each navGroups as group}
      {#if !$sidebarCollapsed}
        <p class="px-5 pt-3 pb-1 text-[10px] font-medium text-outline">
          {group.title}
        </p>
      {:else}
        <div class="mx-4 my-2 border-t border-outline-variant/40"></div>
      {/if}

      {#each group.items as item}
        <button
          type="button"
          on:click|preventDefault={() => activeTab.set(item.id)}
          title={item.shortcut ? `${item.title} (${item.shortcut})` : item.title}
          class="w-[calc(100%-16px)] mx-2 my-0.5 flex items-center px-2.5 py-2 transition-colors rounded-lg text-left text-xs cursor-pointer
          {$activeTab === item.id
            ? 'bg-secondary-container text-on-secondary-container font-medium'
            : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
        >
          <span class="material-symbols-outlined flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-lg' : 'mr-3 text-base'}">
            {item.icon}
          </span>
          {#if !$sidebarCollapsed}
            <span class="truncate flex-1">{item.label}</span>
            {#if item.shortcut}
              <span class="text-[10px] font-mono text-outline">{item.shortcut}</span>
            {/if}
          {/if}
        </button>
      {/each}
    {/each}
  </nav>

  <!-- Bottom Execution Button & Links -->
  <div class="px-3 mt-auto pt-4 border-t border-outline-variant/30">
    <button
      type="button"
      on:click|preventDefault={handleExecutePlan}
      title="Execute Plan"
      class="w-full bg-primary py-2 rounded-lg text-on-primary font-medium text-xs hover:opacity-90 transition-opacity flex items-center justify-center gap-2 mb-3 cursor-pointer"
    >
      <span class="material-symbols-outlined text-base">play_arrow</span>
      {#if !$sidebarCollapsed}<span>Execute Plan</span>{/if}
    </button>

    {#if !$sidebarCollapsed}
      <div class="space-y-0.5">
        <button
          type="button"
          on:click|preventDefault={() => activeTab.set('docs')}
          class="w-full flex items-center px-2.5 py-2 text-xs rounded-lg transition-colors cursor-pointer text-left {$activeTab === 'docs' ? 'bg-secondary-container text-on-secondary-container font-medium' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}">
          <span class="material-symbols-outlined text-base mr-3">description</span>
          <span>Docs</span>
        </button>
        <button
          type="button"
          on:click|preventDefault={() => activeTab.set('support')}
          class="w-full flex items-center px-2.5 py-2 text-xs rounded-lg transition-colors cursor-pointer text-left {$activeTab === 'support' ? 'bg-secondary-container text-on-secondary-container font-medium' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}">
          <span class="material-symbols-outlined text-base mr-3">help</span>
          <span>Support</span>
        </button>
      </div>
    {/if}
  </div>
</aside>
