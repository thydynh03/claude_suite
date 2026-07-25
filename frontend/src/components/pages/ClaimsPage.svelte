<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog } from '../../lib/stores/appState';

  interface Claim {
    id: string;
    author: string;
    subject: string;
    assertion: string;
    falsifier: string;
    kind: 'verifiable' | 'costly' | 'opinion';
    verdict: 'pending' | 'confirmed' | 'refuted' | 'inconclusive' | 'escalated';
    evidence: string;
    exit_code: number;
  }

  let running = false;
  let addr = '';
  let checks: string[] = [];
  let hostWarning = '';
  let port = 9111;
  let busy = false;

  let sessionIds: string[] = [];
  let selected = '';
  let subject = '';
  let joinCommand = '';

  interface Remark { round: number; author: string; claim_id: string; text: string; }

  let phase = '';
  let claims: Claim[] = [];
  let warnings: string[] = [];
  let events: string[] = [];
  let opinions: Claim[] = [];
  let remarks: Remark[] = [];
  let round = 0;
  let maxRound = 2;

  let poll: ReturnType<typeof setInterval> | undefined;

  async function refreshStatus() {
    try {
      const s = await (AppBindings as any).ClaimsHostStatus();
      running = !!s?.running;
      addr = s?.addr || '';
      checks = s?.checks || [];
      hostWarning = s?.warning || '';
      sessionIds = s?.sessions || [];
      if (!selected && sessionIds.length) selected = sessionIds[sessionIds.length - 1];
    } catch (e: any) {
      console.warn('ClaimsHostStatus:', e);
    }
  }

  async function refreshSession() {
    if (!selected) return;
    try {
      const s = await (AppBindings as any).GetClaimSession(selected);
      if (!s?.found) return;
      phase = s.phase || '';
      claims = s.claims || [];
      warnings = s.warnings || [];
      opinions = s.opinions || [];
      remarks = s.remarks || [];
      round = s.round || 0;
      maxRound = s.maxRound || 2;
    } catch (e: any) {
      console.warn('GetClaimSession:', e);
    }
  }

  async function startHost() {
    busy = true;
    try {
      const res = await (AppBindings as any).StartClaimsHost(port);
      if (res?.success) {
        addLog(`Claims host đang lắng nghe trên ${res.addr}`, 'SUCCESS');
      } else {
        addLog(`Không mở được claims host: ${res?.error}`, 'ERROR');
      }
      await refreshStatus();
    } finally {
      busy = false;
    }
  }

  async function stopHost() {
    busy = true;
    try {
      await (AppBindings as any).StopClaimsHost();
      await refreshStatus();
    } finally {
      busy = false;
    }
  }

  async function openSession() {
    if (!subject.trim()) return;
    busy = true;
    try {
      const res = await (AppBindings as any).OpenClaimSession(subject.trim(), 60);
      if (res?.success) {
        selected = res.id;
        joinCommand = res.join_command || '';
        await refreshStatus();
        await refreshSession();
      } else {
        addLog(`Không mở được phiên: ${res?.error}`, 'ERROR');
      }
    } finally {
      busy = false;
    }
  }

  async function forceAdjudicate() {
    if (!selected) return;
    busy = true;
    try {
      await (AppBindings as any).ForceAdjudicateClaims(selected);
      await refreshSession();
    } catch (e: any) {
      addLog(`Không chạy được kiểm chứng: ${e?.message || e}`, 'ERROR');
    } finally {
      busy = false;
    }
  }

  async function finishSession() {
    if (!selected) return;
    busy = true;
    try {
      const outcome = await (AppBindings as any).FinishClaimSession(selected);
      addLog(
        `Phiên ${selected} kết thúc: ${outcome?.blocking?.length || 0} blocking, ` +
        `${outcome?.escalated?.length || 0} cần người quyết`,
        'SUCCESS'
      );
      await refreshSession();
    } catch (e: any) {
      addLog(`Không kết thúc được phiên: ${e?.message || e}`, 'ERROR');
    } finally {
      busy = false;
    }
  }

  async function openDebate() {
    if (!selected) return;
    busy = true;
    try {
      await (AppBindings as any).OpenClaimDebate(selected);
      await refreshSession();
    } catch (e: any) {
      addLog(`Không mở được vòng thảo luận: ${e?.message || e}`, 'ERROR');
    } finally {
      busy = false;
    }
  }

  function remarksFor(claimId: string) {
    return remarks.filter((r) => r.claim_id === claimId);
  }

  function copyJoin() {
    if (joinCommand) navigator.clipboard?.writeText(joinCommand);
  }

  const PHASES = [
    { id: 'collect', label: 'Thu thập (mù)' },
    { id: 'adjudicate', label: 'Chạy kiểm chứng' },
    { id: 'reveal', label: 'Công bố' },
    { id: 'debate', label: 'Thảo luận' },
    { id: 'record', label: 'Chốt' },
  ];

  function verdictStyle(v: string) {
    if (v === 'confirmed') return 'text-rose-600 dark:text-rose-400';
    if (v === 'refuted') return 'text-emerald-600 dark:text-emerald-400';
    if (v === 'escalated' || v === 'inconclusive') return 'text-amber-600 dark:text-amber-400';
    return 'text-on-surface-variant';
  }

  function verdictLabel(c: Claim) {
    if (c.kind === 'opinion') return 'Ý KIẾN — không chặn merge';
    switch (c.verdict) {
      case 'confirmed': return 'XÁC NHẬN — chặn merge';
      case 'refuted': return 'BÁC BỎ';
      case 'inconclusive': return 'KHÔNG KẾT LUẬN — cần người xem';
      case 'escalated': return 'CHUYỂN NGƯỜI QUYẾT';
      default: return 'CHỜ KIỂM CHỨNG';
    }
  }

  onMount(() => {
    refreshStatus();
    poll = setInterval(() => { refreshStatus(); refreshSession(); }, 2000);

    if ((window as any)?.runtime?.EventsOn) {
      (window as any).runtime.EventsOn('claims_event', (d: any) => {
        if (d?.message) events = [...events, `[${d.time || ''}] ${d.message}`].slice(-40);
      });
    }
  });

  onDestroy(() => { if (poll) clearInterval(poll); });

  $: if (selected) refreshSession();
</script>

<div class="space-y-5 font-sans">
  <div class="flex flex-wrap items-end justify-between gap-3">
    <div class="space-y-0.5">
      <div class="flex items-center gap-2">
        <span class="material-symbols-outlined text-xl text-primary">gavel</span>
        <h1 class="text-lg font-bold text-on-surface">Phân xử tranh chấp giữa các Agent</h1>
      </div>
      <p class="text-xs text-on-surface-variant max-w-3xl">
        Khi agent của hai người kết luận trái ngược nhau, thắng thua do <b>kết quả chạy check</b>
        quyết định, không do bên nào lập luận nghe thuyết phục hơn. Claim không kèm được cách
        kiểm chứng sẽ bị ghi là ý kiến và <b>không chặn merge</b>.
      </p>
    </div>

    <div class="flex items-center gap-2">
      {#if running}
        <span class="flex items-center gap-1.5 text-[11px] font-semibold text-emerald-600 dark:text-emerald-400">
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>{addr}
        </span>
        <button type="button" on:click={stopHost} disabled={busy}
          class="border border-outline-variant text-on-surface px-3 py-1.5 rounded-xl text-[11px] font-semibold hover:bg-surface-container-high disabled:opacity-50 cursor-pointer">
          Dừng host
        </button>
      {:else}
        <input type="number" bind:value={port} min="1024" max="65535"
          class="w-20 bg-surface-container-low border border-outline-variant rounded-lg px-2 py-1.5 text-[11px] font-mono text-on-surface outline-none focus:border-primary" />
        <button type="button" on:click={startHost} disabled={busy}
          class="bg-primary text-on-primary px-3 py-1.5 rounded-xl text-[11px] font-semibold hover:opacity-90 disabled:opacity-50 cursor-pointer">
          Mở host
        </button>
      {/if}
    </div>
  </div>

  {#if hostWarning}
    <div class="bg-amber-500/10 border border-amber-500/30 rounded-xl px-4 py-2.5 text-[11px] text-amber-700 dark:text-amber-300 flex items-start gap-2">
      <span class="material-symbols-outlined text-sm">warning</span>
      <span>{hostWarning}</span>
    </div>
  {/if}

  <!-- Open a session -->
  <div class="bg-surface-container-lowest border border-outline-variant rounded-2xl p-4 space-y-3">
    <div class="flex flex-col md:flex-row gap-2">
      <input type="text" bind:value={subject} disabled={!running}
        placeholder="Tranh chấp về cái gì? ví dụ backend/cli/process_windows.go:17"
        class="flex-1 bg-surface-container-low border border-outline-variant rounded-xl px-3 py-2.5 text-xs text-on-surface font-mono outline-none focus:border-primary disabled:opacity-50" />
      <button type="button" on:click={openSession} disabled={!running || busy || !subject.trim()}
        class="bg-primary text-on-primary px-4 py-2.5 rounded-xl text-xs font-semibold hover:opacity-90 disabled:opacity-40 cursor-pointer whitespace-nowrap">
        Mở phiên
      </button>
    </div>

    {#if joinCommand}
      <div class="space-y-1.5">
        <div class="flex items-center justify-between">
          <span class="text-[11px] font-semibold text-on-surface-variant">Gửi lệnh này cho đồng đội — agent IDE nào cũng chạy được:</span>
          <button type="button" on:click={copyJoin}
            class="text-[11px] text-primary font-semibold hover:underline cursor-pointer">Sao chép</button>
        </div>
        <pre class="bg-surface-container-high border border-outline-variant rounded-lg p-3 text-[10px] font-mono text-on-surface overflow-x-auto whitespace-pre-wrap break-all">{joinCommand}</pre>
      </div>
    {/if}

    {#if checks.length}
      <details class="text-[11px] text-on-surface-variant">
        <summary class="cursor-pointer font-semibold">Check có thể trỏ tới ({checks.length})</summary>
        <div class="mt-1.5 flex flex-wrap gap-1.5">
          {#each checks as c}
            <span class="bg-surface-container-high border border-outline-variant rounded px-2 py-0.5 font-mono">{c}</span>
          {/each}
        </div>
        <p class="mt-1.5">Falsifier <b>PASS khi claim SAI</b>, nên check fail là xác nhận lỗi có thật.</p>
      </details>
    {/if}
  </div>

  {#if selected}
    <!-- Phase -->
    <div class="bg-surface-container-lowest border border-outline-variant rounded-2xl p-4 space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-1">
          {#each PHASES as p, i}
            <div class="flex items-center gap-1">
              <span class="px-2 py-1 rounded-lg text-[10px] font-semibold {phase === p.id ? 'bg-primary text-on-primary' : 'text-on-surface-variant'}">{p.label}</span>
              {#if i < PHASES.length - 1}
                <span class="material-symbols-outlined text-xs text-outline">chevron_right</span>
              {/if}
            </div>
          {/each}
        </div>

        <div class="flex items-center gap-2">
          {#if phase === 'collect'}
            <button type="button" on:click={forceAdjudicate} disabled={busy}
              class="border border-outline-variant text-on-surface px-3 py-1.5 rounded-xl text-[11px] font-semibold hover:bg-surface-container-high disabled:opacity-50 cursor-pointer">
              Chạy kiểm chứng ngay
            </button>
          {/if}
          {#if (phase === 'reveal' || phase === 'debate') && opinions.length > 0 && round < maxRound}
            <button type="button" on:click={openDebate} disabled={busy}
              class="border border-outline-variant text-on-surface px-3 py-1.5 rounded-xl text-[11px] font-semibold hover:bg-surface-container-high disabled:opacity-50 cursor-pointer">
              Mở vòng thảo luận {round + 1}/{maxRound}
            </button>
          {/if}
          {#if phase && phase !== 'record'}
            <button type="button" on:click={finishSession} disabled={busy}
              class="bg-primary text-on-primary px-3 py-1.5 rounded-xl text-[11px] font-semibold hover:opacity-90 disabled:opacity-50 cursor-pointer">
              Chốt phiên
            </button>
          {/if}
        </div>
      </div>

      {#if phase === 'collect'}
        <p class="text-[11px] text-on-surface-variant">
          Đang thu thập <b>mù</b>: agent chưa thấy claim của nhau. Chỉ sau khi chạy xong check
          chúng mới được đối chiếu — đảo thứ tự này là biến review thành cuộc thi ai tự tin hơn.
        </p>
      {:else if phase === 'reveal' && opinions.length > 0}
        <p class="text-[11px] text-on-surface-variant">
          Check đã chạy xong. Còn <b>{opinions.length} ý kiến</b> không có gì kiểm chứng được —
          chỉ những cái đó mới đưa ra thảo luận, và tối đa {maxRound} vòng.
        </p>
      {:else if phase === 'debate'}
        <p class="text-[11px] text-on-surface-variant">
          Vòng <b>{round}/{maxRound}</b>. Chỉ ý kiến mới thảo luận được; những gì check đã
          kết luận thì không mở lại bằng lời. Ý kiến thiểu số vẫn được ghi lại nguyên vẹn.
        </p>
      {:else if phase === 'reveal'}
        <p class="text-[11px] text-on-surface-variant">
          Check đã chạy xong và mọi claim đều có kết luận — không còn gì để thảo luận.
        </p>
      {/if}

      {#each warnings as w}
        <div class="bg-amber-500/10 border border-amber-500/30 rounded-lg px-3 py-2 text-[11px] text-amber-700 dark:text-amber-300">
          {w}
        </div>
      {/each}
    </div>

    <!-- Claims -->
    <div class="space-y-2">
      {#each claims as c}
        <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-3 space-y-1.5">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="text-xs font-semibold text-on-surface">{c.assertion}</div>
              <div class="text-[10px] font-mono text-on-surface-variant truncate">{c.subject}</div>
            </div>
            <span class="text-[10px] font-bold whitespace-nowrap {verdictStyle(c.verdict)}">
              {verdictLabel(c)}
            </span>
          </div>

          <div class="flex flex-wrap items-center gap-2 text-[10px] text-on-surface-variant">
            <span>bởi <b class="text-on-surface">{c.author}</b></span>
            {#if c.falsifier}
              <span class="font-mono bg-surface-container-high border border-outline-variant rounded px-1.5">{c.falsifier}</span>
            {:else}
              <span class="italic">không có check — không chặn được merge</span>
            {/if}
            {#if c.exit_code}
              <span class="font-mono">exit {c.exit_code}</span>
            {/if}
          </div>

          {#if c.evidence}
            <pre class="bg-slate-950 text-emerald-400 rounded-lg p-2 text-[10px] font-mono max-h-32 overflow-y-auto whitespace-pre-wrap break-all">{c.evidence}</pre>
          {/if}

          {#if remarksFor(c.id).length}
            <div class="border-l-2 border-outline-variant pl-2.5 space-y-1">
              {#each remarksFor(c.id) as r}
                <div class="text-[10px] text-on-surface-variant">
                  <span class="font-semibold text-on-surface">{r.author}</span>
                  <span class="opacity-60">vòng {r.round}</span>
                  <div class="whitespace-pre-wrap">{r.text}</div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {:else}
        <div class="text-center text-[11px] text-on-surface-variant py-6">
          Chưa có claim nào. Đồng đội chạy lệnh ở trên để nộp.
        </div>
      {/each}
    </div>
  {/if}

  {#if events.length}
    <div class="bg-slate-950 border border-slate-800 rounded-xl p-3 text-[11px] font-mono text-emerald-400 max-h-40 overflow-y-auto space-y-0.5">
      {#each events as e}
        <div class="whitespace-pre-wrap break-all">{e}</div>
      {/each}
    </div>
  {/if}
</div>
