<script lang="ts">
  import { onMount } from 'svelte';
  import TopNavBar from './components/layout/TopNavBar.svelte';
  import SideNavBar from './components/layout/SideNavBar.svelte';
  import CockpitPage from './components/pages/CockpitPage.svelte';
  import TaskBoardPage from './components/pages/TaskBoardPage.svelte';
  import SettingsPage from './components/pages/SettingsPage.svelte';
  import VirtualOffice3D from './components/pages/VirtualOffice3D.svelte';

  import { activeTab, workspaceFolder, addLog } from './lib/stores/appState';
  import * as AppBindings from '../wailsjs/go/main/App';
  import { EventsOn } from '../wailsjs/runtime/runtime';

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
      }
    } catch (e) {
      console.warn('Wails events error:', e);
    }
  });
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
      {/if}
    </main>
  </div>
</div>
