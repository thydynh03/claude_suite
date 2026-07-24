<script lang="ts">
  import { onMount } from 'svelte';
  import type { Task } from '../../lib/types';
  import KanbanView from './KanbanView.svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog } from '../../lib/stores/appState';

  let subTab: 'kanban' | 'builder' | 'reports' = 'builder';
  let requirementText = '';
  let tasks: Task[] = [];
  let isDecomposing = false;

  onMount(async () => {
    await loadTasks();
  });

  async function loadTasks() {
    try {
      tasks = await AppBindings.GetTasks();
    } catch (e) {
      console.error(e);
    }
  }

  async function handleDecompose() {
    if (!requirementText.trim()) return;
    isDecomposing = true;
    addLog('AI Decomposing project requirements...', 'THINKING');
    try {
      tasks = await AppBindings.DecomposePlan(requirementText);
      addLog(`Created ${tasks.length} tasks!`, 'SUCCESS');
    } catch (e) {
      addLog(`Decompose error: ${e}`, 'ERROR');
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
    await AppBindings.StartOrchestrator();
    addLog('Orchestrator started to execute plan!', 'SUCCESS');
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
        Total Tasks: {tasks.length}
      </div>
    </div>
  </div>

  <!-- Sub-Tabs Navigation -->
  <div class="flex justify-center">
    <div class="bg-surface-container-high p-1 rounded-xl flex gap-1 border border-outline-variant">
      <button
        on:click={() => (subTab = 'kanban')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all
        {subTab === 'kanban' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Interactive Kanban
      </button>
      <button
        on:click={() => (subTab = 'builder')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all
        {subTab === 'builder' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        AI Plan Builder
      </button>
      <button
        on:click={() => (subTab = 'reports')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all
        {subTab === 'reports' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Project Reports
      </button>
    </div>
  </div>

  {#if subTab === 'kanban'}
    <KanbanView {tasks} onRefresh={loadTasks} />
  {:else if subTab === 'builder'}
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
            class="flex-1 bg-transparent text-on-surface text-sm p-4 outline-none resize-none placeholder:text-outline-variant border-none"
            placeholder="Xây dựng hệ thống Quản lý Task với REST API, Dashboard UI và Unit Tests..."
          ></textarea>
          <div class="p-3 bg-surface-container flex gap-2 border-t border-outline-variant">
            <button
              on:click={handleDecompose}
              disabled={isDecomposing}
              class="flex-1 bg-secondary text-on-secondary flex items-center justify-center gap-2 py-2 rounded-lg text-xs font-bold hover:brightness-110 transition-all disabled:opacity-50"
            >
              <span class="material-symbols-outlined text-sm">auto_mode</span>
              {isDecomposing ? 'Đang phân rã...' : 'AI Decompose (Opus 4.8)'}
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
            <div class="relative pl-4 border-l-2 border-primary py-2 bg-surface-container-low/40 rounded-r-lg p-3 space-y-2">
              <div class="flex items-start gap-2">
                <span class="font-mono text-xs text-primary font-bold">[{idx + 1}]</span>
                <h3 class="font-semibold text-sm text-on-surface">{task.title}</h3>
              </div>
              <div class="ml-4 space-y-2 text-xs">
                <div class="flex items-center gap-4 font-mono text-on-surface-variant">
                  <span>Priority: <strong class="text-primary">{task.priority}</strong></span>
                  <span>Status: <strong>{task.status}</strong></span>
                </div>
                <div class="bg-surface-container-lowest p-2 rounded border border-outline-variant text-on-surface-variant font-mono">
                  Prompt: {task.prompt || task.description}
                </div>
              </div>
            </div>
          {:else}
            <div class="text-center text-on-surface-variant text-xs p-8 italic">Chưa có task nào. Nhập mô tả và bấm 'AI Decompose'.</div>
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

  <!-- Dashboard Insights -->
  <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant shadow-sm flex flex-col justify-between">
      <span class="text-[10px] font-bold uppercase text-outline">TOKEN ESTIMATE</span>
      <span class="text-2xl font-bold text-secondary">~4.2k</span>
      <div class="w-full h-1.5 bg-surface-container rounded-full overflow-hidden mt-2">
        <div class="w-1/3 h-full bg-secondary"></div>
      </div>
    </div>
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant shadow-sm flex flex-col justify-between">
      <span class="text-[10px] font-bold uppercase text-outline">PLAN CONFIDENCE</span>
      <span class="text-2xl font-bold text-tertiary">98%</span>
      <div class="w-full h-1.5 bg-surface-container rounded-full overflow-hidden mt-2">
        <div class="w-11/12 h-full bg-tertiary"></div>
      </div>
    </div>
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant shadow-sm flex flex-col justify-between">
      <span class="text-[10px] font-bold uppercase text-outline">ORCHESTRATOR LOAD</span>
      <span class="text-2xl font-bold text-on-surface">0%</span>
      <div class="w-full h-1.5 bg-surface-container rounded-full overflow-hidden mt-2">
        <div class="w-0 h-full bg-primary"></div>
      </div>
    </div>
    <div class="bg-surface-container-lowest p-4 rounded-xl border border-outline-variant shadow-sm flex flex-col justify-between">
      <span class="text-[10px] font-bold uppercase text-outline">SYSTEM STATUS</span>
      <div class="flex items-center gap-2 mt-auto">
        <div class="w-3 h-3 rounded-full bg-emerald-500 animate-pulse"></div>
        <span class="text-base font-bold text-emerald-600">READY</span>
      </div>
    </div>
  </div>
</div>
