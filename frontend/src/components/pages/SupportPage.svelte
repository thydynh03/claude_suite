<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog, addToast } from '../../lib/stores/appState';

  /**
   * Diagnostics that can fail.
   *
   * Every card here used to be decorative: `sqliteHealthy` was assigned `true`
   * unconditionally after any successful call, two CLI flags were inferred from
   * "at least one agent row exists" and then never rendered, and each card
   * carried a hardcoded caption ("Wails Go IPC Bridge v2.13",
   * "PRAGMA busy_timeout=5000", "Latest Release") presented as a live reading.
   * A panel that cannot report a problem is not a diagnostic.
   */
  type Health = 'unknown' | 'ok' | 'error';

  let appVersion = '';
  let isDiagnosticsRunning = false;
  let ipcHealth: Health = 'unknown';
  let dbHealth: Health = 'unknown';
  let workspacePath = '';
  let lastRunAt = '';

  const HEALTH_ICON: Record<Health, string> = {
    unknown: 'help',
    ok: 'check_circle',
    error: 'cancel',
  };
  const HEALTH_CLASS: Record<Health, string> = {
    unknown: 'text-on-surface-variant',
    ok: 'text-success',
    error: 'text-error',
  };

  onMount(async () => {
    await runDiagnostics();
  });

  async function runDiagnostics() {
    isDiagnosticsRunning = true;
    ipcHealth = 'unknown';
    dbHealth = 'unknown';

    try {
      appVersion = (await (AppBindings as any).GetAppVersion?.()) || '';
    } catch (e) {
      appVersion = '';
    }

    // IPC: only the round trip itself proves the bridge is up.
    try {
      if ((AppBindings as any).Ping) {
        ipcHealth = (await (AppBindings as any).Ping()) ? 'ok' : 'error';
      }
    } catch (e) {
      ipcHealth = 'error';
    }

    // Database: a real read against SQLite, reported as it comes back.
    try {
      await AppBindings.GetAgents();
      dbHealth = 'ok';
    } catch (e) {
      dbHealth = 'error';
    }

    try {
      const cfg = await AppBindings.GetWorkspaceConfig();
      workspacePath = cfg?.last_workspace_folder || '';
    } catch (e) {
      workspacePath = '';
    }

    lastRunAt = new Date().toLocaleTimeString();
    isDiagnosticsRunning = false;
  }

  function diagnosticsReport(): string {
    const label = (h: Health) => (h === 'ok' ? 'ok' : h === 'error' ? 'error' : 'unknown');
    return [
      'Agent Center — báo cáo chẩn đoán',
      `Thời điểm: ${new Date().toISOString()}`,
      `Phiên bản app: ${appVersion || 'không đọc được'}`,
      `IPC backend: ${label(ipcHealth)}`,
      `SQLite: ${label(dbHealth)}`,
      `Workspace: ${workspacePath || 'chưa chọn'}`,
      `User agent: ${navigator.userAgent}`,
    ].join('\n');
  }

  // The button used to write one log line and then fire a native alert claiming
  // the report had been copied. Nothing was copied. Now it either copies or says
  // it failed.
  async function handleExportLogs() {
    const report = diagnosticsReport();
    try {
      await navigator.clipboard.writeText(report);
      addLog('Đã chép báo cáo chẩn đoán vào clipboard.', 'SUCCESS');
      addToast('Đã chép báo cáo chẩn đoán vào clipboard.', 'SUCCESS');
    } catch (e) {
      addLog(`Không chép được báo cáo chẩn đoán: ${e}`, 'ERROR');
      addToast('Không chép được báo cáo vào clipboard.', 'ERROR');
    }
  }

  const faqs = [
    {
      q: 'Làm sao khi gặp lỗi "API Quota Exhausted" hoặc 429?',
      a: 'Agent Center đã được tích hợp tính năng Smart Fallback tự động. Khi Claude CLI báo lỗi 429, hệ thống sẽ tự động chuyển sang dùng Gemini 3.6 Flash trên Antigravity CLI mà không bị dừng tiến trình.'
    },
    {
      q: 'Khi đóng cửa sổ ứng dụng, kịch bản Scheduler có tiếp tục chạy không?',
      a: 'Các job lên lịch tự động được lưu trực tiếp vào database SQLite. Tuy nhiên ứng dụng cần mở ngầm để gửi tín hiệu trigger. Bạn có thể sử dụng Webhook Server để kích hoạt từ xa.'
    },
    {
      q: 'Ứng dụng lưu Database và Log ở đâu trên máy tính?',
      a: 'Database SQLite và file Log xoay tự động được lưu tại đường dẫn mặc định:\n- Windows: C:\\Users\\<User>\\AppData\\Local\\AgentCenter\\agent_manager.db\n- Log file: C:\\Users\\<User>\\AppData\\Local\\AgentCenter\\logs\\agent_center.log'
    },
    {
      q: 'Tôi có thể đổi mô hình AI cho từng Agent không?',
      a: 'Có. Trong tab Settings -> Agents Registry, bạn có thể chỉnh sửa Model (Claude Opus, Sonnet 4.5, Gemini 3.6 Flash) và Provider riêng cho từng Agent.'
    }
  ];
</script>

<div class="space-y-6 max-w-7xl mx-auto pb-12">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-lg font-semibold flex items-center gap-2 text-on-surface">
        <span class="material-symbols-outlined text-lg text-on-surface-variant">help</span>
        Hỗ trợ kỹ thuật
      </h1>
      <p class="text-on-surface-variant text-sm mt-0.5">Kiểm tra trạng thái hệ thống, chẩn đoán sự cố, câu hỏi thường gặp và liên hệ trợ giúp.</p>
    </div>

    <button
      type="button"
      on:click={runDiagnostics}
      disabled={isDiagnosticsRunning}
      class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium flex items-center gap-1.5 hover:opacity-90 transition-opacity cursor-pointer disabled:opacity-50">
      <span class="material-symbols-outlined text-sm {isDiagnosticsRunning ? 'animate-spin' : ''}">refresh</span>
      {isDiagnosticsRunning ? 'Đang kiểm tra...' : 'Kiểm tra hệ thống'}
    </button>
  </div>

  <!-- Diagnostics status cards -->
  <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
      <div class="flex items-center justify-between gap-2 text-xs font-medium text-on-surface-variant">
        <span>Kết nối IPC backend</span>
        <span class="material-symbols-outlined text-base {HEALTH_CLASS[ipcHealth]}">{HEALTH_ICON[ipcHealth]}</span>
      </div>
      <p class="text-sm font-medium text-on-surface">
        {ipcHealth === 'ok' ? 'Đã sẵn sàng' : ipcHealth === 'error' ? 'Mất kết nối' : 'Chưa kiểm tra'}
      </p>
    </div>

    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
      <div class="flex items-center justify-between gap-2 text-xs font-medium text-on-surface-variant">
        <span>SQLite database</span>
        <span class="material-symbols-outlined text-base {HEALTH_CLASS[dbHealth]}">{HEALTH_ICON[dbHealth]}</span>
      </div>
      <p class="text-sm font-medium text-on-surface">
        {dbHealth === 'ok' ? 'Đọc ghi được' : dbHealth === 'error' ? 'Không đọc được' : 'Chưa kiểm tra'}
      </p>
    </div>

    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
      <div class="flex items-center justify-between gap-2 text-xs font-medium text-on-surface-variant">
        <span>Workspace đang chọn</span>
        <span class="material-symbols-outlined text-base text-on-surface-variant">folder</span>
      </div>
      <p class="text-xs font-medium text-on-surface truncate" title={workspacePath}>
        {workspacePath || 'Chưa chọn workspace'}
      </p>
    </div>

    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
      <div class="flex items-center justify-between gap-2 text-xs font-medium text-on-surface-variant">
        <span>Phiên bản app</span>
        <span class="material-symbols-outlined text-base text-on-surface-variant">verified</span>
      </div>
      <p class="text-sm font-medium text-on-surface">{appVersion || 'Không đọc được'}</p>
      {#if lastRunAt}
        <p class="text-[11px] text-on-surface-variant">Kiểm tra lúc {lastRunAt}</p>
      {/if}
    </div>
  </div>

  <!-- FAQs. Each answer used to be a filled, bordered box inside an already
       bordered card; a divided list carries the same content without the
       double frame. -->
  <div class="bg-surface border border-outline-variant rounded-xl p-6 space-y-4">
    <h2 class="text-sm font-semibold text-on-surface flex items-center gap-2">
      <span class="material-symbols-outlined text-base text-on-surface-variant">quiz</span> Câu hỏi thường gặp
    </h2>

    <div class="divide-y divide-outline-variant">
      {#each faqs as faq}
        <div class="py-3 first:pt-0 last:pb-0 space-y-1.5">
          <h3 class="text-xs font-medium text-on-surface">{faq.q}</h3>
          <p class="text-xs text-on-surface-variant leading-relaxed whitespace-pre-wrap">
            {faq.a}
          </p>
        </div>
      {/each}
    </div>
  </div>

  <!-- Direct support & GitHub channel -->
  <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 flex flex-wrap items-center justify-between gap-4">
    <div class="space-y-1">
      <h3 class="text-sm font-semibold text-on-surface">Cần hỗ trợ trực tiếp hoặc báo lỗi?</h3>
      <p class="text-xs text-on-surface-variant">Nếu bạn phát hiện lỗi hoặc muốn đóng góp ý kiến cải tiến dự án Agent Center.</p>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <button
        type="button"
        on:click={handleExportLogs}
        class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium flex items-center gap-1.5 hover:bg-surface-container-high transition-colors cursor-pointer">
        <span class="material-symbols-outlined text-sm">content_copy</span> Chép báo cáo chẩn đoán
      </button>

      <a
        href="https://github.com/thydynh03/agent_center/issues"
        target="_blank"
        rel="noreferrer"
        class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium flex items-center gap-1.5 hover:bg-surface-container-high transition-colors cursor-pointer">
        <span class="material-symbols-outlined text-sm">bug_report</span> GitHub Issues
      </a>
    </div>
  </div>
</div>
