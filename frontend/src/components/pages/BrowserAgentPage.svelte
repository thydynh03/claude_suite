<script lang="ts">
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
  }

  let targetUrl = 'http://localhost:5173';
  let takeScreenshot = true;
  let isRunning = false;

  let result: BrowserActionResult | null = null;
  let activeTab: 'preview' | 'screenshot' | 'dom' | 'text' = 'preview';

  async function handleRunBrowserAgent(overrideUrl?: string) {
    const urlToRun = overrideUrl || targetUrl.trim();
    if (!urlToRun) return;
    targetUrl = urlToRun;

    isRunning = true;
    result = null;
    addLog(`Kích hoạt Native Chrome CDP Browser Agent truy cập ${urlToRun}...`, 'INFO');

    try {
      if ((AppBindings as any).RunBrowserTask) {
        const res = await (AppBindings as any).RunBrowserTask(urlToRun, takeScreenshot);
        result = res;
        if (res && res.success) {
          addLog(`Browser Agent tải thành công trang "${res.title}"!`, 'SUCCESS');
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
    <div class="flex flex-col md:flex-row gap-3">
      <div class="flex-1 relative">
        <span class="material-symbols-outlined absolute left-3.5 top-3 text-sm text-outline">link</span>
        <input
          type="text"
          bind:value={targetUrl}
          placeholder="Nhập URL trang web (ví dụ: http://localhost:5173 hoặc https://example.com)..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-xl pl-10 pr-4 py-2.5 text-xs text-on-surface font-mono outline-none focus:border-primary"
        />
      </div>

      <label class="flex items-center gap-2 px-3 py-2 bg-surface-container-low border border-outline-variant rounded-xl text-xs font-semibold cursor-pointer select-none text-on-surface">
        <input type="checkbox" bind:checked={takeScreenshot} class="rounded accent-primary" />
        <span>📸 Chụp ảnh màn hình (Screenshot)</span>
      </label>

      <button
        type="button"
        on:click={() => handleRunBrowserAgent()}
        disabled={isRunning || !targetUrl.trim()}
        class="bg-primary text-on-primary px-6 py-2.5 rounded-xl text-xs font-bold hover:opacity-90 transition-all flex items-center justify-center gap-2 shadow-xs disabled:opacity-50 cursor-pointer"
      >
        <span class="material-symbols-outlined text-base {isRunning ? 'animate-spin' : ''}">
          {isRunning ? 'sync' : 'rocket_launch'}
        </span>
        {isRunning ? '🤖 Chrome CDP Agent đang chạy...' : 'Kích hoạt Browser Agent'}
      </button>
    </div>
  </div>

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
        {#if activeTab === 'preview'}
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
