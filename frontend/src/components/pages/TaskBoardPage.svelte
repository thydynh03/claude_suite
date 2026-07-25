<script lang="ts">
  import { onMount } from 'svelte';
  import type { Task } from '../../lib/types';
  import KanbanView from './KanbanView.svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog, orchestratorRunning, tasksStore } from '../../lib/stores/appState';

  let subTab: 'kanban' | 'builder' | 'reports' = 'kanban';
  let requirementText = '';
  // Use global store so tasks persist across tab switches
  let tasks: Task[] = [];
  // Subscribe to global store
  const unsubscribeTasks = tasksStore.subscribe((v) => { tasks = v as Task[]; });

  let isDecomposing = false;

  let planModelChoice = 'claude:claude-opus-4-8';
  let showQuotaModal = false;
  let failedAgentName = '';
  let fallbackAgentName = '';
  let fallbackProvider = '';
  let fallbackModel = '';
  let quotaErrorMessage = '';

  $: estimatedTokens = Math.round((requirementText.length * 3.5) + ((tasks || []).reduce((acc, t) => acc + (t.prompt?.length || 100), 0) * 2.5) + 500);
  $: tokenText = estimatedTokens > 999 ? `~${(estimatedTokens / 1000).toFixed(1)}k` : `~${estimatedTokens}`;
  $: confidence = (tasks || []).length > 3 ? 98 : (tasks || []).length > 0 ? 88 : 0;

  let unoff: any;

  onMount(() => {
    loadTasks();
    if ((window as any)?.runtime) {
      unoff = (window as any).runtime.EventsOn('board_updated', () => {
        loadTasks();
      });
    }
    return () => {
      unsubscribeTasks();
      if (unoff && typeof unoff === 'function') unoff();
    };
  });

  async function loadTasks() {
    try {
      if ((window as any)?.go?.main?.App) {
        const res = await AppBindings.GetTasks();
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
    addLog(`AI Decomposing project requirements with ${providerName} (${model})...`, 'THINKING');
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
      addLog(`Created ${loaded.length} tasks!`, 'SUCCESS');
    } catch (e: any) {
      const errStr = String(e || '');
      addLog(`Decompose error: ${errStr}`, 'ERROR');
      
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
      addLog(`Đã phân rã thành công ${loaded.length} tasks bằng ${fallbackAgentName}!`, 'SUCCESS');
      planModelChoice = `${fallbackProvider}:${fallbackModel}`;
    } catch (err2) {
      addLog(`Lỗi phân rã bằng ${fallbackAgentName}: ${err2}`, 'ERROR');
    } finally {
      isDecomposing = false;
    }
  }

  async function handleClearAll() {
    await AppBindings.ClearAllTasks();
    await loadTasks();
    addLog('Cleared all tasks', 'INFO');
  }

  async function handleExecutePlan() {
    subTab = 'kanban'; // Auto-switch to Interactive Kanban view
    await AppBindings.StartOrchestrator();
    addLog('Orchestrator started! Switched to Interactive Kanban.', 'SUCCESS');
  }
</script>

<div class="space-y-6 max-w-7xl mx-auto pb-12">
  <!-- Hero Banner -->
  <div class="bg-gradient-to-r from-blue-600 to-blue-800 w-full rounded-xl p-6 flex items-center justify-between shadow-sm text-white">
    <div class="flex items-center gap-4">
      <div class="w-10 h-10 bg-white/20 rounded-lg flex items-center justify-center backdrop-blur-md">
        <span class="material-symbols-outlined text-white text-2xl">assignment</span>
      </div>
      <div>
        <h1 class="text-xl font-bold">Task Board & Planning — Quản lý tác vụ</h1>
        <p class="text-white/80 text-xs mt-0.5">Enterprise AI Orchestration Workspace</p>
      </div>
    </div>
    <div class="flex items-center gap-3">
      <div class="bg-white/10 px-4 py-1 rounded-full border border-white/20 text-xs font-semibold">
        Total Tasks: {(tasks || []).length}
      </div>
    </div>
  </div>

  <!-- Sub-Tabs Navigation -->
  <div class="flex justify-center">
    <div class="bg-surface-container-high p-1 rounded-xl flex gap-1 border border-outline-variant">
      <button
        type="button"
        on:click={() => (subTab = 'kanban')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all cursor-pointer
        {subTab === 'kanban' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Interactive Kanban
      </button>
      <button
        type="button"
        on:click={() => (subTab = 'builder')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all cursor-pointer
        {subTab === 'builder' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        AI Plan Builder
      </button>
      <button
        type="button"
        on:click={() => (subTab = 'reports')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all cursor-pointer
        {subTab === 'reports' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Project Reports
      </button>
    </div>
  </div>

  <div class={subTab === 'kanban' ? 'block' : 'hidden'}>
    <KanbanView tasks={$tasksStore} onRefresh={loadTasks} />
  </div>

  {#if subTab === 'builder'}
    <!-- Bento Layout Grid -->
    <div class="grid grid-cols-12 gap-6">
      <!-- Left Panel: Project Description -->
      <div class="col-span-12 lg:col-span-5 flex flex-col gap-4">
        <div class="flex items-center gap-2">
          <span class="material-symbols-outlined text-secondary text-sm">description</span>
          <span class="text-xs uppercase font-bold tracking-wider text-on-surface">Mô tả dự án / Mục tiêu</span>
        </div>
        <div class="flex-1 bg-surface-container-lowest border border-outline-variant rounded-xl flex flex-col overflow-hidden shadow-sm h-[420px]">
          <textarea
            bind:value={requirementText}
            on:keydown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                handleDecompose();
              }
            }}
            class="flex-1 bg-transparent text-on-surface text-sm p-4 outline-none resize-none placeholder:text-outline-variant border-none"
            placeholder="Xây dựng hệ thống Quản lý Task với REST API, Dashboard UI và Unit Tests... (Nhấn Enter để gửi, Shift+Enter xuống dòng)"
          ></textarea>
          <div class="p-3 bg-surface-container flex flex-col gap-2 border-t border-outline-variant">
            <div class="flex items-center gap-2">
              <label for="plan-model-select" class="text-[11px] font-bold text-on-surface-variant whitespace-nowrap">Plan Agent & Model:</label>
              <select
                id="plan-model-select"
                bind:value={planModelChoice}
                class="flex-1 bg-surface-container-low border border-outline-variant rounded-lg p-1.5 text-xs text-on-surface font-semibold outline-none focus:border-primary"
              >
                <option value="claude:claude-opus-4-8">🤖 Claude Agent (Claude 4.8 Opus)</option>
                <option value="claude:claude-sonnet-4-5">🤖 Claude Agent (Claude 4.5 Sonnet)</option>
                <option value="anti:gemini-3.6-flash-high">⚡ Antigravity Agent (Gemini 3.6 Flash)</option>
                <option value="anti:gemini-3.1-pro-high">⚡ Antigravity Agent (Gemini 3.1 Pro)</option>
              </select>
            </div>
            <button
              type="button"
              on:click|preventDefault={handleDecompose}
              disabled={isDecomposing}
              class="w-full bg-secondary text-on-secondary flex items-center justify-center gap-2 py-2.5 rounded-xl text-xs font-bold hover:brightness-110 transition-all disabled:opacity-50 cursor-pointer shadow-sm"
            >
              <span class="material-symbols-outlined text-sm {isDecomposing ? 'animate-spin' : ''}">
                {isDecomposing ? 'sync' : 'auto_mode'}
              </span>
              {isDecomposing ? '🤖 AI đang tạo kế hoạch...' : 'Tạo kế hoạch AI (Decompose Plan)'}
            </button>
          </div>
        </div>
        <div class="text-xs text-emerald-600 font-semibold flex items-center gap-1">
          <span class="material-symbols-outlined text-sm">check_circle</span>
          Đã tạo {tasks.length} tasks. Nhấn 'Execute Plan' để chạy.
        </div>
      </div>

      <!-- Right Panel: Task List -->
      <div class="col-span-12 lg:col-span-7 flex flex-col gap-4">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-secondary text-sm">list_alt</span>
            <span class="text-xs uppercase font-bold tracking-wider text-on-surface">TASKS ĐƯỢC TẠO</span>
          </div>
          <div class="flex gap-2">
            <button on:click={handleClearAll} class="flex items-center gap-1 px-3 py-1 bg-surface-container-highest rounded border border-outline-variant text-xs font-semibold hover:bg-rose-100 hover:text-rose-700 transition-all">
              <span class="material-symbols-outlined text-sm">delete</span> Clear
            </button>
            <button on:click={handleExecutePlan} class="flex items-center gap-1 px-4 py-1 bg-emerald-600 text-white rounded border border-emerald-700 text-xs font-bold hover:bg-emerald-700 transition-all">
              <span class="material-symbols-outlined text-sm">play_arrow</span> Execute Plan
            </button>
          </div>
        </div>

        <div class="flex-1 bg-surface-container-lowest border border-outline-variant rounded-xl shadow-sm overflow-y-auto max-h-[420px] p-4 space-y-4">
          {#each tasks as task, idx}
            <div class="relative border-l-4 border-primary bg-surface-container-low/60 rounded-r-xl p-4 space-y-3 shadow-xs hover:shadow-sm transition-all border border-outline-variant/50">
              <div class="flex items-start justify-between gap-3">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="font-mono text-xs font-black bg-primary/10 text-primary px-2 py-0.5 rounded-md border border-primary/20">#{idx + 1}</span>
                  <h3 class="font-bold text-sm text-on-surface flex items-center gap-2">
                    {task.title}
                  </h3>
                </div>
                <div class="flex items-center gap-2 font-mono text-[11px]">
                  <span class="px-2 py-0.5 rounded-full font-bold uppercase {task.priority === 'high' ? 'bg-rose-500/10 text-rose-600 border border-rose-500/20' : 'bg-blue-500/10 text-blue-600 border border-blue-500/20'}">
                    Priority: {task.priority || 'normal'}
                  </span>
                  <span class="px-2 py-0.5 rounded-full font-bold uppercase bg-surface-container-high text-on-surface-variant border border-outline-variant">
                    {task.status || 'backlog'}
                  </span>
                </div>
              </div>

              {#if task.description && task.description !== task.title}
                <p class="text-xs text-on-surface-variant leading-relaxed bg-surface-container-lowest/60 p-2.5 rounded-lg border border-outline-variant/40">
                  {task.description}
                </p>
              {/if}

              <!-- Structured Prompt Tag Display -->
              <div class="bg-surface-container-lowest p-3 rounded-lg border border-outline-variant font-mono text-[11px] space-y-1.5 leading-relaxed text-on-surface select-text overflow-x-auto whitespace-pre-wrap">
                {task.prompt || task.description}
              </div>
            </div>
          {:else}
            <div class="text-center text-on-surface-variant text-xs p-10 italic flex flex-col items-center justify-center gap-2">
              <span class="material-symbols-outlined text-3xl text-outline-variant">psychology</span>
              <span>Chưa có kế hoạch. Nhập mục tiêu dự án bên trái và bấm 'Tạo kế hoạch AI' để AI tự động Brainstorming chi tiết!</span>
            </div>
          {/each}
        </div>
      </div>
    </div>
  {:else}
    <!-- Reports Sub-tab -->
    <div class="bg-surface-container-lowest p-6 rounded-xl border border-outline-variant">
      <h3 class="font-bold text-base mb-2">Báo cáo dự án & Tasks</h3>
      <p class="text-xs text-on-surface-variant mb-4">Xuất báo cáo Kanban sang định dạng Markdown và HTML.</p>
      <button on:click={async () => { const file = await AppBindings.ExportKanbanReport(); addLog(`Exported report to ${file}`, 'SUCCESS'); }} class="bg-primary text-on-primary px-4 py-2 rounded-xl text-xs font-bold">
        Export Report Now
      </button>
    </div>
  {/if}

  <!-- Dynamic Dashboard Insights -->
  <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant shadow-sm flex flex-col justify-between">
      <span class="text-[10px] font-bold uppercase text-outline">TOKEN ESTIMATE</span>
      <span class="text-2xl font-bold text-secondary">{tokenText}</span>
      <div class="w-full h-1.5 bg-surface-container rounded-full overflow-hidden mt-2">
        <div class="h-full bg-secondary transition-all duration-500" style="width: {Math.min(100, Math.max(10, Math.round(estimatedTokens / 150)))}%"></div>
      </div>
    </div>
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant shadow-sm flex flex-col justify-between">
      <span class="text-[10px] font-bold uppercase text-outline">PLAN CONFIDENCE</span>
      <span class="text-2xl font-bold text-tertiary">{confidence}%</span>
      <div class="w-full h-1.5 bg-surface-container rounded-full overflow-hidden mt-2">
        <div class="h-full bg-tertiary transition-all duration-500" style="width: {confidence}%"></div>
      </div>
    </div>
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant shadow-sm flex flex-col justify-between">
      <span class="text-[10px] font-bold uppercase text-outline">ORCHESTRATOR LOAD</span>
      <span class="text-2xl font-bold {$orchestratorRunning ? 'text-primary' : 'text-on-surface'}">{$orchestratorRunning ? '85%' : '0%'}</span>
      <div class="w-full h-1.5 bg-surface-container rounded-full overflow-hidden mt-2">
        <div class="h-full bg-primary transition-all duration-500" style="width: {$orchestratorRunning ? '85%' : '0%'}"></div>
      </div>
    </div>
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant shadow-sm flex flex-col justify-between">
      <span class="text-[10px] font-bold uppercase text-outline">SYSTEM STATUS</span>
      <div class="flex items-center gap-2 mt-auto">
        <div class="w-3 h-3 rounded-full {$orchestratorRunning ? 'bg-primary animate-ping' : 'bg-emerald-500 animate-pulse'}"></div>
        <span class="text-base font-bold {$orchestratorRunning ? 'text-primary' : 'text-emerald-600'}">
          {$orchestratorRunning ? 'DISPATCHING' : 'READY'}
        </span>
      </div>
    </div>
  </div>
</div>
