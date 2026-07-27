<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { layoutCommits, laneColor } from '../../lib/gitGraph';
  import { addLog, addToast, workspaceFolder } from '../../lib/stores/appState';

  type ChangeRow = { code: string; file: string };
  let branch = '';
  let branches: string[] = [];
  let changes: ChangeRow[] = [];
  let log: any[] = [];

  // Layout is derived from the fetched log, so it cannot go stale relative to
  // the rows it decorates.
  $: graph = layoutCommits(log as any);

  const LANE_W = 14;
  const ROW_H = 36;
  let diffText = '';
  let commitMsg = '';
  let cmdInput = '';
  let cmdOutput = '';
  let busy = false;
  let aiBusy = false;

  const statusLabel: Record<string, string> = {
    'M': 'Modified', 'A': 'Added', 'D': 'Deleted', 'R': 'Renamed',
    '??': 'Untracked', 'AM': 'Added*', 'MM': 'Modified*',
  };
  const statusColor = (c: string) =>
    c.includes('D') ? 'text-error' : c === '??' || c.includes('A') ? 'text-success' : 'text-warning';

  onMount(refreshAll);

  async function refreshAll() {
    await Promise.all([loadBranch(), loadChanges(), loadLog(), loadDiff()]);
  }

  async function loadBranch() {
    try {
      const info = await AppBindings.GetGitBranches();
      branch = (info as any)?.current || '';
      branches = (info as any)?.branches || [];
    } catch (e) {}
  }
  async function loadChanges() {
    try {
      const out = await AppBindings.RunGitCommand(['status', '--porcelain']);
      changes = out.split('\n').filter(Boolean).map((line) => ({
        code: line.slice(0, 2).trim(),
        file: line.slice(3).trim(),
      }));
    } catch (e) { changes = []; }
  }
  async function loadLog() {
    try { log = (await AppBindings.GetGitLog(30)) || []; } catch (e) { log = []; }
  }
  async function loadDiff() {
    try { diffText = await AppBindings.GetWorkspaceDiff(); } catch (e) { diffText = ''; }
  }

  async function aiCommitMessage() {
    aiBusy = true;
    try {
      commitMsg = await AppBindings.GenerateCommitMessage();
      addToast('AI đã tạo commit message.', 'SUCCESS');
    } catch (e: any) {
      addToast('Lỗi AI commit: ' + (e?.message || e), 'ERROR');
    } finally { aiBusy = false; }
  }

  async function stageAndCommit() {
    if (!commitMsg.trim()) { addToast('Nhập commit message trước.', 'WARN'); return; }
    busy = true;
    try {
      await AppBindings.RunGitCommand(['add', '.']);
      await AppBindings.CreateGitCommit(commitMsg);
      addToast('Đã commit.', 'SUCCESS');
      commitMsg = '';
      await refreshAll();
    } catch (e: any) {
      addToast('Lỗi commit: ' + (e?.message || e), 'ERROR');
    } finally { busy = false; }
  }

  async function checkout(name: string) {
    try { await AppBindings.CheckoutGitBranch(name); await refreshAll(); addToast('Đã chuyển sang ' + name, 'INFO'); }
    catch (e: any) { addToast('Lỗi checkout: ' + (e?.message || e), 'ERROR'); }
  }

  async function revert(hash: string) {
    if (!confirm(`Revert commit ${hash.slice(0, 8)}?`)) return;
    try { await AppBindings.RevertGitCommit(hash); await refreshAll(); addToast('Đã revert ' + hash.slice(0, 8), 'WARN'); }
    catch (e: any) { addToast('Lỗi revert: ' + (e?.message || e), 'ERROR'); }
  }

  async function runCmd() {
    const parts = cmdInput.trim().split(/\s+/).filter(Boolean);
    if (parts.length === 0) return;
    busy = true;
    try {
      cmdOutput = await AppBindings.RunGitCommand(parts);
      await refreshAll();
    } catch (e: any) {
      cmdOutput = String(e?.message || e);
    } finally { busy = false; }
  }
</script>

<div class="max-w-6xl mx-auto space-y-4 pb-12">
  <!-- Header -->
  <div class="flex items-center justify-between bg-surface-container-lowest border border-outline-variant rounded-xl p-4">
    <div class="flex items-center gap-3">
      <span class="material-symbols-outlined text-lg text-on-surface-variant">account_tree</span>
      <div>
        <h1 class="text-lg font-semibold text-on-surface">Source Control</h1>
        <p class="text-xs text-on-surface-variant font-mono">{$workspaceFolder || 'Chưa chọn workspace'}</p>
      </div>
    </div>
    <div class="flex items-center gap-2">
      <span class="flex items-center gap-1.5 bg-surface-container-high text-on-surface border border-outline-variant px-3 py-1.5 rounded-lg text-xs font-medium">
        <span class="material-symbols-outlined text-sm text-on-surface-variant">fork_right</span> {branch || '—'}
      </span>
      <button type="button" on:click={refreshAll} class="p-2 rounded-lg hover:bg-surface-container-high text-on-surface-variant hover:text-on-surface transition-colors cursor-pointer" title="Tải lại">
        <span class="material-symbols-outlined text-base">refresh</span>
      </button>
    </div>
  </div>

  <div class="grid grid-cols-12 gap-4">
    <!-- Left: changes + commit -->
    <div class="col-span-12 lg:col-span-5 space-y-4">
      <!-- Commit box -->
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3">
        <div class="flex items-center justify-between">
          <span class="text-sm font-semibold text-on-surface">Commit</span>
          <button type="button" on:click={aiCommitMessage} disabled={aiBusy}
            class="text-xs border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm {aiBusy ? 'animate-spin' : ''}">{aiBusy ? 'sync' : 'auto_awesome'}</span>
            AI viết commit
          </button>
        </div>
        <textarea bind:value={commitMsg} placeholder="Commit message... (hoặc bấm 'AI viết commit')"
          class="w-full h-24 bg-surface-container-low border border-outline-variant rounded-lg p-2.5 text-xs text-on-surface outline-none focus:border-primary resize-none font-mono"></textarea>
        <button type="button" on:click={stageAndCommit} disabled={busy}
          class="w-full bg-primary text-on-primary py-2.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer flex items-center justify-center gap-1.5">
          <span class="material-symbols-outlined text-sm">check</span> Stage tất cả &amp; Commit
        </button>
      </div>

      <!-- Changes -->
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
        <span class="text-sm font-semibold text-on-surface">Changes ({changes.length})</span>
        <div class="max-h-52 overflow-y-auto space-y-1 font-mono text-xs">
          {#each changes as c}
            <div class="flex items-center gap-2 px-2 py-1 rounded-lg hover:bg-surface-container-low">
              <span class="w-8 font-medium {statusColor(c.code)}">{c.code || 'M'}</span>
              <span class="truncate flex-1 text-on-surface">{c.file}</span>
              <span class="text-[11px] text-on-surface-variant">{statusLabel[c.code] || ''}</span>
            </div>
          {:else}
            <div class="text-on-surface-variant italic text-center py-4">Không có thay đổi.</div>
          {/each}
        </div>
      </div>

      <!-- Branches -->
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
        <span class="text-sm font-semibold text-on-surface">Branches</span>
        <div class="max-h-32 overflow-y-auto space-y-1 text-xs">
          {#each branches as b}
            <button type="button" on:click={() => checkout(b)}
              class="w-full text-left flex items-center gap-2 px-2 py-1 rounded-lg hover:bg-surface-container-low transition-colors cursor-pointer {b === branch ? 'text-primary font-medium' : 'text-on-surface'}">
              <span class="material-symbols-outlined text-sm">{b === branch ? 'radio_button_checked' : 'radio_button_unchecked'}</span>
              {b}
            </button>
          {/each}
        </div>
      </div>
    </div>

    <!-- Right: diff + history + command -->
    <div class="col-span-12 lg:col-span-7 space-y-4">
      <!-- Command box -->
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
        <span class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm text-on-surface-variant">terminal</span> Git Command
        </span>
        <div class="flex gap-2">
          <span class="flex items-center text-on-surface-variant font-mono text-xs">git</span>
          <input bind:value={cmdInput} on:keydown={(e) => { if (e.key === 'Enter') runCmd(); }}
            placeholder="status  |  log --oneline -5  |  branch -a"
            class="flex-1 bg-surface-container-low border border-outline-variant rounded-lg px-3 py-1.5 text-xs font-mono text-on-surface outline-none focus:border-primary" />
          <button type="button" on:click={runCmd} disabled={busy}
            class="border border-outline-variant text-on-surface-variant px-3 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5">
            {#if busy}
              <span class="material-symbols-outlined text-sm animate-spin">sync</span>
            {/if}
            Chạy
          </button>
        </div>
        {#if cmdOutput}
          <pre class="bg-surface-container-highest border border-outline-variant text-on-surface p-3 rounded-lg text-[11px] font-mono max-h-40 overflow-auto whitespace-pre-wrap">{cmdOutput}</pre>
        {/if}
        <p class="text-[11px] text-on-surface-variant">Chỉ cho phép lệnh git an toàn (push/--force bị chặn).</p>
      </div>

      <!-- Diff -->
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
        <span class="text-sm font-semibold text-on-surface">Diff</span>
        <div class="bg-surface-container-highest border border-outline-variant p-3 rounded-lg font-mono text-[11px] max-h-56 overflow-auto leading-relaxed">
          <!-- The empty state is rendered here: the backend returns "" for a
               clean tree (an in-band sentinel sentence used to be misread as
               data by the commit paths). -->
          {#if !(diffText || '').trim()}
            <div class="text-on-surface-variant italic">Không có thay đổi nào trong workspace.</div>
          {:else}
            {#each (diffText || '').split('\n') as line}
              <div class={line.startsWith('+') && !line.startsWith('+++') ? 'text-success' : line.startsWith('-') && !line.startsWith('---') ? 'text-error' : line.startsWith('@@') ? 'text-primary' : 'text-on-surface-variant'}>{line || ' '}</div>
            {/each}
          {/if}
        </div>
      </div>

      <!-- History -->
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-2">
        <span class="text-sm font-semibold text-on-surface">History</span>
        <div class="max-h-52 overflow-y-auto divide-y divide-outline-variant/40">
          {#each graph as c, i}
            <div class="flex items-center gap-2 py-2 group">
              <!-- Lane gutter. Each row draws its own node plus the lines down to
                   the next row, so branches and merges are visible instead of the
                   flat list this used to be. -->
              <svg
                width={Math.max(1, c.width) * LANE_W + 4}
                height={ROW_H}
                class="flex-shrink-0 overflow-visible"
                aria-hidden="true"
              >
                {#each c.edges as edge}
                  <path
                    d="M {edge.from * LANE_W + LANE_W / 2} {ROW_H / 2}
                       C {edge.from * LANE_W + LANE_W / 2} {ROW_H},
                         {edge.to * LANE_W + LANE_W / 2} {ROW_H / 2},
                         {edge.to * LANE_W + LANE_W / 2} {ROW_H}"
                    fill="none"
                    stroke={laneColor(edge.to)}
                    stroke-width="1.5"
                  />
                {/each}
                <circle
                  cx={c.lane * LANE_W + LANE_W / 2}
                  cy={ROW_H / 2}
                  r={c.isMerge ? 4.5 : 3.5}
                  fill={c.isMerge ? 'transparent' : laneColor(c.lane)}
                  stroke={laneColor(c.lane)}
                  stroke-width="2"
                />
              </svg>

              <span class="font-mono text-[11px] bg-surface-container-high text-on-surface-variant px-1.5 py-0.5 rounded-lg">{(c.hash || '').slice(0, 7)}</span>
              <div class="flex-1 min-w-0">
                <p class="text-xs text-on-surface truncate">
                  {#each c.refs || [] as ref}
                    <span class="mr-1 px-1.5 py-0.5 rounded-full text-[11px] font-medium align-middle
                      {ref === branch
                        ? 'bg-secondary-container text-on-secondary-container'
                        : 'bg-surface-container-high text-on-surface-variant'}">{ref}</span>
                  {/each}
                  {c.message}
                </p>
                <p class="text-[11px] text-on-surface-variant">{c.author} · {c.date}</p>
              </div>
              <button type="button" on:click={() => revert(c.hash)} title="Revert commit này"
                class="opacity-0 group-hover:opacity-100 focus-visible:opacity-100 text-[11px] text-error border border-outline-variant px-2 py-1 rounded-lg font-medium hover:bg-error/10 cursor-pointer transition-opacity flex-shrink-0">
                Revert
              </button>
            </div>
          {:else}
            <div class="text-on-surface-variant italic text-center py-4 text-xs">Chưa có commit.</div>
          {/each}
        </div>
      </div>
    </div>
  </div>
</div>
