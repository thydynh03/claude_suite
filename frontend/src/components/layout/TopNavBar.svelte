<script lang="ts">
  import { theme, toggleTheme } from '../../lib/stores/theme';
  import { zoom, zoomIn, zoomOut, zoomReset } from '../../lib/stores/zoom';
  import { activeTab, workspaceFolder, orchestratorRunning, isThinking, addLog, addToast, commandPaletteOpen, onboardingOpen, helpOpen } from '../../lib/stores/appState';
  import { currentLang } from '../../lib/stores/i18n';
  import * as AppBindings from '../../../wailsjs/go/main/App';

  let openMenu: 'file' | 'view' | 'window' | 'help' | null = null;

  function toggleMenu(menu: 'file' | 'view' | 'window' | 'help') {
    openMenu = openMenu === menu ? null : menu;
  }

  function closeMenus() {
    openMenu = null;
  }

  // These three used to keep their own `zoomLevel` and write
  // document.body.style.zoom, which was a second zoom entirely: Ctrl+wheel
  // scales documentElement, and body sits inside it, so the two multiplied —
  // "Phóng to" from 150% here on top of 200% there is 300%, and neither
  // readout knew about the other. One store now, one element.
  function handleZoomIn() {
    zoomIn();
    closeMenus();
  }

  function handleZoomOut() {
    zoomOut();
    closeMenus();
  }

  function handleResetZoom() {
    zoomReset();
    closeMenus();
  }

  function openHelp(section: 'shortcuts' | 'guide') {
    if (section === 'shortcuts') {
      helpOpen.set(true);
    } else {
      onboardingOpen.set(true);
    }
    closeMenus();
  }

  function goTab(tab: string) {
    activeTab.set(tab);
    closeMenus();
  }

  function openRepo() {
    const url = 'https://github.com/thydynh03/claude_suite';
    try {
      (AppBindings as any).OpenURLInBrowser(url);
    } catch {
      window.open(url, '_blank');
    }
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
    // This used to open the Settings tab, under a label that says "Command
    // Palette" and a shortcut nothing was bound to.
    commandPaletteOpen.set(true);
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
      addToast(`Đã tạo và chuyển sang git branch mới: ${newBranchName}`, 'SUCCESS');
      newBranchName = '';
      await loadGitData();
    } catch (e) {
      addLog(`Lỗi tạo branch: ${e}`, 'ERROR');
      addToast(`Lỗi tạo branch: ${e}`, 'ERROR');
    } finally {
      isGitActionRunning = false;
    }
  }

  async function handleCheckoutBranch(name: string) {
    isGitActionRunning = true;
    try {
      await (AppBindings as any).CheckoutGitBranch(name);
      addLog(`Đã chuyển sang git branch: ${name}`, 'SUCCESS');
      addToast(`Đã chuyển sang git branch: ${name}`, 'SUCCESS');
      await loadGitData();
    } catch (e) {
      addLog(`Lỗi chuyển branch: ${e}`, 'ERROR');
      addToast(`Lỗi chuyển branch: ${e}`, 'ERROR');
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
        addToast(`Đã revert commit ${hash} thành công`, 'SUCCESS');
        await loadGitData();
      } catch (e) {
        addLog(`Lỗi Revert commit: ${e}`, 'ERROR');
        addToast(`Lỗi Revert commit: ${e}`, 'ERROR');
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
      addToast(`Đã commit thay đổi thành công: ${commitMessage}`, 'SUCCESS');
      commitMessage = '';
      showGitCommitModal = false;
      await loadGitData();
    } catch (e) {
      addLog(`Lỗi Commit: ${e}`, 'ERROR');
      addToast(`Lỗi Commit: ${e}`, 'ERROR');
    } finally {
      isGitActionRunning = false;
    }
  }
</script>

<header class="flex justify-between items-center px-6 w-full fixed top-0 z-50 bg-surface border-b border-outline-variant h-[60px]">
  <div class="flex items-center gap-6">
    <div class="flex items-center gap-2">
      <span class="text-sm font-semibold text-on-surface">Claude Suite</span>
    </div>

    <nav class="hidden md:flex items-center gap-1 relative select-none" on:mouseleave={closeMenus}>
      <!-- File Menu -->
      <div class="relative">
        <button
          type="button"
          on:click={() => toggleMenu('file')}
          class="text-on-surface-variant text-xs font-semibold hover:bg-surface-container-highest transition-colors px-3 py-1.5 rounded-lg flex items-center gap-1 {openMenu ==='file' ? 'bg-surface-container-highest text-on-surface' : ''}"
        >
          File
        </button>
        {#if openMenu === 'file'}
          <div class="absolute left-0 top-full mt-1 w-56 bg-surface-container-lowest border border-outline-variant rounded-xl shadow-lg py-1 z-50 text-xs font-sans text-on-surface divide-y divide-outline-variant">
            <div class="py-1">
              <!-- Shortcut hints name only keys that are actually bound:
                   Ctrl+K in CommandPalette, Ctrl+1..0 in App.svelte. The menu
                   used to advertise Ctrl+Shift+O and Ctrl+Shift+P, neither of
                   which existed anywhere. -->
              <button type="button" on:click={handleNewConversation} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center group">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">chat</span> Cockpit</span>
                <span class="text-[10px] text-outline font-mono">Ctrl+1</span>
              </button>
              <button type="button" on:click={handleCreateProject} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center group">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">view_kanban</span> Task Board</span>
                <span class="text-[10px] text-outline font-mono">Ctrl+2</span>
              </button>
              <button type="button" on:click={handleCommandPalette} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center group">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">terminal</span> Bảng lệnh</span>
                <span class="text-[10px] text-outline font-mono">Ctrl+K</span>
              </button>
            </div>
            <div class="py-1">
              <button type="button" on:click={handleSelectFolder} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">folder_open</span> Mở workspace</span>
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
          class="text-on-surface-variant text-xs font-semibold hover:bg-surface-container-highest transition-colors px-3 py-1.5 rounded-lg flex items-center gap-1 {openMenu ==='view' ? 'bg-surface-container-highest text-on-surface' : ''}"
        >
          View
        </button>
        {#if openMenu === 'view'}
          <div class="absolute left-0 top-full mt-1 w-48 bg-surface-container-lowest border border-outline-variant rounded-xl shadow-lg py-1 z-50 text-xs font-sans text-on-surface">
            <!-- No shortcut hints here: none of these are bound, and Ctrl+0
                 is taken by the Settings tab shortcut in App.svelte, so the
                 old label contradicted what the key actually does. -->
            <button type="button" on:click={handleZoomIn} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">zoom_in</span> Phóng to</span>
              <span class="text-[10px] text-outline font-mono">Ctrl +</span>
            </button>
            <button type="button" on:click={handleZoomOut} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">zoom_out</span> Thu nhỏ</span>
              <span class="text-[10px] text-outline font-mono">Ctrl -</span>
            </button>
            <button type="button" on:click={handleResetZoom} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
              <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">restart_alt</span> Cỡ mặc định</span>
              <!-- Ctrl+Shift+0, not Ctrl+0: App.svelte binds Ctrl+0..9 to the
                   navigation tabs. The readout is live so this menu and the
                   zoom badge can never disagree. -->
              <span class="text-[10px] text-outline font-mono">{Math.round($zoom * 100)}%</span>
            </button>
          </div>
        {/if}
      </div>

      <!-- Window Menu -->
      <div class="relative">
        <button
          type="button"
          on:click={() => toggleMenu('window')}
          class="text-on-surface-variant text-xs font-semibold hover:bg-surface-container-highest transition-colors px-3 py-1.5 rounded-lg flex items-center gap-1 {openMenu ==='window' ? 'bg-surface-container-highest text-on-surface' : ''}"
        >
          Window
        </button>
        {#if openMenu === 'window'}
          <div class="absolute left-0 top-full mt-1 w-44 bg-surface-container-lowest border border-outline-variant rounded-xl shadow-lg py-1 z-50 text-xs font-sans text-on-surface divide-y divide-outline-variant">
            <div class="py-1">
              <button type="button" on:click={handleMinimize} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">minimize</span> Thu nhỏ cửa sổ</span>
              </button>
              <button type="button" on:click={handleMaximize} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">crop_square</span> Phóng cửa sổ</span>
              </button>
            </div>
            <div class="py-1">
              <button type="button" on:click={handleCloseWindow} class="w-full text-left px-4 py-2 hover:bg-error/10 text-error transition-colors flex justify-between items-center">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">close</span> Đóng app</span>
              </button>
            </div>
          </div>
        {/if}
      </div>

      <!-- Help Menu -->
      <div class="relative">
        <button
          type="button"
          on:click={() => toggleMenu('help')}
          class="text-on-surface-variant text-xs font-semibold hover:bg-surface-container-highest transition-colors px-3 py-1.5 rounded-lg flex items-center gap-1 {openMenu ==='help' ? 'bg-surface-container-highest text-on-surface' : ''}"
        >
          Help
        </button>
        {#if openMenu === 'help'}
          <div class="absolute left-0 top-full mt-1 w-60 bg-surface-container-lowest border border-outline-variant rounded-xl shadow-lg py-1 z-50 text-xs font-sans text-on-surface divide-y divide-outline-variant">
            <div class="py-1">
              <!-- Every item here goes somewhere that already exists. A Help
                   menu whose entries lead nowhere is worse than no Help menu:
                   it is the first place a stuck user looks. -->
              <button type="button" on:click={() => openHelp('guide')} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">rocket_launch</span> Hướng dẫn bắt đầu</span>
              </button>
              <button type="button" on:click={() => openHelp('shortcuts')} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex justify-between items-center">
                <span class="flex items-center gap-2"><span class="material-symbols-outlined text-sm">keyboard</span> Phím tắt</span>
                <span class="text-[10px] text-outline font-mono">F1</span>
              </button>
            </div>
            <div class="py-1">
              <button type="button" on:click={() => goTab('docs')} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex items-center gap-2">
                <span class="material-symbols-outlined text-sm">menu_book</span> Tài liệu
              </button>
              <button type="button" on:click={() => goTab('support')} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex items-center gap-2">
                <span class="material-symbols-outlined text-sm">help</span> Hỗ trợ &amp; khắc phục sự cố
              </button>
            </div>
            <div class="py-1">
              <button type="button" on:click={openRepo} class="w-full text-left px-4 py-2 hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors flex items-center gap-2">
                <span class="material-symbols-outlined text-sm">open_in_new</span> Mã nguồn trên GitHub
              </button>
            </div>
          </div>
        {/if}
      </div>
    </nav>

    <!-- Workspace Pill Badge -->
    <button on:click={handleSelectFolder} class="flex items-center gap-2 bg-surface-container border border-outline-variant rounded-lg px-3 py-1 text-xs hover:bg-surface-container-high transition-colors">
      <span class="material-symbols-outlined text-sm text-on-surface-variant">folder</span>
      <span class="font-medium text-on-surface truncate max-w-[180px]">
        {$workspaceFolder ? $workspaceFolder.split('\\').pop() : 'Chọn Workspace'}
      </span>
    </button>
  </div>

  <div class="flex items-center gap-4">
    <!-- Orchestrator Pill -->
    <button on:click={handleToggleOrchestrator} class="flex items-center gap-2 bg-surface-container-low px-3 py-1 rounded-full border border-outline-variant hover:bg-surface-container transition-colors">
      <div class="w-2 h-2 rounded-full {$orchestratorRunning ? 'bg-success' : 'bg-outline'}"></div>
      <span class="text-[11px] font-medium {$orchestratorRunning ? 'text-on-surface' : 'text-on-surface-variant'}">
        Orchestrator {$orchestratorRunning ? 'đang chạy' : 'tạm dừng'}
      </span>
    </button>

    <!-- Thinking Pill -->
    {#if $isThinking}
      <div class="flex items-center gap-2 bg-surface-container px-3 py-1 rounded-full border border-outline-variant text-xs text-on-surface-variant">
        <span class="material-symbols-outlined text-sm animate-spin">progress_activity</span>
        <span class="text-[11px]">Đang suy nghĩ…</span>
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
        <span class="material-symbols-outlined text-lg">account_tree</span>
      </button>
      <button
        type="button"
        on:click|preventDefault={openCommitModal}
        class="text-on-surface-variant hover:text-primary transition-colors p-1 rounded-lg hover:bg-surface-container-high cursor-pointer"
        title="Git Commit, Review & Staging"
      >
        <span class="material-symbols-outlined text-lg">schema</span>
      </button>

      <!-- Language Switcher -->
      <button
        type="button"
        on:click={() => currentLang.update(l => l === 'vi' ? 'en' : 'vi')}
        class="px-2 py-1 hover:bg-surface-container-high text-on-surface-variant rounded-lg text-xs font-medium transition-colors cursor-pointer border border-outline-variant"
        title="Chuyển ngôn ngữ">
        {$currentLang === 'vi' ? 'VI' : 'EN'}
      </button>

      <!-- Theme Switcher -->
      <button on:click={toggleTheme} class="text-on-surface-variant hover:text-on-surface transition-colors p-1" title="Đổi giao diện sáng/tối">
        <span class="material-symbols-outlined text-lg">{$theme === 'dark' ? 'dark_mode' : 'light_mode'}</span>
      </button>
    </div>

    <!-- Prompt Architect Button -->
    <button on:click={() => activeTab.set('cockpit')} class="bg-primary text-on-primary font-medium text-xs px-3 py-1.5 rounded-lg hover:opacity-90 transition-opacity flex items-center gap-1.5">
      <span class="material-symbols-outlined text-sm">psychology</span>
      Prompt Architect
    </button>
  </div>
</header>

<!-- Modal 1: Git Branch Manager & Revert History -->
{#if showGitBranchModal}
<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 text-left">
  <div class="bg-surface border border-outline-variant rounded-xl shadow-lg p-6 w-[560px] space-y-4 max-h-[85vh] flex flex-col">
    <div class="flex justify-between items-center pb-2 border-b border-outline-variant">
      <h3 class="text-sm font-semibold text-on-surface flex items-center gap-2">
        <span class="material-symbols-outlined text-base text-on-surface-variant">account_tree</span>
        Git Branch Manager & Commit Log
      </h3>
      <button type="button" on:click={() => showGitBranchModal = false} class="text-on-surface-variant hover:text-on-surface">
        <span class="material-symbols-outlined text-base">close</span>
      </button>
    </div>

    <!-- Active Branch & Switcher -->
    <div class="bg-surface-container-low border border-outline-variant rounded-xl p-3 space-y-2 text-xs">
      <div class="flex items-center justify-between">
        <span class="font-medium text-on-surface flex items-center gap-1">
          <span class="material-symbols-outlined text-sm text-on-surface-variant">call_split</span>
          Branch hiện tại: <strong class="text-on-surface font-mono text-sm ml-1">{gitBranchInfo?.current || 'master'}</strong>
        </span>
        <span class="text-[10px] bg-surface-container-high text-on-surface-variant px-2 py-0.5 rounded-lg font-mono">
          {gitStatus?.changed_files || 0} file đã sửa
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
          class="bg-primary text-on-primary px-3 py-1.5 rounded-lg font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer flex items-center gap-1"
        >
          <span class="material-symbols-outlined text-sm">add</span> Tạo branch
        </button>
      </div>

      <!-- Available Branches -->
      {#if gitBranchInfo?.branches?.length > 0}
        <div class="pt-2">
          <span class="text-[10px] font-medium text-on-surface-variant block mb-1">Danh sách branch</span>
          <div class="flex flex-wrap gap-1.5">
            {#each gitBranchInfo.branches as b}
              <button 
                type="button"
                on:click={() => handleCheckoutBranch(b)}
                class="px-2.5 py-1 rounded-lg text-[11px] font-mono border transition-colors cursor-pointer flex items-center gap-1
                {b === gitBranchInfo.current ? 'bg-primary text-on-primary border-primary' : 'bg-surface-container-lowest border-outline-variant text-on-surface hover:bg-surface-container'}"
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
      <span class="text-xs font-medium text-on-surface block">Lịch sử commit (nhấn Revert để khôi phục)</span>
      {#each gitCommits as c}
        <div class="bg-surface-container-lowest border border-outline-variant/60 rounded-lg p-3 flex items-center justify-between text-xs hover:bg-surface-container-low transition-colors">
          <div class="space-y-0.5 max-w-[380px]">
            <div class="flex items-center gap-2">
              <span class="font-mono text-on-surface-variant text-[11px] bg-surface-container-high px-1.5 py-0.5 rounded-lg">{c.hash}</span>
              <span class="text-on-surface truncate">{c.message}</span>
            </div>
            <p class="text-[10px] text-on-surface-variant">{c.author} · {c.date}</p>
          </div>
          <button
            type="button"
            on:click={() => handleRevertCommit(c.hash)}
            disabled={isGitActionRunning}
            class="px-2.5 py-1 text-error border border-outline-variant rounded-lg text-[10px] font-medium hover:bg-error/10 transition-colors cursor-pointer flex items-center gap-1"
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
  <div class="bg-surface border border-outline-variant rounded-xl shadow-lg p-6 w-[480px] space-y-4">
    <div class="flex justify-between items-center pb-2 border-b border-outline-variant">
      <h3 class="text-sm font-semibold text-on-surface flex items-center gap-2">
        <span class="material-symbols-outlined text-base text-on-surface-variant">schema</span>
        Git Commit & Code Review
      </h3>
      <button type="button" on:click={() => showGitCommitModal = false} class="text-on-surface-variant hover:text-on-surface">
        <span class="material-symbols-outlined text-base">close</span>
      </button>
    </div>

    <div class="space-y-3 text-xs">
      <div class="bg-surface-container-low border border-outline-variant rounded-lg p-3 flex justify-between items-center">
        <div>
          <span class="font-medium text-on-surface">Trạng thái workspace</span>
          <p class="text-on-surface-variant">{gitStatus?.changed_files || 0} file đã sửa đổi</p>
        </div>
        <span class="px-2.5 py-1 rounded-full font-mono text-[10px] font-medium {gitStatus?.clean ? 'text-success' : 'text-warning'}">
          {gitStatus?.clean ? 'Clean' : 'Modified'}
        </span>
      </div>

      <div>
        <label for="git-commit-msg" class="font-medium text-on-surface block mb-1">Nội dung commit</label>
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
        class="px-4 py-2 bg-surface-container-high text-on-surface-variant font-medium text-xs rounded-lg hover:bg-surface-container-highest transition-colors cursor-pointer">
        Hủy bỏ
      </button>
      <button
        type="button"
        on:click={handleCreateCommit}
        disabled={isGitActionRunning || !commitMessage.trim()}
        class="px-4 py-2 bg-primary text-on-primary font-medium text-xs rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer flex items-center gap-1">
        <span class="material-symbols-outlined text-sm">check</span> Commit
      </button>
    </div>
  </div>
</div>
{/if}
