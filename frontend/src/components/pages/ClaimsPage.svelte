<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog, addToast } from '../../lib/stores/appState';

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
  // Chốt phiên là hành động không quay lại được, nên hỏi lại bằng modal trong
  // app thay vì confirm() của webview (confirm() khoá luôn luồng WebView2).
  let confirmFinish = false;

  let sessionIds: string[] = [];
  let selected = '';
  let subject = '';
  let joinCommand = '';
  let joinTargets: { scope: string; label: string; host: string; command: string; bootstrap: string; mcp: string; prompt: string }[] = [];
  let toolInstalled = true;

  // The three doors into a session used to be one visible command plus two
  // copy-only buttons — the MCP door existed but nobody could see what it
  // copied. A picker that shows the actual command for each door replaces it.
  type JoinMode = 'cli' | 'bootstrap' | 'mcp';
  let joinMode: JoinMode = 'cli';
  const JOIN_MODES: { id: JoinMode; label: string; hint: string }[] = [
    {
      id: 'cli',
      label: 'Đã cài app',
      hint: 'Máy đó đã cài Claude Suite nên có sẵn công cụ claude-suite-claim. Dán lệnh cho agent chạy — nó nộp claim rồi tự chờ kết quả check.',
    },
    {
      id: 'bootstrap',
      label: 'Chưa cài — tự tải',
      hint: 'Một lệnh PowerShell (Windows): tự tải claude-suite-claim.exe từ GitHub về thư mục tạm rồi vào phiên. Không cần cài app.',
    },
    {
      id: 'mcp',
      label: 'MCP — không tải gì',
      hint: 'Đăng ký phiên này làm MCP server cho agent (Claude Code, Gemini CLI…). Agent nhận ngay các tool join_session, submit_claim, say, wait_for_chat. Chạy lệnh xong mà chưa thấy tool thì khởi động lại agent.',
    },
  ];

  function shortHost(h: string) {
    return h.replace(/^ws:\/\//, '');
  }

  let chat: { author: string; text: string; at: string }[] = [];
  let chatDraft = '';
  let arbiterName = 'trọng tài';

  function copyCommand(cmd: string) {
    navigator.clipboard?.writeText(cmd);
    addToast('Đã sao chép lệnh.', 'SUCCESS');
  }

  // ── Remote members (no shared LAN, no shared tailnet) ─────────────────
  // The session owner runs a tunnel on THIS machine and pastes its public
  // URL; the backend turns it into the same four hand-outs the discovered
  // addresses get.
  let joinToken = '';
  let publicHost = '';
  let remoteTarget: { host: string; command: string; bootstrap: string; mcp: string; prompt: string } | null = null;
  let remoteBusy = false;

  async function buildRemoteCommands() {
    if (!publicHost.trim() || !selected) return;
    remoteBusy = true;
    try {
      const res = await (AppBindings as any).BuildJoinCommands(publicHost.trim(), selected, joinToken, subject.trim());
      if (res?.error) {
        addToast(res.error, 'ERROR');
        remoteTarget = null;
      } else {
        remoteTarget = res;
      }
    } catch (e) {
      addToast(`Không tạo được lệnh: ${e}`, 'ERROR');
    } finally {
      remoteBusy = false;
    }
  }

  function copyText(text: string, what: string) {
    navigator.clipboard?.writeText(text);
    addToast(`Đã sao chép ${what}.`, 'SUCCESS');
  }

  async function sendChat() {
    const text = chatDraft.trim();
    if (!text || !selected) return;
    chatDraft = '';
    try {
      await (AppBindings as any).SayInClaimSession(selected, arbiterName, text);
      await refreshChat();
    } catch (e) {
      addToast(`Không gửi được tin nhắn: ${e}`, 'ERROR');
      chatDraft = text;
    }
  }

  async function refreshChat() {
    if (!selected) {
      chat = [];
      return;
    }
    try {
      chat = (await (AppBindings as any).GetClaimChat(selected)) || [];
    } catch {
      chat = [];
    }
  }

  async function createChecksFile() {
    try {
      const path = await (AppBindings as any).CreateChecksFile();
      addToast(`Đã tạo ${path} — sửa lại cho khớp dự án của bạn.`, 'SUCCESS', 9000);
      await refreshStatus();
    } catch (e) {
      addToast(`Không tạo được tệp checks: ${e}`, 'ERROR', 8000);
    }
  }

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
      await refreshChat();
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
        addToast(`Claims host đang lắng nghe trên ${res.addr}`, 'SUCCESS');
      } else {
        addLog(`Không mở được claims host: ${res?.error}`, 'ERROR');
        addToast(`Không mở được claims host: ${res?.error}`, 'ERROR');
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
        joinTargets = res.targets || [];
        toolInstalled = res.tool_installed !== false;
        // Kept for the remote-member panel: building commands for a pasted
        // tunnel URL needs the same session credentials.
        joinToken = res.token || '';
        remoteTarget = null;
        publicHost = '';
        // Open on the door that will actually work here: without the CLI tool
        // the main command only produces "not found".
        joinMode = toolInstalled ? 'cli' : 'mcp';
        await refreshStatus();
        await refreshSession();
      } else {
        addLog(`Không mở được phiên: ${res?.error}`, 'ERROR');
        addToast(`Không mở được phiên: ${res?.error}`, 'ERROR');
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
      addToast(`Không chạy được kiểm chứng: ${e?.message || e}`, 'ERROR');
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
      addToast(`Không kết thúc được phiên: ${e?.message || e}`, 'ERROR');
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
      addToast(`Không mở được vòng thảo luận: ${e?.message || e}`, 'ERROR');
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
    if (v === 'confirmed') return 'text-error';
    if (v === 'refuted') return 'text-success';
    if (v === 'escalated' || v === 'inconclusive') return 'text-warning';
    return 'text-on-surface-variant';
  }

  function verdictLabel(c: Claim) {
    if (c.kind === 'opinion') return 'Ý kiến — không chặn merge';
    switch (c.verdict) {
      case 'confirmed': return 'Xác nhận — chặn merge';
      case 'refuted': return 'Bác bỏ';
      case 'inconclusive': return 'Không kết luận — cần người xem';
      case 'escalated': return 'Chuyển người quyết';
      default: return 'Chờ kiểm chứng';
    }
  }

  let offClaimsEvent: (() => void) | null = null;

  onMount(() => {
    refreshStatus();
    poll = setInterval(() => { refreshStatus(); refreshSession(); }, 2000);

    if ((window as any)?.runtime?.EventsOn) {
      // Captured for cleanup — the page is destroyed on every tab switch and
      // an uncancelled listener keeps appending into a dead component.
      offClaimsEvent = (window as any).runtime.EventsOn('claims_event', (d: any) => {
        if (d?.message) events = [...events, `[${d.time || ''}] ${d.message}`].slice(-40);
      });
    }
  });

  onDestroy(() => {
    if (poll) clearInterval(poll);
    offClaimsEvent?.();
  });

  $: if (selected) refreshSession();
</script>

<div class="space-y-5 font-sans">
  <div class="flex flex-wrap items-end justify-between gap-3">
    <div class="space-y-0.5">
      <div class="flex items-center gap-2">
        <span class="material-symbols-outlined text-lg text-primary">gavel</span>
        <h1 class="text-lg font-semibold text-on-surface">Phân xử tranh chấp giữa các Agent</h1>
      </div>
      <p class="text-xs text-on-surface-variant max-w-3xl">
        Khi agent của hai người kết luận trái ngược nhau, thắng thua do <b>kết quả chạy check</b>
        quyết định, không do bên nào lập luận nghe thuyết phục hơn. Claim không kèm được cách
        kiểm chứng sẽ bị ghi là ý kiến và <b>không chặn merge</b>.
      </p>
    </div>

    <div class="flex items-center gap-2">
      {#if running}
        <span class="flex items-center gap-1.5 text-[11px] font-medium text-success">
          <span class="w-1.5 h-1.5 rounded-full bg-success"></span>{addr}
        </span>
        <button type="button" on:click={stopHost} disabled={busy}
          class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer">
          Dừng host
        </button>
      {:else}
        <input type="number" bind:value={port} min="1024" max="65535"
          class="w-20 bg-surface-container-low border border-outline-variant rounded-lg px-2 py-1.5 text-xs font-mono text-on-surface outline-none focus:border-primary" />
        <button type="button" on:click={startHost} disabled={busy}
          class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer">
          Mở host
        </button>
      {/if}
    </div>
  </div>

  {#if hostWarning}
    <div class="bg-warning/10 border border-warning/30 rounded-xl px-4 py-2.5 text-xs text-on-surface flex items-start gap-2">
      <span class="material-symbols-outlined text-sm text-warning">warning</span>
      <div class="flex-1">
        <p>{hostWarning}</p>
        <!-- The warning used to state the problem and stop there. A workspace
             with no checks makes every claim an unfalsifiable opinion, so the
             feature does nothing for the user's own project until this exists. -->
        <button
          type="button"
          on:click={createChecksFile}
          class="mt-1.5 px-3 py-1.5 rounded-lg border border-outline-variant text-on-surface-variant text-xs font-medium hover:bg-surface-container-high transition-colors cursor-pointer"
        >
          Tạo tệp checks.json cho dự án này
        </button>
      </div>
    </div>
  {/if}

  <!-- Open a session -->
  <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3">
    <div class="flex flex-col md:flex-row gap-2">
      <input type="text" bind:value={subject} disabled={!running}
        placeholder="Tranh chấp về cái gì? ví dụ backend/cli/process_windows.go:17"
        class="flex-1 bg-surface-container-low border border-outline-variant rounded-lg px-3 py-2.5 text-xs text-on-surface font-mono outline-none focus:border-primary disabled:opacity-50" />
      <button type="button" on:click={openSession} disabled={!running || busy || !subject.trim()}
        class="bg-primary text-on-primary px-4 py-2.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer whitespace-nowrap">
        Mở phiên
      </button>
    </div>

    {#if joinTargets.length}
      <div class="space-y-3">
        <!-- The fastest door: one prompt that makes the agent do everything,
             including talking back in the discussion box below. -->
        <div class="bg-primary/5 border border-primary/25 rounded-xl p-3 space-y-2">
          <span class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
            <span class="material-symbols-outlined text-base text-on-surface-variant">smart_toy</span>
            Nhanh nhất: dán một prompt cho AI agent
          </span>
          <p class="text-xs text-on-surface-variant">
            Sao chép prompt rồi dán nguyên văn cho agent (Claude Code, Gemini CLI…). Trong prompt
            có sẵn địa chỉ, token và toàn bộ các bước: kết nối → điều tra → nộp claim → <b>tham gia
            thảo luận</b>. Agent sẽ tự làm từ đầu đến cuối, kể cả trả lời tin nhắn bạn gõ ở ô
            "Thảo luận trong phiên" bên dưới.
          </p>
          <div class="flex items-center gap-2 flex-wrap">
            {#each joinTargets as target}
              <button
                type="button"
                on:click={() => copyText(target.prompt, 'prompt cho AI agent')}
                class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-primary text-on-primary text-xs font-medium hover:opacity-90 transition-opacity cursor-pointer whitespace-nowrap"
              >
                <span class="material-symbols-outlined text-sm">content_copy</span>
                {target.scope === 'local' ? 'Agent trên máy này' : `Agent máy khác — ${shortHost(target.host)}`}
              </button>
            {/each}
          </div>
          <details class="text-xs text-on-surface-variant">
            <summary class="cursor-pointer font-medium">Xem nội dung prompt</summary>
            <pre class="mt-1.5 bg-surface-container-high border border-outline-variant rounded-lg p-3 text-[11px] font-mono text-on-surface overflow-x-auto whitespace-pre-wrap break-words max-h-64 overflow-y-auto">{joinTargets[0].prompt}</pre>
          </details>
        </div>

        <!-- The manual doors, one visible command per door instead of the old
             copy-only buttons: the MCP command existed but nobody ever saw it. -->
        <div class="space-y-2">
          <div class="flex items-center gap-1.5 flex-wrap">
            <span class="text-xs font-medium text-on-surface-variant">Hoặc tự chạy lệnh — máy của agent thuộc trường hợp nào?</span>
            {#each JOIN_MODES as mode}
              <button
                type="button"
                on:click={() => (joinMode = mode.id)}
                class="px-3 py-1.5 rounded-lg text-xs font-medium cursor-pointer border transition-colors {joinMode === mode.id
                  ? 'bg-primary text-on-primary border-primary'
                  : 'border-outline-variant text-on-surface-variant hover:bg-surface-container-high'}"
              >
                {mode.label}
              </button>
            {/each}
          </div>

          {#each JOIN_MODES as mode}
            {#if joinMode === mode.id}
              <p class="text-xs text-on-surface-variant">{mode.hint}</p>
            {/if}
          {/each}

          {#if joinMode === 'cli' && !toolInstalled}
            <div class="bg-warning/10 border border-warning/30 rounded-lg px-3 py-2 text-xs text-on-surface flex items-start gap-2">
              <span class="material-symbols-outlined text-sm text-warning">warning</span>
              <span>
                Máy này chưa có <b>claude-suite-claim</b> trên PATH, nên lệnh dưới sẽ báo "not found".
                Chuyển sang <b>Chưa cài — tự tải</b> hoặc <b>MCP — không tải gì</b>.
              </span>
            </div>
          {/if}

          <!-- One address per case: on a teammate's machine "localhost" is their
               machine, where no session is listening. -->
          {#each joinTargets as target}
            {@const cmd = joinMode === 'cli' ? target.command : joinMode === 'bootstrap' ? target.bootstrap : target.mcp}
            <div class="space-y-1">
              <div class="flex items-center justify-between gap-2">
                <span class="text-xs font-medium text-on-surface-variant">
                  {target.label}
                  <span class="font-mono opacity-70">({target.host})</span>
                </span>
                <button
                  type="button"
                  on:click={() => copyCommand(cmd)}
                  class="text-xs text-primary font-medium hover:underline cursor-pointer whitespace-nowrap"
                >
                  Sao chép
                </button>
              </div>
              <pre class="bg-surface-container-high border border-outline-variant rounded-lg p-3 text-[11px] font-mono text-on-surface overflow-x-auto whitespace-pre-wrap break-all">{cmd}</pre>
            </div>
          {/each}

          {#if joinMode === 'mcp'}
            <p class="text-xs text-on-surface-variant">
              Sau khi đăng ký, agent làm theo thứ tự: <span class="font-mono">join_session</span> (giữ lại
              participant_key) → <span class="font-mono">submit_claim</span> → <span class="font-mono">finish_reporting</span> →
              nghe và trả lời chat bằng <span class="font-mono">wait_for_chat</span> / <span class="font-mono">say</span>.
              Với Gemini CLI, thay <span class="font-mono">claude mcp add</span> bằng <span class="font-mono">gemini mcp add</span>.
            </p>
          {/if}

          {#if joinTargets.some((t) => t.scope === 'lan')}
            <p class="text-xs text-on-surface-variant">
              Máy khác trong LAN: lần đầu kết nối, Windows Firewall trên máy này sẽ hỏi — phải chọn
              <b>Allow</b> thì đồng đội mới vào được.
            </p>
          {/if}

          <!-- Neither LAN nor tailnet reaches this host; a tunnel does. The
               owner runs it here, pastes the public URL, and the backend
               mints the same four hand-outs for it. -->
          <details class="border border-outline-variant rounded-xl px-3 py-2">
            <summary class="cursor-pointer text-xs font-medium text-on-surface">
              Thành viên ở xa — không chung LAN, không chung VPN?
            </summary>
            <div class="mt-2 space-y-2">
              {#if joinTargets.some((t) => t.scope === 'vpn')}
                <p class="text-xs text-on-surface-variant">
                  Máy này có Tailscale — cách gọn nhất là mời đồng đội vào tailnet của bạn, rồi họ dùng
                  địa chỉ <b>"Qua VPN (Tailscale)"</b> ở trên từ bất kỳ đâu.
                </p>
              {/if}
              <p class="text-xs text-on-surface-variant">
                Không dùng VPN: mở một tunnel trên <b>chính máy này</b> rồi đưa URL công khai cho đồng đội.
                Chạy một trong hai lệnh (cần cài <span class="font-mono">cloudflared</span> hoặc <span class="font-mono">ngrok</span>):
              </p>
              <div class="flex items-center gap-2">
                <pre class="flex-1 bg-surface-container-high border border-outline-variant rounded-lg p-2 text-[11px] font-mono text-on-surface overflow-x-auto">cloudflared tunnel --url http://localhost:{port}</pre>
                <button type="button" on:click={() => copyCommand(`cloudflared tunnel --url http://localhost:${port}`)}
                  class="text-xs text-primary font-medium hover:underline cursor-pointer whitespace-nowrap">Sao chép</button>
              </div>
              <div class="flex items-center gap-2">
                <pre class="flex-1 bg-surface-container-high border border-outline-variant rounded-lg p-2 text-[11px] font-mono text-on-surface overflow-x-auto">ngrok http {port}</pre>
                <button type="button" on:click={() => copyCommand(`ngrok http ${port}`)}
                  class="text-xs text-primary font-medium hover:underline cursor-pointer whitespace-nowrap">Sao chép</button>
              </div>
              <p class="text-xs text-on-surface-variant">
                Lệnh in ra một URL dạng <span class="font-mono">https://…</span> — dán vào đây để tạo bộ lệnh cho URL đó
                (URL + token trong lệnh là chìa khoá vào phiên: chỉ gửi cho người mình mời):
              </p>
              <div class="flex items-center gap-2">
                <input
                  type="text"
                  bind:value={publicHost}
                  placeholder="https://abc.trycloudflare.com"
                  class="flex-1 bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-2 text-xs font-mono text-on-surface outline-none focus:border-primary"
                />
                <button
                  type="button"
                  on:click={buildRemoteCommands}
                  disabled={remoteBusy || !publicHost.trim()}
                  class="border border-outline-variant text-on-surface-variant px-3 py-2 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer whitespace-nowrap"
                >
                  {remoteBusy ? 'Đang tạo...' : 'Tạo lệnh'}
                </button>
              </div>
              {#if remoteTarget}
                {@const rt = remoteTarget}
                {@const rcmd = joinMode === 'cli' ? rt.command : joinMode === 'bootstrap' ? rt.bootstrap : rt.mcp}
                <div class="space-y-1">
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-xs font-medium text-on-surface-variant">
                      Qua tunnel <span class="font-mono opacity-70">({rt.host})</span>
                    </span>
                    <span class="flex items-center gap-3">
                      <button type="button" on:click={() => copyText(rt.prompt, 'prompt cho AI agent')}
                        class="text-xs text-primary font-medium hover:underline cursor-pointer whitespace-nowrap">Prompt cho AI</button>
                      <button type="button" on:click={() => copyCommand(rcmd)}
                        class="text-xs text-primary font-medium hover:underline cursor-pointer whitespace-nowrap">Sao chép</button>
                    </span>
                  </div>
                  <pre class="bg-surface-container-high border border-outline-variant rounded-lg p-3 text-[11px] font-mono text-on-surface overflow-x-auto whitespace-pre-wrap break-all">{rcmd}</pre>
                  <p class="text-xs text-on-surface-variant">
                    Tunnel phải chạy suốt phiên; tắt tunnel là đồng đội mất kết nối. URL trycloudflare đổi mỗi lần chạy — phiên mới thì tạo lệnh lại.
                  </p>
                </div>
              {/if}
            </div>
          </details>
        </div>
      </div>
    {/if}

    {#if checks.length}
      <details class="text-xs text-on-surface-variant">
        <summary class="cursor-pointer font-medium">Check có thể trỏ tới ({checks.length})</summary>
        <div class="mt-1.5 flex flex-wrap gap-1.5">
          {#each checks as c}
            <span class="bg-surface-container-high border border-outline-variant rounded-lg px-2 py-0.5 font-mono">{c}</span>
          {/each}
        </div>
        <p class="mt-1.5">Falsifier <b>PASS khi claim SAI</b>, nên check fail là xác nhận lỗi có thật.</p>
      </details>
    {/if}
  </div>

  {#if selected}
    <!-- Phase -->
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-1">
          {#each PHASES as p, i}
            <div class="flex items-center gap-1">
              <span class="px-2 py-1 rounded-lg text-[11px] font-medium {phase === p.id ? 'bg-primary text-on-primary' : 'text-on-surface-variant'}">{p.label}</span>
              {#if i < PHASES.length - 1}
                <span class="material-symbols-outlined text-sm text-outline">chevron_right</span>
              {/if}
            </div>
          {/each}
        </div>

        <div class="flex items-center gap-2">
          {#if phase === 'collect'}
            <button type="button" on:click={forceAdjudicate} disabled={busy}
              class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer">
              Chạy kiểm chứng ngay
            </button>
          {/if}
          {#if (phase === 'reveal' || phase === 'debate') && opinions.length > 0 && round < maxRound}
            <button type="button" on:click={openDebate} disabled={busy}
              class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer">
              Mở vòng thảo luận {round + 1}/{maxRound}
            </button>
          {/if}
          <!-- Backend từ chối chốt khi phiên còn đang thu thập mù, nên nút không
               hiện ở phase 'collect' thay vì bấm xong mới báo lỗi. -->
          {#if phase && phase !== 'record' && phase !== 'collect'}
            <button type="button" on:click={() => (confirmFinish = true)} disabled={busy}
              class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer">
              Chốt phiên
            </button>
          {/if}
        </div>
      </div>

      {#if phase === 'collect'}
        <p class="text-xs text-on-surface-variant">
          Đang thu thập <b>mù</b>: agent chưa thấy claim của nhau. Chỉ sau khi chạy xong check
          chúng mới được đối chiếu — đảo thứ tự này là biến review thành cuộc thi ai tự tin hơn.
          Nếu mới có <b>một</b> agent nộp xong, phiên vẫn được giữ mở thêm ~90 giây cho agent
          còn lại kịp join — bấm "Chạy kiểm chứng ngay" nếu không muốn chờ. Chưa chạy kiểm chứng
          thì chưa chốt phiên được.
        </p>
      {:else if phase === 'reveal' && opinions.length > 0}
        <p class="text-xs text-on-surface-variant">
          Check đã chạy xong. Còn <b>{opinions.length} ý kiến</b> không có gì kiểm chứng được —
          chỉ những cái đó mới đưa ra thảo luận, và tối đa {maxRound} vòng.
        </p>
      {:else if phase === 'debate'}
        <p class="text-xs text-on-surface-variant">
          Vòng <b>{round}/{maxRound}</b>. Chỉ ý kiến mới thảo luận được; những gì check đã
          kết luận thì không mở lại bằng lời. Ý kiến thiểu số vẫn được ghi lại nguyên vẹn.
        </p>
      {:else if phase === 'reveal'}
        <p class="text-xs text-on-surface-variant">
          Check đã chạy xong và mọi claim đều có kết luận — không còn gì để thảo luận.
        </p>
      {/if}

      {#each warnings as w}
        <div class="bg-warning/10 border border-warning/30 rounded-lg px-3 py-2 text-xs text-on-surface">
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
              <div class="text-[11px] font-mono text-on-surface-variant truncate">{c.subject}</div>
            </div>
            <span class="text-[11px] font-semibold whitespace-nowrap {verdictStyle(c.verdict)}">
              {verdictLabel(c)}
            </span>
          </div>

          <div class="flex flex-wrap items-center gap-2 text-[11px] text-on-surface-variant">
            <span>bởi <b class="text-on-surface">{c.author}</b></span>
            {#if c.falsifier}
              <span class="font-mono bg-surface-container-high border border-outline-variant rounded-lg px-1.5">{c.falsifier}</span>
            {:else}
              <span>không có check — không chặn được merge</span>
            {/if}
            {#if c.exit_code}
              <span class="font-mono">exit {c.exit_code}</span>
            {/if}
          </div>

          {#if c.evidence}
            <pre class="bg-surface-container-highest border border-outline-variant text-on-surface rounded-lg p-2 text-[11px] font-mono max-h-32 overflow-y-auto whitespace-pre-wrap break-all">{c.evidence}</pre>
          {/if}

          {#if remarksFor(c.id).length}
            <div class="border-l-2 border-outline-variant pl-2.5 space-y-1">
              {#each remarksFor(c.id) as r}
                <div class="text-[11px] text-on-surface-variant">
                  <span class="font-semibold text-on-surface">{r.author}</span>
                  <span class="opacity-60">vòng {r.round}</span>
                  <div class="whitespace-pre-wrap">{r.text}</div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {:else}
        <div class="text-center text-xs text-on-surface-variant py-6">
          Chưa có claim nào. Đồng đội chạy lệnh ở trên để nộp.
        </div>
      {/each}
    </div>
  {/if}

  <!-- Discussion. Claims are narrow by design — a falsifiable assertion, and a
       remark that attaches to one during debate — which left nowhere for "I think
       the problem is upstream, checking now", i.e. most of a real review. Chat is
       never evidence: only a falsifier settles anything. -->
  {#if selected}
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3">
      <div class="flex items-center justify-between gap-3">
        <h4 class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-base text-on-surface-variant">forum</span>
          Thảo luận trong phiên
        </h4>
        <!-- Nhãn hiện rõ: trước đây chỉ có tooltip nên ô này trông như một hộp
             chữ không rõ để làm gì. -->
        <label class="flex items-center gap-1.5 text-xs font-medium text-on-surface-variant whitespace-nowrap">
          Tên của bạn
          <input
            bind:value={arbiterName}
            class="w-32 bg-surface-container-low border border-outline-variant rounded-lg px-2 py-1 text-xs text-on-surface outline-none focus:border-primary"
          />
        </label>
      </div>

      <div class="max-h-56 overflow-y-auto space-y-2 pr-1">
        {#each chat as m}
          <div class="flex gap-2 text-xs">
            <span class="font-semibold text-primary whitespace-nowrap">{m.author}</span>
            <span class="flex-1 text-on-surface whitespace-pre-wrap break-words">{m.text}</span>
            <span class="text-outline font-mono whitespace-nowrap">
              {m.at ? new Date(m.at).toLocaleTimeString('vi-VN') : ''}
            </span>
          </div>
        {:else}
          <p class="text-xs text-on-surface-variant py-3 text-center">
            Chưa có tin nhắn nào. Bạn nhắn ở ô dưới; agent nghe bằng tool
            <span class="font-mono">wait_for_chat</span> (MCP) hoặc
            <span class="font-mono">claude-suite-claim --listen</span>, và trả lời bằng
            <span class="font-mono">say</span> / <span class="font-mono">--say</span>.
          </p>
        {/each}
      </div>

      <div class="flex gap-2">
        <input
          bind:value={chatDraft}
          on:keydown={(e) => { if (e.key === 'Enter') sendChat(); }}
          placeholder="Nhắn cho các agent trong phiên…"
          class="flex-1 bg-surface-container-low border border-outline-variant rounded-lg px-3 py-2 text-xs text-on-surface outline-none focus:border-primary"
        />
        <button
          type="button"
          on:click={sendChat}
          disabled={!chatDraft.trim()}
          class="bg-primary text-on-primary px-4 py-2 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer"
        >
          Gửi
        </button>
      </div>
      <p class="text-xs text-on-surface-variant">
        Thảo luận không phải bằng chứng — chỉ falsifier mới kết luận được một claim.
        Agent được dặn chỉ trả lời khi bị gọi tên, bị hỏi, hoặc bị phản bác — muốn agent
        nào đáp thì nhắc thẳng tên nó trong tin nhắn.
      </p>
    </div>
  {/if}

  {#if events.length}
    <div class="bg-surface-container-highest border border-outline-variant rounded-xl p-3 text-[11px] font-mono text-on-surface-variant max-h-40 overflow-y-auto space-y-0.5">
      {#each events as e}
        <div class="whitespace-pre-wrap break-all">{e}</div>
      {/each}
    </div>
  {/if}
</div>

<!-- Hỏi lại trước khi chốt: chốt xong là không mở lại phiên được. Dùng modal
     trong app thay cho confirm() của webview. -->
{#if confirmFinish}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
    <div class="w-full max-w-sm bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3 shadow-lg">
      <h4 class="text-sm font-semibold text-on-surface">Chốt phiên này?</h4>
      <p class="text-xs text-on-surface-variant">
        Phiên sẽ chuyển sang trạng thái đã chốt: kết luận của các claim được ghi lại và
        không nộp thêm được nữa. Thao tác này không hoàn tác được.
      </p>
      <div class="flex justify-end gap-2 pt-1">
        <button
          type="button"
          on:click={() => (confirmFinish = false)}
          class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors cursor-pointer"
        >
          Huỷ
        </button>
        <button
          type="button"
          on:click={() => { confirmFinish = false; finishSession(); }}
          disabled={busy}
          class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer"
        >
          Chốt phiên
        </button>
      </div>
    </div>
  </div>
{/if}
