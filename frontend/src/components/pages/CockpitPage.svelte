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
  // Một dải số liệu trung tính: chỉ Failed đổi màu, và chỉ khi thật sự có task
  // hỏng. Sáu ô sáu màu thì không màu nào còn là màu nhấn.
  $: metricCards = [
    { label: 'Running', value: mRunning, icon: 'bolt', cls: 'text-on-surface' },
    { label: 'Queue', value: mQueued, icon: 'hourglass_empty', cls: 'text-on-surface' },
    { label: 'Done', value: mDone, icon: 'check_circle', cls: 'text-on-surface' },
    { label: 'Failed', value: mFailed, icon: 'cancel', cls: mFailed > 0 ? 'text-error' : 'text-on-surface' },
    { label: 'Agents Active', value: mAgentsActive, icon: 'smart_toy', cls: 'text-on-surface' },
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

  // Modal chỉ đóng được bằng hai nút là lối ra quá hẹp: Escape và bấm ra nền
  // là thứ người dùng thử trước tiên.
  function handleWindowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && showFileTreeModal) showFileTreeModal = false;
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
    addLog(`Bắt đầu chạy: "${promptInput.slice(0, 40)}…"`, 'SEND');

    try {
      const res = await AppBindings.RunQuickCLI(promptInput, selectedModel, '', selectedFiles);
      if (res && res.success) {
        cliOutput = res.output;
        addLog(`Chạy xong sau ${res.duration_sec.toFixed(1)}s`, 'SUCCESS');
        addToast(`Chạy xong sau ${res.duration_sec.toFixed(1)}s.`, 'SUCCESS');
      } else if (res) {
        cliOutput = 'Lỗi thực thi: ' + res.error;
        addLog(`Thực thi thất bại: ${res.error}`, 'ERROR');
        addToast(`Thực thi thất bại: ${res.error}`, 'ERROR');
      }
    } catch (e) {
      cliOutput = 'Lỗi kết nối: ' + e;
      addLog(`Không kết nối được CLI: ${e}`, 'ERROR');
      addToast(`Không kết nối được CLI: ${e}`, 'ERROR');
    } finally {
      isRunning = false;
    }
  }

  async function handleDecomposePlan() {
    if (!promptInput.trim()) return;
    addLog('Đang phân rã yêu cầu thành các task…', 'THINKING');
    try {
      await AppBindings.DecomposePlan(promptInput);
      addLog('Đã phân rã kế hoạch — xem ở Task Board.', 'SUCCESS');
      addToast('Đã phân rã kế hoạch — xem ở Task Board.', 'SUCCESS');
    } catch (e) {
      addLog(`Phân rã kế hoạch thất bại: ${e}`, 'ERROR');
      addToast(`Phân rã kế hoạch thất bại: ${e}`, 'ERROR');
    }
  }
</script>

<div class="space-y-6 max-w-7xl mx-auto pb-12">
  <!-- Dashboard Header -->
  <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
    <div>
      <h1 class="text-lg font-semibold flex items-center gap-2 text-on-surface">
        <span class="material-symbols-outlined text-lg text-on-surface-variant">rocket_launch</span>
        Cockpit
      </h1>
      <p class="text-on-surface-variant text-xs mt-1">Điều phối agent, task và code trong cùng một không gian làm việc.</p>
    </div>
    <div class="bg-surface-container-high p-1 rounded-lg flex gap-1 border border-outline-variant">
      <button
        type="button"
        on:click={() => (topTab = 'active')}
        class="px-4 py-1.5 rounded-lg text-xs font-medium flex items-center gap-1 transition-colors cursor-pointer
        {topTab === 'active' ? 'bg-surface-container-lowest text-on-surface' : 'text-on-surface-variant hover:text-on-surface'}">
        <span class="material-symbols-outlined text-sm">bolt</span> Đang hoạt động
      </button>
      <button
        type="button"
        on:click={() => (topTab = 'history')}
        class="px-4 py-1.5 rounded-lg text-xs font-medium flex items-center gap-1 transition-colors cursor-pointer
        {topTab === 'history' ? 'bg-surface-container-lowest text-on-surface' : 'text-on-surface-variant hover:text-on-surface'}">
        <span class="material-symbols-outlined text-sm">history</span> Lịch sử ({$logs.length})
      </button>
    </div>
  </div>

  <!-- Aggregated live metrics -->
  <div class="grid grid-cols-3 md:grid-cols-6 gap-3">
    {#each metricCards as m (m.label)}
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-3 flex flex-col gap-1">
        <div class="flex items-center gap-1.5 text-[11px] font-medium text-on-surface-variant">
          <span class="material-symbols-outlined text-sm text-on-surface-variant">{m.icon}</span> {m.label}
        </div>
        <span class="text-2xl font-semibold {m.cls}">{m.value}</span>
      </div>
    {/each}
  </div>
  <div class="flex items-center gap-2 text-xs -mt-2">
    <span class="w-2 h-2 rounded-full {$orchestratorRunning ? 'bg-primary' : 'bg-outline'}"></span>
    <span class="font-medium {$orchestratorRunning ? 'text-primary' : 'text-on-surface-variant'}">
      Orchestrator: {$orchestratorRunning ? 'đang chạy' : 'sẵn sàng'}
    </span>
    <span class="text-on-surface-variant">· Workspace: {$workspaceFolder || 'chưa chọn'}</span>
  </div>

  {#if topTab === 'active'}
    <!-- 3-Step Linear Workflow -->
    <div class="space-y-6">

    <!-- Step 1: Context -->
    <section class="bg-surface-container-lowest rounded-xl border border-outline-variant overflow-hidden">
      <div class="p-4 border-b border-outline-variant bg-surface-container-low/50 flex justify-between items-center">
        <div class="flex items-center gap-2">
          <span class="w-6 h-6 rounded-full bg-primary-container text-on-primary-container flex items-center justify-center font-medium text-xs">1</span>
          <h3 class="font-semibold text-sm text-on-surface">Bước 1: Đính kèm tài liệu (context)</h3>
        </div>
        <span class="text-xs text-on-surface-variant">Đã đính kèm: {selectedFiles.length} tệp</span>
      </div>
      <!-- Ô "kéo thả file vào đây" cũ chỉ là hình trang trí: không có handler
           drop/click nào cả. Đã bỏ, để nút chọn file chiếm trọn chiều ngang. -->
      <div class="p-4 space-y-3 bg-surface-container-lowest">
        <div class="flex items-center justify-between">
          <button
            type="button"
            on:click={() => showFileTreeModal = true}
            class="flex items-center gap-2 border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg hover:bg-surface-container-high transition-colors text-xs font-medium cursor-pointer">
            <span class="material-symbols-outlined text-sm">account_tree</span>
            Chọn file từ Project Tree ({workspaceFiles.length} tệp)
          </button>

          {#if selectedFiles.length > 0}
            <button
              type="button"
              on:click={clearAllFileSelection}
              class="text-xs text-error font-medium hover:underline cursor-pointer">
              Bỏ chọn tất cả ({selectedFiles.length})
            </button>
          {/if}
        </div>

        <!-- Selected Files Badges / Preview List -->
        <div class="bg-surface-container-low/40 rounded-xl p-3 h-36 overflow-y-auto space-y-1.5 border border-outline-variant/60 font-mono text-xs">
          {#each selectedFiles as file}
            <div class="flex items-center justify-between bg-surface-container-lowest border border-outline-variant/80 px-2.5 py-1 rounded-lg text-on-surface">
              <div class="flex items-center gap-2 truncate">
                <span class="material-symbols-outlined text-sm text-on-surface-variant">check_circle</span>
                <span class="truncate">{file}</span>
              </div>
              <button
                type="button"
                on:click={() => toggleFileSelect(file)}
                class="text-on-surface-variant hover:text-error p-0.5 rounded-lg cursor-pointer">
                <span class="material-symbols-outlined text-sm">close</span>
              </button>
            </div>
          {:else}
            <div class="text-on-surface-variant text-xs p-4 text-center font-sans">
              Chưa đính kèm tệp nào. Bấm "Chọn file từ Project Tree" để thêm ngữ cảnh.
            </div>
          {/each}
        </div>
      </div>
    </section>

    <!-- Step 2: Input Prompt -->
    <section class="bg-surface-container-lowest rounded-xl border border-outline-variant overflow-hidden">
      <div class="p-4 border-b border-outline-variant bg-surface-container-low/50 flex justify-between items-center">
        <div class="flex items-center gap-2">
          <span class="w-6 h-6 rounded-full bg-primary-container text-on-primary-container flex items-center justify-center font-medium text-xs">2</span>
          <h3 class="font-semibold text-sm text-on-surface">Bước 2: Nhập yêu cầu (prompt)</h3>
        </div>
      </div>
      <div class="p-4 space-y-4 bg-surface-container-lowest">
        <!-- Model Selection & CLI Console Toggle -->
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs bg-surface-container-low/40 p-3 rounded-xl border border-outline-variant/60">
          <div class="flex items-center gap-3">
            <span class="material-symbols-outlined text-on-surface-variant text-sm">smart_toy</span>
            <label for="cockpit-model-select" class="text-on-surface-variant font-medium whitespace-nowrap">Model AI thực thi</label>
            <ModelSelect
              bind:value={selectedModel}
              selectClass="bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-1.5 text-xs text-on-surface font-mono focus:border-primary"
            />
          </div>

          <!-- CLI Console Toggle Switch -->
          <div class="flex items-center gap-2 bg-surface-container-lowest px-3 py-1.5 rounded-lg border border-outline-variant">
            <label for="cockpit-cli-toggle" class="text-xs font-medium text-on-surface-variant cursor-pointer select-none">Hiện cửa sổ CMD</label>
            <input
              id="cockpit-cli-toggle"
              type="checkbox"
              bind:checked={showCLIConsole}
              on:change={handleToggleShowCLIConsole}
              class="w-4 h-4 accent-primary cursor-pointer"
            />
          </div>
        </div>
        <div class="relative">
          <textarea
            bind:value={promptInput}
            class="w-full bg-surface-container-low/30 h-44 rounded-xl p-4 font-mono text-xs text-on-surface border border-outline-variant focus:border-primary placeholder:text-on-surface-variant/50 resize-none"
            placeholder="Ví dụ: Xây dựng form đăng nhập theo giao diện đính kèm…"
          ></textarea>

          <!-- Quick Actions & Prompt Templates Pills -->
          <!-- Bốn preset ngang hàng nhau nên dùng chung một kiểu chip trung
               tính; emoji cũng bị bỏ khỏi cả nhãn lẫn chuỗi chèn vào prompt. -->
          <div class="absolute bottom-4 left-4 flex flex-wrap gap-2">
            <button
              type="button"
              on:click={() => addQuickPreset('Tối ưu hóa và refactor cấu trúc code cho sạch, tuân thủ DRY và SOLID')}
              class="px-2.5 py-1 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant bg-surface-container-lowest hover:bg-surface-container-high transition-colors cursor-pointer">
              Refactor code
            </button>
            <button
              type="button"
              on:click={() => addQuickPreset('Rà soát và vá lỗ hổng bảo mật theo OWASP, mã hóa dữ liệu nhạy cảm')}
              class="px-2.5 py-1 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant bg-surface-container-lowest hover:bg-surface-container-high transition-colors cursor-pointer">
              Rà soát bảo mật
            </button>
            <button
              type="button"
              on:click={() => addQuickPreset('Tối ưu hiệu năng, giảm bộ nhớ RAM và thời gian phản hồi')}
              class="px-2.5 py-1 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant bg-surface-container-lowest hover:bg-surface-container-high transition-colors cursor-pointer">
              Tối ưu hiệu năng
            </button>
            <button
              type="button"
              on:click={() => addQuickPreset('Viết unit test và E2E test tự động cho các tính năng hiện có')}
              class="px-2.5 py-1 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant bg-surface-container-lowest hover:bg-surface-container-high transition-colors cursor-pointer">
              Viết test
            </button>
          </div>
        </div>

        <div class="flex justify-end gap-2">
          <button type="button" on:click={handleDecomposePlan} class="px-3 py-1.5 rounded-lg border border-outline-variant font-medium text-xs text-on-surface-variant hover:bg-surface-container-high transition-colors flex items-center gap-1.5 cursor-pointer">
            <span class="material-symbols-outlined text-sm">analytics</span>
            Phân rã kế hoạch
          </button>

          <button type="button" on:click={handleRunAuto} disabled={isRunning} class="px-3 py-1.5 rounded-lg bg-primary text-on-primary font-medium text-xs hover:opacity-90 transition-opacity flex items-center gap-1.5 disabled:opacity-50 cursor-pointer">
            <span class="material-symbols-outlined text-sm {isRunning ? 'animate-spin' : ''}">{isRunning ? 'progress_activity' : 'play_arrow'}</span>
            {isRunning ? 'Đang chạy…' : 'Chạy tự động'}
          </button>
        </div>
      </div>
    </section>

    <!-- Step 3: Live Progress -->
    <section class="bg-surface-container-lowest rounded-xl border border-outline-variant overflow-hidden">
      <div class="p-4 border-b border-outline-variant bg-surface-container-low/50 flex justify-between items-center">
        <div class="flex items-center gap-2">
          <span class="w-6 h-6 rounded-full bg-primary-container text-on-primary-container flex items-center justify-center font-medium text-xs">3</span>
          <h3 class="font-semibold text-sm text-on-surface">Bước 3: Tiến trình trực tiếp</h3>
        </div>
      </div>
      <div class="flex h-[360px] bg-surface-container-lowest">
        <!-- Sidebar Status -->
        <div class="w-1/4 border-r border-outline-variant p-4 space-y-4 bg-surface-container-low/30">
          <div>
            <p class="text-[11px] font-medium text-on-surface-variant mb-2">Agent</p>
            <!-- Trạng thái bám theo isRunning: trước đây chấm luôn nhấp nháy và
                 luôn ghi ACTIVE, kể cả khi không có gì đang chạy. -->
            <div class="flex items-center gap-2 mb-4">
              <div class="w-2 h-2 rounded-full {isRunning ? 'bg-primary' : 'bg-outline'}"></div>
              <span class="font-mono text-xs {isRunning ? 'text-primary' : 'text-on-surface-variant'}">
                {activeAgent} · {isRunning ? 'đang chạy' : 'chờ lệnh'}
              </span>
            </div>
          </div>

          <div class="space-y-3">
            <div class="flex justify-between text-xs">
              <span class="text-on-surface font-medium">Trạng thái</span>
              <span class="font-medium {isRunning ? 'text-primary' : 'text-on-surface-variant'}">{isRunning ? 'Đang chạy…' : 'Sẵn sàng'}</span>
            </div>
          </div>
        </div>

        <!-- Real-time Log Container & CLI Output -->
        <div class="flex-1 font-mono text-xs p-4 overflow-y-auto bg-surface-container-highest text-on-surface space-y-3">
          <!-- What the CLI is doing right now. Without this the panel stayed
               empty for the whole run and only filled in at the end, so a prompt
               that takes minutes looked like a button that does nothing. -->
          {#if isRunning}
            <div class="p-3 bg-surface-container-high border border-outline-variant rounded-lg">
              <div class="text-[11px] text-on-surface-variant font-medium mb-1 pb-1 border-b border-outline-variant flex items-center gap-2 font-sans">
                <span class="material-symbols-outlined text-sm animate-spin">progress_activity</span>
                Agent đang chạy ({selectedModel})…
              </div>
              {#if liveLines.length === 0}
                <div class="text-on-surface-variant">Đang khởi động CLI…</div>
              {:else}
                {#each liveLines.slice(-40) as line}
                  <div class="text-on-surface-variant whitespace-pre-wrap break-all">{line}</div>
                {/each}
              {/if}
            </div>
          {/if}

          {#if cliOutput}
            <div class="p-3 bg-surface-container-high border border-outline-variant rounded-lg text-on-surface whitespace-pre-wrap font-mono text-xs">
              <div class="text-[11px] text-on-surface-variant font-medium mb-1 pb-1 border-b border-outline-variant font-sans">Phản hồi từ agent ({selectedModel})</div>
              {cliOutput}
            </div>
          {/if}

          <!-- Chỉ ERROR/WARN/SUCCESS có màu ngữ nghĩa; các mức còn lại giữ trung
               tính để bảng log không thành cầu vồng. -->
          {#each $logs as log}
            <div class="flex gap-2">
              <span class="text-on-surface-variant">[{log.time}]</span>
              <span class="font-medium
                {log.level === 'SUCCESS' ? 'text-success' : ''}
                {log.level === 'ERROR' ? 'text-error' : ''}
                {log.level === 'WARN' ? 'text-warning' : ''}
                {log.level !== 'SUCCESS' && log.level !== 'ERROR' && log.level !== 'WARN' ? 'text-on-surface-variant' : ''}
              ">[{log.level}]</span>
              <span class="text-on-surface">{log.message}</span>
            </div>
          {:else}
            {#if !cliOutput}
              <div class="text-on-surface-variant">Sẵn sàng thực thi. Nhập prompt ở bước 2 rồi bấm "Chạy tự động".</div>
            {/if}
          {/each}
        </div>
      </div>
    </section>
    </div>
  {:else}
    <!-- History View -->
    <div class="bg-surface-container-lowest rounded-xl border border-outline-variant p-6 space-y-4">
      <div class="flex justify-between items-center pb-3 border-b border-outline-variant">
        <div class="flex items-center gap-2">
          <span class="material-symbols-outlined text-on-surface-variant text-base">history</span>
          <h3 class="font-semibold text-sm text-on-surface">Lịch sử thực thi ({$logs.length} nhật ký)</h3>
        </div>
        <button type="button" on:click|preventDefault={() => topTab = 'active'} class="px-3 py-1.5 border border-outline-variant text-on-surface-variant font-medium text-xs rounded-lg hover:bg-surface-container-high transition-colors flex items-center gap-1.5 cursor-pointer">
          <span class="material-symbols-outlined text-sm">arrow_back</span> Quay lại
        </button>
      </div>

      {#if $logs.length > 0}
        <div class="overflow-x-auto max-h-[500px]">
          <table class="w-full text-left text-xs">
            <thead class="bg-surface-container-low text-on-surface-variant text-xs sticky top-0">
              <tr>
                <th class="p-3 font-medium">Thời gian</th>
                <th class="p-3 font-medium">Trạng thái</th>
                <th class="p-3 font-medium">Nội dung nhật ký</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-outline-variant">
              {#each $logs as log}
                <tr class="hover:bg-surface-container-low/50">
                  <td class="p-3 whitespace-nowrap text-on-surface-variant font-mono">{log.time}</td>
                  <td class="p-3 whitespace-nowrap font-medium
                    {log.level === 'SUCCESS' ? 'text-success' : ''}
                    {log.level === 'ERROR' ? 'text-error' : ''}
                    {log.level === 'WARN' ? 'text-warning' : ''}
                    {log.level !== 'SUCCESS' && log.level !== 'ERROR' && log.level !== 'WARN' ? 'text-on-surface-variant' : ''}
                  ">{log.level}</td>
                  <td class="p-3 text-on-surface">{log.message}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <div class="py-12 text-center text-on-surface-variant space-y-2">
          <span class="material-symbols-outlined text-lg text-outline block">history</span>
          <p class="text-xs">Chưa ghi được nhật ký nào trong phiên làm việc này.</p>
        </div>
      {/if}
    </div>
  {/if}
</div>

<!-- Project Tree & File Picker Modal -->
<svelte:window on:keydown={handleWindowKeydown} />
{#if showFileTreeModal}
  <div class="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
    <!-- Nền là một nút thật, nên bấm ra ngoài đóng được modal mà không cần
         gắn click vào thẻ div (và không đẻ ra cảnh báo a11y). -->
    <button
      type="button"
      aria-label="Đóng hộp thoại chọn file"
      class="absolute inset-0 cursor-default"
      on:click={() => showFileTreeModal = false}
    ></button>
    <div class="relative bg-surface-container-lowest border border-outline-variant rounded-xl w-full max-w-2xl max-h-[85vh] flex flex-col shadow-lg overflow-hidden">
      <!-- Modal Header -->
      <div class="p-4 border-b border-outline-variant flex items-center justify-between bg-surface-container-low/50">
        <div class="flex items-center gap-2">
          <span class="material-symbols-outlined text-on-surface-variant text-base">folder_open</span>
          <div>
            <h3 class="font-semibold text-sm text-on-surface">Chọn file từ Project Tree</h3>
            <p class="text-[11px] text-on-surface-variant">Đã chọn {selectedFiles.length} / {workspaceFiles.length} tệp trong workspace</p>
          </div>
        </div>
        <button
          type="button"
          on:click={() => showFileTreeModal = false}
          class="w-8 h-8 rounded-lg hover:bg-surface-container-high flex items-center justify-center text-on-surface-variant cursor-pointer">
          <span class="material-symbols-outlined text-base">close</span>
        </button>
      </div>

      <!-- Filter & Actions Bar -->
      <div class="p-3 border-b border-outline-variant bg-surface-container-lowest flex items-center gap-3">
        <div class="relative flex-1">
          <span class="material-symbols-outlined absolute left-3 top-2 text-sm text-outline">search</span>
          <input
            type="text"
            bind:value={fileSearchQuery}
            placeholder="Tìm theo tên file…"
            class="w-full bg-surface-container-low border border-outline-variant rounded-lg pl-9 pr-3 py-1.5 text-xs text-on-surface font-mono outline-none focus:border-primary"
          />
        </div>
        <button
          type="button"
          on:click={selectAllFilteredFiles}
          class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors cursor-pointer whitespace-nowrap">
          Chọn tất cả kết quả
        </button>
        <button
          type="button"
          on:click={clearAllFileSelection}
          class="border border-outline-variant text-error px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-error/10 transition-colors cursor-pointer whitespace-nowrap">
          Bỏ chọn hết
        </button>
      </div>

      <!-- Scrollable File List -->
      <div class="flex-1 p-3 overflow-y-auto space-y-1 font-mono text-xs max-h-[450px]">
        {#each filteredWorkspaceFiles as file}
          {@const isChecked = selectedFiles.includes(file)}
          <button
            type="button"
            on:click={() => toggleFileSelect(file)}
            class="w-full text-left flex items-center gap-3 p-2 rounded-lg transition-colors border cursor-pointer select-none
            {isChecked ? 'bg-primary-container/30 border-primary text-on-surface font-medium' : 'border-transparent text-on-surface-variant hover:bg-surface-container-low hover:text-on-surface'}"
          >
            <input
              type="checkbox"
              checked={isChecked}
              on:click|stopPropagation={() => toggleFileSelect(file)}
              class="w-4 h-4 accent-primary cursor-pointer"
            />
            <span class="material-symbols-outlined text-sm {isChecked ? 'text-primary' : 'text-on-surface-variant'}">description</span>
            <span class="truncate flex-1">{file}</span>
          </button>
        {:else}
          <div class="py-12 text-center text-on-surface-variant text-xs font-sans">
            {workspaceFiles.length === 0 ? 'Chưa chọn workspace folder, hoặc workspace trống.' : 'Không có file nào khớp từ khoá.'}
          </div>
        {/each}
      </div>

      <!-- Modal Footer -->
      <div class="p-3 border-t border-outline-variant bg-surface-container-low/50 flex justify-end gap-2">
        <button
          type="button"
          on:click={() => showFileTreeModal = false}
          class="bg-primary text-on-primary px-4 py-2 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity cursor-pointer">
          Xong ({selectedFiles.length} tệp đính kèm)
        </button>
      </div>
    </div>
  </div>
{/if}
