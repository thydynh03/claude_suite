<script lang="ts">
  import { onMount } from 'svelte';
  import type { Task } from '../../lib/types';
  import KanbanView from './KanbanView.svelte';
  import ModelSelect from '../ui/ModelSelect.svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog, addToast, orchestratorRunning, tasksStore, agentsStore } from '../../lib/stores/appState';

  let subTab: 'kanban' | 'builder' | 'reports' = 'kanban';
  let requirementText = '';
  // Use global store so tasks persist across tab switches
  let tasks: Task[] = [];
  // Subscribe to global store
  const unsubscribeTasks = tasksStore.subscribe((v) => { tasks = v as Task[]; });

  let isDecomposing = false;
  let isStarting = false;
  let isExporting = false;
  let lastReportPath = '';
  let readiness: { ready?: number; blocked?: any[]; running?: number } | null = null;

  // Composite "provider:model" consumed by handleDecompose; the visible
  // picker is the shared ModelSelect (plain model id), kept in sync below.
  let planModelChoice = 'claude:claude-opus-4-8';
  let planModel = 'claude-opus-4-8';
  function onPlanModelChange(e: CustomEvent<{ value: string; provider: 'claude' | 'anti' }>) {
    planModelChoice = `${e.detail.provider}:${e.detail.value}`;
  }
  // The quota-fallback path rewrites planModelChoice directly — mirror it back
  // into the picker so the UI shows the model that will actually be used.
  $: planModel = planModelChoice.includes(':') ? planModelChoice.split(':')[1] : planModelChoice;
  let showQuotaModal = false;
  let failedAgentName = '';
  let fallbackAgentName = '';
  let fallbackProvider = '';
  let fallbackModel = '';
  let quotaErrorMessage = '';

  $: estimatedTokens = Math.round((requirementText.length * 3.5) + ((tasks || []).reduce((acc, t) => acc + (t.prompt?.length || 100), 0) * 2.5) + 500);
  $: tokenText = estimatedTokens > 999 ? `~${(estimatedTokens / 1000).toFixed(1)}k` : `~${estimatedTokens}`;
  // Real orchestrator load derived from live task statuses (not a hardcoded number)
  $: runningCount = (tasks || []).filter((t) => t.status === 'running').length;
  $: doneCount = (tasks || []).filter((t) => t.status === 'done').length;
  $: totalCount = (tasks || []).length;
  $: orchestratorLoad = totalCount === 0 ? 0
      : Math.round(((runningCount + doneCount) / totalCount) * 100);

  let unoffAgents: any;

  async function initTaskBoard() {
    // board_updated is handled centrally in App.svelte (refreshes tasksStore).
    // Register before the first await: onMount captures its cleanup closure
    // synchronously, so a listener attached after an await would leak whenever
    // the component unmounts while the initial load is still in flight.
    if ((window as any)?.runtime?.EventsOn) {
      unoffAgents = (window as any).runtime.EventsOn('agent_updated', () => {
        loadAgents();
      });
    }
    await loadTasks();
    await loadAgents();
  }

  onMount(() => {
    initTaskBoard();
    return () => {
      unsubscribeTasks();
      if (unoffAgents && typeof unoffAgents === 'function') unoffAgents();
    };
  });

  async function loadAgents() {
    try {
      if ((AppBindings as any).GetAgents) {
        const res = await (AppBindings as any).GetAgents();
        if (Array.isArray(res)) {
          agentsStore.set(res);
        }
      }
    } catch (e) {}
  }

  async function loadTasks() {
    try {
      if ((window as any)?.go?.main?.App) {
        const res = await (AppBindings as any).GetTasks();
        const loaded = Array.isArray(res) ? res : [];
        tasksStore.set(loaded);
      }
    } catch (e) {
      console.error(e);
      tasksStore.set([]);
    }
  }

  async function handleDecompose() {
    if (!requirementText.trim()) return;
    isDecomposing = true;
    const [provider, model] = planModelChoice.split(':');
    const providerName = provider === 'anti' ? 'Antigravity (Gemini)' : 'Claude Agent';
    addLog(`AI đang phân rã yêu cầu bằng ${providerName} (${model})...`, 'THINKING');
    try {
      let res: any;
      if (provider === 'anti') {
        res = await (AppBindings as any).DecomposePlanWithProvider(requirementText, "anti_cli", model);
      } else {
        res = await AppBindings.DecomposePlan(requirementText);
      }
      const loaded = Array.isArray(res) ? res : [];
      tasksStore.set(loaded);
      subTab = 'kanban';
      addLog(`Đã tạo ${loaded.length} task.`, 'SUCCESS');
      addToast(`Đã tạo ${loaded.length} task.`, 'SUCCESS');
    } catch (e: any) {
      const errStr = String(e || '');
      addLog(`Phân rã kế hoạch thất bại: ${errStr}`, 'ERROR');
      addToast(`Phân rã kế hoạch thất bại: ${errStr}`, 'ERROR');

      if (errStr.includes('429') || errStr.toLowerCase().includes('quota') || errStr.toLowerCase().includes('limit') || errStr.toLowerCase().includes('balance') || errStr.toLowerCase().includes('exhausted')) {
        failedAgentName = providerName;
        quotaErrorMessage = errStr;
        if (provider === 'anti') {
          fallbackAgentName = 'Claude Agent (Claude 4.5 Sonnet)';
          fallbackProvider = 'claude';
          fallbackModel = 'claude-sonnet-4-5';
        } else {
          fallbackAgentName = 'Antigravity Agent (Gemini 3.6 Flash)';
          fallbackProvider = 'anti';
          fallbackModel = 'gemini-3.6-flash-high';
        }
        showQuotaModal = true;
      }
    } finally {
      isDecomposing = false;
    }
  }

  async function handleConfirmFallbackDecompose() {
    showQuotaModal = false;
    isDecomposing = true;
    addLog(`Chuyển sang ${fallbackAgentName} để phân rã kế hoạch...`, 'INFO');
    try {
      let res: any;
      if (fallbackProvider === 'anti') {
        res = await (AppBindings as any).DecomposePlanWithProvider(requirementText, "anti_cli", fallbackModel);
      } else {
        res = await AppBindings.DecomposePlan(requirementText);
      }
      const loaded = Array.isArray(res) ? res : [];
      tasksStore.set(loaded);
      // Same ending as the main path: land on the board, say it the same way.
      // This branch used to leave the user on the plan tab wondering where the
      // tasks went, and phrased the same event differently.
      subTab = 'kanban';
      addLog(`Đã tạo ${loaded.length} task bằng ${fallbackAgentName}.`, 'SUCCESS');
      addToast(`Đã tạo ${loaded.length} task bằng ${fallbackAgentName}.`, 'SUCCESS');
      planModelChoice = `${fallbackProvider}:${fallbackModel}`;
    } catch (err2) {
      addLog(`Lỗi phân rã bằng ${fallbackAgentName}: ${err2}`, 'ERROR');
      addToast(`Lỗi phân rã bằng ${fallbackAgentName}: ${err2}`, 'ERROR');
    } finally {
      isDecomposing = false;
    }
  }

  async function handleClearAll() {
    // Deleting the whole board cannot be undone, and the plan may have taken a
    // model call to produce.
    if (!confirm('Xoá toàn bộ task trên bảng? Không hoàn tác được.')) return;
    try {
      await AppBindings.ClearAllTasks();
      await loadTasks();
      addLog('Đã xoá toàn bộ task.', 'INFO');
      addToast('Đã xoá toàn bộ task.', 'SUCCESS');
    } catch (e) {
      addLog(`Không xoá được task: ${e}`, 'ERROR');
      addToast(`Không xoá được task: ${e}`, 'ERROR');
    }
  }

  // Says what the orchestrator can actually do with the current board.
  //
  // This used to start it and announce success regardless: pressing it twice, or
  // pressing it with a backlog whose only task waits on two failed ones, looked
  // exactly like a successful start. The board then sat still and the button
  // looked broken — it was not, there was simply nothing it could dispatch.
  async function handleExecutePlan() {
    if (isStarting) return;
    isStarting = true;
    subTab = 'kanban';
    try {
      await loadTasks();
      await loadAgents();

      const started = await AppBindings.StartOrchestrator();
      orchestratorRunning.set(true);

      const r = await (AppBindings as any).GetDispatchReadiness();
      readiness = r;

      const ready = r?.ready ?? 0;
      const blocked = (r?.blocked || []).length;
      const dead = (r?.blocked || []).filter((b: any) => b.dead).length;

      if (ready > 0) {
        addToast(
          started
            ? `Đã khởi động — ${ready} task sẵn sàng giao.`
            : `Orchestrator đang chạy sẵn — ${ready} task sẵn sàng giao.`,
          'SUCCESS'
        );
      } else if (dead > 0) {
        addToast(
          `Không giao được task nào: ${dead} task đang chờ task đã lỗi. Hãy Retry các task lỗi trước.`,
          'ERROR',
          9000
        );
      } else if (blocked > 0) {
        addToast(`Chưa giao được task nào: ${blocked} task còn chờ task khác hoàn thành.`, 'INFO');
      } else {
        addToast('Backlog trống — chưa có task nào để giao.', 'INFO');
      }
      addLog(`Chạy kế hoạch: ${ready} sẵn sàng, ${blocked} bị chặn, ${dead} hỏng.`, 'INFO');
    } catch (e) {
      addLog(`Không khởi động được orchestrator: ${e}`, 'ERROR');
      addToast(`Không khởi động được orchestrator: ${e}`, 'ERROR');
    } finally {
      isStarting = false;
    }
  }

  async function handleExportReport() {
    if (isExporting) return;
    if ((tasks || []).length === 0) {
      addToast('Bảng đang trống — không có task nào để xuất.', 'INFO');
      return;
    }
    isExporting = true;
    try {
      const file = await AppBindings.ExportKanbanReport();
      addLog(`Đã xuất báo cáo: ${file}`, 'SUCCESS');
      lastReportPath = file;
      addToast(`Đã xuất báo cáo: ${file}`, 'SUCCESS', 8000);
    } catch (e) {
      addLog(`Xuất báo cáo thất bại: ${e}`, 'ERROR');
      addToast(`Xuất báo cáo thất bại: ${e}`, 'ERROR');
    } finally {
      isExporting = false;
    }
  }
</script>

<div class="space-y-6 max-w-7xl mx-auto pb-12">
  <!-- Page header -->
  <div class="flex items-end justify-between gap-4 border-b border-outline-variant pb-4">
    <div>
      <h1 class="text-lg font-semibold text-on-surface">Task Board</h1>
      <p class="text-xs text-on-surface-variant mt-0.5">Lập kế hoạch và điều phối sub-agent trên workspace hiện tại.</p>
    </div>
    <div class="text-xs text-on-surface-variant whitespace-nowrap">
      {(tasks || []).length} task
    </div>
  </div>

  <!-- Sub-Tabs Navigation -->
  <div class="flex justify-center">
    <div class="bg-surface-container-high p-1 rounded-xl flex gap-1 border border-outline-variant">
      <button
        type="button"
        on:click={() => (subTab = 'kanban')}
        class="px-4 py-1.5 text-xs font-medium rounded-lg transition-colors cursor-pointer
        {subTab === 'kanban' ? 'bg-surface-container-lowest text-on-surface' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Kanban
      </button>
      <button
        type="button"
        on:click={() => (subTab = 'builder')}
        class="px-4 py-1.5 text-xs font-medium rounded-lg transition-colors cursor-pointer
        {subTab === 'builder' ? 'bg-surface-container-lowest text-on-surface' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Kế hoạch AI
      </button>
      <button
        type="button"
        on:click={() => (subTab = 'reports')}
        class="px-4 py-1.5 text-xs font-medium rounded-lg transition-colors cursor-pointer
        {subTab === 'reports' ? 'bg-surface-container-lowest text-on-surface' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Báo cáo
      </button>
    </div>
  </div>

  <div class={subTab === 'kanban' ? 'block' : 'hidden'}>
    <KanbanView onRefresh={loadTasks} />
  </div>

  {#if subTab === 'builder'}
    <!-- Bento Layout Grid -->
    <div class="grid grid-cols-12 gap-6">
      <!-- Left Panel: Project Description -->
      <div class="col-span-12 lg:col-span-5 flex flex-col gap-4">
        <div class="flex items-center gap-2">
          <span class="material-symbols-outlined text-sm text-on-surface-variant">description</span>
          <span class="text-sm font-semibold text-on-surface">Mô tả dự án / mục tiêu</span>
        </div>
        <div class="flex-1 bg-surface-container-lowest border border-outline-variant rounded-xl flex flex-col overflow-hidden h-[420px] transition-colors">
          <textarea
            bind:value={requirementText}
            on:keydown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                handleDecompose();
              }
            }}
            class="flex-1 bg-transparent text-on-surface text-sm p-4 resize-none placeholder:text-outline border-none"
            placeholder="Xây dựng hệ thống Quản lý Task với REST API, Dashboard UI và Unit Tests... (Nhấn Enter để gửi, Shift+Enter xuống dòng)"
          ></textarea>
          <div class="p-3 bg-surface-container flex flex-col gap-2 border-t border-outline-variant">
            <div class="flex items-center gap-2">
              <span class="text-xs font-medium text-on-surface-variant whitespace-nowrap">Agent &amp; model</span>
              <div class="flex-1">
                <ModelSelect
                  value={planModel}
                  on:change={onPlanModelChange}
                  ariaLabel="Agent và model dùng để lập kế hoạch"
                  selectClass="w-full bg-surface-container-low border border-outline-variant rounded-lg p-1.5 text-xs text-on-surface focus:border-primary"
                />
              </div>
            </div>
            <button
              type="button"
              on:click|preventDefault={handleDecompose}
              disabled={isDecomposing}
              class="w-full bg-primary text-on-primary flex items-center justify-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer"
            >
              <span class="material-symbols-outlined text-sm {isDecomposing ? 'animate-spin' : ''}">
                {isDecomposing ? 'sync' : 'auto_mode'}
              </span>
              {isDecomposing ? 'AI đang tạo kế hoạch...' : 'Tạo kế hoạch AI'}
            </button>
          </div>
        </div>
        {#if tasks.length > 0}
          <div class="text-xs text-success flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm">check_circle</span>
            Đã tạo {tasks.length} task. Bấm "Chạy kế hoạch" để orchestrator giao việc.
          </div>
        {/if}
      </div>

      <!-- Right Panel: Task List -->
      <div class="col-span-12 lg:col-span-7 flex flex-col gap-4">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-sm text-on-surface-variant">list_alt</span>
            <span class="text-sm font-semibold text-on-surface">Task đã tạo</span>
          </div>
          <div class="flex gap-2">
            <button type="button" on:click={handleClearAll}
              class="border border-outline-variant text-error hover:bg-error/10 text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer flex items-center gap-1.5">
              <span class="material-symbols-outlined text-sm">delete</span> Xoá tất cả
            </button>
            <button type="button" on:click={handleExecutePlan} disabled={isStarting}
              class="border border-outline-variant text-on-surface-variant hover:bg-surface-container-high text-xs font-medium px-3 py-1.5 rounded-lg transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5">
              <span class="material-symbols-outlined text-sm {isStarting ? 'animate-spin' : ''}">{isStarting ? 'progress_activity' : 'play_arrow'}</span> Chạy kế hoạch
            </button>
          </div>
        </div>

        <div class="flex-1 bg-surface-container-lowest border border-outline-variant rounded-xl overflow-y-auto max-h-[420px] p-4 space-y-3">
          {#each tasks as task, idx}
            <div class="border border-outline-variant rounded-xl p-4 space-y-2">
              <div class="flex items-start justify-between gap-3">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="font-mono text-[11px] text-outline">#{idx + 1}</span>
                  <h3 class="text-sm font-semibold text-on-surface">{task.title}</h3>
                </div>
                <div class="flex items-center gap-2 text-[11px]">
                  <span class="px-2 py-0.5 rounded-full font-medium
                    {task.priority === 'high' ? 'bg-error/10 text-error' : 'bg-surface-container-high text-on-surface-variant'}">
                    {task.priority || 'normal'}
                  </span>
                  <span class="px-2 py-0.5 rounded-full font-medium bg-surface-container-high text-on-surface-variant">
                    {task.status || 'backlog'}
                  </span>
                </div>
              </div>

              {#if task.description && task.description !== task.title}
                <p class="text-xs text-on-surface-variant leading-relaxed">
                  {task.description}
                </p>
              {/if}

              <!-- Structured Prompt Tag Display -->
              <div class="bg-surface-container-low rounded-lg p-2.5 font-mono text-[11px] leading-relaxed text-on-surface-variant select-text overflow-x-auto whitespace-pre-wrap">
                {task.prompt || task.description}
              </div>
            </div>
          {:else}
            <div class="text-center text-on-surface-variant text-xs p-10 flex flex-col items-center justify-center gap-2">
              <span class="material-symbols-outlined text-base text-outline">psychology</span>
              <span>Chưa có kế hoạch. Nhập mục tiêu dự án bên trái và bấm "Tạo kế hoạch AI".</span>
            </div>
          {/each}
        </div>
      </div>
    </div>
  {:else if subTab === 'reports'}
    <!-- Reports Sub-tab -->
    <div class="bg-surface-container-lowest p-6 rounded-xl border border-outline-variant">
      <h3 class="text-sm font-semibold text-on-surface mb-2">Báo cáo dự án &amp; task</h3>
      <p class="text-xs text-on-surface-variant mb-4">Xuất báo cáo Kanban sang định dạng Markdown và HTML.</p>
      <button
        type="button"
        on:click={handleExportReport}
        disabled={isExporting}
        class="bg-primary text-on-primary text-xs font-medium px-3 py-1.5 rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer"
      >
        {isExporting ? 'Đang xuất...' : 'Xuất báo cáo'}
      </button>
      {#if lastReportPath}
        <p class="text-[11px] text-on-surface-variant mt-3 font-mono break-all">
          File gần nhất: {lastReportPath}
        </p>
      {/if}
    </div>
  {/if}

  <!-- Dynamic Dashboard Insights -->
  <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant flex flex-col justify-between gap-1">
      <span class="text-xs font-medium text-on-surface-variant">Token ước tính</span>
      <span class="text-lg font-semibold text-on-surface">{tokenText}</span>
      <span class="text-[11px] text-outline">Ước lượng thô theo độ dài prompt, không phải số đo thật.</span>
    </div>
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant flex flex-col justify-between gap-1">
      <span class="text-xs font-medium text-on-surface-variant">Tiến độ orchestrator</span>
      <span class="text-lg font-semibold text-on-surface">{orchestratorLoad}%</span>
      <div class="w-full h-1 bg-surface-container rounded-full overflow-hidden">
        <div class="h-full bg-primary" style="width: {orchestratorLoad}%"></div>
      </div>
      <span class="text-[11px] text-outline">{runningCount} đang chạy · {doneCount}/{totalCount} xong</span>
    </div>
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant flex flex-col justify-between gap-1">
      <span class="text-xs font-medium text-on-surface-variant">Trạng thái hệ thống</span>
      <div class="flex items-center gap-2">
        <div class="w-2 h-2 rounded-full {runningCount > 0 ? 'bg-primary' : $orchestratorRunning ? 'bg-primary' : 'bg-outline'}"></div>
        <span class="text-sm font-semibold text-on-surface">
          {runningCount > 0 ? 'Đang giao việc' : $orchestratorRunning ? 'Đang quét backlog' : 'Sẵn sàng'}
        </span>
      </div>
    </div>
  </div>
</div>

<!-- Quota fallback — the 429 path used to set showQuotaModal with nothing
     listening, so the promised "switch agent" dialog never appeared. -->
{#if showQuotaModal}
<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
  <div class="bg-surface border border-outline-variant rounded-xl shadow-lg p-6 w-[460px] space-y-4">
    <div class="flex items-center gap-2 pb-2 border-b border-outline-variant">
      <span class="material-symbols-outlined text-base text-on-surface-variant">swap_horiz</span>
      <h3 class="text-sm font-semibold text-on-surface">Agent đã hết quota</h3>
    </div>

    <div class="space-y-3 text-xs text-on-surface-variant">
      <p>
        <span class="text-on-surface font-medium">{failedAgentName}</span> không phân rã được kế hoạch
        (hết quota hoặc bị giới hạn tốc độ).
      </p>
      {#if quotaErrorMessage}
        <div class="bg-surface-container-highest border border-outline-variant rounded-lg p-2.5 font-mono text-[11px] leading-relaxed max-h-28 overflow-y-auto whitespace-pre-wrap break-all">
          {quotaErrorMessage}
        </div>
      {/if}
      <p>
        Chạy lại bằng <span class="text-on-surface font-medium">{fallbackAgentName}</span>?
        Picker model sẽ được cập nhật theo agent này.
      </p>
    </div>

    <div class="flex justify-end gap-2 pt-3 border-t border-outline-variant">
      <button
        type="button"
        on:click={() => (showQuotaModal = false)}
        class="border border-outline-variant text-on-surface-variant hover:bg-surface-container-high text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer">
        Huỷ
      </button>
      <button
        type="button"
        on:click={handleConfirmFallbackDecompose}
        class="bg-primary text-on-primary text-xs font-medium px-3 py-1.5 rounded-lg hover:opacity-90 transition-opacity cursor-pointer flex items-center gap-1.5">
        <span class="material-symbols-outlined text-sm">auto_mode</span> Dùng agent dự phòng
      </button>
    </div>
  </div>
</div>
{/if}
