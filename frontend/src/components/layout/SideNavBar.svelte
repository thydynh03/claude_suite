<script lang="ts">
  import { activeTab, orchestratorRunning, sidebarCollapsed } from '../../lib/stores/appState';
  import { t } from '../../lib/stores/i18n';
  import * as AppBindings from '../../../wailsjs/go/main/App';

  // Reports what the orchestrator actually did rather than assuming it started.
  // Setting this true unconditionally left the sidebar claiming a run that never
  // began, which then disagreed with the board.
  // Ctrl+1..9 across the whole list, in the order shown. Announced in each
  // button's tooltip so the shortcut is discoverable rather than documented.
  const navGroups = [
    {
      title: 'Làm việc',
      items: [
        { id: 'cockpit', label: $t('cockpit'), title: 'AI Cockpit', icon: 'rocket_launch', color: 'text-primary', shortcut: 'Ctrl+1' },
        { id: 'kanban', label: $t('kanban'), title: 'Task Board & Planning', icon: 'assignment', color: 'text-secondary', shortcut: 'Ctrl+2' },
        { id: 'editor', label: $t('code_studio'), title: 'Agent IDE & Code Studio', icon: 'code', color: 'text-primary', shortcut: 'Ctrl+3' },
        { id: 'git', label: 'Source Control', title: 'Source Control', icon: 'account_tree', color: 'text-primary', shortcut: 'Ctrl+4' },
      ],
    },
    {
      title: 'Tự động hoá',
      items: [
        { id: 'scheduler', label: 'Scheduler', title: 'Scheduler & Jobs', icon: 'schedule', color: 'text-tertiary', shortcut: 'Ctrl+5' },
        { id: 'browser', label: 'Browser Agent', title: 'Browser Agent', icon: 'travel_explore', color: 'text-secondary', shortcut: 'Ctrl+6' },
        { id: 'claims', label: 'Phân xử', title: 'Claims & Adjudication', icon: 'gavel', color: 'text-tertiary', shortcut: 'Ctrl+7' },
      ],
    },
    {
      title: 'Giám sát',
      items: [
        { id: 'office', label: 'Virtual Office', title: 'Virtual Office 3D', icon: 'meeting_room', color: 'text-secondary', shortcut: 'Ctrl+8' },
      ],
    },
    {
      title: 'Cấu hình',
      items: [
        { id: 'roles', label: 'Agent Roles', title: 'Agent Roles', icon: 'group', color: 'text-tertiary', shortcut: 'Ctrl+9' },
        { id: 'settings', label: $t('settings'), title: 'Studio & Settings', icon: 'settings', color: 'text-secondary', shortcut: 'Ctrl+0' },
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
  <div class="px-4 mb-6 flex items-center justify-between">
    <div class="flex items-center gap-2 overflow-hidden">
      {#if !$sidebarCollapsed}
        <div class="truncate">
          <h2 class="font-bold text-primary leading-tight text-sm truncate">Agent Center</h2>
          <p class="text-[9px] text-on-surface-variant uppercase font-bold tracking-wider">v4.8 Orchestrator</p>
        </div>
      {/if}
    </div>
    <button
      type="button"
      on:click|preventDefault={() => sidebarCollapsed.update(c => !c)}
      class="p-1 rounded-lg hover:bg-surface-container-high text-on-surface-variant transition-colors flex items-center justify-center cursor-pointer"
      title={$sidebarCollapsed ? "Mở rộng Sidebar (>>)" : "Thu gọn Sidebar (<<)"}
    >
      <span class="material-symbols-outlined text-lg">{$sidebarCollapsed ? 'chevron_right' : 'chevron_left'}</span>
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
        <p class="px-5 pt-3 pb-1 text-[9px] font-bold uppercase tracking-widest text-outline">
          {group.title}
        </p>
      {:else}
        <div class="mx-4 my-2 border-t border-outline-variant/40"></div>
      {/if}

      {#each group.items as item}
        <button
          type="button"
          on:click|preventDefault={() => activeTab.set(item.id)}
          title="{item.title} ({item.shortcut})"
          class="w-[calc(100%-16px)] mx-2 my-0.5 flex items-center p-2.5 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
          {$activeTab === item.id
            ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold'
            : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
        >
          <span class="material-symbols-outlined {item.color} flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">
            {item.icon}
          </span>
          {#if !$sidebarCollapsed}
            <span class="truncate flex-1">{item.label}</span>
            <span class="text-[9px] font-mono text-outline">{item.shortcut}</span>
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
      class="w-full bg-primary py-3 rounded-xl text-on-primary font-bold text-sm hover:opacity-90 transition-all flex items-center justify-center gap-2 mb-4 shadow-sm active:scale-95 cursor-pointer"
    >
      <span class="material-symbols-outlined">play_arrow</span>
      {#if !$sidebarCollapsed}<span>Execute Plan</span>{/if}
    </button>

    {#if !$sidebarCollapsed}
      <div class="space-y-1">
        <button 
          type="button"
          on:click|preventDefault={() => activeTab.set('docs')}
          class="w-full text-on-surface-variant hover:text-on-surface flex items-center p-2 text-xs rounded-xl hover:bg-surface-container-low transition-all cursor-pointer text-left {$activeTab === 'docs' ? 'bg-secondary-container text-on-secondary-container font-bold' : ''}">
          <span class="material-symbols-outlined text-base mr-3 text-primary">description</span>
          <span class="uppercase tracking-wider font-semibold">DOCS</span>
        </button>
        <button 
          type="button"
          on:click|preventDefault={() => activeTab.set('support')}
          class="w-full text-on-surface-variant hover:text-on-surface flex items-center p-2 text-xs rounded-xl hover:bg-surface-container-low transition-all cursor-pointer text-left {$activeTab === 'support' ? 'bg-secondary-container text-on-secondary-container font-bold' : ''}">
          <span class="material-symbols-outlined text-base mr-3 text-secondary">help</span>
          <span class="uppercase tracking-wider font-semibold">SUPPORT</span>
        </button>
      </div>
    {/if}
  </div>
</aside>
