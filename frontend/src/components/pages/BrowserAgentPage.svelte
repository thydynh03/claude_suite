<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog } from '../../lib/stores/appState';

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
      liveLogs = [...liveLogs, `[${new Date().toLocaleTimeString()}] 💬 Bạn đã trả lời: "${answer}"`];
      pendingAsk = null;
      otherAnswer = '';
    } catch (e: any) {
      addLog(`Lỗi gửi phản hồi cho Agent: ${e?.message || e}`, 'ERROR');
    } finally {
      submittingAsk = false;
    }
  }

  onMount(async () => {
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

    if ((window as any)?.runtime?.EventsOn) {
      (window as any).runtime.EventsOn('browser_agent_log', (data: any) => {
        if (data?.log) {
          liveLogs = [...liveLogs, `[${data.time || ''}] ${data.log}`];
        }
      });

      (window as any).runtime.EventsOn('browser_ask_user', (data: any) => {
        if (!data?.id) return;
        pendingAsk = {
          id: data.id,
          question: data.question || '',
          options: Array.isArray(data.options) ? data.options : [],
          allowOther: data.allow_other !== false,
        };
        otherAnswer = '';
      });

      (window as any).runtime.EventsOn('browser_ask_close', (data: any) => {
        if (pendingAsk && data?.id === pendingAsk.id) {
          pendingAsk = null;
          otherAnswer = '';
        }
      });
    }
  });

  let isOpeningChrome = false;
  let chromeAgentReady = false;

  // Opens the agent's own Chrome profile so the user can sign in to a site once.
  // The user's personal Chrome is never touched or closed.
  async function handleOpenAgentChrome() {
    isOpeningChrome = true;
    chromeAgentReady = false;
    liveLogs = [`[${new Date().toLocaleTimeString()}] 🌐 Đang mở cửa sổ Chrome Agent...`];
    try {
      const res = await (AppBindings as any).OpenAgentChromeWindow(targetUrl.trim() || 'https://github.com');
      if (res?.logs?.length) liveLogs = [...liveLogs, ...res.logs.map((l: string) => `[${new Date().toLocaleTimeString()}] ${l}`)];
      if (res?.success) {
        chromeAgentReady = true;
        addLog('✅ Chrome Agent đã mở. Đăng nhập các trang bạn cần — profile sẽ ghi nhớ cho lần sau.', 'SUCCESS');
      } else {
        addLog(`❌ Mở Chrome Agent thất bại: ${res?.error}`, 'ERROR');
      }
    } catch (e: any) {
      addLog(`Lỗi OpenAgentChromeWindow: ${e?.message || e}`, 'ERROR');
    } finally {
      isOpeningChrome = false;
    }
  }

  async function handleStopBrowserAgent() {
    try {
      if ((AppBindings as any).StopBrowserTask) {
        await (AppBindings as any).StopBrowserTask();
        addLog('Đã phát lệnh Dừng Browser Agent!', 'WARNING');
      }
    } catch (e: any) {
      console.error('Lỗi khi dừng Browser Agent:', e);
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
    liveLogs = [`[${new Date().toLocaleTimeString()}] 🚀 Kích hoạt Browser Agent truy cập ${urlToRun}...`];
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
        } else {
          addLog(`Browser Agent thất bại: ${res?.error || 'Lỗi không xác định'}`, 'ERROR');
        }
      } else {
        addLog('Wails binding RunBrowserTask chưa sẵn sàng', 'ERROR');
      }
    } catch (e: any) {
      addLog(`Lỗi thực thi Browser Agent: ${e?.message || e}`, 'ERROR');
    } finally {
      isRunning = false;
    }
  }
</script>

<div class="space-y-6 font-sans">
  <!-- Header -->
  <div class="flex flex-wrap items-end justify-between gap-3">
    <div class="space-y-0.5">
      <div class="flex items-center gap-2">
        <span class="material-symbols-outlined text-xl text-primary">language</span>
        <h1 class="text-lg font-bold text-on-surface">Browser Agent</h1>
      </div>
      <p class="text-xs text-on-surface-variant">Giao việc cho AI điều khiển Chrome: mở trang, đọc nội dung, điền form, bấm nút.</p>
    </div>

    <div class="flex flex-wrap gap-2">
      <button
        type="button"
        on:click={() => handleRunBrowserAgent('http://localhost:5173')}
        disabled={isRunning}
        class="text-on-surface-variant border border-outline-variant px-2.5 py-1 rounded-lg text-[11px] font-semibold hover:bg-surface-container-high disabled:opacity-40 cursor-pointer transition-all">
        localhost:5173
      </button>
      <button
        type="button"
        on:click={() => handleRunBrowserAgent('https://news.ycombinator.com')}
        disabled={isRunning}
        class="text-on-surface-variant border border-outline-variant px-2.5 py-1 rounded-lg text-[11px] font-semibold hover:bg-surface-container-high disabled:opacity-40 cursor-pointer transition-all">
        Hacker News
      </button>
    </div>
  </div>

  <!-- Single control card: what to do on top, how to do it underneath -->
  <div class="bg-surface-container-lowest border border-outline-variant p-4 rounded-2xl space-y-3">
    <div class="relative">
      <span class="material-symbols-outlined absolute left-3.5 top-2.5 text-sm text-outline">link</span>
      <input
        type="text"
        bind:value={targetUrl}
        placeholder="https://github.com/owner/repo/pull/123"
        class="w-full bg-surface-container-low border border-outline-variant rounded-xl pl-10 pr-4 py-2.5 text-xs text-on-surface font-mono outline-none focus:border-primary"
      />
    </div>

    <div class="flex flex-col md:flex-row gap-2">
      <div class="flex-1 relative">
        <span class="material-symbols-outlined absolute left-3.5 top-2.5 text-sm text-outline">psychology</span>
        <input
          type="text"
          bind:value={instructionPrompt}
          placeholder="Bạn muốn agent làm gì trên trang này?"
          class="w-full bg-surface-container-low border border-outline-variant rounded-xl pl-10 pr-4 py-2.5 text-xs text-on-surface outline-none focus:border-primary"
        />
      </div>

      <button
        type="button"
        on:click={() => handleRunBrowserAgent()}
        disabled={!isRunning && !targetUrl.trim()}
        class="px-5 py-2.5 rounded-xl text-xs font-bold transition-all flex items-center justify-center gap-1.5 cursor-pointer whitespace-nowrap disabled:opacity-40 {isRunning ? 'bg-rose-600 hover:bg-rose-700 text-white' : 'bg-primary text-on-primary hover:opacity-90'}"
      >
        <span class="material-symbols-outlined text-base">{isRunning ? 'stop_circle' : 'rocket_launch'}</span>
        {isRunning ? 'Dừng Agent' : 'Kích hoạt Agent'}
      </button>
    </div>

    <!-- Settings: one quiet row, no nested box -->
    <div class="flex flex-wrap items-center gap-x-4 gap-y-2 pt-3 border-t border-outline-variant/60 text-[11px] text-on-surface-variant">
      <div class="flex items-center gap-1.5">
        <label for="browser-role-select" class="font-semibold">Role</label>
        <select id="browser-role-select" bind:value={selectedRole} class="bg-surface-container-low border border-outline-variant rounded-lg px-2 py-1 text-[11px] text-on-surface font-mono outline-none focus:border-primary">
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
        <label for="browser-model-select" class="font-semibold">Model</label>
        <select id="browser-model-select" bind:value={selectedModel} class="bg-surface-container-low border border-outline-variant rounded-lg px-2 py-1 text-[11px] text-on-surface font-mono outline-none focus:border-primary">
          <optgroup label="Claude Agent">
            <option value="claude-sonnet-4-5">Claude 4.5 Sonnet</option>
            <option value="claude-opus-4-8">Claude 4.8 Opus</option>
            <option value="claude-haiku-4-5">Claude 4.5 Haiku</option>
          </optgroup>
          <optgroup label="Antigravity Agent (Gemini)">
            <option value="gemini-3.6-flash-high">Gemini 3.6 Flash (High)</option>
            <option value="gemini-3.1-pro-high">Gemini 3.1 Pro (High)</option>
          </optgroup>
        </select>
      </div>

      <div class="flex items-center gap-1.5">
        <label for="browser-max-steps" class="font-semibold">Steps</label>
        <input id="browser-max-steps" type="number" min="1" max="20" bind:value={maxSteps} class="w-11 bg-surface-container-low border border-outline-variant rounded-lg px-1.5 py-1 text-center text-[11px] text-on-surface font-mono outline-none focus:border-primary" />
      </div>

      <label class="flex items-center gap-1.5 cursor-pointer select-none">
        <input type="checkbox" bind:checked={takeScreenshot} class="rounded accent-primary" />
        <span>Screenshot</span>
      </label>

      <!-- Headless is ignored while the persistent profile is on (you must be able
           to see the window to sign in), so don't offer a control that does nothing. -->
      {#if !usePersistentProfile}
        <label class="flex items-center gap-1.5 cursor-pointer select-none">
          <input type="checkbox" bind:checked={isHeadless} class="rounded accent-primary" />
          <span>Chạy ngầm</span>
        </label>
      {/if}

      <label class="flex items-center gap-1.5 cursor-pointer select-none">
        <input type="checkbox" bind:checked={usePersistentProfile} class="rounded accent-primary" />
        <span>Profile riêng (nhớ đăng nhập)</span>
      </label>

      {#if usePersistentProfile}
        <button
          type="button"
          on:click={handleOpenAgentChrome}
          disabled={isOpeningChrome}
          class="ml-auto flex items-center gap-1 text-primary font-semibold hover:underline disabled:opacity-50 cursor-pointer">
          <span class="material-symbols-outlined text-sm">open_in_new</span>
          {isOpeningChrome ? 'Đang mở…' : chromeAgentReady ? 'Mở lại cửa sổ Chrome Agent' : 'Đăng nhập trong Chrome Agent'}
        </button>
      {/if}
    </div>
  </div>

  <!-- Real-time Live Log Stream Console -->
  {#if isRunning || liveLogs.length > 0}
    <div class="bg-slate-950 border border-slate-800 p-4 rounded-2xl shadow-inner space-y-2 text-xs font-mono">
      <div class="flex items-center justify-between font-sans pb-2 border-b border-slate-800 text-slate-300">
        <div class="flex items-center gap-2">
          <span class="w-2.5 h-2.5 rounded-full {isRunning ? 'bg-emerald-500 animate-ping' : 'bg-slate-500'}"></span>
          <span class="font-bold text-xs">Live Execution Logs Stream ({liveLogs.length}):</span>
        </div>
        {#if isRunning}
          <span class="text-[10px] text-emerald-400 font-bold tracking-wider">● RUNNING CHROME CDP IN REALTIME</span>
        {/if}
      </div>
      <div class="max-h-48 overflow-y-auto space-y-1.5 leading-relaxed text-emerald-400 pr-2">
        {#each liveLogs as log}
          <div class="flex items-start gap-2">
            <span class="text-slate-500 text-[10px] select-none">❯</span>
            <span class="whitespace-pre-wrap break-all">{log}</span>
          </div>
        {/each}
      </div>

      <!-- Agent is parked waiting on this answer; it resumes the same run. -->
      {#if pendingAsk}
        <div class="mt-3 bg-amber-500/10 border border-amber-500/40 rounded-xl p-3 space-y-3 font-sans">
          <div class="flex items-start gap-2 text-amber-300">
            <span class="material-symbols-outlined text-base flex-shrink-0">contact_support</span>
            <div class="space-y-0.5">
              <div class="font-bold text-xs">Agent đang chờ bạn quyết định</div>
              <p class="text-[11px] text-amber-200/90 whitespace-pre-wrap leading-relaxed">{pendingAsk.question}</p>
            </div>
          </div>

          {#if pendingAsk.options.length > 0}
            <div class="flex flex-wrap gap-2">
              {#each pendingAsk.options as opt}
                <button
                  type="button"
                  on:click={() => submitAsk(opt.value)}
                  disabled={submittingAsk}
                  class="bg-amber-500/20 text-amber-100 border border-amber-500/40 px-3 py-1.5 rounded-lg text-[11px] font-bold hover:bg-amber-500/30 disabled:opacity-50 cursor-pointer transition-all">
                  {opt.label}
                </button>
              {/each}
            </div>
          {/if}

          {#if pendingAsk.allowOther}
            <div class="flex items-center gap-2">
              <span class="text-[11px] font-bold text-amber-300 whitespace-nowrap">Khác:</span>
              <input
                type="text"
                bind:value={otherAnswer}
                on:keydown={(e) => { if (e.key === 'Enter') submitAsk(otherAnswer); }}
                placeholder="Nhập chỉ dẫn của bạn, AI sẽ làm theo..."
                disabled={submittingAsk}
                class="flex-1 bg-slate-900 border border-amber-500/30 rounded-lg px-3 py-1.5 text-[11px] text-amber-100 outline-none focus:border-amber-400 disabled:opacity-50"
              />
              <button
                type="button"
                on:click={() => submitAsk(otherAnswer)}
                disabled={submittingAsk || !otherAnswer.trim()}
                class="bg-amber-500 text-slate-950 px-3 py-1.5 rounded-lg text-[11px] font-bold hover:opacity-90 disabled:opacity-40 cursor-pointer transition-all whitespace-nowrap">
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
    <div class="p-4 bg-amber-500/10 border border-amber-500/30 rounded-2xl flex items-start gap-3 text-amber-600 dark:text-amber-400">
      <span class="material-symbols-outlined text-xl flex-shrink-0">warning</span>
      <div class="space-y-1 text-xs">
        <div class="font-bold text-sm">🛑 AI Dừng Vòng Lặp: Yêu Cầu Người Dùng Can Thiệp!</div>
        <p class="leading-relaxed">{result.user_intervention || 'AI phát hiện trang web cần đăng nhập, xác thực CAPTCHA/2FA hoặc không thể tìm thấy nút tương tác.'}</p>
      </div>
    </div>
  {/if}

  <!-- Results Output Dashboard -->
  {#if result}
    <div class="space-y-4">
      <!-- Status Summary Card -->
      <div class="bg-surface-container-low border border-outline-variant p-4 rounded-2xl flex flex-wrap items-center justify-between gap-3 text-xs">
        <div class="space-y-1">
          <div class="font-bold text-on-surface text-sm flex items-center gap-2">
            {result.title || 'Untitled Page'}
            {#if result.success}
              <span class="text-[10px] bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30 px-2 py-0.5 rounded-full font-bold">200 OK</span>
            {:else}
              <span class="text-[10px] bg-rose-500/15 text-rose-600 border border-rose-500/30 px-2 py-0.5 rounded-full font-bold">FAILED</span>
            {/if}
          </div>
          <div class="font-mono text-on-surface-variant text-[11px]">{result.url}</div>
        </div>

        <div class="flex gap-1.5 bg-surface-container-lowest p-1 rounded-xl border border-outline-variant">
          {#if result.ai_response}
            <button
              type="button"
              on:click={() => activeTab = 'ai'}
              class="px-3 py-1 rounded-lg text-xs font-bold transition-all cursor-pointer {activeTab === 'ai' ? 'bg-primary text-on-primary shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}">
              🧠 AI Response & Plan
            </button>
          {/if}
          <button
            type="button"
            on:click={() => activeTab = 'preview'}
            class="px-3 py-1 rounded-lg text-xs font-bold transition-all cursor-pointer {activeTab === 'preview' ? 'bg-primary text-on-primary shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}">
            📋 Execution Stream ({result.logs.length})
          </button>
          {#if result.screenshot_base64}
            <button
              type="button"
              on:click={() => activeTab = 'screenshot'}
              class="px-3 py-1 rounded-lg text-xs font-bold transition-all cursor-pointer {activeTab === 'screenshot' ? 'bg-primary text-on-primary shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}">
              📸 Screenshot
            </button>
          {/if}
          <button
            type="button"
            on:click={() => activeTab = 'dom'}
            class="px-3 py-1 rounded-lg text-xs font-bold transition-all cursor-pointer {activeTab === 'dom' ? 'bg-primary text-on-primary shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}">
            🧱 DOM Snippet
          </button>
          <button
            type="button"
            on:click={() => activeTab = 'text'}
            class="px-3 py-1 rounded-lg text-xs font-bold transition-all cursor-pointer {activeTab === 'text' ? 'bg-primary text-on-primary shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}">
            📝 Text Content
          </button>
        </div>
      </div>

      <!-- Tab Content Area -->
      <div class="bg-surface-container-lowest border border-outline-variant p-4 rounded-2xl shadow-xs">
        {#if activeTab === 'ai'}
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <div class="text-xs font-bold text-on-surface flex items-center gap-2">
                <span class="material-symbols-outlined text-primary text-sm">psychology</span>
                Phản hồi & Phân tích từ Bộ não AI ({selectedModel}):
              </div>
              <span class="text-[10px] bg-primary/10 text-primary border border-primary/20 px-2 py-0.5 rounded-full font-mono">{selectedRole}</span>
            </div>
            <div class="bg-surface-container-low p-4 rounded-xl border border-outline-variant text-on-surface text-xs whitespace-pre-wrap leading-relaxed font-sans shadow-inner">
              {result.ai_response}
            </div>
          </div>
        {:else if activeTab === 'preview'}
          <div class="space-y-2 font-mono text-xs">
            <div class="text-xs font-bold text-on-surface mb-2 font-sans">Dòng thời gian thực thi (Chrome DevTools Protocol Execution Stream):</div>
            {#each result.logs as log}
              <div class="p-2.5 rounded-xl bg-surface-container-low border border-outline-variant/60 text-on-surface flex items-start gap-2">
                <span class="material-symbols-outlined text-sm text-primary flex-shrink-0">task_alt</span>
                <span class="leading-relaxed">{log}</span>
              </div>
            {/each}
          </div>
        {:else if activeTab === 'screenshot'}
          <div class="space-y-2">
            <div class="text-xs font-bold text-on-surface mb-2">Ảnh màn hình Full-Page do Chrome CDP tự động chụp:</div>
            <div class="border border-outline-variant rounded-xl overflow-hidden max-h-[600px] overflow-y-auto bg-black/40 p-2">
              <img src={result.screenshot_base64} alt="Browser Agent Screenshot" class="w-full h-auto rounded-lg shadow-md" />
            </div>
          </div>
        {:else if activeTab === 'dom'}
          <div class="space-y-2 font-mono text-xs">
            <div class="text-xs font-bold text-on-surface mb-2 font-sans">Cấu trúc DOM trích xuất (HTML Snippet):</div>
            <textarea readonly class="w-full bg-slate-950 text-emerald-400 p-4 rounded-xl font-mono text-xs h-96 outline-none border border-slate-800 leading-relaxed whitespace-pre font-normal">{result.html_snippet}</textarea>
          </div>
        {:else if activeTab === 'text'}
          <div class="space-y-2 font-mono text-xs">
            <div class="text-xs font-bold text-on-surface mb-2 font-sans">Nội dung văn bản bóc tách từ Body (Text Content):</div>
            <textarea readonly class="w-full bg-surface-container-low text-on-surface p-4 rounded-xl font-mono text-xs h-96 outline-none border border-outline-variant leading-relaxed whitespace-pre font-normal">{result.text_content}</textarea>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
