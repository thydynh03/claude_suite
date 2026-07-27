<script lang="ts">
  import { onMount } from 'svelte';
  import { addToast } from '../../lib/stores/appState';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import type { models } from '../../../wailsjs/go/models';

  // Named, not `typeof tab` — a type query resolves to the narrowed type
  // ('map' right after the initializer), which rejects the other two entries.
  type MemTab = 'map' | 'lessons' | 'regressions';
  let tab: MemTab = 'map';

  // Same segmented pill the Settings sub-tabs use — this page used to invent a
  // third tab style (secondary-container fill on a bottom border).
  const MEM_TABS: { id: MemTab; label: string }[] = [
    { id: 'map', label: 'Project Map' },
    { id: 'lessons', label: 'Lesson' },
    { id: 'regressions', label: 'Bug đã sửa' },
  ];

  // ── Map tab state ──
  let stats: models.ProjectMapStats | null = null;
  let mapError = '';
  let rebuilding = false;
  let refreshingSummaries = false;

  // Module graph view (directory-level aggregation, drawn as SVG).
  let showGraph = false;
  let graphData: models.ModuleGraph | null = null;
  let graphLoading = false;

  type LayoutNode = { id: string; name: string; files: number; x: number; y: number; r: number };
  type LayoutEdge = { x1: number; y1: number; x2: number; y2: number; count: number };
  let layoutNodes: LayoutNode[] = [];
  let layoutEdges: LayoutEdge[] = [];

  // Circle layout: modules on a ring, node size by file count, edge width by
  // aggregated import count. Recomputed reactively — no function calls in the
  // markup (untracked-template-calls guard).
  $: {
    const nodes = graphData?.nodes ?? [];
    const edges = graphData?.edges ?? [];
    const cx = 420, cy = 280, radius = 210;
    const maxFiles = nodes.reduce((m, n) => Math.max(m, n.files), 1);
    layoutNodes = nodes.map((n, i) => {
      const angle = (i / Math.max(nodes.length, 1)) * Math.PI * 2 - Math.PI / 2;
      return {
        id: n.id,
        name: n.name,
        files: n.files,
        x: cx + radius * Math.cos(angle),
        y: cy + radius * Math.sin(angle),
        r: 6 + 12 * Math.sqrt(n.files / maxFiles),
      };
    });
    const pos = new Map(layoutNodes.map((n) => [n.id, n]));
    layoutEdges = edges.flatMap((e) => {
      const s = pos.get(e.source);
      const t = pos.get(e.target);
      return s && t ? [{ x1: s.x, y1: s.y, x2: t.x, y2: t.y, count: e.count }] : [];
    });
  }

  async function toggleGraph() {
    showGraph = !showGraph;
    if (showGraph && !graphData) {
      graphLoading = true;
      try {
        graphData = await AppBindings.GetProjectGraph();
      } catch (e: any) {
        addToast('Tải module graph thất bại: ' + String(e?.message ?? e), 'ERROR');
        showGraph = false;
      } finally {
        graphLoading = false;
      }
    }
  }

  // ── Lessons tab state ──
  let pendingLessons: models.MemoryLesson[] = [];
  let activeLessons: models.MemoryLesson[] = [];
  let lessonsError = '';
  let reviewingId = '';

  // ── Regressions tab state ──
  let regressions: models.Regression[] = [];
  let regressionsError = '';
  let guardForId = '';
  let guardName = '';
  let guardArgsText = '';
  let approvingGuard = false;

  $: guardArgs = guardArgsText.split('\n').map((s) => s.trim()).filter((s) => s.length > 0);

  async function loadMap() {
    mapError = '';
    try {
      stats = await AppBindings.GetProjectMap();
    } catch (e: any) {
      stats = null;
      mapError = String(e?.message ?? e);
    }
  }

  async function loadLessons() {
    lessonsError = '';
    try {
      pendingLessons = (await AppBindings.GetMemoryLessons('pending')) ?? [];
      activeLessons = (await AppBindings.GetMemoryLessons('active')) ?? [];
    } catch (e: any) {
      pendingLessons = [];
      activeLessons = [];
      lessonsError = String(e?.message ?? e);
    }
  }

  async function loadRegressions() {
    regressionsError = '';
    try {
      regressions = (await AppBindings.GetRegressions()) ?? [];
    } catch (e: any) {
      regressions = [];
      regressionsError = String(e?.message ?? e);
    }
  }

  async function loadAll() {
    graphData = null; // force a re-fetch next time the graph opens
    await Promise.all([loadMap(), loadLessons(), loadRegressions()]);
  }

  async function rebuildMap() {
    rebuilding = true;
    try {
      const report = await AppBindings.RebuildProjectMap();
      addToast(`Đã build project map: ${report.files} file, ${report.nodes} node (${report.duration_sec.toFixed(1)}s).`, 'SUCCESS');
      await loadMap();
    } catch (e: any) {
      addToast('Build project map thất bại: ' + String(e?.message ?? e), 'ERROR');
    } finally {
      rebuilding = false;
    }
  }

  async function refreshSummaries() {
    refreshingSummaries = true;
    try {
      const n = await AppBindings.RefreshProjectSummaries();
      addToast(n > 0 ? `Đã làm mới summary cho ${n} file.` : 'Không có summary nào đang chờ làm mới.', 'SUCCESS');
      await loadMap();
    } catch (e: any) {
      addToast('Làm mới summaries thất bại: ' + String(e?.message ?? e), 'ERROR');
    } finally {
      refreshingSummaries = false;
    }
  }

  async function review(lessonID: string, decision: 'approve' | 'reject') {
    reviewingId = lessonID;
    try {
      await AppBindings.ReviewMemoryLesson(lessonID, decision);
      addToast(decision === 'approve' ? 'Lesson đã được duyệt — sẽ tiêm vào các task sau.' : 'Lesson đã bị loại.', 'SUCCESS');
      await loadLessons();
    } catch (e: any) {
      addToast('Duyệt lesson thất bại: ' + String(e?.message ?? e), 'ERROR');
    } finally {
      reviewingId = '';
    }
  }

  function openGuardForm(reg: models.Regression) {
    guardForId = reg.regression_id;
    guardName = suggestCheckName(reg.title);
    guardArgsText = '';
  }

  function suggestCheckName(title: string): string {
    return title
      .toLowerCase()
      .replace(/\[[^\]]*\]/g, '')
      .trim()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 40) || 'regression-guard';
  }

  async function approveGuard() {
    if (!guardName.trim() || guardArgs.length === 0) return;
    approvingGuard = true;
    try {
      await AppBindings.ApproveRegressionGuard(guardForId, guardName.trim(), guardArgs);
      addToast(`Đã ghi guard "${guardName.trim()}" vào .claude-suite/checks.json.`, 'SUCCESS');
      guardForId = '';
      await loadRegressions();
    } catch (e: any) {
      addToast('Ghi guard thất bại: ' + String(e?.message ?? e), 'ERROR');
    } finally {
      approvingGuard = false;
    }
  }

  function fmtDate(v: any): string {
    try {
      const d = new Date(v);
      return isNaN(d.getTime()) ? '' : d.toLocaleString();
    } catch (_) {
      return '';
    }
  }

  onMount(loadAll);
</script>

<div class="max-w-6xl mx-auto">
  <div class="flex items-center justify-between mb-4">
    <div>
      <h1 class="text-lg font-semibold text-on-surface flex items-center gap-2">
        <span class="material-symbols-outlined text-lg text-on-surface-variant">psychology</span>
        Memory và Project Map
      </h1>
      <p class="text-xs text-on-surface-variant mt-1">
        Những gì sub-agent được "nhớ": bản đồ code, lesson đã học, và các bug đã sửa — tất cả được tiêm vào prompt qua Context Pack.
      </p>
    </div>
    <button
      type="button"
      class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer flex items-center gap-1"
      on:click={loadAll}
      title="Tải lại toàn bộ dữ liệu memory"
    >
      <span class="material-symbols-outlined text-sm">refresh</span> Tải lại
    </button>
  </div>

  <!-- Tabs -->
  <div class="flex mb-5 overflow-x-auto">
    <div class="bg-surface-container-high p-1 rounded-xl flex flex-wrap gap-1 border border-outline-variant">
      {#each MEM_TABS as t}
        <button
          type="button"
          on:click={() => (tab = t.id)}
          class="px-4 py-1.5 text-xs font-medium rounded-lg transition-colors cursor-pointer whitespace-nowrap
          {tab === t.id ? 'bg-surface-container-lowest text-on-surface' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          {t.label}
          {#if t.id === 'lessons' && pendingLessons.length > 0}<span class="ml-1 px-1.5 py-0.5 rounded-full bg-primary text-on-primary text-[11px]">{pendingLessons.length}</span>{/if}
        </button>
      {/each}
    </div>
  </div>

  {#if tab === 'map'}
    {#if mapError}
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 text-sm text-on-surface-variant">{mapError}</div>
    {:else if stats}
      <div class="grid grid-cols-2 md:grid-cols-4 gap-3 mb-5">
        <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4">
          <p class="text-xs font-medium text-on-surface-variant">Files</p>
          <p class="text-lg font-semibold text-on-surface">{stats.files}</p>
        </div>
        <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4">
          <p class="text-xs font-medium text-on-surface-variant">Nodes</p>
          <p class="text-lg font-semibold text-on-surface">{stats.nodes}</p>
        </div>
        <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4">
          <p class="text-xs font-medium text-on-surface-variant">Edges</p>
          <p class="text-lg font-semibold text-on-surface">{stats.edges}</p>
        </div>
        <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4">
          <p class="text-xs font-medium text-on-surface-variant">Trạng thái</p>
          <p class="text-lg font-semibold {stats.staleness.status === 'fresh' ? 'text-success' : stats.staleness.status === 'unknown' ? 'text-on-surface-variant' : 'text-warning'}">
            {stats.staleness.status}{#if stats.staleness.status === 'stale'} ({stats.staleness.commits_behind} commit){/if}
          </p>
        </div>
      </div>
      {#if stats.files === 0}
        <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 mb-5 text-sm text-on-surface-variant">
          Chưa có project map cho workspace này. Bấm <strong>Build lại map</strong> để quét lần đầu (chỉ phân tích tĩnh, không tốn token).
        </div>
      {:else}
        <p class="text-xs text-on-surface-variant mb-5">
          Phân tích lần cuối: {fmtDate(stats.analyzed_at)} · commit <span class="font-mono">{(stats.git_commit_hash || '').slice(0, 7) || 'n/a'}</span>
          · map tự cập nhật sau mỗi task; bản digest nằm ở <span class="font-mono">.claude-suite/project-map.md</span>
        </p>
      {/if}
      <div class="flex gap-2">
        <button
          type="button"
          disabled={rebuilding}
          class="px-3 py-1.5 rounded-lg font-medium text-xs bg-primary text-on-primary hover:opacity-90 transition-opacity cursor-pointer disabled:opacity-50 flex items-center gap-1.5"
          on:click={rebuildMap}
        >
          <span class="material-symbols-outlined text-sm {rebuilding ? 'animate-spin' : ''}">{rebuilding ? 'progress_activity' : 'account_tree'}</span>
          Build lại map
        </button>
        <button
          type="button"
          disabled={refreshingSummaries}
          class="px-3 py-1.5 rounded-lg font-medium text-xs border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer disabled:opacity-50 flex items-center gap-1.5"
          on:click={refreshSummaries}
          title="Dùng model rẻ (Haiku) viết summary cho các file đang chờ — tốn một lượng token nhỏ"
        >
          <span class="material-symbols-outlined text-sm {refreshingSummaries ? 'animate-spin' : ''}">{refreshingSummaries ? 'progress_activity' : 'auto_awesome'}</span>
          Làm mới summary (LLM)
        </button>
        <button
          type="button"
          disabled={graphLoading}
          class="px-3 py-1.5 rounded-lg font-medium text-xs border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer disabled:opacity-50 flex items-center gap-1.5"
          on:click={toggleGraph}
        >
          <span class="material-symbols-outlined text-sm {graphLoading ? 'animate-spin' : ''}">{graphLoading ? 'progress_activity' : 'hub'}</span>
          {showGraph ? 'Ẩn module graph' : 'Xem module graph'}
        </button>
      </div>

      {#if showGraph && graphData}
        {#if layoutNodes.length === 0}
          <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 mt-5 text-sm text-on-surface-variant">
            Chưa có module nào trong map — build map trước.
          </div>
        {:else}
          <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 mt-5 overflow-x-auto">
            <p class="text-xs text-on-surface-variant mb-2">
              Top {layoutNodes.length} thư mục theo số file; đường nối = import giữa các thư mục (đậm hơn = nhiều import hơn).
            </p>
            <svg viewBox="0 0 840 560" class="w-full min-w-[640px]" role="img" aria-label="Module import graph">
              {#each layoutEdges as e}
                <line x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2}
                  stroke="currentColor" class="text-primary"
                  stroke-opacity={Math.min(0.15 + e.count * 0.1, 0.7)}
                  stroke-width={Math.min(1 + e.count * 0.5, 4)} />
              {/each}
              {#each layoutNodes as n (n.id)}
                <circle cx={n.x} cy={n.y} r={n.r} class="text-secondary" fill="currentColor" fill-opacity="0.25"
                  stroke="currentColor" stroke-width="1.5" />
                <text x={n.x} y={n.y - n.r - 5} text-anchor="middle"
                  class="fill-current text-on-surface" font-size="10" font-family="monospace">
                  {n.name} ({n.files})
                </text>
              {/each}
            </svg>
          </div>
        {/if}
      {/if}
    {:else}
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 text-sm text-on-surface-variant flex items-center gap-2">
        <span class="material-symbols-outlined text-sm animate-spin">progress_activity</span>
        Đang tải…
      </div>
    {/if}
  {:else if tab === 'lessons'}
    {#if lessonsError}
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 text-sm text-on-surface-variant">{lessonsError}</div>
    {:else}
      <h2 class="text-sm font-semibold text-on-surface mb-2 flex items-center gap-1.5">
        <span class="material-symbols-outlined text-base text-on-surface-variant">pending_actions</span>
        Chờ duyệt ({pendingLessons.length})
      </h2>
      {#if pendingLessons.length === 0}
        <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 mb-5 text-sm text-on-surface-variant">
          Chưa có lesson nào chờ duyệt. Lesson do LLM chưng cất sẽ xuất hiện ở đây sau khi các agent chạy đủ nhiều — và chỉ được tiêm vào prompt khi bạn duyệt.
        </div>
      {:else}
        <div class="space-y-2 mb-5">
          {#each pendingLessons as l (l.lesson_id)}
            <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <p class="text-sm text-on-surface">{l.action}</p>
                  <p class="text-xs text-on-surface-variant mt-1">
                    <span class="font-mono">[{Math.round(l.confidence * 100)}%]</span>
                    {#if l.trigger}· khi: {l.trigger}{/if}
                    {#if l.evidence}· {l.evidence}{/if}
                  </p>
                </div>
                <div class="flex gap-1 flex-shrink-0">
                  <button type="button" disabled={reviewingId === l.lesson_id}
                    class="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary text-on-primary hover:opacity-90 transition-opacity cursor-pointer disabled:opacity-50"
                    on:click={() => review(l.lesson_id, 'approve')}>Duyệt</button>
                  <button type="button" disabled={reviewingId === l.lesson_id}
                    class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer disabled:opacity-50"
                    on:click={() => review(l.lesson_id, 'reject')}>Loại</button>
                </div>
              </div>
            </div>
          {/each}
        </div>
      {/if}

      <h2 class="text-sm font-semibold text-on-surface mb-2 flex items-center gap-1.5">
        <span class="material-symbols-outlined text-base text-on-surface-variant">check_circle</span>
        Đang hoạt động ({activeLessons.length})
      </h2>
      {#if activeLessons.length === 0}
        <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 text-sm text-on-surface-variant">
          Chưa có lesson nào hoạt động. Lesson "mechanical" (suy ra từ chu trình fail→fixed) tự kích hoạt; lesson LLM cần bạn duyệt ở trên.
        </div>
      {:else}
        <div class="space-y-2">
          {#each activeLessons as l (l.lesson_id)}
            <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <p class="text-sm text-on-surface">{l.action}</p>
                  <p class="text-xs text-on-surface-variant mt-1">
                    <span class="font-mono">[{Math.round(l.confidence * 100)}%]</span>
                    · {l.kind === 'mechanical' ? 'máy suy ra' : 'LLM chưng cất'}
                    · đã dùng {l.use_count} lần
                  </p>
                </div>
                <button type="button" disabled={reviewingId === l.lesson_id}
                  class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer disabled:opacity-50 flex-shrink-0"
                  on:click={() => review(l.lesson_id, 'reject')}
                  title="Ngừng tiêm lesson này vào prompt">Lưu trữ</button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    {/if}
  {:else}
    {#if regressionsError}
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 text-sm text-on-surface-variant">{regressionsError}</div>
    {:else if regressions.length === 0}
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 text-sm text-on-surface-variant">
        Chưa có bug đã sửa nào được ghi nhận. Khi một task fail rồi được sửa xong, chu trình đó sẽ tự xuất hiện ở đây — và các task sau đụng cùng vùng code sẽ được cảnh báo.
      </div>
    {:else}
      <div class="space-y-2">
        {#each regressions as r (r.regression_id)}
          <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="text-sm font-semibold text-on-surface">{r.title}</p>
                <p class="text-xs text-on-surface-variant mt-1 font-mono truncate" title={r.symptom}>{r.symptom}</p>
                {#if r.files && r.files.length > 0}
                  <p class="text-[11px] text-on-surface-variant mt-1">files: <span class="font-mono">{r.files.slice(0, 4).join(', ')}</span></p>
                {/if}
              </div>
              <div class="flex-shrink-0">
                {#if r.guard_status === 'approved'}
                  <span class="px-2 py-1 rounded-full text-[11px] font-medium bg-surface-container-high text-on-surface-variant">
                    guard: {r.guard_check_id}
                  </span>
                {:else}
                  <button type="button"
                    class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer"
                    on:click={() => openGuardForm(r)}
                    title="Viết một lệnh kiểm chứng vào .claude-suite/checks.json để chặn bug này quay lại">Thêm guard…</button>
                {/if}
              </div>
            </div>

            {#if guardForId === r.regression_id}
              <div class="mt-3 border-t border-outline-variant pt-3">
                <p class="text-xs text-on-surface-variant mb-2">
                  Guard là một lệnh <strong>argv</strong> (mỗi tham số một dòng — không phải chuỗi shell) được ghi vào
                  <span class="font-mono">.claude-suite/checks.json</span>. Lệnh <em>fail</em> nghĩa là bug đã quay lại.
                </p>
                <input type="text" bind:value={guardName} placeholder="tên check (kebab-case)"
                  class="w-full mb-2 px-3 py-2 rounded-lg bg-surface-container-low border border-outline-variant text-sm font-mono text-on-surface focus:border-primary" />
                <textarea bind:value={guardArgsText} rows="3"
                  placeholder={'go\ntest\n./backend/services/'}
                  class="w-full mb-2 px-3 py-2 rounded-lg bg-surface-container-low border border-outline-variant text-sm font-mono text-on-surface focus:border-primary"></textarea>
                {#if guardArgs.length > 0}
                  <p class="text-[11px] text-on-surface-variant mb-2">argv sẽ ghi: <span class="font-mono">[{guardArgs.map((a) => JSON.stringify(a)).join(', ')}]</span></p>
                {/if}
                <div class="flex gap-2">
                  <button type="button" disabled={approvingGuard || !guardName.trim() || guardArgs.length === 0}
                    class="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary text-on-primary hover:opacity-90 transition-opacity cursor-pointer disabled:opacity-50 flex items-center gap-1.5"
                    on:click={approveGuard}>
                    {#if approvingGuard}<span class="material-symbols-outlined text-sm animate-spin">progress_activity</span>{/if}
                    Duyệt và ghi guard
                  </button>
                  <button type="button"
                    class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer"
                    on:click={() => (guardForId = '')}>Huỷ</button>
                </div>
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
