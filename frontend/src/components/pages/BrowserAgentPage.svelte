<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import ModelSelect from '../ui/ModelSelect.svelte';
  import { addLog, addToast } from '../../lib/stores/appState';

  interface BrowserActionResult {
    url: string;
    title: string;
    html_snippet: string;
    text_content: string;
    screenshot_base64: string;
    logs: string[];
    success: boolean;
    error: string;
    ai_response?: string;
    status?: string;
    current_step?: number;
    max_steps?: number;
    user_intervention?: string;
  }

  let targetUrl = '';
  let instructionPrompt = '';
  let takeScreenshot = true;
  let isHeadless = true;
  let usePersistentProfile = true;
  let maxSteps = 5;
  let isRunning = false;

  let liveLogs: string[] = [];
  let liveFrame = '';
  let liveFrameStep = 0;

  let roles: string[] = [];
  let selectedRole = 'front-end-agent.md';
  let selectedModel = 'claude-sonnet-4-5';

  let result: BrowserActionResult | null = null;
  let activeTab: 'preview' | 'screenshot' | 'dom' | 'text' | 'ai' = 'preview';

  // ── Agent asks the user mid-run ────────────────────────────────────────
  // The agent goroutine parks on a channel while this form is on screen, so
  // answering it resumes the same run — nothing is torn down and restarted.
  interface AskOption { label: string; value: string; }
  let pendingAsk: { id: string; question: string; options: AskOption[]; allowOther: boolean } | null = null;
  let otherAnswer = '';
  let submittingAsk = false;

  async function submitAsk(value: string) {
    if (!pendingAsk || submittingAsk) return;
    const answer = (value || '').trim();
    if (!answer) return;

    submittingAsk = true;
    try {
      await (AppBindings as any).ResolveBrowserAsk(pendingAsk.id, answer);
      liveLogs = [...liveLogs, `[${new Date().toLocaleTimeString()}] Bạn đã trả lời: "${answer}"`];
      pendingAsk = null;
      otherAnswer = '';
    } catch (e: any) {
      addLog(`Lỗi gửi phản hồi cho Agent: ${e?.message || e}`, 'ERROR');
      addToast(`Lỗi gửi phản hồi cho Agent: ${e?.message || e}`, 'ERROR');
    } finally {
      submittingAsk = false;
    }
  }

  // Cancel functions for the Wails listeners. Registered synchronously and
  // torn down in onDestroy: this page is destroyed on every tab switch, and
  // the old async-onMount registration leaked every visit's closures — each
  // appending to liveLogs of a dead component forever (a cleanup returned
  // from an async onMount is ignored by Svelte, hence onDestroy).
  let offBrowserEvents: (() => void)[] = [];

  onMount(async () => {
    if ((window as any)?.runtime?.EventsOn) {
      const on = (window as any).runtime.EventsOn;
      offBrowserEvents = [
        on('browser_agent_log', (data: any) => {
          if (data?.log) {
            // Bounded like the frame below: an unbounded live log grew with
            // every step of every run for the app's lifetime.
            liveLogs = [...liveLogs, `[${data.time || ''}] ${data.log}`].slice(-500);
          }
        }),
        // A frame per step. Only the latest is kept: this is a live view, not a
        // recording, and holding every frame of a 20-step run in memory as base64
        // PNGs is megabytes for something nobody scrolls back through.
        on('browser_agent_frame', (data: any) => {
          if (!data?.image) return;
          liveFrame = data.image;
          liveFrameStep = data.step || 0;
        }),
        on('browser_ask_user', (data: any) => {
          if (!data?.id) return;
          pendingAsk = {
            id: data.id,
            question: data.question || '',
            options: Array.isArray(data.options) ? data.options : [],
            allowOther: data.allow_other !== false,
          };
          otherAnswer = '';
        }),
        on('browser_ask_close', (data: any) => {
          if (pendingAsk && data?.id === pendingAsk.id) {
            pendingAsk = null;
            otherAnswer = '';
          }
        }),
      ];
    }

    try {
      if ((AppBindings as any).ListRoles) {
        roles = await (AppBindings as any).ListRoles() || [];
        if (roles.length > 0) {
          selectedRole = roles.includes('front-end-agent.md') ? 'front-end-agent.md' : roles[0];
        }
      }
    } catch (e: any) {
      console.warn('Failed to fetch roles:', e);
    }
  });

  onDestroy(() => {
    for (const off of offBrowserEvents) off?.();
    offBrowserEvents = [];
  });

  let isOpeningChrome = false;
  let chromeAgentReady = false;

  // Opens the agent's own Chrome profile so the user can sign in to a site once.
  // The user's personal Chrome is never touched or closed.
  async function handleOpenAgentChrome() {
    isOpeningChrome = true;
    chromeAgentReady = false;
    liveLogs = [`[${new Date().toLocaleTimeString()}] Đang mở cửa sổ Chrome Agent…`];
    try {
      const res = await (AppBindings as any).OpenAgentChromeWindow(targetUrl.trim() || 'https://github.com');
      if (res?.logs?.length) liveLogs = [...liveLogs, ...res.logs.map((l: string) => `[${new Date().toLocaleTimeString()}] ${l}`)];
      if (res?.success) {
        chromeAgentReady = true;
        addLog('Chrome Agent mở ở chế độ đăng nhập (không bật CDP nên Google không chặn). Đăng nhập xong hãy đóng cửa sổ đó rồi bấm "Kích hoạt Agent".', 'SUCCESS');
        addToast('Chrome Agent mở ở chế độ đăng nhập (không bật CDP nên Google không chặn). Đăng nhập xong hãy đóng cửa sổ đó rồi bấm "Kích hoạt Agent".', 'SUCCESS');
      } else {
        addLog(`Mở Chrome Agent thất bại: ${res?.error}`, 'ERROR');
        addToast(`Mở Chrome Agent thất bại: ${res?.error}`, 'ERROR');
      }
    } catch (e: any) {
      addLog(`Lỗi OpenAgentChromeWindow: ${e?.message || e}`, 'ERROR');
      addToast(`Lỗi OpenAgentChromeWindow: ${e?.message || e}`, 'ERROR');
    } finally {
      isOpeningChrome = false;
    }
  }

  async function handleStopBrowserAgent() {
    try {
      if ((AppBindings as any).StopBrowserTask) {
        await (AppBindings as any).StopBrowserTask();
        addLog('Đã phát lệnh Dừng Browser Agent!', 'WARNING');
        addToast('Đã gửi lệnh dừng — agent sẽ dừng ở cuối bước hiện tại.', 'SUCCESS');
      }
    } catch (e: any) {
      addToast(`Không dừng được Browser Agent: ${e}`, 'ERROR');
      addLog(`StopBrowserTask failed: ${e}`, 'ERROR');
    }
  }

  async function handleRunBrowserAgent(overrideUrl?: string) {
    if (isRunning) {
      await handleStopBrowserAgent();
      return;
    }

    const urlToRun = overrideUrl || targetUrl.trim();
    if (!urlToRun) return;
    targetUrl = urlToRun;

    isRunning = true;
    result = null;
    pendingAsk = null;
    otherAnswer = '';
    liveLogs = [`[${new Date().toLocaleTimeString()}] Kích hoạt Browser Agent truy cập ${urlToRun}…`];
    addLog(`Kích hoạt Native Chrome CDP Browser Agent (Model: ${selectedModel}, Profile: ${usePersistentProfile ? "Agent (bền vững)" : "Tạm thời"}, MaxSteps: ${maxSteps}) truy cập ${urlToRun}...`, 'INFO');

    try {
      if ((AppBindings as any).RunBrowserTask) {
        const res = await (AppBindings as any).RunBrowserTask(urlToRun, instructionPrompt, selectedRole, selectedModel, takeScreenshot, isHeadless, usePersistentProfile, maxSteps);
        result = res;
        if (res && res.ai_response) {
          activeTab = 'ai';
        } else {
          activeTab = 'preview';
        }
        if (res && res.success) {
          addLog(`Browser Agent hoàn tất trang "${res.title}"! (Trạng thái: ${res.status || 'OK'})`, 'SUCCESS');
          addToast(`Browser Agent hoàn tất trang "${res.title}"! (Trạng thái: ${res.status || 'OK'})`, 'SUCCESS');
        } else {
          addLog(`Browser Agent thất bại: ${res?.error || 'Lỗi không xác định'}`, 'ERROR');
          addToast(`Browser Agent thất bại: ${res?.error || 'Lỗi không xác định'}`, 'ERROR');
        }
      } else {
        addLog('Wails binding RunBrowserTask chưa sẵn sàng', 'ERROR');
        addToast('Wails binding RunBrowserTask chưa sẵn sàng', 'ERROR');
      }
    } catch (e: any) {
      addLog(`Lỗi thực thi Browser Agent: ${e?.message || e}`, 'ERROR');
      addToast(`Lỗi thực thi Browser Agent: ${e?.message || e}`, 'ERROR');
    } finally {
      isRunning = false;
    }
  }
</script>

<div class="space-y-6 font-sans">
  <!-- Header -->
  <!-- Không còn nút chạy nhanh localhost:5173 / Hacker News: đó là lối tắt của
       người phát triển, vô nghĩa với người dùng thật. -->
  <div class="flex flex-wrap items-end justify-between gap-3">
    <div class="space-y-0.5">
      <div class="flex items-center gap-2">
        <span class="material-symbols-outlined text-lg text-primary">language</span>
        <h1 class="text-lg font-semibold text-on-surface">Browser Agent</h1>
      </div>
      <p class="text-xs text-on-surface-variant">Giao việc cho AI điều khiển Chrome: mở trang, đọc nội dung, điền form, bấm nút.</p>
    </div>
  </div>

  <!-- Single control card: what to do on top, how to do it underneath -->
  <div class="bg-surface-container-lowest border border-outline-variant p-4 rounded-xl space-y-3">
    <div class="relative">
      <span class="material-symbols-outlined absolute left-3.5 top-2.5 text-sm text-outline">link</span>
      <input
        type="text"
        bind:value={targetUrl}
        placeholder="https://github.com/owner/repo/pull/123"
        class="w-full bg-surface-container-low border border-outline-variant rounded-lg pl-10 pr-4 py-2.5 text-xs text-on-surface font-mono outline-none focus:border-primary"
      />
    </div>

    <div class="flex flex-col md:flex-row gap-2">
      <div class="flex-1 relative">
        <span class="material-symbols-outlined absolute left-3.5 top-2.5 text-sm text-outline">psychology</span>
        <input
          type="text"
          bind:value={instructionPrompt}
          placeholder="Bạn muốn agent làm gì trên trang này?"
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg pl-10 pr-4 py-2.5 text-xs text-on-surface outline-none focus:border-primary"
        />
      </div>

      <button
        type="button"
        on:click={() => handleRunBrowserAgent()}
        disabled={!isRunning && !targetUrl.trim()}
        class="px-4 py-2.5 rounded-lg text-xs font-medium transition-colors flex items-center justify-center gap-1.5 cursor-pointer whitespace-nowrap disabled:opacity-50 {isRunning ? 'border border-outline-variant text-error hover:bg-error/10' : 'bg-primary text-on-primary hover:opacity-90'}"
      >
        <span class="material-symbols-outlined text-sm">{isRunning ? 'stop_circle' : 'rocket_launch'}</span>
        {isRunning ? 'Dừng agent' : 'Kích hoạt agent'}
      </button>
    </div>

    <!-- Settings: one quiet row, no nested box -->
    <div class="flex flex-wrap items-center gap-x-4 gap-y-2 pt-3 border-t border-outline-variant/60 text-xs text-on-surface-variant">
      <div class="flex items-center gap-1.5">
        <label for="browser-role-select" class="font-medium">Role</label>
        <select id="browser-role-select" bind:value={selectedRole} class="bg-surface-container-low border border-outline-variant rounded-lg px-2 py-1 text-xs text-on-surface font-mono outline-none focus:border-primary">
          {#each roles as role}
            <option value={role}>{role}</option>
          {/each}
          {#if roles.length === 0}
            <option value="front-end-agent.md">front-end-agent.md</option>
            <option value="agents.md">agents.md</option>
          {/if}
        </select>
      </div>

      <div class="flex items-center gap-1.5">
        <label for="browser-model-select" class="font-medium">Model</label>
        <ModelSelect
          bind:value={selectedModel}
          selectClass="bg-surface-container-low border border-outline-variant rounded-lg px-2 py-1 text-xs text-on-surface font-mono outline-none focus:border-primary"
        />
      </div>

      <div class="flex items-center gap-1.5">
        <label for="browser-max-steps" class="font-medium">Steps</label>
        <input id="browser-max-steps" type="number" min="1" max="20" bind:value={maxSteps} class="w-11 bg-surface-container-low border border-outline-variant rounded-lg px-1.5 py-1 text-center text-xs text-on-surface font-mono outline-none focus:border-primary" />
      </div>

      <label class="flex items-center gap-1.5 cursor-pointer select-none">
        <input type="checkbox" bind:checked={takeScreenshot} class="accent-primary" />
        <span>Chụp màn hình</span>
      </label>

      <!-- Headless is ignored while the persistent profile is on (you must be able
           to see the window to sign in), so don't offer a control that does nothing. -->
      {#if !usePersistentProfile}
        <label class="flex items-center gap-1.5 cursor-pointer select-none">
          <input type="checkbox" bind:checked={isHeadless} class="accent-primary" />
          <span>Chạy ngầm</span>
        </label>
      {/if}

      <label class="flex items-center gap-1.5 cursor-pointer select-none">
        <input type="checkbox" bind:checked={usePersistentProfile} class="accent-primary" />
        <span>Profile riêng (nhớ đăng nhập)</span>
      </label>

      {#if usePersistentProfile}
        <button
          type="button"
          on:click={handleOpenAgentChrome}
          disabled={isOpeningChrome}
          class="ml-auto flex items-center gap-1 text-primary font-medium hover:underline disabled:opacity-50 cursor-pointer">
          <span class="material-symbols-outlined text-sm">open_in_new</span>
          {isOpeningChrome ? 'Đang mở…' : chromeAgentReady ? 'Mở lại cửa sổ Chrome Agent' : 'Đăng nhập trong Chrome Agent'}
        </button>
      {/if}
    </div>
  </div>

  <!-- Real-time Live Log Stream Console -->
  {#if isRunning || liveLogs.length > 0}
    <div class="bg-surface-container-highest border border-outline-variant p-4 rounded-xl space-y-2 text-xs font-mono">
      <div class="flex items-center justify-between font-sans pb-2 border-b border-outline-variant text-on-surface-variant">
        <div class="flex items-center gap-2">
          <span class="w-2 h-2 rounded-full {isRunning ? 'bg-success' : 'bg-outline'}"></span>
          <span class="font-semibold text-xs text-on-surface">Nhật ký thực thi ({liveLogs.length})</span>
        </div>
        {#if isRunning}
          <span class="text-[11px] text-on-surface-variant">Đang chạy</span>
        {/if}
      </div>
      <!-- What the agent is looking at right now. A headless run used to be
           entirely invisible until the single screenshot at the end. -->
      {#if liveFrame}
        <div class="mb-3 rounded-lg overflow-hidden border border-outline-variant">
          <div class="px-2 py-1 bg-surface-container-high text-[11px] font-medium text-on-surface-variant flex items-center justify-between font-sans">
            <span>Màn hình agent — bước {liveFrameStep}</span>
            {#if isRunning}
              <span class="text-success">trực tiếp</span>
            {/if}
          </div>
          <img src={liveFrame} alt="Ảnh màn hình trình duyệt của agent ở bước {liveFrameStep}" class="w-full max-h-72 object-contain bg-surface-container" />
        </div>
      {/if}

      <div class="max-h-48 overflow-y-auto space-y-1 leading-relaxed text-on-surface-variant pr-2">
        {#each liveLogs as log}
          <div class="whitespace-pre-wrap break-all">{log}</div>
        {/each}
      </div>

      <!-- Agent is parked waiting on this answer; it resumes the same run. -->
      {#if pendingAsk}
        <div class="mt-3 bg-warning/10 border border-warning/30 rounded-xl p-3 space-y-3 font-sans">
          <div class="flex items-start gap-2 text-on-surface">
            <span class="material-symbols-outlined text-base flex-shrink-0 text-warning">contact_support</span>
            <div class="space-y-0.5">
              <div class="font-semibold text-xs">Agent đang chờ bạn quyết định</div>
              <p class="text-xs text-on-surface-variant whitespace-pre-wrap leading-relaxed">{pendingAsk.question}</p>
            </div>
          </div>

          {#if pendingAsk.options.length > 0}
            <div class="flex flex-wrap gap-2">
              {#each pendingAsk.options as opt}
                <button
                  type="button"
                  on:click={() => submitAsk(opt.value)}
                  disabled={submittingAsk}
                  class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer">
                  {opt.label}
                </button>
              {/each}
            </div>
          {/if}

          {#if pendingAsk.allowOther}
            <div class="flex items-center gap-2">
              <span class="text-xs font-medium text-on-surface-variant whitespace-nowrap">Khác:</span>
              <input
                type="text"
                bind:value={otherAnswer}
                on:keydown={(e) => { if (e.key === 'Enter') submitAsk(otherAnswer); }}
                placeholder="Nhập chỉ dẫn của bạn, AI sẽ làm theo…"
                disabled={submittingAsk}
                class="flex-1 bg-surface-container-low border border-outline-variant rounded-lg px-3 py-1.5 text-xs text-on-surface outline-none focus:border-primary disabled:opacity-50"
              />
              <button
                type="button"
                on:click={() => submitAsk(otherAnswer)}
                disabled={submittingAsk || !otherAnswer.trim()}
                class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer whitespace-nowrap">
                Gửi
              </button>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  {/if}

  <!-- User Intervention Warning Alert Banner -->
  {#if result && (result.status === 'need_user_intervention' || result.user_intervention)}
    <div class="p-4 bg-warning/10 border border-warning/30 rounded-xl flex items-start gap-3 text-on-surface">
      <span class="material-symbols-outlined text-base flex-shrink-0 text-warning">warning</span>
      <div class="space-y-1 text-xs">
        <div class="font-semibold text-sm">Agent đã dừng — cần bạn can thiệp</div>
        <p class="leading-relaxed text-on-surface-variant">{result.user_intervention || 'AI phát hiện trang web cần đăng nhập, xác thực CAPTCHA/2FA hoặc không tìm thấy nút để tương tác.'}</p>
      </div>
    </div>
  {/if}

  <!-- Results Output Dashboard -->
  {#if result}
    <div class="space-y-4">
      <!-- Status Summary Card -->
      <div class="bg-surface-container-low border border-outline-variant p-4 rounded-xl flex flex-wrap items-center justify-between gap-3 text-xs">
        <div class="space-y-1">
          <div class="font-semibold text-on-surface text-sm flex items-center gap-2">
            {result.title || 'Trang không có tiêu đề'}
            <!-- Trạng thái thật của lần chạy. Trước đây badge ghi cứng "200 OK"
                 dù component không hề nhận HTTP status nào từ backend. -->
            {#if result.success}
              <span class="text-[11px] bg-success/10 text-success border border-success/30 px-2 py-0.5 rounded-full font-medium">Thành công</span>
            {:else}
              <span class="text-[11px] bg-error/10 text-error border border-error/30 px-2 py-0.5 rounded-full font-medium">Thất bại</span>
            {/if}
          </div>
          <div class="font-mono text-on-surface-variant text-[11px]">{result.url}</div>
        </div>

        <div class="flex gap-1.5 bg-surface-container-lowest p-1 rounded-lg border border-outline-variant">
          {#if result.ai_response}
            <button
              type="button"
              on:click={() => activeTab = 'ai'}
              class="px-3 py-1 rounded-lg text-xs font-medium transition-colors cursor-pointer flex items-center gap-1.5 {activeTab === 'ai' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}">
              <span class="material-symbols-outlined text-sm">psychology</span> Phản hồi AI
            </button>
          {/if}
          <button
            type="button"
            on:click={() => activeTab = 'preview'}
            class="px-3 py-1 rounded-lg text-xs font-medium transition-colors cursor-pointer flex items-center gap-1.5 {activeTab === 'preview' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}">
            <span class="material-symbols-outlined text-sm">terminal</span> Nhật ký ({result.logs.length})
          </button>
          {#if result.screenshot_base64}
            <button
              type="button"
              on:click={() => activeTab = 'screenshot'}
              class="px-3 py-1 rounded-lg text-xs font-medium transition-colors cursor-pointer flex items-center gap-1.5 {activeTab === 'screenshot' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}">
              <span class="material-symbols-outlined text-sm">image</span> Ảnh màn hình
            </button>
          {/if}
          <button
            type="button"
            on:click={() => activeTab = 'dom'}
            class="px-3 py-1 rounded-lg text-xs font-medium transition-colors cursor-pointer flex items-center gap-1.5 {activeTab === 'dom' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}">
            <span class="material-symbols-outlined text-sm">code</span> DOM
          </button>
          <button
            type="button"
            on:click={() => activeTab = 'text'}
            class="px-3 py-1 rounded-lg text-xs font-medium transition-colors cursor-pointer flex items-center gap-1.5 {activeTab === 'text' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}">
            <span class="material-symbols-outlined text-sm">notes</span> Văn bản
          </button>
        </div>
      </div>

      <!-- Tab Content Area -->
      <div class="bg-surface-container-lowest border border-outline-variant p-4 rounded-xl">
        {#if activeTab === 'ai'}
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <div class="text-sm font-semibold text-on-surface flex items-center gap-2">
                <span class="material-symbols-outlined text-on-surface-variant text-base">psychology</span>
                Phản hồi và phân tích của AI ({selectedModel})
              </div>
              <span class="text-[11px] bg-surface-container-high text-on-surface-variant border border-outline-variant px-2 py-0.5 rounded-full font-mono">{selectedRole}</span>
            </div>
            <div class="bg-surface-container-low p-4 rounded-xl border border-outline-variant text-on-surface text-xs whitespace-pre-wrap leading-relaxed font-sans">
              {result.ai_response}
            </div>
          </div>
        {:else if activeTab === 'preview'}
          <div class="space-y-2">
            <div class="text-sm font-semibold text-on-surface mb-2">Dòng thời gian thực thi</div>
            <!-- Danh sách dòng mono, không bọc mỗi dòng log trong một khung
                 riêng: hàng chục khung viền cho hàng chục dòng chỉ tạo nhiễu. -->
            <div class="bg-surface-container-highest border border-outline-variant rounded-xl p-3 font-mono text-xs text-on-surface-variant space-y-1 max-h-96 overflow-y-auto">
              {#each result.logs as log}
                <div class="leading-relaxed whitespace-pre-wrap break-all">{log}</div>
              {/each}
            </div>
          </div>
        {:else if activeTab === 'screenshot'}
          <div class="space-y-2">
            <div class="text-sm font-semibold text-on-surface mb-2">Ảnh màn hình toàn trang</div>
            <div class="border border-outline-variant rounded-xl overflow-hidden max-h-[600px] overflow-y-auto bg-surface-container p-2">
              <img src={result.screenshot_base64} alt="Ảnh màn hình do Browser Agent chụp" class="w-full h-auto rounded-lg" />
            </div>
          </div>
        {:else if activeTab === 'dom'}
          <div class="space-y-2">
            <div class="text-sm font-semibold text-on-surface mb-2">Cấu trúc DOM trích xuất</div>
            <textarea readonly class="w-full bg-surface-container-highest text-on-surface p-4 rounded-xl font-mono text-xs h-96 border border-outline-variant leading-relaxed whitespace-pre font-normal">{result.html_snippet}</textarea>
          </div>
        {:else if activeTab === 'text'}
          <div class="space-y-2">
            <div class="text-sm font-semibold text-on-surface mb-2">Nội dung văn bản của trang</div>
            <textarea readonly class="w-full bg-surface-container-low text-on-surface p-4 rounded-xl font-mono text-xs h-96 border border-outline-variant leading-relaxed whitespace-pre font-normal">{result.text_content}</textarea>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
