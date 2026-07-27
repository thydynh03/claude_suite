<script lang="ts">
  import { onMount } from 'svelte';
  import { flip } from 'svelte/animate';
  import { fade } from 'svelte/transition';
  import type { Task } from '../../lib/types';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { tick } from 'svelte';
  import { models } from '../../../wailsjs/go/models';
  import { addLog, addToast, tasksStore, agentsStore, taskLogsStore, taskScreenshotsStore, clearTaskLog } from '../../lib/stores/appState';
  import Dropdown from '../ui/Dropdown.svelte';

  export let onRefresh: () => void;

  // One source, not two. This used to be a prop the parent passed *and* a
  // subscription that overwrote it, so whichever fired last won and a board
  // that had just changed could be replaced by the stale copy. That is why new
  // tasks only appeared after switching tabs and back.
  $: tasks = ($tasksStore || []) as Task[];

  // Derived from the board rather than fetched separately: a second source would
  // drift from the cards it annotates the moment a task changed status. The
  // backend has the same rule in TaskRepository.DispatchReadiness, which is what
  // the orchestrator actually dispatches on; this is the display of it.
  $: blockedById = (() => {
    const byId = new Map(tasks.map((t) => [t.task_id, t]));
    const out: Record<string, { blockers: any[]; dead: boolean }> = {};

    for (const t of tasks) {
      if (t.status !== 'backlog' && t.status !== 'queued') continue;

      const blockers: any[] = [];
      let dead = false;
      for (const depId of t.depends_on || []) {
        const dep = byId.get(depId);
        if (dep && dep.status === 'done') continue;
        const status = dep ? dep.status : 'missing';
        // A dependency that failed, or that no longer exists, never clears on
        // its own — the task is stuck until someone intervenes.
        if (status === 'failed' || status === 'missing') dead = true;
        blockers.push({ task_id: depId, title: dep ? dep.title : '', status });
      }

      if (blockers.length > 0) out[t.task_id] = { blockers, dead };
    }
    return out;
  })();

  // Retries every failed or missing blocker at once. Retrying them one by one
  // from the failed column is possible but means finding them first, which is
  // exactly the work this badge exists to save.
  async function retryBlockers(blk: { blockers: any[] }) {
    const stuck = blk.blockers.filter((b) => b.status === 'failed');
    if (stuck.length === 0) {
      addToast('Task đang chặn đã bị xoá — hãy sửa phần phụ thuộc của task này.', 'ERROR');
      return;
    }
    let ok = 0;
    for (const b of stuck) {
      try {
        await (AppBindings as any).RetryTask(b.task_id);
        ok++;
      } catch (e) {
        addLog(`Retry ${b.task_id} thất bại: ${e}`, 'ERROR');
        addToast(`Retry ${b.task_id} thất bại: ${e}`, 'ERROR');
      }
    }
    onRefresh();
    addToast(
      ok === stuck.length
        ? `Đã đưa ${ok} task lỗi về backlog để chạy lại.`
        : `Retry được ${ok}/${stuck.length} task.`,
      ok === stuck.length ? 'SUCCESS' : 'ERROR'
    );
  }

  onMount(() => {
    (async () => {
      try {
        if ((AppBindings as any).GetMaxConcurrency) {
          const n = await (AppBindings as any).GetMaxConcurrency();
          if (typeof n === 'number' && n > 0) maxConcurrency = n;
        }
        if ((AppBindings as any).GetVerifyBuild) {
          verifyBuild = await (AppBindings as any).GetVerifyBuild();
        }
      } catch (e) {}
    })();

    // board_updated is handled centrally in App.svelte, which refreshes
    // tasksStore; this view is a plain reader of that store.
  });

  let refreshing = false;

  // board_updated normally keeps this current. The button is for the moment it
  // did not, so the answer is never "switch tabs and come back".
  async function refreshBoard() {
    refreshing = true;
    try {
      const res = await AppBindings.GetTasks();
      tasksStore.set(Array.isArray(res) ? res : []);
      if (onRefresh) await onRefresh();
    } catch (e: any) {
      addLog(`Không tải lại được bảng: ${e?.message || e}`, 'ERROR');
      addToast(`Không tải lại được bảng: ${e?.message || e}`, 'ERROR');
    } finally {
      refreshing = false;
    }
  }

  $: agentOptions = [
    { value: '', label: 'Tự động phân công' },
    ...($agentsStore || []).map(a => ({
      value: a.name,
      label: `${a.name}`
    }))
  ];

  let showAddModal = false;
  // Track the open task by id and derive the live object so the drawer's status,
  // Stop/Retry buttons and log stream stay in sync as the board updates.
  let detailTaskId: string | null = null;
  $: detailTask = detailTaskId ? ((Array.isArray(tasks) ? tasks : []).find((t) => t.task_id === detailTaskId) || null) : null;

  // Max parallel sub-agents (backend SetMaxConcurrency).
  let maxConcurrency = 3;

  // Live log for the currently open task + auto-scroll to newest line.
  let logContainer: HTMLDivElement | null = null;
  $: currentTaskLogs = detailTaskId ? (($taskLogsStore[detailTaskId]) || []) : [];
  // Rendering all 500 buffered lines on every incoming log froze the drawer for
  // a running task. The tail is what anyone is reading anyway.
  $: visibleTaskLogs = currentTaskLogs.slice(-120);
  $: if (currentTaskLogs.length && logContainer) {
    tick().then(() => { if (logContainer) logContainer.scrollTop = logContainer.scrollHeight; });
  }

  async function updateConcurrency(n: number) {
    const prev = maxConcurrency;
    maxConcurrency = n;
    try {
      await (AppBindings as any).SetMaxConcurrency(n);
      addLog(`Số agent song song tối đa: ${n}`, 'INFO');
      addToast(`Chạy tối đa ${n} agent song song.`, 'SUCCESS');
    } catch (e) {
      // Put the control back: leaving it moved would show a limit the
      // orchestrator is not actually using (same rollback as toggleVerify).
      maxConcurrency = prev;
      addToast(`Không đặt được số agent song song: ${e}`, 'ERROR');
    }
  }

  let verifyBuild = true;
  async function toggleVerify() {
    verifyBuild = !verifyBuild;
    try {
      await (AppBindings as any).SetVerifyBuild(verifyBuild);
      addLog(`Verify build trước khi Done: ${verifyBuild ? 'bật' : 'tắt'}`, 'INFO');
      addToast(`Verify build trước khi Done: ${verifyBuild ? 'bật' : 'tắt'}.`, 'SUCCESS');
    } catch (e) {
      // Put the toggle back: leaving it on while the backend refused it tells
      // the user their builds are verified when they are not.
      verifyBuild = !verifyBuild;
      addToast(`Không đổi được cài đặt verify build: ${e}`, 'ERROR');
    }
  }

  // Git diff viewer — shows what the agent changed in the workspace.
  let showDiff = false;
  let diffText = '';
  let loadingDiff = false;
  async function loadDiff() {
    showDiff = !showDiff;
    if (!showDiff) return;
    loadingDiff = true;
    try {
      diffText = await (AppBindings as any).GetWorkspaceDiff();
    } catch (e) {
      diffText = 'Không tải được diff: ' + e;
    } finally {
      loadingDiff = false;
    }
  }

  function copyTaskLog() {
    const text = currentTaskLogs.map((l) => `[${l.time}] ${l.message}`).join('\n');
    navigator.clipboard.writeText(text);
    addLog('Đã sao chép log của task.', 'SUCCESS');
    addToast('Đã sao chép log của task.', 'SUCCESS');
  }
  let searchQuery = '';
  let filterPriority = 'all';

  let newTaskTitle = '';
  let newTaskDescription = '';
  let newTaskPriority = 'normal';
  let newTaskAssignedTo = '';
  let creatingTask = false;

  const columns = [
    { key: 'backlog', title: 'Backlog', icon: 'inventory_2' },
    { key: 'queued', title: 'Queued', icon: 'hourglass_empty' },
    { key: 'running', title: 'Running', icon: 'bolt' },
    { key: 'done', title: 'Done', icon: 'check_circle' },
    { key: 'failed', title: 'Failed', icon: 'cancel' },
  ];

  // A `$:` derivation, not a function called from the template. The columns
  // used to render through `{@const colTasks = getTasksByStatus(col.key)}`,
  // and a call in the markup reads its body untracked — only the argument is
  // a dependency, and col.key never changes. Measured in KanbanView.test.ts:
  // with a clean jsdom flush the counts and card lists freeze at their
  // mounted values while the blocked badges beside them (driven by reactive
  // `blockedById`) keep following the store. Same class, same fix, as the
  // Cockpit metrics freeze.
  $: tasksByStatus = (() => {
    const out: Record<string, Task[]> = { backlog: [], queued: [], running: [], done: [], failed: [] };
    const list = Array.isArray(tasks) ? tasks : [];
    const q = searchQuery.toLowerCase().trim();
    for (const t of list) {
      if (!t || !(t.status in out)) continue;
      if (filterPriority !== 'all' && t.priority !== filterPriority) continue;
      if (q) {
        const titleMatch = t.title?.toLowerCase().includes(q);
        const promptMatch = t.prompt?.toLowerCase().includes(q);
        const descMatch = t.description?.toLowerCase().includes(q);
        if (!titleMatch && !promptMatch && !descMatch) continue;
      }
      out[t.status].push(t);
    }
    return out;
  })();

  async function handleAddTask() {
    if (!newTaskTitle.trim() || creatingTask) return;
    const title = newTaskTitle;
    const desc = newTaskDescription || newTaskTitle;
    const priority = newTaskPriority;
    const assignedTo = newTaskAssignedTo;

    creatingTask = true;
    try {
      await AppBindings.CreateTask(models.Task.createFrom({
        task_id: '',
        title: title,
        description: desc,
        prompt: desc,
        priority: priority,
        status: 'backlog',
        assigned_to: assignedTo,
        depends_on: [],
        retry_count: 0,
        max_retries: 3,
        result: '',
        session_id: '',
        parent_id: '',
      }));
      // Only now is the task real: the modal used to close and clear the
      // fields *before* the await, so a failed CreateTask destroyed the
      // prompt the user had just written and said nothing.
      newTaskTitle = '';
      newTaskDescription = '';
      showAddModal = false;
      addLog(`Đã tạo Task mới: ${title}`, 'SUCCESS');
      addToast(`Đã tạo Task mới: ${title}`, 'SUCCESS');
      if (onRefresh) await onRefresh();
    } catch (e) {
      addLog(`Không tạo được task: ${e}`, 'ERROR');
      addToast(`Không tạo được task: ${e}`, 'ERROR');
    } finally {
      creatingTask = false;
    }
  }



  async function handleMoveStatus(taskID: string, newStatus: string) {
    const list = Array.isArray(tasks) ? tasks : [];
    const t = list.find((x) => x.task_id === taskID);
    if (t) t.status = newStatus;
    tasks = [...list];

    try {
      await AppBindings.UpdateTaskStatus(taskID, newStatus);
      addLog(`Task status updated to ${newStatus}`, 'INFO');
    } catch (e) {
      // The card was moved optimistically, so a failure here leaves the board
      // showing a status the database does not have. Say so and re-read.
      addToast(`Không đổi được trạng thái task: ${e}`, 'ERROR');
      addLog(`UpdateTaskStatus failed: ${e}`, 'ERROR');
    }
    if (onRefresh) onRefresh();
  }

  async function handleAssignAgent(taskID: string, assignedTo: string) {
    const list = Array.isArray(tasks) ? tasks : [];
    const t = list.find((x) => x.task_id === taskID);
    if (t) t.assigned_to = assignedTo;
    tasks = [...list];

    try {
      if ((AppBindings as any).AssignTask) {
        await (AppBindings as any).AssignTask(taskID, assignedTo);
        addLog(`Đã phân công task cho ${assignedTo || 'tự động'}.`, 'SUCCESS');
        addToast(`Đã phân công task cho ${assignedTo || 'tự động'}.`, 'SUCCESS');
      }
    } catch (e) {
      // The card already shows the new assignee; if the backend refused it the
      // board is lying until the next refresh, so say so out loud.
      addLog(`Không phân công được task: ${e}`, 'ERROR');
      addToast(`Không phân công được task: ${e}`, 'ERROR');
    }
    if (onRefresh) onRefresh();
  }

  let selectedTaskIDs: string[] = [];
  let draggedTaskID: string | null = null;
  let dragOverColumn: string | null = null;

  function handleDragStart(e: DragEvent, taskID: string) {
    draggedTaskID = taskID;
    if (e.dataTransfer) {
      e.dataTransfer.setData('text/plain', taskID);
      e.dataTransfer.effectAllowed = 'move';
    }
  }

  function handleDragOver(e: DragEvent, colKey: string) {
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
    dragOverColumn = colKey;
  }

  function handleDragLeave() {
    dragOverColumn = null;
  }

  async function handleDrop(e: DragEvent, targetStatus: string) {
    e.preventDefault();
    dragOverColumn = null;
    const taskID = e.dataTransfer?.getData('text/plain') || draggedTaskID;
    if (taskID) {
      await handleMoveStatus(taskID, targetStatus);
    }
    draggedTaskID = null;
  }

  async function handleClearAll() {
    if (confirm('Bạn có chắc chắn muốn xóa toàn bộ Tasks không?')) {
      try {
        await AppBindings.ClearAllTasks();
        tasks = [];
        tasksStore.set([]);
        selectedTaskIDs = [];
        if (onRefresh) await onRefresh();
        addLog('Đã xóa toàn bộ tasks.', 'SUCCESS');
        addToast('Đã xóa toàn bộ tasks.', 'SUCCESS');
      } catch (e) {
        addLog(`Không xoá được toàn bộ task: ${e}`, 'ERROR');
        addToast(`Không xoá được toàn bộ task: ${e}`, 'ERROR');
      }
    }
  }

  async function handleDeleteSelected() {
    if (selectedTaskIDs.length === 0) return;
    if (confirm(`Bạn có chắc muốn xóa ${selectedTaskIDs.length} tasks đã chọn?`)) {
      const wanted = selectedTaskIDs.length;
      let removed = 0;
      let firstError = '';
      for (const id of selectedTaskIDs) {
        try {
          await AppBindings.DeleteTask(id);
          removed++;
        } catch (e) {
          // One bad id used to throw out of the loop and leave the rest
          // selected, deleted-or-not, with no message at all.
          if (!firstError) firstError = String(e);
        }
      }
      selectedTaskIDs = [];
      // The store is the single source now, so refresh it rather than editing a
      // local copy the next board event would overwrite.
      await refreshBoard();
      if (removed === wanted) {
        addLog(`Đã xóa ${removed} tasks đã chọn.`, 'SUCCESS');
        addToast(`Đã xóa ${removed} tasks đã chọn.`, 'SUCCESS');
      } else {
        addLog(`Chỉ xoá được ${removed}/${wanted} task: ${firstError}`, 'ERROR');
        addToast(`Chỉ xoá được ${removed}/${wanted} task: ${firstError}`, 'ERROR');
      }
    }
  }

  async function handleStopTask(taskID: string) {
    try {
      const ok = await (AppBindings as any).StopTask(taskID);
      addLog(ok ? 'Đã gửi yêu cầu dừng task.' : 'Task không đang chạy để dừng.', ok ? 'WARN' : 'INFO');
      addToast(ok ? 'Đã gửi yêu cầu dừng task.' : 'Task này không đang chạy.', ok ? 'SUCCESS' : 'INFO');
    } catch (e) {
      addToast(`Không dừng được task: ${e}`, 'ERROR');
      addLog(`StopTask failed: ${e}`, 'ERROR');
    }
  }

  async function handleRetryTask(taskID: string) {
    try {
      await (AppBindings as any).RetryTask(taskID);
      addLog('Đã đưa task về Backlog để chạy lại.', 'INFO');
      addToast('Đã đưa task về Backlog để chạy lại.', 'SUCCESS');
      if (onRefresh) await onRefresh();
    } catch (e) {
      addToast(`Retry thất bại: ${e}`, 'ERROR');
      addLog(`RetryTask failed: ${e}`, 'ERROR');
    }
  }

  async function handleClearDone() {
    try {
      const doneTasks = tasks.filter(t => t.status === 'done');
      if (doneTasks.length === 0) {
        addLog('Không có task nào ở trạng thái Done để xóa.', 'WARN');
        addToast('Không có task nào ở trạng thái Done để xoá.', 'INFO');
        return;
      }
      await (AppBindings as any).DeleteDoneTasks();
      const remaining = tasks.filter(t => t.status !== 'done');
      tasks = [...remaining];
      tasksStore.set([...remaining]);
      if (onRefresh) await onRefresh();
      addLog(`Đã dọn dẹp ${doneTasks.length} tasks đã hoàn thành (Done).`, 'SUCCESS');
      addToast(`Đã dọn dẹp ${doneTasks.length} tasks đã hoàn thành (Done).`, 'SUCCESS');
    } catch (e) {
      addLog(`Không dọn được task đã xong: ${e}`, 'ERROR');
      addToast(`Không dọn được task đã xong: ${e}`, 'ERROR');
    }
  }
</script>

<div class="space-y-4">
  <!-- Actions & Search Filter Bar -->
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div class="flex items-center gap-2 flex-1 max-w-md">
      <button
        type="button"
        on:click|preventDefault={() => showAddModal = true}
        class="bg-primary text-on-primary text-xs font-medium px-3 py-1.5 rounded-lg hover:opacity-90 transition-opacity cursor-pointer flex items-center gap-1.5 whitespace-nowrap">
        <span class="material-symbols-outlined text-sm">add</span> Thêm task
      </button>

      <!-- Search Input -->
      <div class="relative flex-1">
        <span class="material-symbols-outlined absolute left-2.5 top-2 text-sm text-on-surface-variant">search</span>
        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Tìm kiếm task..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg pl-8 pr-3 py-1.5 text-xs text-on-surface focus:border-primary"
        />
      </div>

      <!-- Priority Filter -->
      <select
        bind:value={filterPriority}
        aria-label="Lọc theo độ ưu tiên"
        class="bg-surface-container-low border border-outline-variant rounded-lg px-2.5 py-1.5 text-xs text-on-surface cursor-pointer">
        <option value="all">Tất cả độ ưu tiên</option>
        <option value="high">Cao</option>
        <option value="normal">Bình thường</option>
        <option value="low">Thấp</option>
      </select>
    </div>

    <div class="flex items-center gap-2">
      <!-- Parallel agents control -->
      <div class="flex items-center gap-1.5 bg-surface-container-low border border-outline-variant rounded-lg px-2.5 py-1.5" title="Số sub-agent chạy song song tối đa">
        <span class="material-symbols-outlined text-sm text-on-surface-variant">groups</span>
        <span class="text-[11px] font-medium text-on-surface-variant whitespace-nowrap">Song song</span>
        <select
          value={maxConcurrency}
          aria-label="Số agent chạy song song"
          on:change={(e) => updateConcurrency(parseInt((e.currentTarget as HTMLSelectElement).value))}
          class="bg-transparent text-xs text-on-surface cursor-pointer">
          {#each [1,2,3,4,5,6] as n}
            <option value={n}>{n}</option>
          {/each}
        </select>
      </div>
      <!-- Verify build before Done -->
      <button type="button" on:click={toggleVerify}
        title="Agent tự chạy go build / npm build để verify trước khi báo Done"
        class="flex items-center gap-1.5 border rounded-lg px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer
        {verifyBuild ? 'bg-secondary-container text-on-secondary-container border-outline-variant' : 'border-outline-variant text-on-surface-variant hover:bg-surface-container-high'}">
        <span class="material-symbols-outlined text-sm">{verifyBuild ? 'verified' : 'block'}</span>
        Verify build
      </button>
      {#if selectedTaskIDs.length > 0}
        <button type="button" on:click|preventDefault={handleDeleteSelected}
          class="border border-outline-variant text-error hover:bg-error/10 text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm">delete</span> Xoá đã chọn ({selectedTaskIDs.length})
        </button>
      {/if}
      <button type="button" on:click|preventDefault={refreshBoard} disabled={refreshing}
        title="Tải lại bảng từ cơ sở dữ liệu"
        class="border border-outline-variant text-on-surface-variant hover:bg-surface-container-high text-xs font-medium px-3 py-1.5 rounded-lg transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5">
        <span class="material-symbols-outlined text-sm {refreshing ? 'animate-spin' : ''}">refresh</span>
        Tải lại
      </button>
      <button type="button" on:click|preventDefault={handleClearDone}
        class="border border-outline-variant text-on-surface-variant hover:bg-surface-container-high text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer flex items-center gap-1.5">
        <span class="material-symbols-outlined text-sm">delete_sweep</span> Dọn task đã xong
      </button>
      <button type="button" on:click|preventDefault={handleClearAll}
        class="border border-outline-variant text-error hover:bg-error/10 text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer flex items-center gap-1.5">
        <span class="material-symbols-outlined text-sm">delete_forever</span> Xoá tất cả
      </button>
    </div>
  </div>

  <!-- 5 Columns Kanban Grid -->
  <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
    {#each columns as col}
      {@const colTasks = tasksByStatus[col.key] || []}
      <div class="flex flex-col gap-2">
        <!-- Column Header -->
        <div class="flex items-center justify-between px-1">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-base text-on-surface-variant">{col.icon}</span>
            <h3 class="text-sm font-semibold text-on-surface">{col.title} ({colTasks.length})</h3>
          </div>
        </div>

        <!-- Column Card Body with Drag & Drop target -->
        <div
          role="region"
          aria-label="{col.title} Column"
          class="min-h-[420px] border border-dashed rounded-xl p-3 space-y-3 overflow-y-auto max-h-[500px] transition-colors
          {dragOverColumn === col.key ? 'bg-primary/5 border-primary' : 'bg-surface-container-low/40 border-outline-variant'}"
          on:dragover={(e) => handleDragOver(e, col.key)}
          on:dragleave={handleDragLeave}
          on:drop={(e) => handleDrop(e, col.key)}
        >
          {#each colTasks as task (task.task_id)}
            <div
              role="group"
              draggable="true"
              animate:flip={{ duration: 180 }}
              in:fade={{ duration: 160 }}
              on:dragstart={(e) => handleDragStart(e, task.task_id)}
              class="bg-surface-container-lowest border border-outline-variant rounded-xl p-3 space-y-2 transition-colors cursor-grab active:cursor-grabbing group relative"
            >
              <!-- Exactly one keyboard-activatable element opens the inspector.
                   The checkbox and the drag handle sit beside it, not inside
                   it: they used to be nested in a role="button". -->
              <div class="flex items-center gap-2">
                <input
                  type="checkbox"
                  value={task.task_id}
                  aria-label="Chọn task {task.title}"
                  bind:group={selectedTaskIDs}
                  class="accent-primary cursor-pointer w-4 h-4 shrink-0"
                />
                <div
                  role="button"
                  tabindex="0"
                  on:click={() => detailTaskId = task.task_id}
                  on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); detailTaskId = task.task_id; } }}
                  class="flex-1 min-w-0 font-medium text-xs text-on-surface group-hover:text-primary transition-colors cursor-pointer truncate">
                  {task.title}
                </div>
                <span class="material-symbols-outlined text-sm text-outline group-hover:text-on-surface-variant cursor-grab shrink-0">drag_indicator</span>
              </div>

              {#if task.prompt}
                <p class="text-[11px] text-on-surface-variant line-clamp-2 pl-6">{task.prompt}</p>
              {/if}

              <!-- Why this task is not moving. A blocked task looked exactly like
                   a task waiting its turn, which is what made Execute Plan seem
                   dead: the board was full and nothing could ever be dispatched. -->
              {#if blockedById[task.task_id]}
                {@const blk = blockedById[task.task_id]}
                <div class="ml-6 rounded-lg px-2 py-1.5 border border-outline-variant text-[11px] leading-snug
                  {blk.dead ? 'bg-error/10 text-error' : 'bg-warning/10 text-warning'}">
                  <div class="flex items-center gap-1 font-medium">
                    <span class="material-symbols-outlined text-sm">
                      {blk.dead ? 'block' : 'hourglass_top'}
                    </span>
                    {blk.dead ? 'Bị chặn bởi task lỗi' : 'Đang chờ task khác'}
                  </div>
                  <ul class="mt-0.5 space-y-0.5">
                    {#each blk.blockers as b}
                      <li class="truncate" title="{b.title || b.task_id} — {b.status}">
                        • {b.title || b.task_id} <span class="opacity-70">({b.status})</span>
                      </li>
                    {/each}
                  </ul>
                  {#if blk.dead}
                    <button
                      type="button"
                      on:click|stopPropagation={() => retryBlockers(blk)}
                      class="mt-1.5 px-2 py-1 rounded-lg border border-outline-variant text-xs font-medium hover:bg-error/10 cursor-pointer"
                    >
                      Retry task đang chặn
                    </button>
                  {/if}
                </div>
              {/if}

              <!-- Assignment Dropdown -->
              <div class="space-y-1 pt-1 border-t border-outline-variant/30 ml-6">
                <span class="text-[11px] font-medium text-on-surface-variant">Phụ trách</span>
                <Dropdown
                  options={agentOptions}
                  value={task.assigned_to || ''}
                  on:change={(e) => handleAssignAgent(task.task_id, e.detail)}
                />
              </div>

              <!-- Status Dropdown & Priority -->
              <div class="flex items-center justify-between pt-1 border-t border-outline-variant/40 text-[11px] ml-6">
                <span class="px-2 py-0.5 rounded-full bg-surface-container-high text-on-surface-variant font-medium">{task.priority}</span>

                <!-- Quick status switcher -->
                <div class="w-28">
                  <Dropdown
                    options={[
                      { value: 'backlog', label: 'Backlog' },
                      { value: 'queued', label: 'Queued' },
                      { value: 'running', label: 'Running' },
                      { value: 'done', label: 'Done' },
                      { value: 'failed', label: 'Failed' }
                    ]}
                    value={task.status}
                    on:change={(e) => handleMoveStatus(task.task_id, e.detail)}
                  />
                </div>
              </div>
            </div>
          {:else}
            <div class="flex flex-col items-center justify-center h-48 text-center text-on-surface-variant">
              <span class="material-symbols-outlined text-base mb-1 text-outline">{col.icon}</span>
              <p class="text-xs">Kéo task vào đây</p>
            </div>
          {/each}
        </div>
      </div>
    {/each}
  </div>
</div>

{#if showAddModal}
<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
  <div class="bg-surface border border-outline-variant rounded-xl shadow-lg p-6 w-[450px] space-y-4">
    <div class="flex justify-between items-center pb-2 border-b border-outline-variant">
      <h3 class="text-sm font-semibold text-on-surface flex items-center gap-2">
        <span class="material-symbols-outlined text-base text-on-surface-variant">add_task</span> Tạo task mới
      </h3>
      <button type="button" on:click={() => showAddModal = false} aria-label="Đóng"
        class="text-on-surface-variant hover:text-on-surface cursor-pointer">
        <span class="material-symbols-outlined text-base">close</span>
      </button>
    </div>

    <div class="space-y-3 text-xs">
      <div>
        <label for="new-task-title" class="text-xs font-medium text-on-surface-variant block mb-1">Tiêu đề</label>
        <input
          id="new-task-title"
          type="text"
          bind:value={newTaskTitle}
          placeholder="Ví dụ: [CODE] Lập trình giao diện Login..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2.5 focus:border-primary text-on-surface"
        />
      </div>

      <div>
        <label for="new-task-desc" class="text-xs font-medium text-on-surface-variant block mb-1">Mô tả / prompt chi tiết</label>
        <textarea
          id="new-task-desc"
          bind:value={newTaskDescription}
          placeholder="Mô tả chi tiết công việc hoặc prompt cho AI Agent..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2.5 h-24 focus:border-primary text-on-surface resize-none"
        ></textarea>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="new-task-priority" class="text-xs font-medium text-on-surface-variant block mb-1">Độ ưu tiên</label>
          <select id="new-task-priority" bind:value={newTaskPriority} class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2 text-on-surface cursor-pointer">
            <option value="high">Cao</option>
            <option value="normal">Bình thường</option>
            <option value="low">Thấp</option>
          </select>
        </div>

        <div>
          <label for="new-task-assignee" class="text-xs font-medium text-on-surface-variant block mb-1">Phân công agent</label>
          <select id="new-task-assignee" bind:value={newTaskAssignedTo} class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2 text-on-surface cursor-pointer">
            {#each agentOptions as opt}
              <option value={opt.value}>{opt.label}</option>
            {/each}
          </select>
        </div>
      </div>
    </div>

    <div class="flex justify-end gap-2 pt-3 border-t border-outline-variant">
      <button
        type="button"
        on:click={() => showAddModal = false}
        class="border border-outline-variant text-on-surface-variant hover:bg-surface-container-high text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer">
        Huỷ
      </button>
      <button
        type="button"
        on:click={handleAddTask}
        disabled={creatingTask}
        class="bg-primary text-on-primary text-xs font-medium px-3 py-1.5 rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer flex items-center gap-1.5">
        <span class="material-symbols-outlined text-sm {creatingTask ? 'animate-spin' : ''}">{creatingTask ? 'progress_activity' : 'check'}</span>
        {creatingTask ? 'Đang tạo...' : 'Tạo task'}
      </button>
    </div>
  </div>
</div>
{/if}

{#if detailTask}
<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
  <div class="bg-surface border border-outline-variant rounded-xl shadow-lg p-6 w-full max-w-3xl max-h-[90vh] flex flex-col space-y-4">
    <!-- Modal Header -->
    <div class="flex justify-between items-start pb-3 border-b border-outline-variant">
      <div>
        <div class="flex items-center gap-2">
          <span class="text-xs font-medium text-on-surface-variant">Task Inspector</span>
          <span class="px-2 py-0.5 rounded-full text-[11px] font-medium
            {detailTask.status === 'running' ? 'bg-primary/10 text-primary' : detailTask.status === 'done' ? 'bg-success/10 text-success' : 'bg-surface-container-high text-on-surface-variant'}">
            {detailTask.status}
          </span>
        </div>
        <h3 class="text-sm font-semibold text-on-surface mt-1">
          {detailTask.title}
        </h3>
      </div>
      <button type="button" on:click={() => detailTaskId = null} aria-label="Đóng"
        class="text-on-surface-variant hover:text-on-surface p-1 rounded-lg hover:bg-surface-container-high transition-colors cursor-pointer">
        <span class="material-symbols-outlined text-base">close</span>
      </button>
    </div>

    <!-- Main Content Area -->
    <div class="flex-1 overflow-y-auto space-y-4 pr-1 text-xs">

      <!-- Subagent Card & Live Status -->
      <div class="bg-surface-container-low p-4 rounded-xl border border-outline-variant space-y-3">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-base text-on-surface-variant">smart_toy</span>
            <div>
              <div class="text-[11px] font-medium text-on-surface-variant">Sub-agent phụ trách</div>
              <h4 class="text-sm font-semibold text-on-surface">{detailTask.assigned_to || 'Chưa phân công'}</h4>
            </div>
          </div>

          <div class="flex items-center gap-2">
            <span class="text-[11px] font-medium text-on-surface-variant">Đổi agent</span>
            <select
              value={detailTask.assigned_to || ''}
              aria-label="Đổi agent phụ trách"
              on:change={(e) => handleAssignAgent(detailTask.task_id, e.currentTarget.value)}
              class="bg-surface-container-lowest border border-outline-variant rounded-lg p-1.5 text-xs text-on-surface focus:border-primary cursor-pointer"
            >
              {#each agentOptions as opt}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          </div>
        </div>

        {#if detailTask.status === 'running'}
          <div class="p-2.5 bg-surface-container border border-outline-variant rounded-lg flex items-center gap-2 text-on-surface-variant text-xs">
            <span class="material-symbols-outlined text-sm animate-spin text-primary">sync</span>
            <span>Sub-agent đang xử lý task này. Xem log bên dưới.</span>
          </div>
        {/if}
      </div>

      <!-- Task Prompt / Action breakdown -->
      <div class="space-y-1">
        <span class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm text-on-surface-variant">description</span> Prompt chi tiết
        </span>
        <div class="bg-surface-container-lowest p-3 rounded-xl border border-outline-variant font-mono text-on-surface-variant whitespace-pre-wrap text-[11px] leading-relaxed">
          {detailTask.prompt || detailTask.description || 'Không có prompt'}
        </div>
      </div>

      <!-- Realtime Log Execution Output Stream -->
      <div class="space-y-1">
        <div class="flex items-center justify-between">
          <span class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm text-on-surface-variant">terminal</span> Log sub-agent ({currentTaskLogs.length})
          </span>
          <div class="flex items-center gap-1.5">
            <button type="button" on:click={copyTaskLog}
              class="border border-outline-variant text-on-surface-variant hover:bg-surface-container-high text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer flex items-center gap-1.5">
              <span class="material-symbols-outlined text-sm">content_copy</span> Sao chép
            </button>
            <button type="button" on:click={() => detailTaskId && clearTaskLog(detailTaskId)}
              class="border border-outline-variant text-on-surface-variant hover:bg-surface-container-high text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer flex items-center gap-1.5">
              <span class="material-symbols-outlined text-sm">clear_all</span> Xoá log
            </button>
          </div>
        </div>
        <div bind:this={logContainer} class="bg-surface-container-highest text-on-surface-variant p-3 rounded-xl border border-outline-variant font-mono text-[11px] h-44 overflow-y-auto space-y-1 leading-relaxed scroll-smooth">
          <!-- Keyed by the store-assigned seq: time+message repeats during
               streaming, and a duplicate key is a thrown error in Svelte 5 —
               the drawer died mid-render with its overlay covering the app.
               The index fallback cannot collide either. -->
          {#each visibleTaskLogs as l, i (l.seq ?? `fallback-${i}`)}
            <div class="flex items-start gap-2">
              <span class="text-outline font-mono">[{l.time || 'NOW'}]</span>
              <span class={l.level === 'ERROR' ? 'text-error' : l.level === 'SUCCESS' ? 'text-success' : l.level === 'WARN' ? 'text-warning' : l.level === 'TOOL' ? 'text-primary' : 'text-on-surface-variant'}>
                {l.message}
              </span>
            </div>
          {:else}
            <div class="text-outline italic">Chưa có log cho task này. Log sẽ hiện realtime khi agent thực thi, bao gồm cả tool calls.</div>
          {/each}
        </div>
      </div>

      <!-- AI Result Deliverable -->
      {#if detailTask.result}
      <div class="space-y-1">
        <div class="flex items-center justify-between">
          <span class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm text-on-surface-variant">check_circle</span> Kết quả thực thi
          </span>
          <button
            type="button"
            on:click={() => { if (detailTask?.result) { navigator.clipboard.writeText(detailTask.result); addLog('Đã sao chép kết quả AI!', 'SUCCESS'); addToast('Đã sao chép kết quả.', 'SUCCESS'); } }}
            class="border border-outline-variant text-on-surface-variant hover:bg-surface-container-high text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm">content_copy</span> Sao chép
          </button>
        </div>
        <div class="bg-surface-container-lowest p-3 rounded-xl border border-outline-variant font-mono text-on-surface max-h-52 overflow-y-auto whitespace-pre-wrap text-[11px] leading-relaxed">
          {detailTask.result}
        </div>
      </div>
      {/if}

      <!-- E2E screenshot capture -->
      {#if detailTaskId && $taskScreenshotsStore[detailTaskId]}
      <div class="space-y-1">
        <span class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm text-on-surface-variant">photo_camera</span> Ảnh chụp E2E (Chrome CDP)
        </span>
        <a href={$taskScreenshotsStore[detailTaskId]} target="_blank" rel="noreferrer" class="block">
          <img src={$taskScreenshotsStore[detailTaskId]} alt="E2E screenshot"
            class="w-full rounded-xl border border-outline-variant max-h-72 object-contain object-top bg-surface-container hover:opacity-90 transition-opacity" />
        </a>
      </div>
      {/if}

      <!-- Git Diff — what the agent changed in the workspace -->
      <div class="space-y-1">
        <button type="button" on:click={loadDiff}
          class="w-full flex items-center justify-between bg-surface-container-low hover:bg-surface-container px-3 py-2 rounded-xl border border-outline-variant transition-colors cursor-pointer">
          <span class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm text-on-surface-variant">difference</span> Thay đổi mã (Git diff)
          </span>
          <span class="material-symbols-outlined text-sm text-on-surface-variant">{showDiff ? 'expand_less' : 'expand_more'}</span>
        </button>
        {#if showDiff}
          <div class="bg-surface-container-highest p-3 rounded-xl border border-outline-variant font-mono text-[11px] max-h-60 overflow-auto leading-relaxed">
            {#if loadingDiff}
              <span class="text-on-surface-variant italic">Đang tải diff...</span>
            {:else if !diffText.trim()}
              <!-- Backend returns "" for a clean tree; the text lives here now. -->
              <span class="text-on-surface-variant italic">Không có thay đổi nào trong workspace.</span>
            {:else}
              {#each diffText.split('\n') as line}
                <div class={line.startsWith('+') && !line.startsWith('+++') ? 'text-success' : line.startsWith('-') && !line.startsWith('---') ? 'text-error' : line.startsWith('@@') ? 'text-primary' : 'text-on-surface-variant'}>{line || ' '}</div>
              {/each}
            {/if}
          </div>
        {/if}
      </div>

    </div>

    <!-- Modal Footer -->
    <div class="flex justify-between items-center pt-3 border-t border-outline-variant">
      <div class="text-[11px] text-on-surface-variant font-mono">
        Session ID: <strong class="text-on-surface">{detailTask.session_id || 'N/A'}</strong>
      </div>
      <div class="flex items-center gap-2">
        {#if detailTask.status === 'running'}
          <button
            type="button"
            on:click={() => detailTask && handleStopTask(detailTask.task_id)}
            class="border border-outline-variant text-error hover:bg-error/10 text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm">stop_circle</span> Dừng task
          </button>
        {/if}
        {#if detailTask.status === 'failed' || detailTask.status === 'done'}
          <button
            type="button"
            on:click={() => detailTask && handleRetryTask(detailTask.task_id)}
            class="border border-outline-variant text-on-surface-variant hover:bg-surface-container-high text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm">replay</span> Chạy lại
          </button>
        {/if}
        <button
          type="button"
          on:click={() => detailTaskId = null}
          class="border border-outline-variant text-on-surface-variant hover:bg-surface-container-high text-xs font-medium px-3 py-1.5 rounded-lg transition-colors cursor-pointer">
          Đóng
        </button>
      </div>
    </div>
  </div>
</div>
{/if}
