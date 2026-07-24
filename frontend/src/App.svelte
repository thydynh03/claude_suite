<script lang="ts">
  import { onMount } from 'svelte';
  import TopNavBar from './components/layout/TopNavBar.svelte';
  import SideNavBar from './components/layout/SideNavBar.svelte';
  import CockpitPage from './components/pages/CockpitPage.svelte';
  import TaskBoardPage from './components/pages/TaskBoardPage.svelte';
  import SettingsPage from './components/pages/SettingsPage.svelte';
  import VirtualOffice3D from './components/pages/VirtualOffice3D.svelte';
  import SchedulerPage from './components/pages/SchedulerPage.svelte';
  import DocsPage from './components/pages/DocsPage.svelte';
  import SupportPage from './components/pages/SupportPage.svelte';

  import { activeTab, workspaceFolder, addLog, sidebarCollapsed, tasksStore, agentsStore } from './lib/stores/appState';
  import * as AppBindings from '../wailsjs/go/main/App';
  import { EventsOn } from '../wailsjs/runtime/runtime';

  let showApprovalModal = false;
  let approvalAgent = '';
  let approvalTask = '';

  onMount(async () => {
    // Wait until Wails IPC & Go bindings are fully injected by WebView2
    await new Promise<void>((resolve) => {
      const check = setInterval(() => {
        if ((window as any)?.go?.main?.App) {
          clearInterval(check);
          resolve();
        }
      }, 50);
      setTimeout(() => {
        clearInterval(check);
        resolve();
      }, 3000);
    });

    try {
      if ((window as any)?.go?.main?.App) {
        const cfg = await AppBindings.GetWorkspaceConfig();
        if (cfg && cfg.last_workspace_folder) {
          workspaceFolder.set(cfg.last_workspace_folder);
        }
        // Bootstrap global stores
        try {
          const res = await AppBindings.GetTasks();
          tasksStore.set(Array.isArray(res) ? res : []);
        } catch (_) {}
        try {
          let ag = await AppBindings.GetAgents();
          if (!ag || ag.length === 0) {
            await AppBindings.ResetAgentsToDefaults();
            ag = await AppBindings.GetAgents();
          }
          agentsStore.set(ag || []);
        } catch (_) {}
      }
    } catch (e) {
      console.warn('Wails config load error:', e);
    }

    try {
      EventsOn('log_entry', (data: any) => {
        if (data && data.message) {
          addLog(data.message, data.level || 'INFO', data.time || '');
        }
      });
      EventsOn('ask_approval', (data: any) => {
        if (data) {
          approvalAgent = data.agentName || 'Agent';
          approvalTask = data.taskTitle || 'Unknown task';
          showApprovalModal = true;
        }
      });
      // Refresh global stores whenever orchestrator emits board_updated
      EventsOn('board_updated', async () => {
        try {
          const res = await AppBindings.GetTasks();
          tasksStore.set(Array.isArray(res) ? res : []);
        } catch (_) {}
        try {
          const ag = await AppBindings.GetAgents();
          agentsStore.set(ag || []);
        } catch (_) {}
      });
    } catch (e) {
      console.warn('Wails events error:', e);
    }
  });

  function resolveApproval(approved: boolean) {
    showApprovalModal = false;
    if ((window as any)?.go?.main?.App?.ResolveApproval) {
      (window as any).go.main.App.ResolveApproval(approved);
    }
  }

  async function resolveApprovalAlways() {
    showApprovalModal = false;
    if ((window as any)?.go?.main?.App?.SetAutoApproveAll) {
      await (window as any).go.main.App.SetAutoApproveAll(true);
    }
    if ((window as any)?.go?.main?.App?.ResolveApproval) {
      (window as any).go.main.App.ResolveApproval(true);
    }
  }
</script>

<div class="flex flex-col h-screen font-body-md text-on-surface bg-background">
  <!-- Top Bar -->
  <TopNavBar />

  <div class="flex flex-1 pt-[60px]">
    <!-- Side Nav Bar -->
    <SideNavBar />

    <!-- Main Canvas Viewport -->
    <main class="{$sidebarCollapsed ? 'ml-[72px]' : 'ml-[240px]'} flex-1 p-6 h-[calc(100vh-60px)] overflow-y-auto bg-background transition-all duration-300">
      {#if $activeTab === 'cockpit'}
        <CockpitPage />
      {:else if $activeTab === 'kanban'}
        <TaskBoardPage />
      {:else if $activeTab === 'settings'}
        <SettingsPage />
      {:else if $activeTab === 'office'}
        <VirtualOffice3D />
      {:else if $activeTab === 'scheduler'}
        <SchedulerPage />
      {:else if $activeTab === 'docs'}
        <DocsPage />
      {:else if $activeTab === 'support'}
        <SupportPage />
      {/if}
    </main>
  </div>
</div>

{#if showApprovalModal}
<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 backdrop-blur-sm">
  <div class="bg-surface border border-border rounded-xl shadow-2xl p-6 w-[450px]">
    <div class="flex items-center gap-3 mb-4">
      <span class="text-3xl">⚠️</span>
      <h2 class="text-xl font-bold text-on-surface">Yêu cầu xác nhận (Approval)</h2>
    </div>
    
    <p class="text-on-surface-muted mb-4">
      Agent <strong class="text-accent">{approvalAgent}</strong> yêu cầu sự đồng ý của bạn trước khi thực thi task:
    </p>
    
    <div class="bg-background border border-border p-3 rounded-lg mb-6">
      <p class="font-mono text-sm text-on-surface">{approvalTask}</p>
    </div>

    <div class="flex flex-wrap justify-end gap-2">
      <button 
        type="button"
        class="px-4 py-2 rounded-xl font-bold text-xs bg-surface-container-highest text-on-surface-variant hover:bg-surface-container-high transition-all cursor-pointer"
        on:click={() => resolveApproval(false)}>
        Từ chối (Reject)
      </button>
      <button 
        type="button"
        class="px-4 py-2 rounded-xl font-bold text-xs bg-emerald-600/10 text-emerald-600 border border-emerald-600/30 hover:bg-emerald-600/20 transition-all cursor-pointer flex items-center gap-1"
        on:click={resolveApprovalAlways}>
        <span class="material-symbols-outlined text-sm">all_inclusive</span> Luôn luôn cho phép
      </button>
      <button 
        type="button"
        class="px-4 py-2 rounded-xl font-bold text-xs bg-primary text-on-primary hover:opacity-90 transition-all shadow-sm cursor-pointer"
        on:click={() => resolveApproval(true)}>
        Cho phép (Approve)
      </button>
    </div>
  </div>
</div>
{/if}

