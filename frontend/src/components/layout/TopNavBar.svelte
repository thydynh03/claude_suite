<script lang="ts">
  import { theme, toggleTheme } from '../../lib/stores/theme';
  import { activeTab, workspaceFolder, orchestratorRunning, isThinking } from '../../lib/stores/appState';
  import * as AppBindings from '../../../wailsjs/go/main/App';

  let openMenu: 'file' | 'view' | 'window' | null = null;
  let zoomLevel = 100;

  function toggleMenu(menu: 'file' | 'view' | 'window') {
    openMenu = openMenu === menu ? null : menu;
  }

  function closeMenus() {
    openMenu = null;
  }

  function handleZoomIn() {
    zoomLevel = Math.min(zoomLevel + 10, 150);
    document.body.style.zoom = `${zoomLevel}%`;
    closeMenus();
  }

  function handleZoomOut() {
    zoomLevel = Math.max(zoomLevel - 10, 70);
    document.body.style.zoom = `${zoomLevel}%`;
    closeMenus();
  }

  function handleResetZoom() {
    zoomLevel = 100;
    document.body.style.zoom = '100%';
    closeMenus();
  }

  function handleMinimize() {
    if ((window as any)?.runtime?.WindowMinimise) {
      (window as any).runtime.WindowMinimise();
    }
    closeMenus();
  }

  function handleMaximize() {
    if ((window as any)?.runtime?.WindowToggleMaximise) {
      (window as any).runtime.WindowToggleMaximise();
    }
    closeMenus();
  }

  function handleCloseWindow() {
    if ((window as any)?.runtime?.Quit) {
      (window as any).runtime.Quit();
    }
    closeMenus();
  }

  function handleNewConversation() {
    activeTab.set('cockpit');
    closeMenus();
  }

  function handleCreateProject() {
    activeTab.set('kanban');
    closeMenus();
  }

  function handleCommandPalette() {
    activeTab.set('settings');
    closeMenus();
  }

  async function handleSelectFolder() {
    try {
      const folder = await AppBindings.SelectWorkspaceFolder();
      if (folder) {
        workspaceFolder.set(folder);
      }
    } catch (e) {
      console.error(e);
    }
    closeMenus();
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

    <nav class="hidden md:flex items-center gap-1 relative select-none" on:mouseleave={closeMenus}>
      <!-- File Menu -->
      <div class="relative">
        <button
          type="button"
          on:click={() => toggleMenu('file')}
          class="text-on-surface-variant text-xs font-semibold hover:bg-surface-container-highest transition-colors px-3 py-1.5 rounded-md flex items-center gap-1 {openMenu === 'file' ? 'bg-surface-container-highest text-on-surface font-bold' : ''}"
        >
          File
        </button>
        {#if openMenu === 'file'}
          <div class="absolute left-0 top-full mt-1 w-56 bg-slate-900 border border-slate-700 rounded-lg shadow-2xl py-1 z-50 text-xs font-sans text-slate-200 divide-y divide-slate-800">
            <div class="py-1">
              <button type="button" on:click={handleNewConversation} class="w-full text-left px-4 py-2 hover:bg-slate-800 flex justify-between items-center group">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm text-blue-400">chat</span> New Conversation</span>
                <span class="text-[10px] text-slate-400 font-mono">Ctrl+Shift+O</span>
              </button>
              <button type="button" on:click={handleCreateProject} class="w-full text-left px-4 py-2 hover:bg-slate-800 flex justify-between items-center group">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm text-purple-400">create_new_folder</span> Create Project</span>
              </button>
              <button type="button" on:click={handleCommandPalette} class="w-full text-left px-4 py-2 hover:bg-slate-800 flex justify-between items-center group">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm text-emerald-400">terminal</span> Command Palette</span>
                <span class="text-[10px] text-slate-400 font-mono">Ctrl+Shift+P</span>
              </button>
            </div>
            <div class="py-1">
              <button type="button" on:click={handleSelectFolder} class="w-full text-left px-4 py-2 hover:bg-slate-800 flex justify-between items-center">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm text-amber-400">folder_open</span> Open Workspace</span>
              </button>
            </div>
          </div>
        {/if}
      </div>

      <!-- View Menu -->
      <div class="relative">
        <button
          type="button"
          on:click={() => toggleMenu('view')}
          class="text-on-surface-variant text-xs font-semibold hover:bg-surface-container-highest transition-colors px-3 py-1.5 rounded-md flex items-center gap-1 {openMenu === 'view' ? 'bg-surface-container-highest text-on-surface font-bold' : ''}"
        >
          View
        </button>
        {#if openMenu === 'view'}
          <div class="absolute left-0 top-full mt-1 w-48 bg-slate-900 border border-slate-700 rounded-lg shadow-2xl py-1 z-50 text-xs font-sans text-slate-200">
            <button type="button" on:click={handleZoomIn} class="w-full text-left px-4 py-2 hover:bg-slate-800 flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">zoom_in</span> Zoom In</span>
              <span class="text-[10px] text-slate-400 font-mono">Ctrl++</span>
            </button>
            <button type="button" on:click={handleZoomOut} class="w-full text-left px-4 py-2 hover:bg-slate-800 flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">zoom_out</span> Zoom Out</span>
              <span class="text-[10px] text-slate-400 font-mono">Ctrl+-</span>
            </button>
            <button type="button" on:click={handleResetZoom} class="w-full text-left px-4 py-2 hover:bg-slate-800 flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">restart_alt</span> Reset Zoom</span>
              <span class="text-[10px] text-slate-400 font-mono">Ctrl+0</span>
            </button>
          </div>
        {/if}
      </div>

      <!-- Window Menu -->
      <div class="relative">
        <button
          type="button"
          on:click={() => toggleMenu('window')}
          class="text-on-surface-variant text-xs font-semibold hover:bg-surface-container-highest transition-colors px-3 py-1.5 rounded-md flex items-center gap-1 {openMenu === 'window' ? 'bg-surface-container-highest text-on-surface font-bold' : ''}"
        >
          Window
        </button>
        {#if openMenu === 'window'}
          <div class="absolute left-0 top-full mt-1 w-44 bg-slate-900 border border-slate-700 rounded-lg shadow-2xl py-1 z-50 text-xs font-sans text-slate-200">
            <button type="button" on:click={handleMinimize} class="w-full text-left px-4 py-2 hover:bg-slate-800 flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">minimize</span> Minimize</span>
            </button>
            <button type="button" on:click={handleMaximize} class="w-full text-left px-4 py-2 hover:bg-slate-800 flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">crop_square</span> Maximize</span>
            </button>
            <button type="button" on:click={handleCloseWindow} class="w-full text-left px-4 py-2 hover:bg-slate-800 text-rose-400 flex justify-between items-center border-t border-slate-800 mt-1 pt-2">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">close</span> Close</span>
            </button>
          </div>
        {/if}
      </div>
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
