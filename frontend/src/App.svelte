<script lang="ts">
  import { onMount } from 'svelte';
  import TopNavBar from './components/layout/TopNavBar.svelte';
  import SideNavBar from './components/layout/SideNavBar.svelte';
  import CockpitPage from './components/pages/CockpitPage.svelte';
  import TaskBoardPage from './components/pages/TaskBoardPage.svelte';
  import SettingsPage from './components/pages/SettingsPage.svelte';
  import VirtualOffice3D from './components/pages/VirtualOffice3D.svelte';
  import SchedulerPage from './components/pages/SchedulerPage.svelte';

  import { activeTab, workspaceFolder, addLog } from './lib/stores/appState';
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
      }
    } catch (e) {
      console.warn('Wails config load error:', e);
    }

    try {
      if ((window as any)?.runtime?.EventsOnMultiple) {
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
      }
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
</script>

<div class="flex flex-col h-screen font-body-md text-on-surface bg-background">
  <!-- Top Bar -->
  <TopNavBar />

  <div class="flex flex-1 pt-[60px]">
    <!-- Side Nav Bar -->
    <SideNavBar />

    <!-- Main Canvas Viewport -->
    <main class="ml-[240px] flex-1 p-6 h-[calc(100vh-60px)] overflow-y-auto bg-background">
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

    <div class="flex justify-end gap-3">
      <button 
        class="px-5 py-2 rounded-lg font-bold text-sm bg-surface2 text-on-surface-muted hover:bg-border transition-colors"
        on:click={() => resolveApproval(false)}>
        Từ chối (Reject)
      </button>
      <button 
        class="px-5 py-2 rounded-lg font-bold text-sm bg-primary hover:bg-primary-hover text-white transition-colors"
        on:click={() => resolveApproval(true)}>
        Cho phép (Approve)
      </button>
    </div>
  </div>
</div>
{/if}

