<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import ModelSelect from '../ui/ModelSelect.svelte';
  import { addToast } from '../../lib/stores/appState';

  type Job = {
    id: string;
    kind: string;
    prompt: string;
    target_time: string;
    repeat: boolean;
    provider: string;
    model: string;
    enabled: boolean;
    wait_for_quota: boolean;
    last_run_at: string;
    last_status: string;
    last_error: string;
    run_count: number;
  };

  // Each kind runs a different capability of the app, so the picker explains what
  // it will actually do rather than showing a bare identifier.
  const KINDS = [
    { id: 'prompt', icon: 'chat', label: 'Gửi prompt', blurb: 'Gửi một prompt cho agent, như Cockpit.' },
    { id: 'plan', icon: 'account_tree', label: 'Lập & chạy kế hoạch', blurb: 'Phân rã thành tasks rồi để orchestrator chạy.' },
    { id: 'e2e', icon: 'travel_explore', label: 'Kiểm thử E2E', blurb: 'Chạy Chrome kiểm thử; lỗi thì tự sinh task sửa.' },
    { id: 'git-commit', icon: 'commit', label: 'AI viết commit', blurb: 'Commit phần đang dở với message do AI soạn.' },
    { id: 'digest', icon: 'summarize', label: 'Tổng kết', blurb: 'Gửi tóm tắt trong ngày qua webhook.' },
  ];

  // Composite "provider:model" consumed by ScheduleKind; the visible picker
  // is the shared ModelSelect (plain model id), kept in sync below.
  let modelChoice = 'claude:claude-sonnet-4-5';
  let schedulerModel = 'claude-sonnet-4-5';
  function onSchedulerModelChange(e: CustomEvent<{ value: string; provider: 'claude' | 'anti' }>) {
    modelChoice = `${e.detail.provider}:${e.detail.value}`;
  }

  let kind = 'prompt';
  let mode: 'time' | 'countdown' = 'time';
  let timeHour = '02';
  let timeMin = '00';
  let cdHour = '00';
  let cdMin = '30';
  let cdSec = '00';
  let prompt = '';
  let repeat = true;
  // Hold a due job until the anti pool has measured-usable quota — the way to
  // land heavy work on the moment a Gemini quota cycle resets.
  let waitForQuota = false;
  let busy = false;

  $: usesAntiPool = needsModel && modelChoice.startsWith('anti:');

  // ── Git watch (canh chừng workspace bẩn) ─────────────────────────────
  let gw = { enabled: false, threshold_hours: 4, auto_commit: false };
  let gwSaving = false;

  async function loadGitWatch() {
    try {
      if ((AppBindings as any).GetGitWatchConfig) {
        const cfg = await (AppBindings as any).GetGitWatchConfig();
        if (cfg) gw = cfg;
      }
    } catch (e) {}
  }

  async function saveGitWatch() {
    gwSaving = true;
    try {
      const hours = Math.max(1, Math.floor(Number(gw.threshold_hours) || 4));
      gw.threshold_hours = hours;
      await (AppBindings as any).SaveGitWatchConfig(gw);
      addToast(
        gw.enabled
          ? `Đã bật canh chừng Git: ${gw.auto_commit ? 'tự commit' : 'nhắc'} sau ${hours}h workspace bẩn.`
          : 'Đã tắt canh chừng Git.',
        'SUCCESS'
      );
      await loadGitWatch();
    } catch (e: any) {
      addToast('Không lưu được cài đặt canh chừng Git: ' + (e?.message ?? e), 'ERROR');
    } finally {
      gwSaving = false;
    }
  }

  let jobs: Job[] = [];
  let logs: { msg: string; level: string; time: string }[] = [];

  $: activeKind = KINDS.find((k) => k.id === kind) ?? KINDS[0];
  // Only the kinds that send text to an agent need one.
  $: needsPrompt = kind === 'prompt' || kind === 'plan' || kind === 'e2e';
  $: needsModel = kind === 'prompt' || kind === 'plan';

  onMount(() => {
    refreshJobs();
    loadGitWatch();
    const runtime = (window as any)?.runtime;
    if (!runtime?.EventsOn) return;
    // Cancel functions returned so the listeners die with the page: left
    // behind, every past visit's closure re-fetched jobs into a dead
    // component on each scheduler event.
    const offUpdated = runtime.EventsOn('scheduler_updated', refreshJobs);
    const offLog = runtime.EventsOn('scheduler_log', (data: any) => {
      if (!data) return;
      logs = [
        ...logs.slice(-200),
        {
          msg: data.msg ?? String(data),
          level: data.level ?? 'INFO',
          time: new Date().toLocaleTimeString('en-GB'),
        },
      ];
    });
    return () => {
      offUpdated?.();
      offLog?.();
    };
  });

  async function refreshJobs() {
    try {
      jobs = ((await AppBindings.GetScheduledJobs()) ?? []) as unknown as Job[];
    } catch (e) {
      console.error('load scheduled jobs:', e);
    }
  }

  function targetTime(): Date {
    if (mode === 'time') {
      const at = new Date();
      at.setHours(parseInt(timeHour) || 0, parseInt(timeMin) || 0, 0, 0);
      // A time already past today means the user meant tomorrow.
      if (at <= new Date()) at.setDate(at.getDate() + 1);
      return at;
    }
    const seconds = (parseInt(cdHour) || 0) * 3600 + (parseInt(cdMin) || 0) * 60 + (parseInt(cdSec) || 0);
    return new Date(Date.now() + seconds * 1000);
  }

  async function schedule() {
    if (needsPrompt && !prompt.trim()) {
      addToast('Hãy nhập nội dung cho lịch này.', 'WARN');
      return;
    }

    const [provider, model] = needsModel ? modelChoice.split(':') : ['', ''];
    busy = true;
    try {
      await (AppBindings as any).ScheduleKind(kind, prompt, targetTime().toISOString(), repeat, provider, model, usesAntiPool && waitForQuota);
      addToast(`Đã đặt lịch: ${activeKind.label}`, 'SUCCESS');
      prompt = '';
      await refreshJobs();
    } catch (e: any) {
      addToast('Không đặt được lịch: ' + (e?.message ?? e), 'ERROR');
    } finally {
      busy = false;
    }
  }

  async function cancelJob(id: string) {
    try {
      await AppBindings.CancelScheduledJob(id);
      await refreshJobs();
    } catch (e: any) {
      addToast('Không huỷ được lịch: ' + (e?.message ?? e), 'ERROR');
    }
  }

  async function toggleJob(job: Job) {
    try {
      await (AppBindings as any).SetScheduledJobEnabled(job.id, !job.enabled);
      await refreshJobs();
    } catch (e: any) {
      addToast('Không đổi được trạng thái: ' + (e?.message ?? e), 'ERROR');
    }
  }

  const kindOf = (id: string) => KINDS.find((k) => k.id === id) ?? KINDS[0];

  function whenLabel(iso: string): string {
    const at = new Date(iso);
    if (Number.isNaN(at.getTime())) return '—';
    const today = new Date();
    const sameDay = at.toDateString() === today.toDateString();
    const time = at.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
    return sameDay ? `Hôm nay ${time}` : `${at.toLocaleDateString('vi-VN')} ${time}`;
  }

  // A run that never happened has a zero time, which arrives as year 1.
  const hasRun = (job: Job) => !!job.last_status && new Date(job.last_run_at).getFullYear() > 1970;
</script>

<div class="max-w-7xl mx-auto space-y-4 pb-12">
  <div class="flex items-center justify-between bg-surface-container-lowest border border-outline-variant rounded-xl p-4">
    <div class="flex items-center gap-3">
      <span class="material-symbols-outlined text-2xl text-primary">schedule</span>
      <div>
        <h1 class="text-lg font-bold text-on-surface">Lịch tự động</h1>
        <p class="text-xs text-on-surface-variant">Đặt việc chạy khi bạn không ngồi trước máy. Lịch được lưu lại sau khi tắt app.</p>
      </div>
    </div>
    <span class="text-xs font-bold text-on-surface-variant bg-surface-container-high border border-outline-variant px-3 py-1.5 rounded-lg">
      {jobs.length} lịch
    </span>
  </div>

  <div class="grid grid-cols-12 gap-4">
    <!-- Create -->
    <div class="col-span-12 lg:col-span-5 space-y-4">
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3">
        <span class="text-xs font-bold uppercase text-on-surface-variant">Loại công việc</span>
        <div class="grid grid-cols-1 gap-1.5">
          {#each KINDS as k}
            <button
              type="button"
              on:click={() => (kind = k.id)}
              class="flex items-start gap-3 rounded-xl border px-3 py-2.5 text-left transition-colors cursor-pointer
              {kind === k.id
                ? 'border-primary bg-primary/10'
                : 'border-outline-variant hover:bg-surface-container-low'}"
            >
              <span class="material-symbols-outlined text-lg {kind === k.id ? 'text-primary' : 'text-on-surface-variant'}">{k.icon}</span>
              <span class="min-w-0">
                <span class="block text-xs font-bold {kind === k.id ? 'text-primary' : 'text-on-surface'}">{k.label}</span>
                <span class="block text-[11px] leading-snug text-on-surface-variant">{k.blurb}</span>
              </span>
            </button>
          {/each}
        </div>
      </div>

      {#if needsPrompt}
        <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
          <span class="text-xs font-bold uppercase text-on-surface-variant">
            {kind === 'plan' ? 'Mô tả việc cần làm' : kind === 'e2e' ? 'Yêu cầu kiểm thử' : 'Nội dung prompt'}
          </span>
          <textarea
            bind:value={prompt}
            rows="4"
            placeholder={kind === 'plan'
              ? 'Ví dụ: Thêm trang cài đặt hồ sơ người dùng, kèm unit test...'
              : kind === 'e2e'
                ? 'Ví dụ: Mở http://localhost:5173 và [EXPECT: Đăng nhập]'
                : 'Ví dụ: Rà soát code trong workspace và đề xuất cải thiện...'}
            class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-3 text-xs text-on-surface outline-none focus:border-primary resize-none font-mono leading-relaxed"
          ></textarea>
        </div>
      {/if}

      {#if needsModel}
        <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
          <span class="text-xs font-bold uppercase text-on-surface-variant">Chạy bằng</span>
          <ModelSelect
            value={schedulerModel}
            on:change={onSchedulerModelChange}
            selectClass="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2 text-xs font-semibold text-on-surface outline-none focus:border-primary cursor-pointer"
          />
          <p class="text-[11px] text-on-surface-variant">
            Hai nhà cung cấp có hạn mức riêng — hẹn việc nặng vào bên vừa reset quota.
          </p>
          {#if usesAntiPool}
            <label class="flex items-start gap-2 cursor-pointer pt-1">
              <input type="checkbox" bind:checked={waitForQuota} class="w-4 h-4 rounded accent-primary cursor-pointer mt-0.5" />
              <span class="min-w-0">
                <span class="block text-xs font-medium text-on-surface">⏳ Chờ quota hồi rồi mới chạy</span>
                <span class="block text-[11px] leading-snug text-on-surface-variant">
                  Đến giờ mà cả pool đang cạn (bị 429 trong vòng 24h đo được), lịch sẽ đợi và
                  tự chạy ngay khi có tài khoản sẵn sàng — không đốt lượt chạy vào tường.
                </span>
              </span>
            </label>
          {/if}
        </div>
      {/if}

      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3">
        <div class="flex items-center justify-between">
          <span class="text-xs font-bold uppercase text-on-surface-variant">Thời điểm</span>
          <div class="flex gap-1 bg-surface-container-high rounded-lg p-0.5">
            <button
              type="button"
              on:click={() => (mode = 'time')}
              class="px-2.5 py-1 rounded-md text-[11px] font-bold transition-colors cursor-pointer
              {mode === 'time' ? 'bg-surface-container-lowest text-on-surface' : 'text-on-surface-variant'}"
            >Giờ cụ thể</button>
            <button
              type="button"
              on:click={() => (mode = 'countdown')}
              class="px-2.5 py-1 rounded-md text-[11px] font-bold transition-colors cursor-pointer
              {mode === 'countdown' ? 'bg-surface-container-lowest text-on-surface' : 'text-on-surface-variant'}"
            >Đếm ngược</button>
          </div>
        </div>

        {#if mode === 'time'}
          <div class="flex items-center gap-2">
            <input type="number" min="0" max="23" bind:value={timeHour}
              class="w-16 bg-surface-container-low border border-outline-variant rounded-lg p-2 text-center font-mono text-lg font-bold text-primary outline-none focus:border-primary" />
            <span class="text-lg font-bold text-on-surface-variant">:</span>
            <input type="number" min="0" max="59" bind:value={timeMin}
              class="w-16 bg-surface-container-low border border-outline-variant rounded-lg p-2 text-center font-mono text-lg font-bold text-primary outline-none focus:border-primary" />
            <div class="flex gap-1 ml-2">
              {#each ['02:00', '08:00', '18:00'] as preset}
                <button type="button" class="px-2 py-1 text-[11px] font-bold rounded-lg border border-outline-variant text-on-surface-variant hover:bg-surface-container-high cursor-pointer"
                  on:click={() => { [timeHour, timeMin] = preset.split(':'); }}>{preset}</button>
              {/each}
            </div>
          </div>
        {:else}
          <div class="flex items-end gap-2">
            {#each [['Giờ', 'cdHour'], ['Phút', 'cdMin'], ['Giây', 'cdSec']] as [label, field]}
              <div class="flex flex-col gap-1">
                <span class="text-[10px] uppercase font-bold text-on-surface-variant">{label}</span>
                {#if field === 'cdHour'}
                  <input type="number" min="0" max="99" bind:value={cdHour} class="w-16 bg-surface-container-low border border-outline-variant rounded-lg p-2 text-center font-mono text-lg font-bold text-primary outline-none focus:border-primary" />
                {:else if field === 'cdMin'}
                  <input type="number" min="0" max="59" bind:value={cdMin} class="w-16 bg-surface-container-low border border-outline-variant rounded-lg p-2 text-center font-mono text-lg font-bold text-primary outline-none focus:border-primary" />
                {:else}
                  <input type="number" min="0" max="59" bind:value={cdSec} class="w-16 bg-surface-container-low border border-outline-variant rounded-lg p-2 text-center font-mono text-lg font-bold text-primary outline-none focus:border-primary" />
                {/if}
              </div>
            {/each}
          </div>
        {/if}

        <label class="flex items-center gap-2 cursor-pointer pt-1">
          <input type="checkbox" bind:checked={repeat} class="w-4 h-4 rounded accent-primary cursor-pointer" />
          <span class="text-xs font-medium text-on-surface">Lặp lại hằng ngày</span>
        </label>

        <button
          type="button"
          on:click={schedule}
          disabled={busy}
          class="w-full bg-primary text-on-primary py-2.5 rounded-xl text-xs font-bold hover:opacity-90 disabled:opacity-50 transition-all cursor-pointer flex items-center justify-center gap-1.5"
        >
          <span class="material-symbols-outlined text-sm">{busy ? 'sync' : 'alarm_add'}</span>
          {busy ? 'Đang đặt...' : 'Đặt lịch'}
        </button>
      </div>
    </div>

    <!-- Jobs + log -->
    <div class="col-span-12 lg:col-span-7 space-y-4">
      <!-- Git watch: AutoSnapshot guards the diff during each task; this
           guards what comes after — a night of agent work sitting as one
           loose diff until somebody remembers. -->
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3">
        <div class="flex items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-lg text-primary">visibility</span>
            <div>
              <span class="block text-xs font-bold text-on-surface">Canh chừng Git</span>
              <span class="block text-[11px] text-on-surface-variant">Workspace bẩn quá lâu chưa commit thì nhắc — hoặc để AI tự commit.</span>
            </div>
          </div>
          <label class="flex items-center gap-2 cursor-pointer">
            <input type="checkbox" bind:checked={gw.enabled} class="w-4 h-4 rounded accent-primary cursor-pointer" />
            <span class="text-xs font-bold {gw.enabled ? 'text-primary' : 'text-on-surface-variant'}">{gw.enabled ? 'BẬT' : 'TẮT'}</span>
          </label>
        </div>
        {#if gw.enabled}
          <div class="flex flex-wrap items-center gap-4 pt-1">
            <label class="flex items-center gap-2 text-xs text-on-surface">
              Ngưỡng
              <input type="number" min="1" max="72" bind:value={gw.threshold_hours}
                class="w-14 bg-surface-container-low border border-outline-variant rounded-lg p-1.5 text-center font-mono font-bold text-primary outline-none focus:border-primary" />
              giờ bẩn liên tục
            </label>
            <label class="flex items-center gap-2 cursor-pointer text-xs text-on-surface">
              <input type="checkbox" bind:checked={gw.auto_commit} class="w-4 h-4 rounded accent-primary cursor-pointer" />
              Tự commit (AI soạn message) thay vì chỉ nhắc
            </label>
          </div>
        {/if}
        <button
          type="button"
          on:click={saveGitWatch}
          disabled={gwSaving}
          class="bg-primary text-on-primary px-4 py-1.5 rounded-lg text-xs font-bold hover:opacity-90 disabled:opacity-50 cursor-pointer"
        >
          {gwSaving ? 'Đang lưu...' : 'Lưu cài đặt'}
        </button>
      </div>

      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
        <span class="text-xs font-bold uppercase text-on-surface-variant">Lịch đã đặt</span>
        <div class="space-y-2 max-h-[420px] overflow-y-auto">
          {#each jobs as job (job.id)}
            {@const info = kindOf(job.kind)}
            <div class="border border-outline-variant rounded-xl p-3 space-y-2 {job.enabled ? '' : 'opacity-60'}">
              <div class="flex items-start justify-between gap-3">
                <div class="flex items-start gap-2.5 min-w-0">
                  <span class="material-symbols-outlined text-lg text-primary mt-0.5">{info.icon}</span>
                  <div class="min-w-0">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="text-xs font-bold text-on-surface">{info.label}</span>
                      {#if job.repeat}
                        <span class="text-[10px] font-bold uppercase text-secondary bg-secondary/10 border border-secondary/20 px-1.5 py-0.5 rounded">↻ hằng ngày</span>
                      {/if}
                      {#if job.wait_for_quota}
                        <span class="text-[10px] font-bold uppercase text-amber-600 bg-amber-500/10 border border-amber-500/20 px-1.5 py-0.5 rounded" title="Đến giờ mà pool cạn quota thì lịch đợi, chạy ngay khi quota hồi">⏳ chờ quota</span>
                      {/if}
                      {#if !job.enabled}
                        <span class="text-[10px] font-bold uppercase text-on-surface-variant bg-surface-container-high px-1.5 py-0.5 rounded">tạm dừng</span>
                      {/if}
                    </div>
                    {#if job.prompt}
                      <p class="text-[11px] text-on-surface-variant line-clamp-2 mt-0.5">{job.prompt}</p>
                    {/if}
                    <p class="text-[11px] font-mono text-on-surface-variant mt-1">
                      {whenLabel(job.target_time)}{#if job.model} · {job.model}{/if}
                    </p>
                  </div>
                </div>

                <div class="flex items-center gap-1 flex-shrink-0">
                  <button type="button" on:click={() => toggleJob(job)}
                    title={job.enabled ? 'Tạm dừng' : 'Bật lại'}
                    class="p-1.5 rounded-lg text-on-surface-variant hover:bg-surface-container-high cursor-pointer">
                    <span class="material-symbols-outlined text-base">{job.enabled ? 'pause' : 'play_arrow'}</span>
                  </button>
                  <button type="button" on:click={() => cancelJob(job.id)} title="Xoá lịch"
                    class="p-1.5 rounded-lg text-rose-600 hover:bg-rose-500/10 cursor-pointer">
                    <span class="material-symbols-outlined text-base">delete</span>
                  </button>
                </div>
              </div>

              {#if hasRun(job)}
                <div class="flex items-start gap-1.5 text-[11px] border-t border-outline-variant/60 pt-2">
                  <span class="material-symbols-outlined text-sm {job.last_status === 'ok' ? 'text-emerald-600' : 'text-rose-600'}">
                    {job.last_status === 'ok' ? 'check_circle' : 'error'}
                  </span>
                  <span class="text-on-surface-variant">
                    Lần chạy gần nhất {whenLabel(job.last_run_at)} · đã chạy {job.run_count} lần
                    {#if job.last_error}<span class="block text-rose-600 font-mono break-all mt-0.5">{job.last_error}</span>{/if}
                  </span>
                </div>
              {/if}
            </div>
          {:else}
            <div class="py-10 text-center text-xs text-on-surface-variant italic">
              Chưa có lịch nào. Việc đã đặt vẫn còn sau khi tắt app.
            </div>
          {/each}
        </div>
      </div>

      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
        <div class="flex items-center justify-between">
          <span class="text-xs font-bold uppercase text-on-surface-variant">Nhật ký ({logs.length})</span>
          <button type="button" on:click={() => (logs = [])}
            class="text-[11px] font-bold text-on-surface-variant hover:text-on-surface cursor-pointer">Xoá</button>
        </div>
        <div class="bg-black/90 rounded-lg p-3 font-mono text-[11px] h-40 overflow-y-auto space-y-1">
          {#each logs as log}
            <div class="flex gap-2">
              <span class="text-slate-500 flex-shrink-0">[{log.time}]</span>
              <span class={log.level === 'SUCCESS' ? 'text-emerald-400' : log.level === 'ERROR' ? 'text-rose-400' : log.level === 'WARN' ? 'text-amber-300' : 'text-slate-300'}>
                {log.msg}
              </span>
            </div>
          {:else}
            <div class="text-slate-600 italic">Chưa có hoạt động nào.</div>
          {/each}
        </div>
      </div>
    </div>
  </div>
</div>
