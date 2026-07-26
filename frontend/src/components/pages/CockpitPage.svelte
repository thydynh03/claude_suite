<script lang="ts">
  import { onMount } from 'svelte';
  import { workspaceFolder, logs, addLog, addToast, tasksStore, agentsStore, orchestratorRunning } from '../../lib/stores/appState';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import ModelSelect from '../ui/ModelSelect.svelte';

  // Aggregated live metrics derived from real task/agent state.
  $: tAll = ($tasksStore || []) as any[];
  $: mRunning = tAll.filter((t) => t.status === 'running').length;
  $: mQueued = tAll.filter((t) => t.status === 'backlog' || t.status === 'queued').length;
  $: mDone = tAll.filter((t) => t.status === 'done').length;
  $: mFailed = tAll.filter((t) => t.status === 'failed').length;
  $: mAgentsActive = ($agentsStore || []).filter((a: any) => a.status === 'running').length;
  $: mTokens = ($agentsStore || []).reduce((s: number, a: any) => s + (a.tokens_used || 0), 0);
  $: mTokensText = mTokens > 999999 ? `${(mTokens / 1e6).toFixed(2)}M` : mTokens > 999 ? `${(mTokens / 1000).toFixed(1)}k` : `${mTokens}`;

  // A reactive value, not a function called from the markup. Svelte reads a
  // template function call untracked, so `{#each metricCards() as m}` rendered
  // once and then never again: the numbers froze at mount while the Kanban
  // board next door kept moving. Covered by CockpitPage.test.ts.
  $: metricCards = [
    { label: 'Running', value: mRunning, icon: 'bolt', cls: 'text-primary' },
    { label: 'Queue', value: mQueued, icon: 'hourglass_empty', cls: 'text-secondary' },
    { label: 'Done', value: mDone, icon: 'check_circle', cls: 'text-emerald-600' },
    { label: 'Failed', value: mFailed, icon: 'cancel', cls: 'text-rose-600' },
    { label: 'Agents Active', value: mAgentsActive, icon: 'smart_toy', cls: 'text-tertiary' },
    { label: 'Tokens', value: mTokensText, icon: 'toll', cls: 'text-on-surface' },
  ];

  let promptInput = '';
  let workspaceFiles: string[] = [];
  let selectedFiles: string[] = [];
  let isRunning = false;
  let selectedModel = 'claude-sonnet-4-5';
  // Show the actual selected id — a hardcoded label map here mislabeled every
  // model it didn't know about.
  $: activeAgent = (selectedModel.includes('gemini') || selectedModel.includes('thinking')
    ? 'Antigravity · ' : 'Claude · ') + selectedModel;
  let topTab = 'active'; // 'active' | 'history'
  let cliOutput = '';
  // Lines the CLI emits while it is still working. Kept apart from cliOutput so
  // the final answer replaces the progress rather than being appended to it.
  let liveLines: string[] = [];
  let unoffQuickLog: any;
  let showCLIConsole = false;

  let showFileTreeModal = false;
  let fileSearchQuery = '';

  $: filteredWorkspaceFiles = workspaceFiles.filter(f => f.toLowerCase().includes(fileSearchQuery.toLowerCase().trim()));

  function toggleFileSelect(file: string) {
    if (selectedFiles.includes(file)) {
      selectedFiles = selectedFiles.filter(f => f !== file);
    } else {
      selectedFiles = [...selectedFiles, file];
    }
  }

  function selectAllFilteredFiles() {
    const combined = new Set([...selectedFiles, ...filteredWorkspaceFiles]);
    selectedFiles = Array.from(combined);
  }

  function clearAllFileSelection() {
    selectedFiles = [];
  }

  onMount(() => {
    // Registered before any await: onMount captures its cleanup closure
    // synchronously, so a listener attached after one would leak.
    if ((window as any)?.runtime?.EventsOn) {
      unoffQuickLog = (window as any).runtime.EventsOn('quick_cli_log', (data: any) => {
        if (!isRunning) return;
        const line = `${data?.time || ''} ${data?.message || ''}`.trim();
        // Bounded: a long run can emit thousands of lines and the panel only
        // shows the tail anyway.
        liveLines = [...liveLines.slice(-300), line];
      });
    }
    initCockpit();
    return () => {
      if (typeof unoffQuickLog === 'function') unoffQuickLog();
    };
  });

  async function initCockpit() {
    try {
      if ((window as any)?.go?.main?.App) {
        workspaceFiles = await AppBindings.ScanWorkspaceFiles();
      }
      if ((AppBindings as any).GetShowCLIConsole) {
        showCLIConsole = await (AppBindings as any).GetShowCLIConsole();
      }
    } catch (e) {
      console.error(e);
    }
  }

  async function handleToggleShowCLIConsole() {
    try {
      if ((AppBindings as any).SetShowCLIConsole) {
        await (AppBindings as any).SetShowCLIConsole(showCLIConsole);
        addLog(`Cài đặt cửa sổ CMD: ${showCLIConsole ? 'BẬT (Hiện CMD)' : 'TẮT (Ẩn ngầm)'}`, 'INFO');
        addToast(`Cửa sổ CMD: ${showCLIConsole ? 'hiện' : 'ẩn'}.`, 'SUCCESS');
      }
    } catch (e) {
      // The checkbox already moved, so leaving this silent shows a setting the
      // backend never accepted.
      showCLIConsole = !showCLIConsole;
      addToast(`Không đổi được cài đặt cửa sổ CMD: ${e}`, 'ERROR');
    }
  }

  function addQuickPreset(text: string) {
    if (promptInput) {
      promptInput += ' ' + text;
    } else {
      promptInput = text;
    }
  }

  async function handleRunAuto() {
    if (!promptInput.trim() || isRunning) return;
    isRunning = true;
    liveLines = [];
    cliOutput = '';
    addLog(`Initiated execution sequence for: "${promptInput.slice(0, 40)}..."`, 'SEND');

    try {
      const res = await AppBindings.RunQuickCLI(promptInput, selectedModel, '', selectedFiles);
      if (res && res.success) {
        cliOutput = res.output;
        addLog(`Execution finished successfully (${res.duration_sec.toFixed(1)}s)`, 'SUCCESS');
        addToast(`Execution finished successfully (${res.duration_sec.toFixed(1)}s)`, 'SUCCESS');
        addToast(`Chạy xong sau ${res.duration_sec.toFixed(1)}s.`, 'SUCCESS');
      } else if (res) {
        cliOutput = 'Lỗi thực thi: ' + res.error;
        addLog(`Execution failed: ${res.error}`, 'ERROR');
        addToast(`Execution failed: ${res.error}`, 'ERROR');
        addToast(`Thực thi thất bại: ${res.error}`, 'ERROR');
      }
    } catch (e) {
      cliOutput = 'Lỗi kết nối: ' + e;
      addLog(`Error executing CLI: ${e}`, 'ERROR');
      addToast(`Error executing CLI: ${e}`, 'ERROR');
      addToast(`Không kết nối được CLI: ${e}`, 'ERROR');
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
      addToast('Plan decomposed! Check Task Board.', 'SUCCESS');
    } catch (e) {
      addLog(`Decompose failed: ${e}`, 'ERROR');
      addToast(`Decompose failed: ${e}`, 'ERROR');
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
    <div class="bg-surface-container-high p-1 rounded-xl flex gap-1 border border-outline-variant">
      <button 
        type="button"
        on:click={() => (topTab = 'active')} 
        class="px-5 py-1.5 rounded-lg text-xs font-bold flex items-center gap-1 transition-all cursor-pointer
        {topTab === 'active' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}">
        <span class="material-symbols-outlined text-sm">bolt</span> ACTIVE
      </button>
      <button 
        type="button"
        on:click={() => (topTab = 'history')} 
        class="px-5 py-1.5 rounded-lg text-xs font-bold flex items-center gap-1 transition-all cursor-pointer
        {topTab === 'history' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}">
        <span class="material-symbols-outlined text-sm">history</span> HISTORY ({$logs.length})
      </button>
    </div>
  </div>

  <!-- Aggregated live metrics -->
  <div class="grid grid-cols-3 md:grid-cols-6 gap-3">
    {#each metricCards as m (m.label)}
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-3 flex flex-col gap-1 shadow-sm">
        <div class="flex items-center gap-1.5 text-[10px] uppercase font-bold text-on-surface-variant">
          <span class="material-symbols-outlined text-sm {m.cls}">{m.icon}</span> {m.label}
        </div>
        <span class="text-2xl font-bold {m.cls}">{m.value}</span>
      </div>
    {/each}
  </div>
  <div class="flex items-center gap-2 text-xs -mt-2">
    <span class="w-2.5 h-2.5 rounded-full {$orchestratorRunning ? 'bg-primary animate-pulse' : 'bg-emerald-500'}"></span>
    <span class="font-bold {$orchestratorRunning ? 'text-primary' : 'text-emerald-600'}">
      Orchestrator: {$orchestratorRunning ? 'ĐANG CHẠY' : 'SẴN SÀNG'}
    </span>
    <span class="text-on-surface-variant">· Workspace: {$workspaceFolder || 'chưa chọn'}</span>
  </div>

  {#if topTab === 'active'}
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
          <div class="flex items-center justify-between">
            <button
              type="button"
              on:click={() => showFileTreeModal = true}
              class="flex items-center gap-2 bg-primary text-on-primary px-4 py-2 rounded-xl hover:opacity-90 transition-all shadow-sm text-xs font-bold cursor-pointer">
              <span class="material-symbols-outlined text-sm">account_tree</span>
              Chọn Files từ Project Tree ({workspaceFiles.length} files)
            </button>

            {#if selectedFiles.length > 0}
              <button
                type="button"
                on:click={clearAllFileSelection}
                class="text-[11px] text-rose-600 font-semibold hover:underline">
                Bỏ chọn tất cả ({selectedFiles.length})
              </button>
            {/if}
          </div>

          <!-- Selected Files Badges / Preview List -->
          <div class="bg-surface-container-low/40 rounded-xl p-3 h-36 overflow-y-auto space-y-1.5 border border-outline-variant/60 font-mono text-xs">
            {#each selectedFiles as file}
              <div class="flex items-center justify-between bg-surface-container-lowest border border-outline-variant/80 px-2.5 py-1 rounded-lg text-on-surface">
                <div class="flex items-center gap-2 truncate">
                  <span class="material-symbols-outlined text-sm text-emerald-600">check_circle</span>
                  <span class="truncate">{file}</span>
                </div>
                <button
                  type="button"
                  on:click={() => toggleFileSelect(file)}
                  class="text-on-surface-variant hover:text-rose-600 p-0.5 rounded cursor-pointer">
                  <span class="material-symbols-outlined text-sm">close</span>
                </button>
              </div>
            {:else}
              <div class="text-on-surface-variant text-xs italic p-4 text-center">
                Chưa đính kèm file nào. Nhấn nút "Chọn Files từ Project Tree" để đính kèm ngữ cảnh.
              </div>
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
        <!-- Model Selection & CLI Console Toggle -->
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs bg-surface-container-low/40 p-3 rounded-xl border border-outline-variant/60">
          <div class="flex items-center gap-3">
            <span class="material-symbols-outlined text-primary text-sm">smart_toy</span>
            <label for="cockpit-model-select" class="text-on-surface font-bold whitespace-nowrap">Model AI thực thi:</label>
            <ModelSelect
              bind:value={selectedModel}
              selectClass="bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-1.5 text-xs text-on-surface font-mono outline-none"
            />
          </div>

          <!-- CLI Console Toggle Switch -->
          <div class="flex items-center gap-2 bg-surface-container-lowest px-3 py-1.5 rounded-lg border border-outline-variant">
            <label for="cockpit-cli-toggle" class="text-[11px] font-bold text-on-surface cursor-pointer select-none">Hiện cửa sổ CMD:</label>
            <input 
              id="cockpit-cli-toggle"
              type="checkbox" 
              bind:checked={showCLIConsole}
              on:change={handleToggleShowCLIConsole}
              class="w-4 h-4 rounded text-primary cursor-pointer"
            />
          </div>
        </div>
        <div class="relative">
          <textarea
            bind:value={promptInput}
            class="w-full bg-surface-container-low/30 h-44 rounded-xl p-4 font-mono text-xs text-on-surface border border-outline-variant focus:ring-2 focus:ring-primary outline-none placeholder:text-on-surface-variant/50 resize-none"
            placeholder="Ví dụ: Xây dựng form đăng nhập theo giao diện đính kèm..."
          ></textarea>

          <!-- Quick Actions & Prompt Templates Pills -->
          <div class="absolute bottom-4 left-4 flex flex-wrap gap-2">
            <button
              type="button"
              on:click={() => addQuickPreset('🛠 Tối ưu hóa & Refactor cấu trúc code sạch sẽ, tuân thủ DRY & SOLID')}
              class="px-2.5 py-1 rounded-lg text-[11px] font-bold bg-primary/10 text-primary border border-primary/20 hover:bg-primary/20 transition-all cursor-pointer shadow-xs">
              🛠 Refactor Clean Code
            </button>
            <button
              type="button"
              on:click={() => addQuickPreset('🔒 Rà soát và vá toàn bộ lỗ hổng bảo mật OWASP, mã hóa dữ liệu sensitive')}
              class="px-2.5 py-1 rounded-lg text-[11px] font-bold bg-amber-500/10 text-amber-600 border border-amber-500/20 hover:bg-amber-500/20 transition-all cursor-pointer shadow-xs">
              🔒 Security Audit
            </button>
            <button
              type="button"
              on:click={() => addQuickPreset('⚡️ Tối ưu hóa hiệu năng, giảm bộ nhớ RAM và thời gian phản hồi')}
              class="px-2.5 py-1 rounded-lg text-[11px] font-bold bg-emerald-500/10 text-emerald-600 border border-emerald-500/20 hover:bg-emerald-500/20 transition-all cursor-pointer shadow-xs">
              ⚡️ Performance Boost
            </button>
            <button
              type="button"
              on:click={() => addQuickPreset('🧪 Viết unit tests và E2E web browser test tự động kiểm thử tính năng')}
              class="px-2.5 py-1 rounded-lg text-[11px] font-bold bg-purple-500/10 text-purple-600 border border-purple-500/20 hover:bg-purple-500/20 transition-all cursor-pointer shadow-xs">
              🧪 Unit & E2E Tests
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
              <span class="text-on-surface font-medium">Trạng thái</span>
              <span class="font-bold {isRunning ? 'text-primary' : 'text-emerald-600'}">{isRunning ? 'Đang chạy...' : 'Sẵn sàng'}</span>
            </div>
            <div class="w-full bg-surface-container-highest h-2 rounded-full overflow-hidden">
              <div class="bg-primary h-full transition-all {isRunning ? 'animate-pulse w-full' : 'w-0'}"></div>
            </div>
          </div>
        </div>

        <!-- Real-time Log Container & CLI Output -->
        <div class="flex-1 font-mono text-xs p-4 overflow-y-auto bg-slate-900 text-slate-100 space-y-3">
          <!-- What the CLI is doing right now. Without this the panel stayed
               empty for the whole run and only filled in at the end, so a prompt
               that takes minutes looked like a button that does nothing. -->
          {#if isRunning}
            <div class="p-3 bg-slate-800/90 border border-slate-700 rounded-lg shadow-inner">
              <div class="text-[10px] text-slate-400 uppercase font-bold mb-1 pb-1 border-b border-slate-700 flex items-center gap-2">
                <span class="material-symbols-outlined text-[13px] animate-spin">progress_activity</span>
                Agent đang chạy ({selectedModel})…
              </div>
              {#if liveLines.length === 0}
                <div class="text-slate-400">Đang khởi động CLI…</div>
              {:else}
                {#each liveLines.slice(-40) as line}
                  <div class="text-slate-300 whitespace-pre-wrap break-all">{line}</div>
                {/each}
              {/if}
            </div>
          {/if}

          {#if cliOutput}
            <div class="p-3 bg-slate-800/90 border border-slate-700 rounded-lg text-emerald-400 whitespace-pre-wrap font-mono text-xs shadow-inner">
              <div class="text-[10px] text-slate-400 uppercase font-bold mb-1 pb-1 border-b border-slate-700">Phản hồi từ Agent ({selectedModel}):</div>
              {cliOutput}
            </div>
          {/if}

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
            {#if !cliOutput}
              <div class="text-slate-500 italic">Sẵn sàng thực thi. Nhập prompt ở Step 2 và bấm 'Chạy Tự Động'.</div>
            {/if}
          {/each}
        </div>
      </div>
    </section>
    </div>
  {:else}
    <!-- History View -->
    <div class="bg-surface-container-lowest rounded-xl border border-outline-variant p-6 space-y-4 shadow-sm">
      <div class="flex justify-between items-center pb-3 border-b border-outline-variant">
        <div class="flex items-center gap-2">
          <span class="material-symbols-outlined text-primary text-xl">history</span>
          <h3 class="font-bold text-sm text-on-surface">Lịch sử thực thi & Log tiến trình ({$logs.length} nhật ký)</h3>
        </div>
        <button type="button" on:click|preventDefault={() => topTab = 'active'} class="px-3 py-1 bg-primary text-on-primary font-bold text-xs rounded-lg hover:opacity-90 transition-all flex items-center gap-1">
          <span class="material-symbols-outlined text-sm">arrow_back</span> Quay lại Active Workspace
        </button>
      </div>

      {#if $logs.length > 0}
        <div class="overflow-x-auto max-h-[500px]">
          <table class="w-full text-left text-xs font-mono">
            <thead class="bg-surface-container-low text-on-surface-variant uppercase text-[10px] sticky top-0">
              <tr>
                <th class="p-3">Thời gian</th>
                <th class="p-3">Trạng thái</th>
                <th class="p-3">Nội dung nhật ký</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-outline-variant">
              {#each $logs as log}
                <tr class="hover:bg-surface-container-low/50">
                  <td class="p-3 whitespace-nowrap text-on-surface-variant font-bold">[{log.time}]</td>
                  <td class="p-3 whitespace-nowrap font-bold">
                    <span class="px-2 py-0.5 rounded text-[10px]
                      {log.level === 'SUCCESS' ? 'bg-emerald-500/20 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30' : ''}
                      {log.level === 'ERROR' ? 'bg-rose-500/20 text-rose-600 dark:text-rose-400 border border-rose-500/30' : ''}
                      {log.level === 'WARN' ? 'bg-amber-500/20 text-amber-600 dark:text-amber-400 border border-amber-500/30' : ''}
                      {log.level === 'THINKING' ? 'bg-purple-500/20 text-purple-600 dark:text-purple-400 border border-purple-500/30' : ''}
                      {log.level === 'SEND' ? 'bg-blue-500/20 text-blue-600 dark:text-blue-400 border border-blue-500/30' : ''}
                    ">[{log.level}]</span>
                  </td>
                  <td class="p-3 text-on-surface font-sans">{log.message}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <div class="py-12 text-center text-on-surface-variant italic">
          <span class="material-symbols-outlined text-4xl mb-2 text-outline">history</span>
          <p>Chưa có nhật ký hoạt động nào được ghi lại trong phiên làm việc này.</p>
        </div>
      {/if}
    </div>
  {/if}
</div>

<!-- Project Tree & File Picker Modal -->
{#if showFileTreeModal}
  <div class="fixed inset-0 bg-black/60 backdrop-blur-xs z-50 flex items-center justify-center p-4">
    <div class="bg-surface-container-lowest border border-outline-variant rounded-2xl w-full max-w-2xl max-h-[85vh] flex flex-col shadow-2xl overflow-hidden">
      <!-- Modal Header -->
      <div class="p-4 border-b border-outline-variant flex items-center justify-between bg-surface-container-low/50">
        <div class="flex items-center gap-2">
          <span class="material-symbols-outlined text-primary text-xl">folder_open</span>
          <div>
            <h3 class="font-bold text-sm text-on-surface">Chọn Files từ Project Tree</h3>
            <p class="text-[11px] text-on-surface-variant">Đã chọn {selectedFiles.length} / {workspaceFiles.length} tệp tin trong Workspace</p>
          </div>
        </div>
        <button
          type="button"
          on:click={() => showFileTreeModal = false}
          class="w-8 h-8 rounded-full hover:bg-surface-container-high flex items-center justify-center text-on-surface-variant cursor-pointer">
          <span class="material-symbols-outlined text-lg">close</span>
        </button>
      </div>

      <!-- Filter & Actions Bar -->
      <div class="p-3 border-b border-outline-variant bg-surface-container-lowest flex items-center gap-3">
        <div class="relative flex-1">
          <span class="material-symbols-outlined absolute left-3 top-2 text-sm text-outline">search</span>
          <input
            type="text"
            bind:value={fileSearchQuery}
            placeholder="Tìm kiếm theo tên file..."
            class="w-full bg-surface-container-low border border-outline-variant rounded-xl pl-9 pr-3 py-1.5 text-xs text-on-surface font-mono outline-none focus:border-primary"
          />
        </div>
        <button
          type="button"
          on:click={selectAllFilteredFiles}
          class="bg-surface-container-highest text-on-surface border border-outline-variant px-3 py-1.5 rounded-xl text-xs font-bold hover:bg-surface-container-high cursor-pointer whitespace-nowrap">
          Chọn tất cả kết quả
        </button>
        <button
          type="button"
          on:click={clearAllFileSelection}
          class="bg-rose-500/10 text-rose-600 border border-rose-500/30 px-3 py-1.5 rounded-xl text-xs font-bold hover:bg-rose-500/20 cursor-pointer whitespace-nowrap">
          Xóa đã chọn
        </button>
      </div>

      <!-- Scrollable File List -->
      <div class="flex-1 p-3 overflow-y-auto space-y-1 font-mono text-xs max-h-[450px]">
        {#each filteredWorkspaceFiles as file}
          {@const isChecked = selectedFiles.includes(file)}
          <button
            type="button"
            on:click={() => toggleFileSelect(file)}
            class="w-full text-left flex items-center gap-3 p-2 rounded-xl transition-all border cursor-pointer select-none
            {isChecked ? 'bg-primary-container/30 border-primary text-on-surface font-bold' : 'border-transparent text-on-surface-variant hover:bg-surface-container-low hover:text-on-surface'}"
          >
            <input
              type="checkbox"
              checked={isChecked}
              on:click|stopPropagation={() => toggleFileSelect(file)}
              class="w-4 h-4 rounded border-outline-variant text-primary focus:ring-0 cursor-pointer"
            />
            <span class="material-symbols-outlined text-sm {isChecked ? 'text-primary' : 'text-amber-500'}">description</span>
            <span class="truncate flex-1">{file}</span>
          </button>
        {:else}
          <div class="py-12 text-center text-on-surface-variant text-xs italic">
            {workspaceFiles.length === 0 ? 'Chưa chọn workspace folder hoặc workspace trống' : 'Không tìm thấy file nào khớp từ khóa'}
          </div>
        {/each}
      </div>

      <!-- Modal Footer -->
      <div class="p-3 border-t border-outline-variant bg-surface-container-low/50 flex justify-end gap-2">
        <button
          type="button"
          on:click={() => showFileTreeModal = false}
          class="bg-primary text-on-primary px-6 py-2 rounded-xl text-xs font-bold hover:opacity-90 transition-all cursor-pointer">
          Hoàn thành ({selectedFiles.length} file đính kèm)
        </button>
      </div>
    </div>
  </div>
{/if}
