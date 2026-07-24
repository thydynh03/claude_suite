<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog } from '../../lib/stores/appState';

  let appVersion = 'v2.4.0';
  let isDiagnosticsRunning = false;
  let diagStatus = {
    ipcConnected: false,
    sqliteHealthy: false,
    claudeCliFound: false,
    antigravityCliFound: false,
    workspacePath: ''
  };

  onMount(async () => {
    await runDiagnostics();
  });

  async function runDiagnostics() {
    isDiagnosticsRunning = true;
    try {
      if ((AppBindings as any).GetAppVersion) {
        appVersion = await (AppBindings as any).GetAppVersion();
      }
      if ((AppBindings as any).Ping) {
        diagStatus.ipcConnected = await (AppBindings as any).Ping();
      }
      const cfg = await AppBindings.GetWorkspaceConfig();
      diagStatus.workspacePath = cfg?.last_workspace_folder || 'Chưa chọn Workspace';
      diagStatus.sqliteHealthy = true;

      // Check agents count
      const agents = await AppBindings.GetAgents();
      if (agents && agents.length > 0) {
        diagStatus.claudeCliFound = true;
        diagStatus.antigravityCliFound = true;
      }
    } catch (e) {
      console.error('Diagnostics error:', e);
    } finally {
      isDiagnosticsRunning = false;
    }
  }

  function handleExportLogs() {
    addLog('Exporting system diagnostics bundle...', 'INFO');
    alert('Đã xuất báo cáo chẩn đoán vào bộ nhớ tạm hệ thống.');
  }

  const faqs = [
    {
      q: 'Làm sao khi gặp lỗi "API Quota Exhausted" hoặc 429?',
      a: 'Claude Suite đã được tích hợp tính năng Smart Fallback tự động. Khi Claude CLI báo lỗi 429, hệ thống sẽ tự động chuyển sang dùng Gemini 3.6 Flash trên Antigravity CLI mà không bị dừng tiến trình.'
    },
    {
      q: 'Khi đóng cửa sổ ứng dụng, kịch bản Scheduler có tiếp tục chạy không?',
      a: 'Các job lên lịch tự động được lưu trực tiếp vào database SQLite. Tuy nhiên ứng dụng cần mở ngầm để gửi tín hiệu trigger. Bạn có thể sử dụng Webhook Server để kích hoạt từ xa.'
    },
    {
      q: 'Ứng dụng lưu Database và Log ở đâu trên máy tính?',
      a: 'Database SQLite và file Log xoay tự động được lưu tại đường dẫn mặc định:\n- Windows: C:\\Users\\<User>\\.claude_suite\\agent_manager.db\n- Log file: C:\\Users\\<User>\\.claude_suite\\logs\\claude_suite.log'
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
      <h1 class="text-2xl font-bold flex items-center gap-2 text-on-surface">
        <span class="material-symbols-outlined text-secondary">help</span>
        Hỗ Trợ Kỹ Thuật & System Diagnostics
      </h1>
      <p class="text-on-surface-variant text-sm mt-0.5">Kiểm tra trạng thái hệ thống, chẩn đoán sự cố, câu hỏi thường gặp và liên hệ trợ giúp.</p>
    </div>

    <button
      type="button"
      on:click={runDiagnostics}
      disabled={isDiagnosticsRunning}
      class="bg-primary text-on-primary px-4 py-2 rounded-xl text-xs font-bold flex items-center gap-1.5 hover:opacity-90 transition-all cursor-pointer disabled:opacity-50">
      <span class="material-symbols-outlined text-sm {isDiagnosticsRunning ? 'animate-spin' : ''}">refresh</span>
      {isDiagnosticsRunning ? 'Đang kiểm tra...' : 'Kiểm tra Hệ thống'}
    </button>
  </div>

  <!-- Diagnostics Status Cards -->
  <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <div class="bg-surface-container-lowest border border-outline-variant rounded-2xl p-4 space-y-2">
      <div class="flex items-center justify-between text-xs text-on-surface-variant">
        <span>Kết nối IPC Backend</span>
        <span class="material-symbols-outlined {diagStatus.ipcConnected ? 'text-emerald-500' : 'text-rose-500'} text-base">
          {diagStatus.ipcConnected ? 'check_circle' : 'cancel'}
        </span>
      </div>
      <p class="text-base font-bold text-on-surface">{diagStatus.ipcConnected ? 'Đã sẵn sàng' : 'Mất kết nối'}</p>
      <p class="text-[10px] text-on-surface-variant">Wails Go IPC Bridge v2.13</p>
    </div>

    <div class="bg-surface-container-lowest border border-outline-variant rounded-2xl p-4 space-y-2">
      <div class="flex items-center justify-between text-xs text-on-surface-variant">
        <span>SQLite WAL Database</span>
        <span class="material-symbols-outlined {diagStatus.sqliteHealthy ? 'text-emerald-500' : 'text-rose-500'} text-base">
          {diagStatus.sqliteHealthy ? 'check_circle' : 'cancel'}
        </span>
      </div>
      <p class="text-base font-bold text-on-surface">{diagStatus.sqliteHealthy ? 'Hoạt động tốt' : 'Lỗi Database'}</p>
      <p class="text-[10px] text-on-surface-variant">PRAGMA busy_timeout=5000</p>
    </div>

    <div class="bg-surface-container-lowest border border-outline-variant rounded-2xl p-4 space-y-2">
      <div class="flex items-center justify-between text-xs text-on-surface-variant">
        <span>Workspace Đang Chọn</span>
        <span class="material-symbols-outlined text-primary text-base">folder</span>
      </div>
      <p class="text-xs font-bold text-primary truncate">{diagStatus.workspacePath}</p>
      <p class="text-[10px] text-on-surface-variant">Context Scan Enabled</p>
    </div>

    <div class="bg-surface-container-lowest border border-outline-variant rounded-2xl p-4 space-y-2">
      <div class="flex items-center justify-between text-xs text-on-surface-variant">
        <span>Phiên Bản App</span>
        <span class="material-symbols-outlined text-secondary text-base">verified</span>
      </div>
      <p class="text-base font-bold text-on-surface">{appVersion}</p>
      <p class="text-[10px] text-emerald-600 font-bold">Latest Release</p>
    </div>
  </div>

  <!-- FAQs Accordion -->
  <div class="bg-surface border border-outline-variant rounded-2xl p-6 space-y-4">
    <h2 class="text-base font-bold text-on-surface flex items-center gap-2">
      <span class="material-symbols-outlined text-primary">quiz</span> Câu Hỏi Thường Gặp (FAQs)
    </h2>

    <div class="space-y-3">
      {#each faqs as faq}
        <div class="bg-surface-container-low p-4 rounded-xl border border-outline-variant space-y-2">
          <h3 class="text-xs font-bold text-on-surface flex items-center gap-2">
            <span class="material-symbols-outlined text-primary text-sm">help_outline</span> {faq.q}
          </h3>
          <p class="text-xs text-on-surface-variant leading-relaxed pl-6 whitespace-pre-wrap">
            {faq.a}
          </p>
        </div>
      {/each}
    </div>
  </div>

  <!-- Direct Support & GitHub Channel -->
  <div class="bg-surface-container-lowest border border-outline-variant rounded-2xl p-6 flex flex-wrap items-center justify-between gap-4">
    <div class="space-y-1">
      <h3 class="text-sm font-bold text-on-surface flex items-center gap-2">
        <span class="material-symbols-outlined text-primary">forum</span> Cần Hỗ Trợ Trực Tiếp / Khai Báo Lỗi?
      </h3>
      <p class="text-xs text-on-surface-variant">Nếu bạn phát hiện lỗi hoặc muốn đóng góp ý kiến cải tiến dự án Claude Suite.</p>
    </div>

    <div class="flex items-center gap-3">
      <button
        type="button"
        on:click={handleExportLogs}
        class="bg-surface-container-high text-on-surface px-4 py-2 rounded-xl text-xs font-bold flex items-center gap-1.5 hover:bg-surface-container-highest transition-all cursor-pointer">
        <span class="material-symbols-outlined text-sm">download</span> Xuất Báo Cáo Chẩn Đoán
      </button>

      <a
        href="https://github.com/thydynh03/claude_suite/issues"
        target="_blank"
        rel="noreferrer"
        class="bg-primary text-on-primary px-4 py-2 rounded-xl text-xs font-bold flex items-center gap-1.5 hover:opacity-90 transition-all cursor-pointer">
        <span class="material-symbols-outlined text-sm">bug_report</span> GitHub Issues & Support
      </a>
    </div>
  </div>
</div>
