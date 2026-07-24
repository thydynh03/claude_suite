<script lang="ts">
  import { activeTab, orchestratorRunning } from '../../lib/stores/appState';
  import * as AppBindings from '../../../wailsjs/go/main/App';

  async function handleExecutePlan() {
    await AppBindings.StartOrchestrator();
    orchestratorRunning.set(true);
  }
</script>

<aside class="fixed left-0 top-[60px] h-[calc(100vh-60px)] w-[240px] flex flex-col py-4 bg-surface-container-low border-r border-outline-variant z-40">
  <!-- Brand Header -->
  <div class="px-6 mb-6">
    <div class="flex items-center gap-2 mb-1">
      <div class="w-8 h-8 rounded-lg bg-primary flex items-center justify-center shadow-sm">
        <span class="material-symbols-outlined text-on-primary text-xl">smart_toy</span>
      </div>
      <div>
        <h2 class="font-bold text-primary leading-tight text-base">Agent Center</h2>
        <p class="text-[10px] text-on-surface-variant uppercase font-bold tracking-wider">v4.8 Orchestrator</p>
      </div>
    </div>
  </div>

  <!-- Navigation Items -->
  <nav class="flex-1 space-y-1">
    <button
      on:click={() => activeTab.set('cockpit')}
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider
      {$activeTab === 'cockpit' ? 'bg-secondary-container text-on-secondary-container shadow-sm' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined mr-3 text-primary">rocket_launch</span>
      AI COCKPIT
    </button>

    <button
      on:click={() => activeTab.set('kanban')}
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider
      {$activeTab === 'kanban' ? 'bg-secondary-container text-on-secondary-container shadow-sm' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined mr-3 text-secondary">assignment</span>
      TASK BOARD & PLANNING
    </button>

    <button
      on:click={() => activeTab.set('settings')}
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider
      {$activeTab === 'settings' ? 'bg-secondary-container text-on-secondary-container shadow-sm' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined mr-3 text-secondary">settings</span>
      STUDIO & SETTINGS
    </button>

    <button
      on:click={() => activeTab.set('office')}
      class="w-[calc(100%-16px)] mx-2 my-1 flex items-center p-3 transition-all rounded-xl text-left font-medium text-[11px] uppercase tracking-wider
      {$activeTab === 'office' ? 'bg-secondary-container text-on-secondary-container shadow-sm' : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'}"
    >
      <span class="material-symbols-outlined mr-3 text-secondary">apartment</span>
      VIRTUAL OFFICE
    </button>
  </nav>

  <!-- Bottom Execution Button & Links -->
  <div class="px-4 mt-auto pt-4 border-t border-outline-variant/30">
    <button
      on:click={handleExecutePlan}
      class="w-full bg-primary py-3 rounded-xl text-on-primary font-bold text-sm hover:opacity-90 transition-all flex items-center justify-center gap-2 mb-4 shadow-sm active:scale-95"
    >
      <span class="material-symbols-outlined">play_arrow</span>
      Execute Plan
    </button>

    <div class="space-y-1">
      <a href="#docs" class="text-on-surface-variant hover:text-on-surface flex items-center p-2 text-xs rounded-xl hover:bg-surface-container-low transition-all">
        <span class="material-symbols-outlined text-base mr-3">description</span>
        <span class="uppercase tracking-wider font-semibold">DOCS</span>
      </a>
      <a href="#support" class="text-on-surface-variant hover:text-on-surface flex items-center p-2 text-xs rounded-xl hover:bg-surface-container-low transition-all">
        <span class="material-symbols-outlined text-base mr-3">help</span>
        <span class="uppercase tracking-wider font-semibold">SUPPORT</span>
      </a>
    </div>
  </div>
</aside>
