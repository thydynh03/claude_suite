<script lang="ts">
  import { onMount } from 'svelte';
  import { workspaceFolder, logs, addLog } from '../../lib/stores/appState';
  import * as AppBindings from '../../../wailsjs/go/main/App';

  let promptInput = '';
  let workspaceFiles: string[] = [];
  let selectedFiles: string[] = [];
  let isRunning = false;
  let activeAgent = 'Claude-3.5-Sonnet';
  let thinkingPercent = 75;

  onMount(async () => {
    try {
      workspaceFiles = await AppBindings.ScanWorkspaceFiles();
    } catch (e) {
      console.error(e);
    }
  });

  function addQuickPreset(text: string) {
    if (promptInput) {
      promptInput += ' ' + text;
    } else {
      promptInput = text;
    }
  }

  async function handleRunAuto() {
    if (!promptInput.trim()) return;
    isRunning = true;
    addLog(`Initiated execution sequence for: "${promptInput.slice(0, 40)}..."`, 'SEND');

    try {
      const res = await AppBindings.RunQuickCLI(promptInput, 'claude-sonnet-4-5', '', selectedFiles);
      if (res && res.success) {
        addLog(`Execution finished successfully (${res.duration_sec.toFixed(1)}s)`, 'SUCCESS');
      } else if (res) {
        addLog(`Execution failed: ${res.error}`, 'ERROR');
      }
    } catch (e) {
      addLog(`Error executing CLI: ${e}`, 'ERROR');
    } finally {
      isRunning = false;
    }
  }

  async function handleDecomposePlan() {
    if (!promptInput.trim()) return;
    addLog('Decomposing project requirements using Opus 4.8...', 'THINKING');
    try {
      await AppBindings.DecomposePlan(promptInput);
      addLog('Plan decomposed! Check Task Board.', 'SUCCESS');
    } catch (e) {
      addLog(`Decompose failed: ${e}`, 'ERROR');
    }
  }
</script>

<div class="space-y-6 max-w-7xl mx-auto pb-12">
  <!-- Dashboard Header -->
  <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-bold flex items-center gap-2 text-on-surface">
        <span class="material-symbols-outlined text-primary">rocket_launch</span>
        AI Cockpit — Không gian điều khiển tự động
      </h1>
      <p class="text-on-surface-variant text-sm mt-1">Orchestrate agents, tasks, and code in a unified technical workspace.</p>
    </div>
    <div class="flex items-center gap-2 bg-surface-container-lowest p-1 rounded-full border border-outline-variant shadow-sm">
      <button class="bg-secondary-container text-on-secondary-container px-4 py-1 rounded-full text-xs font-bold flex items-center gap-1">
        <span class="material-symbols-outlined text-sm">bolt</span> ACTIVE
      </button>
      <button class="text-on-surface-variant px-4 py-1 rounded-full text-xs font-bold hover:text-on-surface hover:bg-surface-container-low transition-colors">
        HISTORY
      </button>
    </div>
  </div>

  <!-- 3-Step Linear Workflow -->
  <div class="space-y-6">

    <!-- Step 1: Context -->
    <section class="bg-surface-container-lowest rounded-xl border border-outline-variant overflow-hidden shadow-sm">
      <div class="p-4 border-b border-outline-variant bg-surface-container-low/50 flex justify-between items-center">
        <div class="flex items-center gap-2">
          <span class="w-6 h-6 rounded-full bg-primary-container text-on-primary-container flex items-center justify-center font-bold text-xs">1</span>
          <h3 class="font-semibold text-sm text-on-surface">Step 1: Đính kèm tài liệu (Context)</h3>
        </div>
        <span class="text-xs text-on-surface-variant font-mono">Context size: {selectedFiles.length} files</span>
      </div>
      <div class="p-4 grid grid-cols-1 md:grid-cols-2 gap-4 bg-surface-container-lowest">
        <!-- File Selection Area -->
        <div class="space-y-3">
          <div class="flex gap-2">
            <button class="flex items-center gap-2 bg-surface-container-lowest px-4 py-2 rounded-lg border border-outline-variant hover:bg-primary-container/20 hover:border-primary/50 transition-all text-on-surface shadow-sm text-xs font-medium">
              <span class="material-symbols-outlined text-on-surface-variant">folder_open</span>
              Chọn Files từ Project Tree
            </button>
          </div>
          <!-- File Tree List -->
          <div class="bg-surface-container-low/40 rounded-lg p-3 h-36 overflow-y-auto space-y-1 border border-outline-variant/60 font-mono text-xs">
            {#each workspaceFiles.slice(0, 10) as file}
              <label class="flex items-center gap-2 text-on-surface hover:bg-surface-container-low p-1 rounded transition-colors cursor-pointer">
                <input type="checkbox" value={file} bind:group={selectedFiles} class="rounded bg-surface-container-lowest border-outline-variant text-primary" />
                <span class="material-symbols-outlined text-sm text-amber-500">description</span>
                <span class="truncate">{file}</span>
              </label>
            {:else}
              <div class="text-on-surface-variant text-xs italic p-2">{$workspaceFolder ? 'Quét xong workspace files' : 'Chưa chọn workspace folder'}</div>
            {/each}
          </div>
        </div>

        <!-- Dropzone Visual Placeholder -->
        <div class="flex items-center justify-center border-2 border-dashed border-outline-variant rounded-xl bg-surface-container-low/30 hover:bg-surface-container-low/60 transition-colors cursor-pointer p-6">
          <div class="text-center text-on-surface-variant">
            <span class="material-symbols-outlined text-4xl opacity-40 block mb-2">upload_file</span>
            <p class="text-xs">Drop additional assets here to inject context</p>
          </div>
        </div>
      </div>
    </section>

    <!-- Step 2: Input Prompt -->
    <section class="bg-surface-container-lowest rounded-xl border border-outline-variant overflow-hidden shadow-sm">
      <div class="p-4 border-b border-outline-variant bg-surface-container-low/50 flex justify-between items-center">
        <div class="flex items-center gap-2">
          <span class="w-6 h-6 rounded-full bg-primary-container text-on-primary-container flex items-center justify-center font-bold text-xs">2</span>
          <h3 class="font-semibold text-sm text-on-surface">Step 2: Nhập Yêu Cầu (Prompt)</h3>
        </div>
      </div>
      <div class="p-4 space-y-4 bg-surface-container-lowest">
        <div class="relative">
          <textarea
            bind:value={promptInput}
            class="w-full bg-surface-container-low/30 h-44 rounded-xl p-4 font-mono text-xs text-on-surface border border-outline-variant focus:ring-2 focus:ring-primary outline-none placeholder:text-on-surface-variant/50 resize-none"
            placeholder="Ví dụ: Xây dựng form đăng nhập theo giao diện đính kèm..."
          ></textarea>

          <!-- Quick Actions Pills -->
          <div class="absolute bottom-4 left-4 flex flex-wrap gap-2">
            <button on:click={() => addQuickPreset('Sửa lỗi:')} class="flex items-center gap-1 px-3 py-1 bg-tertiary-container/20 text-tertiary border border-tertiary/30 rounded-full text-[10px] font-bold uppercase hover:bg-tertiary-container/40 transition-all bg-white">
              <span class="material-symbols-outlined text-xs">build</span> Sửa lỗi
            </button>
            <button on:click={() => addQuickPreset('Thêm tính năng:')} class="flex items-center gap-1 px-3 py-1 bg-secondary-container/30 text-primary border border-secondary/30 rounded-full text-[10px] font-bold uppercase hover:bg-secondary-container/50 transition-all bg-white">
              <span class="material-symbols-outlined text-xs">add_circle</span> Thêm tính năng
            </button>
            <button on:click={() => addQuickPreset('Tối ưu performance:')} class="flex items-center gap-1 px-3 py-1 bg-emerald-100 text-emerald-800 border border-emerald-300 rounded-full text-[10px] font-bold uppercase hover:bg-emerald-200 transition-all bg-white">
              <span class="material-symbols-outlined text-xs">auto_fix_high</span> Tối ưu
            </button>
            <button on:click={() => addQuickPreset('Viết documentation:')} class="flex items-center gap-1 px-3 py-1 bg-surface-container-highest text-on-surface-variant border border-outline-variant rounded-full text-[10px] font-bold uppercase hover:bg-outline-variant/30 transition-all bg-white">
              <span class="material-symbols-outlined text-xs">menu_book</span> Viết Docs
            </button>
          </div>
        </div>

        <div class="flex justify-end gap-3">
          <button on:click={handleDecomposePlan} class="px-6 py-3 rounded-xl border border-outline-variant font-semibold text-xs text-on-surface bg-surface-container-lowest hover:bg-surface-container-low transition-all flex items-center gap-2 shadow-sm">
            <span class="material-symbols-outlined text-on-surface-variant">analytics</span>
            Phân Rã Kế Hoạch
          </button>

          <button on:click={handleRunAuto} disabled={isRunning} class="px-6 py-3 rounded-xl bg-emerald-600 text-white font-bold text-xs hover:bg-emerald-700 active:scale-95 transition-all flex items-center gap-2 shadow-md disabled:opacity-50">
            <span class="material-symbols-outlined">{isRunning ? 'sync' : 'play_arrow'}</span>
            {isRunning ? 'Đang chạy...' : 'Chạy Tự Động'}
          </button>
        </div>
      </div>
    </section>

    <!-- Step 3: Live Progress -->
    <section class="bg-surface-container-lowest rounded-xl border border-outline-variant overflow-hidden shadow-sm">
      <div class="p-4 border-b border-outline-variant bg-surface-container-low/50 flex justify-between items-center">
        <div class="flex items-center gap-2">
          <span class="w-6 h-6 rounded-full bg-outline-variant text-on-surface flex items-center justify-center font-bold text-xs">3</span>
          <h3 class="font-semibold text-sm text-on-surface">Step 3: Live Progress</h3>
        </div>
      </div>
      <div class="flex h-[360px] bg-surface-container-lowest">
        <!-- Sidebar Status -->
        <div class="w-1/4 border-r border-outline-variant p-4 space-y-4 bg-surface-container-low/30">
          <div>
            <p class="text-[10px] uppercase font-bold text-on-surface-variant mb-2">AGENT STATUS</p>
            <div class="flex items-center gap-2 mb-4">
              <div class="w-3 h-3 rounded-full bg-primary animate-pulse shadow-sm"></div>
              <span class="font-mono text-xs text-primary font-bold">{activeAgent}: ACTIVE</span>
            </div>
          </div>

          <div class="space-y-3">
            <div class="flex justify-between text-xs">
              <span class="text-on-surface font-medium">Thinking Process</span>
              <span class="text-primary font-bold">{thinkingPercent}%</span>
            </div>
            <div class="w-full bg-surface-container-highest h-2 rounded-full overflow-hidden">
              <div class="bg-primary h-full transition-all" style="width: {thinkingPercent}%"></div>
            </div>
          </div>
        </div>

        <!-- Real-time Log Container -->
        <div class="flex-1 font-mono text-xs p-4 overflow-y-auto bg-slate-900 text-slate-100 space-y-1">
          {#each $logs as log}
            <div class="flex gap-2">
              <span class="text-slate-400">[{log.time}]</span>
              <span class="font-bold
                {log.level === 'SUCCESS' ? 'text-emerald-400' : ''}
                {log.level === 'ERROR' ? 'text-rose-400' : ''}
                {log.level === 'WARN' ? 'text-amber-400' : ''}
                {log.level === 'THINKING' ? 'text-purple-400' : ''}
                {log.level === 'SEND' ? 'text-blue-400' : ''}
              ">[{log.level}]</span>
              <span class="text-slate-200">{log.message}</span>
            </div>
          {:else}
            <div class="text-slate-500 italic">Sẵn sàng thực thi. Nhập prompt ở Step 2 và bấm 'Chạy Tự Động'.</div>
          {/each}
        </div>
      </div>
    </section>
  </div>
</div>
