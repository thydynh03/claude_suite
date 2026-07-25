<script lang="ts">
  import { activeTab, orchestratorRunning, sidebarCollapsed } from '../../lib/stores/appState';
  import { t } from '../../lib/stores/i18n';
  import * as AppBindings from '../../../wailsjs/go/main/App';

  // Reports what the orchestrator actually did rather than assuming it started.
  // Setting this true unconditionally left the sidebar claiming a run that never
  // began, which then disagreed with the board.
  async function handleExecutePlan() {
    const started = await AppBindings.StartOrchestrator();
    orchestratorRunning.set(started === true);
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

  <!-- Navigation Items -->
  <nav class="flex-1 space-y-1">
    <button
      type="button"
      on:click|preventDefault={() => activeTab.set('cockpit')}
      title="AI Cockpit"
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
      {$activeTab === 'cockpit' ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined text-primary flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">rocket_launch</span>
      {#if !$sidebarCollapsed}
        <span class="truncate">{$t('cockpit')}</span>
      {/if}
    </button>

    <button
      type="button"
      on:click|preventDefault={() => activeTab.set('kanban')}
      title="Task Board & Planning"
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
      {$activeTab === 'kanban' ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined text-secondary flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">assignment</span>
      {#if !$sidebarCollapsed}
        <span class="truncate">{$t('kanban')}</span>
      {/if}
    </button>

    <button
      type="button"
      on:click|preventDefault={() => activeTab.set('editor')}
      title="Agent IDE & Code Studio"
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
      {$activeTab === 'editor' ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined text-primary flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">code</span>
      {#if !$sidebarCollapsed}
        <span class="truncate">{$t('code_studio')}</span>
      {/if}
    </button>

    <button
      type="button"
      on:click|preventDefault={() => activeTab.set('settings')}
      title="Studio & Settings"
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
      {$activeTab === 'settings' ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined text-secondary flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">settings</span>
      {#if !$sidebarCollapsed}
        <span class="truncate">{$t('settings')}</span>
      {/if}
    </button>

    <button
      type="button"
      on:click|preventDefault={() => activeTab.set('office')}
      title="Virtual Office"
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
      {$activeTab === 'office' ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined text-secondary flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">apartment</span>
      {#if !$sidebarCollapsed}
        <span class="truncate">{$t('virtual_office')}</span>
      {/if}
    </button>

    <button
      type="button"
      on:click|preventDefault={() => activeTab.set('scheduler')}
      title="Scheduler"
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
      {$activeTab === 'scheduler' ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined text-secondary flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">schedule</span>
      {#if !$sidebarCollapsed}
        <span class="truncate">{$t('scheduler')}</span>
      {/if}
    </button>

    <button
      type="button"
      on:click|preventDefault={() => activeTab.set('claims')}
      title="Adjudication"
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
      {$activeTab === 'claims' ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined text-secondary flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">gavel</span>
      {#if !$sidebarCollapsed}
        <span class="truncate">Phân xử Agent</span>
      {/if}
    </button>

    <button
      type="button"
      on:click|preventDefault={() => activeTab.set('browser')}
      title="Browser Agent"
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
      {$activeTab === 'browser' ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined text-secondary flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">language</span>
      {#if !$sidebarCollapsed}
        <span class="truncate">{$t('browser_agent')}</span>
      {/if}
    </button>

    <button
      type="button"
      on:click|preventDefault={() => activeTab.set('git')}
      title="Source Control"
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
      {$activeTab === 'git' ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined text-primary flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">account_tree</span>
      {#if !$sidebarCollapsed}
        <span class="truncate">SOURCE CONTROL</span>
      {/if}
    </button>
    <button
      type="button"
      on:click|preventDefault={() => activeTab.set('roles')}
      title="Agent Roles"
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider cursor-pointer
      {$activeTab === 'roles' ? 'bg-secondary-container text-on-secondary-container shadow-sm font-bold' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined text-tertiary flex-shrink-0 {$sidebarCollapsed ? 'mx-auto text-xl' : 'mr-3 text-lg'}">group</span>
      {#if !$sidebarCollapsed}
        <span class="truncate">AGENT ROLES</span>
      {/if}
    </button>
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
