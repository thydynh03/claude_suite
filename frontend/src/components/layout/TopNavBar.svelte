<script lang="ts">
  import { theme, toggleTheme } from '../../lib/stores/theme';
  import { activeTab, workspaceFolder, orchestratorRunning, isThinking, addLog } from '../../lib/stores/appState';
  import { currentLang } from '../../lib/stores/i18n';
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

  // Git Version Control Modal state
  let showGitBranchModal = false;
  let showGitCommitModal = false;
  let gitStatus: any = null;
  let gitBranchInfo: any = { current: 'master', branches: [] };
  let gitCommits: any[] = [];
  let newBranchName = '';
  let commitMessage = '';
  let isGitActionRunning = false;

  async function openBranchModal() {
    showGitBranchModal = true;
    await loadGitData();
  }

  async function openCommitModal() {
    showGitCommitModal = true;
    await loadGitData();
  }

  async function loadGitData() {
    try {
      if ((AppBindings as any).GetGitStatus) {
        gitStatus = await (AppBindings as any).GetGitStatus();
      }
      if ((AppBindings as any).GetGitBranches) {
        gitBranchInfo = await (AppBindings as any).GetGitBranches();
      }
      if ((AppBindings as any).GetGitLog) {
        gitCommits = await (AppBindings as any).GetGitLog(10);
      }
    } catch (e) {
      console.error('Git error:', e);
    }
  }

  async function handleCreateBranch() {
    if (!newBranchName.trim()) return;
    isGitActionRunning = true;
    try {
      await (AppBindings as any).CreateGitBranch(newBranchName.trim());
      addLog(`Đã tạo và chuyển sang git branch mới: ${newBranchName}`, 'SUCCESS');
      newBranchName = '';
      await loadGitData();
    } catch (e) {
      addLog(`Lỗi tạo branch: ${e}`, 'ERROR');
    } finally {
      isGitActionRunning = false;
    }
  }

  async function handleCheckoutBranch(name: string) {
    isGitActionRunning = true;
    try {
      await (AppBindings as any).CheckoutGitBranch(name);
      addLog(`Đã chuyển sang git branch: ${name}`, 'SUCCESS');
      await loadGitData();
    } catch (e) {
      addLog(`Lỗi chuyển branch: ${e}`, 'ERROR');
    } finally {
      isGitActionRunning = false;
    }
  }

  async function handleRevertCommit(hash: string) {
    if (confirm(`Bạn có chắc muốn Revert commit ${hash}?`)) {
      isGitActionRunning = true;
      try {
        await (AppBindings as any).RevertGitCommit(hash);
        addLog(`Đã revert commit ${hash} thành công`, 'SUCCESS');
        await loadGitData();
      } catch (e) {
        addLog(`Lỗi Revert commit: ${e}`, 'ERROR');
      } finally {
        isGitActionRunning = false;
      }
    }
  }

  async function handleCreateCommit() {
    if (!commitMessage.trim()) return;
    isGitActionRunning = true;
    try {
      await (AppBindings as any).CreateGitCommit(commitMessage.trim());
      addLog(`Đã commit thay đổi thành công: ${commitMessage}`, 'SUCCESS');
      commitMessage = '';
      showGitCommitModal = false;
      await loadGitData();
    } catch (e) {
      addLog(`Lỗi Commit: ${e}`, 'ERROR');
    } finally {
      isGitActionRunning = false;
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
          <div class="absolute left-0 top-full mt-1 w-56 bg-surface-container-lowest border border-outline-variant rounded-xl shadow-xl py-1 z-50 text-xs font-sans text-on-surface divide-y divide-outline-variant">
            <div class="py-1">
              <button type="button" on:click={handleNewConversation} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center group">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm text-primary">chat</span> New Conversation</span>
                <span class="text-[10px] text-outline font-mono">Ctrl+Shift+O</span>
              </button>
              <button type="button" on:click={handleCreateProject} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center group">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm text-secondary">create_new_folder</span> Create Project</span>
              </button>
              <button type="button" on:click={handleCommandPalette} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center group">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm text-emerald-600">terminal</span> Command Palette</span>
                <span class="text-[10px] text-outline font-mono">Ctrl+Shift+P</span>
              </button>
            </div>
            <div class="py-1">
              <button type="button" on:click={handleSelectFolder} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm text-amber-500">folder_open</span> Open Workspace</span>
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
          <div class="absolute left-0 top-full mt-1 w-48 bg-surface-container-lowest border border-outline-variant rounded-xl shadow-xl py-1 z-50 text-xs font-sans text-on-surface">
            <button type="button" on:click={handleZoomIn} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">zoom_in</span> Zoom In</span>
              <span class="text-[10px] text-outline font-mono">Ctrl++</span>
            </button>
            <button type="button" on:click={handleZoomOut} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">zoom_out</span> Zoom Out</span>
              <span class="text-[10px] text-outline font-mono">Ctrl+-</span>
            </button>
            <button type="button" on:click={handleResetZoom} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">restart_alt</span> Reset Zoom</span>
              <span class="text-[10px] text-outline font-mono">Ctrl+0</span>
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
          <div class="absolute left-0 top-full mt-1 w-44 bg-surface-container-lowest border border-outline-variant rounded-xl shadow-xl py-1 z-50 text-xs font-sans text-on-surface divide-y divide-outline-variant">
            <div class="py-1">
              <button type="button" on:click={handleMinimize} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">minimize</span> Minimize</span>
              </button>
              <button type="button" on:click={handleMaximize} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">crop_square</span> Maximize</span>
              </button>
            </div>
            <div class="py-1">
              <button type="button" on:click={handleCloseWindow} class="w-full text-left px-4 py-2 hover:bg-rose-500/10 text-rose-600 transition-colors flex justify-between items-center font-bold">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">close</span> Close</span>
              </button>
            </div>
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
      <button
        type="button"
        on:click|preventDefault={openBranchModal}
        class="text-on-surface-variant hover:text-primary transition-colors p-1 rounded-lg hover:bg-surface-container-high cursor-pointer"
        title="Git Branch Manager, Log & Revert"
      >
        <span class="material-symbols-outlined text-xl">account_tree</span>
      </button>
      <button
        type="button"
        on:click|preventDefault={openCommitModal}
        class="text-on-surface-variant hover:text-primary transition-colors p-1 rounded-lg hover:bg-surface-container-high cursor-pointer"
        title="Git Commit, Review & Staging"
      >
        <span class="material-symbols-outlined text-xl">schema</span>
      </button>

      <!-- Language Switcher -->
      <button 
        type="button"
        on:click={() => currentLang.update(l => l === 'vi' ? 'en' : 'vi')} 
        class="px-2 py-1 bg-surface-container-high hover:bg-surface-container-highest text-on-surface rounded-lg text-xs font-bold transition-all cursor-pointer flex items-center gap-1 border border-outline-variant" 
        title="Switch Language (Chuyển ngôn ngữ)">
        <span>{$currentLang === 'vi' ? '🇻🇳 VI' : '🇺🇸 EN'}</span>
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

<!-- Modal 1: Git Branch Manager & Revert History -->
{#if showGitBranchModal}
<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 text-left">
  <div class="bg-surface border border-outline-variant rounded-2xl shadow-2xl p-6 w-[560px] space-y-4 max-h-[85vh] flex flex-col">
    <div class="flex justify-between items-center pb-2 border-b border-outline-variant">
      <h3 class="text-base font-bold text-on-surface flex items-center gap-2">
        <span class="material-symbols-outlined text-primary">account_tree</span>
        Git Branch Manager & Commit Log
      </h3>
      <button type="button" on:click={() => showGitBranchModal = false} class="text-on-surface-variant hover:text-on-surface">
        <span class="material-symbols-outlined text-xl">close</span>
      </button>
    </div>

    <!-- Active Branch & Switcher -->
    <div class="bg-surface-container-low border border-outline-variant rounded-xl p-3 space-y-2 text-xs">
      <div class="flex items-center justify-between">
        <span class="font-bold text-on-surface flex items-center gap-1">
          <span class="material-symbols-outlined text-sm text-secondary">call_split</span>
          Current Branch: <strong class="text-primary font-mono text-sm ml-1">{gitBranchInfo?.current || 'master'}</strong>
        </span>
        <span class="text-[10px] bg-primary/10 text-primary px-2 py-0.5 rounded font-mono">
          {gitStatus?.changed_files || 0} modified file(s)
        </span>
      </div>

      <!-- Create New Branch -->
      <div class="flex items-center gap-2 pt-2 border-t border-outline-variant/40">
        <input 
          type="text" 
          bind:value={newBranchName} 
          placeholder="Tên branch mới (ví dụ: feature/login)..." 
          class="flex-1 bg-surface-container-lowest border border-outline-variant px-3 py-1.5 rounded-lg text-xs outline-none focus:border-primary text-on-surface"
        />
        <button 
          type="button" 
          on:click={handleCreateBranch} 
          disabled={isGitActionRunning}
          class="bg-primary text-on-primary px-3 py-1.5 rounded-lg font-bold hover:opacity-90 transition-all disabled:opacity-50 cursor-pointer flex items-center gap-1"
        >
          <span class="material-symbols-outlined text-sm">add_link</span> Tạo Branch
        </button>
      </div>

      <!-- Available Branches -->
      {#if gitBranchInfo?.branches?.length > 0}
        <div class="pt-2">
          <span class="text-[10px] font-bold uppercase text-on-surface-variant block mb-1">Danh sách Branch:</span>
          <div class="flex flex-wrap gap-1.5">
            {#each gitBranchInfo.branches as b}
              <button 
                type="button"
                on:click={() => handleCheckoutBranch(b)}
                class="px-2.5 py-1 rounded-lg text-[11px] font-mono border transition-all cursor-pointer flex items-center gap-1
                {b === gitBranchInfo.current ? 'bg-primary text-on-primary border-primary font-bold shadow-sm' : 'bg-surface-container-lowest border-outline-variant text-on-surface hover:bg-surface-container'}"
              >
                <span class="material-symbols-outlined text-xs">commit</span> {b}
              </button>
            {/each}
          </div>
        </div>
      {/if}
    </div>

    <!-- Commit History & Revert -->
    <div class="flex-1 overflow-y-auto space-y-2 pr-1">
      <span class="text-xs font-bold text-on-surface uppercase block">Lịch sử Commit (Nhấn Revert để khôi phục):</span>
      {#each gitCommits as c}
        <div class="bg-surface-container-lowest border border-outline-variant/60 rounded-xl p-3 flex items-center justify-between text-xs hover:border-primary transition-all">
          <div class="space-y-0.5 max-w-[380px]">
            <div class="flex items-center gap-2">
              <span class="font-mono text-primary font-bold text-[11px] bg-primary/10 px-1.5 py-0.5 rounded">{c.hash}</span>
              <span class="font-semibold text-on-surface truncate">{c.message}</span>
            </div>
            <p class="text-[10px] text-on-surface-variant">{c.author} · {c.date}</p>
          </div>
          <button 
            type="button" 
            on:click={() => handleRevertCommit(c.hash)}
            disabled={isGitActionRunning}
            class="px-2.5 py-1 bg-rose-500/10 text-rose-600 border border-rose-500/30 rounded-lg text-[10px] font-bold hover:bg-rose-500/20 transition-all cursor-pointer flex items-center gap-1"
          >
            <span class="material-symbols-outlined text-xs">undo</span> Revert
          </button>
        </div>
      {:else}
        <p class="text-center text-xs text-on-surface-variant py-4 italic">Chưa có lịch sử commit.</p>
      {/each}
    </div>
  </div>
</div>
{/if}

<!-- Modal 2: Git Commit & Code Review -->
{#if showGitCommitModal}
<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 text-left">
  <div class="bg-surface border border-outline-variant rounded-2xl shadow-2xl p-6 w-[480px] space-y-4">
    <div class="flex justify-between items-center pb-2 border-b border-outline-variant">
      <h3 class="text-base font-bold text-on-surface flex items-center gap-2">
        <span class="material-symbols-outlined text-secondary">schema</span>
        Git Commit & Code Review
      </h3>
      <button type="button" on:click={() => showGitCommitModal = false} class="text-on-surface-variant hover:text-on-surface">
        <span class="material-symbols-outlined text-xl">close</span>
      </button>
    </div>

    <div class="space-y-3 text-xs">
      <div class="bg-surface-container-low border border-outline-variant rounded-xl p-3 flex justify-between items-center">
        <div>
          <span class="font-bold text-on-surface">Trạng thái Workspace:</span>
          <p class="text-on-surface-variant">{gitStatus?.changed_files || 0} file(s) đã sửa đổi</p>
        </div>
        <span class="px-2.5 py-1 rounded-full font-mono text-[10px] font-bold uppercase {gitStatus?.clean ? 'bg-emerald-500/10 text-emerald-600' : 'bg-amber-500/10 text-amber-600'}">
          {gitStatus?.clean ? 'Clean' : 'Modified'}
        </span>
      </div>

      <div>
        <label for="git-commit-msg" class="font-bold text-on-surface block mb-1">Nội dung Commit (Message):</label>
        <textarea 
          id="git-commit-msg"
          bind:value={commitMessage}
          placeholder="Ví dụ: feat: hoàn thiện giao diện Git Version Control..."
          class="w-full bg-surface-container-lowest border border-outline-variant rounded-lg p-2.5 h-24 outline-none focus:border-primary text-on-surface resize-none font-sans"
        ></textarea>
      </div>
    </div>

    <div class="flex justify-end gap-2 pt-3 border-t border-outline-variant">
      <button 
        type="button" 
        on:click={() => showGitCommitModal = false} 
        class="px-4 py-2 bg-surface-container-high text-on-surface-variant font-bold text-xs rounded-xl hover:bg-surface-container-highest transition-all cursor-pointer">
        Hủy bỏ
      </button>
      <button 
        type="button" 
        on:click={handleCreateCommit} 
        disabled={isGitActionRunning || !commitMessage.trim()}
        class="px-4 py-2 bg-primary text-on-primary font-bold text-xs rounded-xl hover:opacity-90 transition-all disabled:opacity-50 cursor-pointer flex items-center gap-1">
        <span class="material-symbols-outlined text-sm">check_circle</span> Commit Changes
      </button>
    </div>
  </div>
</div>
{/if}
