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

  let targetUrl = 'http://localhost:5173';
  let instructionPrompt = '';
  let takeScreenshot = true;
  let isHeadless = true;
  let useRealProfile = true;
  let maxSteps = 5;
  let isRunning = false;

  let roles: string[] = [];
  let selectedRole = 'front-end-agent.md';
  let selectedModel = 'claude-sonnet-4-5';

  let result: BrowserActionResult | null = null;
  let activeTab: 'preview' | 'screenshot' | 'dom' | 'text' | 'ai' = 'preview';

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
  });

  async function handleRunBrowserAgent(overrideUrl?: string) {
    const urlToRun = overrideUrl || targetUrl.trim();
    if (!urlToRun) return;
    targetUrl = urlToRun;

    isRunning = true;
    result = null;
    addLog(`Kích hoạt Native Chrome CDP Browser Agent (Model: ${selectedModel}, Profile: ${useRealProfile ? 'Real Chrome' : 'Temp'}, MaxSteps: ${maxSteps}) truy cập ${urlToRun}...`, 'INFO');

    try {
      if ((AppBindings as any).RunBrowserTask) {
        const res = await (AppBindings as any).RunBrowserTask(urlToRun, instructionPrompt, selectedRole, selectedModel, takeScreenshot, isHeadless, useRealProfile, maxSteps);
        result = res;
        if (res && res.ai_response) {
          activeTab = 'ai';
        } else {
          activeTab = 'preview';
        }
        if (res && res.success) {
          addLog(`Browser Agent tải thành công trang "${res.title}"! (Trạng thái: ${res.status || 'OK'})`, 'SUCCESS');
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
  <!-- Top Banner -->
  <div class="bg-surface-container-low border border-outline-variant p-6 rounded-3xl shadow-xs flex flex-wrap items-center justify-between gap-4">
    <div class="space-y-1">
      <div class="flex items-center gap-2">
        <span class="material-symbols-outlined text-2xl text-primary">language</span>
        <h1 class="text-xl font-bold text-on-surface">Native Go Chrome CDP Browser Agent</h1>
        <span class="text-xs bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30 px-2.5 py-0.5 rounded-full font-bold">100% Native Go (0 Python/Playwright Deps)</span>
      </div>
      <p class="text-xs text-on-surface-variant max-w-2xl">
        Porting toàn bộ kiến trúc 3-Agent Triad (Planner ➔ Chrome CDP Executor ➔ DOM Critique Inspector) từ dự án TheAgenticBrowser sang Native Go. Đã tích hợp trực tiếp vào file `.exe` duy nhất.
      </p>
    </div>

    <!-- Quick Presets -->
    <div class="flex flex-wrap gap-2">
      <button
        type="button"
        on:click={() => handleRunBrowserAgent('http://localhost:5173')}
        disabled={isRunning}
        class="bg-surface-container-highest text-on-surface border border-outline-variant px-3 py-1.5 rounded-xl text-xs font-bold hover:bg-surface-container-high cursor-pointer transition-all">
        🖥️ Local App (localhost:5173)
      </button>
      <button
        type="button"
        on:click={() => handleRunBrowserAgent('https://news.ycombinator.com')}
        disabled={isRunning}
        class="bg-surface-container-highest text-on-surface border border-outline-variant px-3 py-1.5 rounded-xl text-xs font-bold hover:bg-surface-container-high cursor-pointer transition-all">
        📰 Hacker News
      </button>
    </div>
  </div>

  <!-- Control & Input Box -->
  <div class="bg-surface-container-lowest border border-outline-variant p-5 rounded-2xl shadow-xs space-y-4">
    <div class="flex flex-col gap-3">
      <!-- Agent & Model Selection Bar -->
      <div class="flex flex-wrap items-center justify-between gap-3 bg-surface-container-low/50 p-3 rounded-xl border border-outline-variant/60 text-xs">
        <div class="flex flex-wrap items-center gap-4">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-primary text-sm">badge</span>
            <label for="browser-role-select" class="font-bold text-on-surface whitespace-nowrap">Agent Role (Persona):</label>
            <select id="browser-role-select" bind:value={selectedRole} class="bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-1.5 text-xs text-on-surface font-mono outline-none focus:border-primary">
              {#each roles as role}
                <option value={role}>{role}</option>
              {/each}
              {#if roles.length === 0}
                <option value="front-end-agent.md">front-end-agent.md</option>
                <option value="agents.md">agents.md</option>
              {/if}
            </select>
          </div>

          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-primary text-sm">smart_toy</span>
            <label for="browser-model-select" class="font-bold text-on-surface whitespace-nowrap">Model AI Thực Thi:</label>
            <select id="browser-model-select" bind:value={selectedModel} class="bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-1.5 text-xs text-on-surface font-mono outline-none focus:border-primary">
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
        </div>

        <div class="flex items-center gap-4">
          <label class="flex items-center gap-1.5 cursor-pointer text-[11px] font-semibold text-on-surface">
            <input type="checkbox" bind:checked={useRealProfile} class="rounded accent-primary" />
            <span>🔑 Profile Chrome thật (Giữ Login GitHub/Google)</span>
          </label>

          <div class="flex items-center gap-1.5">
            <span class="font-bold text-[11px] text-on-surface">Max Steps:</span>
            <input type="number" min="1" max="20" bind:value={maxSteps} class="w-12 bg-surface-container-lowest border border-outline-variant rounded px-1.5 py-0.5 text-center text-xs font-mono" />
          </div>

          <div class="text-[10px] text-on-surface-variant font-mono">
            Powered by 3-Agent Triad Loop
          </div>
        </div>
      </div>

      <div class="flex flex-col md:flex-row gap-3">
        <div class="flex-1 relative">
          <span class="material-symbols-outlined absolute left-3.5 top-3 text-sm text-outline">link</span>
          <input
            type="text"
            bind:value={targetUrl}
            placeholder="Nhập URL trang web (ví dụ: http://localhost:5173 hoặc https://news.ycombinator.com)..."
            class="w-full bg-surface-container-low border border-outline-variant rounded-xl pl-10 pr-4 py-2.5 text-xs text-on-surface font-mono outline-none focus:border-primary"
          />
        </div>
      </div>
      
      <div class="flex flex-col md:flex-row gap-3">
        <div class="flex-1 relative">
          <span class="material-symbols-outlined absolute left-3.5 top-3 text-sm text-outline">psychology</span>
          <input
            type="text"
            bind:value={instructionPrompt}
            placeholder='Prompt mục tiêu (ví dụ: "Hãy vào HackerNews và tóm tắt 3 bài viết top 1 cho tôi", hoặc "Viết comment vào PR này")...'
            class="w-full bg-surface-container-low border border-outline-variant rounded-xl pl-10 pr-4 py-2.5 text-xs text-on-surface outline-none focus:border-primary"
          />
        </div>

        <div class="flex items-center gap-2">
          <label class="flex items-center gap-2 px-3 py-2 bg-surface-container-low border border-outline-variant rounded-xl text-xs font-semibold cursor-pointer select-none text-on-surface">
            <input type="checkbox" bind:checked={takeScreenshot} class="rounded accent-primary" />
            <span class="whitespace-nowrap">📸 Screenshot</span>
          </label>

          <label class="flex items-center gap-2 px-3 py-2 bg-surface-container-low border border-outline-variant rounded-xl text-xs font-semibold cursor-pointer select-none text-on-surface">
            <input type="checkbox" bind:checked={isHeadless} class="rounded accent-primary" />
            <span class="whitespace-nowrap">👻 Headless (Chạy ngầm)</span>
          </label>

          <button
            type="button"
            on:click={() => handleRunBrowserAgent()}
            disabled={isRunning || !targetUrl.trim()}
            class="bg-primary text-on-primary px-6 py-2.5 rounded-xl text-xs font-bold hover:opacity-90 transition-all flex items-center justify-center gap-2 shadow-xs disabled:opacity-50 cursor-pointer whitespace-nowrap"
          >
            <span class="material-symbols-outlined text-base {isRunning ? 'animate-spin' : ''}">
              {isRunning ? 'sync' : 'rocket_launch'}
            </span>
            {isRunning ? 'Đang chạy AI Agent...' : 'Kích hoạt Agent'}
          </button>
        </div>
      </div>
    </div>
  </div>

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
