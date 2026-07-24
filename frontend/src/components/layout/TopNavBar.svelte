<script lang="ts">
  import { theme, toggleTheme } from '../../lib/stores/theme';
  import { activeTab, workspaceFolder, orchestratorRunning, isThinking } from '../../lib/stores/appState';
  import * as AppBindings from '../../../wailsjs/go/main/App';

  async function handleSelectFolder() {
    try {
      const folder = await AppBindings.SelectWorkspaceFolder();
      if (folder) {
        workspaceFolder.set(folder);
      }
    } catch (e) {
      console.error(e);
    }
  }

  async function handleToggleOrchestrator() {
    if ($orchestratorRunning) {
      await AppBindings.StopOrchestrator();
      orchestratorRunning.set(false);
    } else {
      await AppBindings.StartOrchestrator();
      orchestratorRunning.set(true);
    }
  }
</script>

<header class="flex justify-between items-center px-6 w-full fixed top-0 z-50 bg-surface border-b border-outline-variant h-[60px]">
  <div class="flex items-center gap-6">
    <div class="flex items-center gap-2">
      <span class="material-symbols-outlined text-primary text-2xl font-bold">webhook</span>
      <span class="text-lg font-bold text-on-surface">Claude Suite</span>
    </div>

    <nav class="hidden md:flex items-center gap-4">
      <button on:click={handleSelectFolder} class="text-on-surface-variant text-sm hover:bg-surface-container-highest transition-colors px-2 py-1 rounded">
        File
      </button>
      <button class="text-on-surface-variant text-sm hover:bg-surface-container-highest transition-colors px-2 py-1 rounded">
        View
      </button>
      <button class="text-on-surface-variant text-sm hover:bg-surface-container-highest transition-colors px-2 py-1 rounded">
        Window
      </button>
    </nav>

    <!-- Workspace Pill Badge -->
    <button on:click={handleSelectFolder} class="flex items-center gap-2 bg-surface-container border border-outline-variant rounded-lg px-3 py-1 text-xs hover:bg-surface-container-high transition-all">
      <span class="material-symbols-outlined text-sm text-primary">folder</span>
      <span class="font-medium text-on-surface truncate max-w-[180px]">
        {$workspaceFolder ? $workspaceFolder.split('\\').pop() : 'Chọn Workspace'}
      </span>
    </button>
  </div>

  <div class="flex items-center gap-4">
    <!-- Orchestrator Pill -->
    <button on:click={handleToggleOrchestrator} class="flex items-center gap-2 bg-surface-container-low px-3 py-1 rounded-full border border-outline-variant hover:bg-surface-container transition-all">
      <div class="w-2 h-2 rounded-full {$orchestratorRunning ? 'bg-primary animate-pulse' : 'bg-outline'}"></div>
      <span class="text-[11px] font-bold uppercase tracking-wider {$orchestratorRunning ? 'text-primary' : 'text-on-surface-variant'}">
        Orchestrator: {$orchestratorRunning ? 'Active' : 'Inactive'}
      </span>
    </button>

    <!-- Thinking Pill -->
    {#if $isThinking}
      <div class="flex items-center gap-2 bg-tertiary-container/30 text-tertiary px-3 py-1 rounded-full border border-tertiary/30 text-xs animate-pulse">
        <span class="material-symbols-outlined text-sm">psychology</span>
        <span class="font-semibold text-[11px]">AI Thinking...</span>
      </div>
    {/if}

    <!-- Toolbar Icons -->
    <div class="flex items-center gap-2">
      <button class="text-on-surface-variant hover:text-primary transition-colors p-1" title="Tree Explorer">
        <span class="material-symbols-outlined text-xl">account_tree</span>
      </button>
      <button class="text-on-surface-variant hover:text-primary transition-colors p-1" title="Network Topology">
        <span class="material-symbols-outlined text-xl">lan</span>
      </button>

      <!-- Theme Switcher -->
      <button on:click={toggleTheme} class="text-on-surface-variant hover:text-primary transition-colors p-1" title="Toggle Theme">
        <span class="material-symbols-outlined text-xl">{$theme === 'dark' ? 'dark_mode' : 'light_mode'}</span>
      </button>
    </div>

    <!-- Prompt Architect Button -->
    <button on:click={() => activeTab.set('cockpit')} class="bg-primary-container text-on-primary-container font-semibold text-xs uppercase px-4 py-2 rounded-xl hover:opacity-90 transition-all flex items-center gap-1 shadow-sm">
      <span class="material-symbols-outlined text-sm">psychology</span>
      Prompt Architect
    </button>
  </div>
</header>
