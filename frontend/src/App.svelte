<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import TopNavBar from './components/layout/TopNavBar.svelte';
  import SideNavBar from './components/layout/SideNavBar.svelte';
  import CockpitPage from './components/pages/CockpitPage.svelte';
  import TaskBoardPage from './components/pages/TaskBoardPage.svelte';
  import SettingsPage from './components/pages/SettingsPage.svelte';
  import VirtualOffice3D from './components/pages/VirtualOffice3D.svelte';
  import SchedulerPage from './components/pages/SchedulerPage.svelte';
  import ClaimsPage from './components/pages/ClaimsPage.svelte';
  import DocsPage from './components/pages/DocsPage.svelte';
  import SupportPage from './components/pages/SupportPage.svelte';
  import CodeStudioPage from './components/pages/CodeStudioPage.svelte';
  import BrowserAgentPage from './components/pages/BrowserAgentPage.svelte';
  import GitPanel from './components/pages/GitPanel.svelte';
  import AgentRolesPage from './components/pages/AgentRolesPage.svelte';
  import MemoryPage from './components/pages/MemoryPage.svelte';

  import ToastHost from './components/ui/ToastHost.svelte';
  import CommandPalette from './components/ui/CommandPalette.svelte';
  import ZoomIndicator from './components/ui/ZoomIndicator.svelte';
  import { attachZoomControls } from './lib/stores/zoom';
  import OnboardingTour from './components/ui/OnboardingTour.svelte';
  import { activeTab, workspaceFolder, addLog, addTaskLog, addToast, setTaskScreenshot, sidebarCollapsed, tasksStore, agentsStore, onboardingOpen } from './lib/stores/appState';
  import * as AppBindings from '../wailsjs/go/main/App';
  import { EventsOn } from '../wailsjs/runtime/runtime';

  let showOnboarding = false;
  // Allow re-opening the tour on demand (command palette / Support page).
  const unsubOnboarding = onboardingOpen.subscribe((v) => {
    if (v) {
      showOnboarding = true;
      onboardingOpen.set(false);
    }
  });

  async function closeOnboarding() {
    showOnboarding = false;
    try {
      await (AppBindings as any).SetOnboardingSeen(true);
    } catch (_) {}
  }

  let showApprovalModal = false;
  let approvalAgent = '';
  let approvalTask = '';
  let approvalTaskId = '';
  let approvalRejectEl: HTMLButtonElement | null = null;

  // The safest action takes focus when the dialog appears, so a stray Enter
  // never approves a task the user has not read yet.
  $: if (showApprovalModal && approvalRejectEl) approvalRejectEl.focus();

  // The sidebar and the docs page both advertise Ctrl+1..0, and nothing
  // implemented them — the tooltip promised a shortcut that did nothing.
  // Order matches the sidebar exactly for every tab that HAS a shortcut, so
  // the number in the tooltip is the number that works. All ten slots are
  // taken; tabs added later (memory) ship without a shortcut rather than
  // renumbering everyone's muscle memory.
  const TAB_SHORTCUTS = [
    'cockpit', 'kanban', 'editor', 'git',
    'scheduler', 'browser', 'claims',
    'office', 'roles', 'settings',
  ];

  let detachZoom: (() => void) | null = null;
  onDestroy(() => detachZoom?.());

  function handleGlobalKeydown(e: KeyboardEvent) {
    // Escape closes the approval dialog. Closing it without an answer would
    // leave the waiting task blocked forever, so Escape means "reject".
    if (showApprovalModal && e.key === 'Escape') {
      e.preventDefault();
      resolveApproval(false);
      return;
    }
    if (!e.ctrlKey && !e.metaKey) return;
    if (e.altKey || e.shiftKey) return;

    // Digits only, and not while typing into a field where Ctrl+number might
    // mean something to the control itself.
    const digit = /^[0-9]$/.test(e.key) ? Number(e.key) : -1;
    if (digit < 0) return;

    const index = digit === 0 ? 9 : digit - 1;
    const tab = TAB_SHORTCUTS[index];
    if (!tab) return;

    e.preventDefault();
    activeTab.set(tab);
  }

  onMount(async () => {
    // Event listeners FIRST, before any await. Wails drops an event that has
    // no listener, and the backend flushes everything startup said at
    // OnDomReady — which fires long before the IPC-wait and the four bootstrap
    // round-trips below would finish. Registered here (a deferred module
    // script, so still before DOMContentLoaded) the queue lands in a live
    // listener instead of nowhere.
    registerEventListeners();

    // Ctrl+wheel and Ctrl+plus/minus. Registered here rather than at module
    // scope so it is torn down with the component, and before the awaits below
    // for the same reason the Wails listeners are: a user who scrolls during
    // the bootstrap round-trips should not have the gesture land on nothing.
    detachZoom = attachZoomControls();

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
      if (AppBindings && AppBindings.GetTasks) {
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

    // First-run welcome tour: show once after install (and again if its content
    // version changes). Delayed slightly so the workspace paints behind it first.
    try {
      if ((AppBindings as any).ShouldShowOnboarding) {
        const should = await (AppBindings as any).ShouldShowOnboarding();
        if (should) {
          setTimeout(() => { showOnboarding = true; }, 550);
        }
      }
    } catch (_) {}

  });

  function registerEventListeners() {
    try {
      EventsOn('log_entry', (data: any) => {
        if (data && data.message) {
          addLog(data.message, data.level || 'INFO', data.time || '');
          // Surface important outcomes as toasts (not every log line).
          const msg = String(data.message);
          if (data.level === 'ERROR' || /\bDONE\b|FAILED|rejected|Orchestrator/i.test(msg)) {
            addToast(msg, data.level || 'INFO');
          }
        }
      });
      EventsOn('task_log', (data: any) => {
        if (data && data.task_id) {
          addTaskLog(data.task_id, data.message || '', data.level || 'INFO', data.time || '');
        }
      });
      EventsOn('task_screenshot', (data: any) => {
        if (data && data.task_id && data.data) {
          setTaskScreenshot(data.task_id, data.data);
        }
      });
      EventsOn('ask_approval', (data: any) => {
        if (data) {
          approvalAgent = data.agentName || 'Agent';
          approvalTask = data.taskTitle || 'Unknown task';
          approvalTaskId = data.taskId || '';
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
      // Keep the agent list in sync everywhere (Settings, 3D office, Kanban,
      // Cockpit) whenever an agent is created/updated/deleted anywhere.
      EventsOn('agent_updated', async () => {
        try {
          const ag = await AppBindings.GetAgents();
          agentsStore.set(ag || []);
        } catch (_) {}
      });
    } catch (e) {
      console.warn('Wails events error:', e);
    }
  }

  function resolveApproval(approved: boolean) {
    showApprovalModal = false;
    if ((window as any)?.go?.main?.App?.ResolveApproval) {
      (window as any).go.main.App.ResolveApproval(approvalTaskId, approved);
    }
  }

  async function resolveApprovalAlways() {
    showApprovalModal = false;
    if ((window as any)?.go?.main?.App?.SetAutoApproveAll) {
      await (window as any).go.main.App.SetAutoApproveAll(true);
    }
    if ((window as any)?.go?.main?.App?.ResolveApproval) {
      (window as any).go.main.App.ResolveApproval(approvalTaskId, true);
    }
  }
</script>

<svelte:window on:keydown={handleGlobalKeydown} />

<div class="flex flex-col h-screen text-on-surface bg-background">
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
      {:else if $activeTab === 'editor'}
        <CodeStudioPage />
      {:else if $activeTab === 'settings'}
        <SettingsPage />
      {:else if $activeTab === 'office'}
        <VirtualOffice3D />
      {:else if $activeTab === 'scheduler'}
        <SchedulerPage />
      {:else if $activeTab === 'browser'}
        <BrowserAgentPage />
      {:else if $activeTab === 'claims'}
        <ClaimsPage />
      {:else if $activeTab === 'git'}
        <GitPanel />
      {:else if $activeTab === 'memory'}
        <MemoryPage />
      {:else if $activeTab === 'roles'}
        <AgentRolesPage />
      {:else if $activeTab === 'docs'}
        <DocsPage />
      {:else if $activeTab === 'support'}
        <SupportPage />
      {/if}
    </main>
  </div>
</div>

<ToastHost />
<CommandPalette />
<ZoomIndicator />
<OnboardingTour open={showOnboarding} on:close={closeOnboarding} />

{#if showApprovalModal}
<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
  <div
    class="bg-surface border border-outline-variant rounded-xl shadow-lg p-6 w-[450px] max-w-full"
    role="dialog"
    aria-modal="true"
    aria-labelledby="approval-title"
  >
    <h2 id="approval-title" class="text-base font-semibold text-on-surface mb-3">Yêu cầu xác nhận</h2>

    <p class="text-sm text-on-surface-variant mb-4">
      Agent <strong class="text-primary font-medium">{approvalAgent}</strong> cần bạn đồng ý trước khi thực thi task:
    </p>

    <div class="bg-surface-container-highest border border-outline-variant p-3 rounded-lg mb-6">
      <p class="font-mono text-xs text-on-surface break-words">{approvalTask}</p>
    </div>

    <!-- "Luôn cho phép" is not a third way of answering this task: it turns
         off the confirmation prompt for every future task and cannot be undone
         from here. It sits apart from the answer pair and says what it costs,
         because styled like its neighbours it was one mis-click away. -->
    <div class="flex flex-wrap items-center justify-end gap-2">
      <button
        type="button"
        class="mr-auto px-2 py-1 rounded-lg text-[11px] text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high transition-colors cursor-pointer flex items-center gap-1.5"
        title="Tắt hẳn bước xác nhận cho mọi task sau này (bật lại trong Cài đặt)"
        on:click={resolveApprovalAlways}>
        <span class="material-symbols-outlined text-sm">all_inclusive</span>
        Cho phép và không hỏi lại nữa
      </button>
      <!-- Escape resolves this dialog as a rejection, which is a decision, not
           a dismissal — so the dialog says so rather than letting a reflex key
           silently kill a task. -->
      <button
        type="button"
        bind:this={approvalRejectEl}
        class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer flex items-center gap-1.5"
        on:click={() => resolveApproval(false)}>
        Từ chối
        <kbd class="text-[10px] font-mono text-outline">Esc</kbd>
      </button>
      <button
        type="button"
        class="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary text-on-primary hover:opacity-90 transition-opacity cursor-pointer"
        on:click={() => resolveApproval(true)}>
        Cho phép
      </button>
    </div>
  </div>
</div>
{/if}

