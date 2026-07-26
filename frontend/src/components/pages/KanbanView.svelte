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
    { value: '', label: 'Unassigned (Auto Dispatch)' },
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
    maxConcurrency = n;
    try {
      await (AppBindings as any).SetMaxConcurrency(n);
      addLog(`Số agent song song tối đa: ${n}`, 'INFO');
      addToast(`Chạy tối đa ${n} agent song song.`, 'SUCCESS');
    } catch (e) {
      // The local variable already moved, so a failure here leaves the control
      // showing a limit the orchestrator is not using.
      addToast(`Không đặt được số agent song song: ${e}`, 'ERROR');
    }
  }

  let verifyBuild = true;
  async function toggleVerify() {
    verifyBuild = !verifyBuild;
    try {
      await (AppBindings as any).SetVerifyBuild(verifyBuild);
      addLog(`Verify build trước khi Done: ${verifyBuild ? 'BẬT' : 'TẮT'}`, 'INFO');
      addToast(`Verify build trước khi Done: ${verifyBuild ? 'BẬT' : 'TẮT'}.`, 'SUCCESS');
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

  const columns = [
    { key: 'backlog', title: 'Backlog', icon: 'inventory_2', color: 'text-outline' },
    { key: 'queued', title: 'Queued', icon: 'hourglass_empty', color: 'text-secondary' },
    { key: 'running', title: 'Running', icon: 'bolt', color: 'text-primary' },
    { key: 'done', title: 'Done', icon: 'check_circle', color: 'text-emerald-600' },
    { key: 'failed', title: 'Failed', icon: 'cancel', color: 'text-rose-600' },
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
    if (!newTaskTitle.trim()) return;
    const title = newTaskTitle;
    const desc = newTaskDescription || newTaskTitle;
    const priority = newTaskPriority;
    const assignedTo = newTaskAssignedTo;

    newTaskTitle = '';
    newTaskDescription = '';
    showAddModal = false;

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
      newTaskTitle = '';
      newTaskDescription = '';
      addLog(`Đã tạo Task mới: ${title}`, 'SUCCESS');
      addToast(`Đã tạo Task mới: ${title}`, 'SUCCESS');
      if (onRefresh) await onRefresh();
    } catch (e) {
      console.error('CreateTask error:', e);
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
        addLog(`Task assigned to ${assignedTo || 'Unassigned'}`, 'SUCCESS');
        addToast(`Task assigned to ${assignedTo || 'Unassigned'}`, 'SUCCESS');
      }
    } catch (e) {
      console.error(e);
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
        console.error('ClearAll error:', e);
      }
    }
  }

  async function handleDeleteSelected() {
    if (selectedTaskIDs.length === 0) return;
    if (confirm(`Bạn có chắc muốn xóa ${selectedTaskIDs.length} tasks đã chọn?`)) {
      const removed = selectedTaskIDs.length;
      for (const id of selectedTaskIDs) {
        await AppBindings.DeleteTask(id);
      }
      selectedTaskIDs = [];
      // The store is the single source now, so refresh it rather than editing a
      // local copy the next board event would overwrite.
      await refreshBoard();
      addLog(`Đã xóa ${removed} tasks đã chọn.`, 'SUCCESS');
      addToast(`Đã xóa ${removed} tasks đã chọn.`, 'SUCCESS');
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
      console.error('ClearDone error:', e);
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
        class="bg-primary text-on-primary px-4 py-2 rounded-xl text-xs font-bold flex items-center gap-1.5 hover:opacity-90 transition-all shadow-sm cursor-pointer whitespace-nowrap">
        <span class="material-symbols-outlined text-sm font-bold">add</span> Add Task
      </button>

      <!-- Search Input -->
      <div class="relative flex-1">
        <span class="material-symbols-outlined absolute left-3 top-2.5 text-sm text-outline">search</span>
        <input 
          type="text" 
          bind:value={searchQuery}
          placeholder="Tìm kiếm task..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-xl pl-8 pr-3 py-1.5 text-xs text-on-surface outline-none focus:border-primary font-medium"
        />
      </div>

      <!-- Priority Filter -->
      <select 
        bind:value={filterPriority}
        class="bg-surface-container-low border border-outline-variant rounded-xl px-2.5 py-1.5 text-xs text-on-surface font-semibold outline-none cursor-pointer">
        <option value="all">Tất cả Priority</option>
        <option value="high">HIGH (Cao)</option>
        <option value="normal">NORMAL (Bình thường)</option>
        <option value="low">LOW (Thấp)</option>
      </select>
    </div>

    <div class="flex items-center gap-2">
      <!-- Parallel agents control -->
      <div class="flex items-center gap-1.5 bg-surface-container-low border border-outline-variant rounded-xl px-2.5 py-1.5" title="Số sub-agent chạy song song tối đa">
        <span class="material-symbols-outlined text-sm text-secondary">groups</span>
        <span class="text-[10px] font-bold text-on-surface-variant uppercase whitespace-nowrap">Song song</span>
        <select
          value={maxConcurrency}
          on:change={(e) => updateConcurrency(parseInt((e.currentTarget as HTMLSelectElement).value))}
          class="bg-transparent text-xs font-bold text-on-surface outline-none cursor-pointer">
          {#each [1,2,3,4,5,6] as n}
            <option value={n}>{n}</option>
          {/each}
        </select>
      </div>
      <!-- Verify build before Done -->
      <button type="button" on:click={toggleVerify}
        title="Agent tự chạy go build / npm build để verify trước khi báo Done"
        class="flex items-center gap-1.5 border rounded-xl px-2.5 py-1.5 text-[10px] font-bold uppercase transition-all cursor-pointer
        {verifyBuild ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/30' : 'bg-surface-container-low text-on-surface-variant border-outline-variant'}">
        <span class="material-symbols-outlined text-sm">{verifyBuild ? 'verified' : 'block'}</span>
        Verify build
      </button>
      {#if selectedTaskIDs.length > 0}
        <button type="button" on:click|preventDefault={handleDeleteSelected} class="bg-rose-600 text-white px-3 py-1.5 rounded-lg text-xs font-bold flex items-center gap-1 animate-pulse cursor-pointer">
          <span class="material-symbols-outlined text-sm">delete</span> Xóa đã chọn ({selectedTaskIDs.length})
        </button>
      {/if}
      <button type="button" on:click|preventDefault={refreshBoard} disabled={refreshing}
        title="Tải lại bảng từ cơ sở dữ liệu"
        class="bg-surface-container-highest border border-outline-variant px-3.5 py-1.5 rounded-xl text-xs font-semibold hover:bg-surface-container-high disabled:opacity-50 transition-all flex items-center gap-1.5 cursor-pointer">
        <span class="material-symbols-outlined text-sm {refreshing ? 'animate-spin' : ''}">refresh</span>
        Tải lại
      </button>
      <button type="button" on:click|preventDefault={handleClearDone} class="bg-surface-container-highest border border-outline-variant px-3.5 py-1.5 rounded-xl text-xs font-semibold hover:bg-surface-container-high transition-all flex items-center gap-1.5 cursor-pointer">
        <span class="material-symbols-outlined text-sm">delete_sweep</span> Clear Done
      </button>
      <button type="button" on:click|preventDefault={handleClearAll} class="bg-rose-500/10 text-rose-600 border border-rose-500/30 px-3.5 py-1.5 rounded-xl text-xs font-bold hover:bg-rose-500/20 transition-all flex items-center gap-1.5 cursor-pointer">
        <span class="material-symbols-outlined text-sm">delete_forever</span> Clear All
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
            <span class="material-symbols-outlined {col.color}">{col.icon}</span>
            <h3 class="text-xs font-bold uppercase text-on-surface-variant">{col.title} ({colTasks.length})</h3>
          </div>
        </div>

        <!-- Column Card Body with Drag & Drop target -->
        <div
          role="region"
          aria-label="{col.title} Column"
          class="min-h-[420px] border border-dashed rounded-xl p-3 space-y-3 overflow-y-auto max-h-[500px] transition-all
          {dragOverColumn === col.key ? 'bg-primary-container/20 border-primary border-2 scale-[1.01]' : 'bg-surface-container-low/40 border-outline-variant'}"
          on:dragover={(e) => handleDragOver(e, col.key)}
          on:dragleave={handleDragLeave}
          on:drop={(e) => handleDrop(e, col.key)}
        >
          {#each colTasks as task (task.task_id)}
            <div
              role="button"
              tabindex="0"
              draggable="true"
              animate:flip={{ duration: 320 }}
              in:fade={{ duration: 160 }}
              on:dragstart={(e) => handleDragStart(e, task.task_id)}
              class="bg-surface-container-lowest border border-outline-variant border-l-4 border-l-primary rounded-xl p-3 space-y-2 shadow-sm hover:shadow-md transition-all cursor-grab active:cursor-grabbing group relative"
            >
              <div 
                role="button" 
                tabindex="0"
                on:click={() => detailTaskId = task.task_id}
                on:keydown={(e) => { if (e.key === 'Enter') detailTaskId = task.task_id; }}
                class="flex items-center justify-between gap-2 cursor-pointer">
                <div class="flex items-center gap-2 flex-1 cursor-pointer">
                  <input
                    type="checkbox"
                    value={task.task_id}
                    bind:group={selectedTaskIDs}
                    class="rounded border-outline-variant text-primary focus:ring-primary cursor-pointer w-4 h-4"
                  />
                  <div class="font-semibold text-xs text-on-surface group-hover:text-primary transition-colors">{task.title}</div>
                </div>
                <span class="material-symbols-outlined text-sm text-outline group-hover:text-on-surface-variant cursor-grab">drag_indicator</span>
              </div>

              {#if task.prompt}
                <p class="text-[11px] text-on-surface-variant line-clamp-2 pl-6">{task.prompt}</p>
              {/if}

              <!-- Why this task is not moving. A blocked task looked exactly like
                   a task waiting its turn, which is what made Execute Plan seem
                   dead: the board was full and nothing could ever be dispatched. -->
              {#if blockedById[task.task_id]}
                {@const blk = blockedById[task.task_id]}
                <div class="ml-6 rounded-lg px-2 py-1.5 border text-[10px] leading-snug
                  {blk.dead
                    ? 'bg-red-500/10 border-red-500/30 text-red-600 dark:text-red-400'
                    : 'bg-amber-500/10 border-amber-500/30 text-amber-700 dark:text-amber-400'}">
                  <div class="flex items-center gap-1 font-bold">
                    <span class="material-symbols-outlined text-[13px]">
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
                      class="mt-1 px-2 py-0.5 rounded border border-red-500/40 font-bold hover:bg-red-500/20 cursor-pointer"
                    >
                      Retry task đang chặn
                    </button>
                  {/if}
                </div>
              {/if}

              <!-- Assignment Dropdown -->
              <div class="space-y-1 pt-1 border-t border-outline-variant/30 ml-6">
                <span class="text-[9px] font-bold text-on-surface-variant uppercase">Assignee:</span>
                <Dropdown
                  options={agentOptions}
                  value={task.assigned_to || ''}
                  on:change={(e) => handleAssignAgent(task.task_id, e.detail)}
                />
              </div>

              <!-- Status Dropdown & Priority -->
              <div class="flex items-center justify-between pt-1 border-t border-outline-variant/40 text-[10px] ml-6">
                <span class="px-2 py-0.5 rounded-full bg-secondary-container text-on-secondary-container font-mono font-bold uppercase">{task.priority}</span>

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
            <div class="flex flex-col items-center justify-center h-48 text-center text-on-surface-variant opacity-50">
              <span class="material-symbols-outlined text-3xl mb-1">{col.icon}</span>
              <p class="text-xs">Kéo thả Task vào đây</p>
            </div>
          {/each}
        </div>
      </div>
    {/each}
  </div>
</div>

{#if showAddModal}
<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
  <div class="bg-surface border border-outline-variant rounded-2xl shadow-2xl p-6 w-[450px] space-y-4">
    <div class="flex justify-between items-center pb-2 border-b border-outline-variant">
      <h3 class="text-base font-bold text-on-surface flex items-center gap-2">
        <span class="material-symbols-outlined text-primary">add_task</span> Tạo Task Mới (Kanban)
      </h3>
      <button type="button" on:click={() => showAddModal = false} class="text-on-surface-variant hover:text-on-surface">
        <span class="material-symbols-outlined text-xl">close</span>
      </button>
    </div>

    <div class="space-y-3 text-xs">
      <div>
        <label for="new-task-title" class="font-bold text-on-surface block mb-1">Tiêu đề Task:</label>
        <input 
          id="new-task-title"
          type="text"
          bind:value={newTaskTitle}
          placeholder="Ví dụ: [CODE] Lập trình giao diện Login..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2.5 outline-none focus:border-primary text-on-surface font-semibold"
        />
      </div>

      <div>
        <label for="new-task-desc" class="font-bold text-on-surface block mb-1">Mô tả / Prompt chi tiết:</label>
        <textarea
          id="new-task-desc"
          bind:value={newTaskDescription}
          placeholder="Mô tả chi tiết công việc hoặc prompt cho AI Agent..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2.5 h-24 outline-none focus:border-primary text-on-surface resize-none"
        ></textarea>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="new-task-priority" class="font-bold text-on-surface block mb-1">Độ ưu tiên:</label>
          <select id="new-task-priority" bind:value={newTaskPriority} class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2 text-on-surface outline-none">
            <option value="high">HIGH (Cao)</option>
            <option value="normal">NORMAL (Bình thường)</option>
            <option value="low">LOW (Thấp)</option>
          </select>
        </div>

        <div>
          <label for="new-task-assignee" class="font-bold text-on-surface block mb-1">Phân công Agent:</label>
          <select id="new-task-assignee" bind:value={newTaskAssignedTo} class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2 text-on-surface outline-none">
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
        class="px-4 py-2 bg-surface-container-high text-on-surface-variant font-bold text-xs rounded-xl hover:bg-surface-container-highest transition-all cursor-pointer">
        Hủy bỏ
      </button>
      <button 
        type="button" 
        on:click={handleAddTask} 
        class="px-4 py-2 bg-primary text-on-primary font-bold text-xs rounded-xl hover:opacity-90 transition-all cursor-pointer flex items-center gap-1">
        <span class="material-symbols-outlined text-sm">check</span> Tạo Task
      </button>
    </div>
  </div>
</div>
{/if}

{#if detailTask}
<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-md p-4 animate-in fade-in duration-200">
  <div class="bg-surface border border-outline-variant rounded-2xl shadow-2xl p-6 w-full max-w-3xl max-h-[90vh] flex flex-col space-y-4">
    <!-- Modal Header -->
    <div class="flex justify-between items-start pb-3 border-b border-outline-variant">
      <div>
        <div class="flex items-center gap-2">
          <span class="px-2 py-0.5 rounded font-mono text-[10px] font-bold uppercase bg-primary/10 text-primary border border-primary/20">
            Task Inspector & Sub-Agent Monitor
          </span>
          <span class="px-2 py-0.5 rounded font-mono text-[10px] font-bold uppercase {detailTask.status === 'running' ? 'bg-primary/20 text-primary animate-pulse' : detailTask.status === 'done' ? 'bg-emerald-500/20 text-emerald-500' : 'bg-surface-container-high text-on-surface-variant'}">
            {detailTask.status}
          </span>
        </div>
        <h3 class="text-base font-bold text-on-surface mt-1 flex items-center gap-2">
          {detailTask.title}
        </h3>
      </div>
      <button type="button" on:click={() => detailTaskId = null} class="text-on-surface-variant hover:text-on-surface p-1 rounded-lg hover:bg-surface-container-high transition-all cursor-pointer">
        <span class="material-symbols-outlined text-xl">close</span>
      </button>
    </div>

    <!-- Main Content Area -->
    <div class="flex-1 overflow-y-auto space-y-4 pr-1 text-xs">
      
      <!-- Subagent Card & Live Status -->
      <div class="bg-surface-container-low p-4 rounded-xl border border-outline-variant/60 space-y-3 shadow-xs">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
              <span class="material-symbols-outlined text-xl">smart_toy</span>
            </div>
            <div>
              <div class="text-[10px] uppercase font-bold text-outline">Sub-Agent Phân Công</div>
              <h4 class="font-bold text-sm text-on-surface">{detailTask.assigned_to || 'Chưa phân công (Unassigned)'}</h4>
            </div>
          </div>

          <div class="flex items-center gap-2">
            <span class="text-[11px] font-bold text-on-surface-variant">Đổi Agent:</span>
            <select
              value={detailTask.assigned_to || ''}
              on:change={(e) => handleAssignAgent(detailTask.task_id, e.currentTarget.value)}
              class="bg-surface-container-lowest border border-outline-variant rounded-lg p-1.5 text-xs text-on-surface font-semibold outline-none focus:border-primary cursor-pointer"
            >
              {#each agentOptions as opt}
                <option value={opt.value}>{opt.label}</option>
              {/each}
            </select>
          </div>
        </div>

        {#if detailTask.status === 'running'}
          <div class="p-2.5 bg-primary/10 border border-primary/20 rounded-lg flex items-center gap-2 text-primary font-bold text-xs animate-pulse">
            <span class="material-symbols-outlined text-base animate-spin">sync</span>
            <span>Sub-Agent đang xử lý task này ngầm... Vui lòng xem log bên dưới!</span>
          </div>
        {/if}
      </div>

      <!-- Task Prompt / Action breakdown -->
      <div class="space-y-1">
        <span class="font-bold text-on-surface block">📌 Prompt / Yêu cầu chi tiết:</span>
        <div class="bg-surface-container-lowest p-3 rounded-xl border border-outline-variant font-mono text-on-surface-variant whitespace-pre-wrap text-[11px] leading-relaxed">
          {detailTask.prompt || detailTask.description || 'Không có prompt'}
        </div>
      </div>

      <!-- Realtime Log Execution Output Stream -->
      <div class="space-y-1">
        <div class="flex items-center justify-between">
          <span class="font-bold text-on-surface flex items-center gap-1">
            <span class="material-symbols-outlined text-sm text-secondary">terminal</span> Realtime Sub-Agent Log ({currentTaskLogs.length}):
          </span>
          <div class="flex items-center gap-1.5">
            <button type="button" on:click={copyTaskLog}
              class="text-[10px] bg-surface-container-high text-on-surface hover:bg-surface-container-highest px-2 py-0.5 rounded font-bold border border-outline-variant cursor-pointer flex items-center gap-1">
              <span class="material-symbols-outlined text-xs">content_copy</span> Copy
            </button>
            <button type="button" on:click={() => detailTaskId && clearTaskLog(detailTaskId)}
              class="text-[10px] bg-surface-container-high text-on-surface hover:bg-rose-500/10 hover:text-rose-600 px-2 py-0.5 rounded font-bold border border-outline-variant cursor-pointer flex items-center gap-1">
              <span class="material-symbols-outlined text-xs">clear_all</span> Clear
            </button>
          </div>
        </div>
        <div bind:this={logContainer} class="bg-black/90 text-emerald-400 p-3 rounded-xl border border-slate-800 font-mono text-[11px] h-44 overflow-y-auto space-y-1 shadow-inner leading-relaxed scroll-smooth">
          {#each visibleTaskLogs as l (l.time + l.message)}
            <div class="flex items-start gap-2">
              <span class="text-slate-500 font-mono">[{l.time || 'NOW'}]</span>
              <span class={l.level === 'ERROR' ? 'text-rose-400' : l.level === 'SUCCESS' ? 'text-emerald-400' : l.level === 'WARN' ? 'text-amber-300' : l.level === 'TOOL' ? 'text-cyan-300 font-semibold' : 'text-slate-300'}>
                {l.message}
              </span>
            </div>
          {:else}
            <div class="text-slate-600 italic">Chưa có log cho task này. Log sẽ hiện realtime khi Agent thực thi (bao gồm cả tool calls 🔧).</div>
          {/each}
        </div>
      </div>

      <!-- AI Result Deliverable -->
      {#if detailTask.result}
      <div class="space-y-1">
        <div class="flex items-center justify-between">
          <span class="font-bold text-emerald-600 dark:text-emerald-400 flex items-center gap-1">
            <span class="material-symbols-outlined text-sm">check_circle</span> Kết quả Thực thi AI Deliverable:
          </span>
          <button
            type="button"
            on:click={() => { if (detailTask?.result) { navigator.clipboard.writeText(detailTask.result); addLog('Đã sao chép kết quả AI!', 'SUCCESS'); } }}
            class="text-[10px] bg-surface-container-high text-on-surface hover:bg-surface-container-highest px-2 py-0.5 rounded font-bold border border-outline-variant cursor-pointer flex items-center gap-1">
            <span class="material-symbols-outlined text-xs">content_copy</span> Sao chép Kết quả
          </button>
        </div>
        <div class="bg-surface-container-lowest p-3 rounded-xl border border-emerald-500/30 font-mono text-on-surface max-h-52 overflow-y-auto whitespace-pre-wrap text-[11px] leading-relaxed">
          {detailTask.result}
        </div>
      </div>
      {/if}

      <!-- E2E screenshot capture -->
      {#if detailTaskId && $taskScreenshotsStore[detailTaskId]}
      <div class="space-y-1">
        <span class="font-bold text-on-surface flex items-center gap-1">
          <span class="material-symbols-outlined text-sm text-secondary">photo_camera</span> Ảnh chụp E2E (Chrome CDP):
        </span>
        <a href={$taskScreenshotsStore[detailTaskId]} target="_blank" rel="noreferrer" class="block">
          <img src={$taskScreenshotsStore[detailTaskId]} alt="E2E screenshot"
            class="w-full rounded-xl border border-outline-variant max-h-72 object-contain object-top bg-black/40 hover:opacity-90 transition-opacity" />
        </a>
      </div>
      {/if}

      <!-- Git Diff — what the agent changed in the workspace -->
      <div class="space-y-1">
        <button type="button" on:click={loadDiff}
          class="w-full flex items-center justify-between bg-surface-container-low hover:bg-surface-container px-3 py-2 rounded-xl border border-outline-variant transition-all cursor-pointer">
          <span class="font-bold text-on-surface flex items-center gap-1">
            <span class="material-symbols-outlined text-sm text-secondary">difference</span> Thay đổi mã (Git Diff)
          </span>
          <span class="material-symbols-outlined text-sm text-on-surface-variant">{showDiff ? 'expand_less' : 'expand_more'}</span>
        </button>
        {#if showDiff}
          <div class="bg-black/90 p-3 rounded-xl border border-slate-800 font-mono text-[11px] max-h-60 overflow-auto leading-relaxed">
            {#if loadingDiff}
              <span class="text-slate-400 italic">Đang tải diff...</span>
            {:else if !diffText.trim()}
              <!-- Backend returns "" for a clean tree; the text lives here now. -->
              <span class="text-slate-400 italic">Không có thay đổi nào trong workspace.</span>
            {:else}
              {#each diffText.split('\n') as line}
                <div class={line.startsWith('+') && !line.startsWith('+++') ? 'text-emerald-400' : line.startsWith('-') && !line.startsWith('---') ? 'text-rose-400' : line.startsWith('@@') ? 'text-cyan-300' : 'text-slate-300'}>{line || ' '}</div>
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
            class="px-4 py-2 bg-rose-500/10 text-rose-600 border border-rose-500/30 font-bold text-xs rounded-xl hover:bg-rose-500/20 transition-all cursor-pointer flex items-center gap-1">
            <span class="material-symbols-outlined text-sm">stop_circle</span> Dừng Task
          </button>
        {/if}
        {#if detailTask.status === 'failed' || detailTask.status === 'done'}
          <button
            type="button"
            on:click={() => detailTask && handleRetryTask(detailTask.task_id)}
            class="px-4 py-2 bg-amber-500/10 text-amber-600 border border-amber-500/30 font-bold text-xs rounded-xl hover:bg-amber-500/20 transition-all cursor-pointer flex items-center gap-1">
            <span class="material-symbols-outlined text-sm">replay</span> Chạy lại
          </button>
        {/if}
        <button
          type="button"
          on:click={() => detailTaskId = null}
          class="px-5 py-2 bg-primary text-on-primary font-bold text-xs rounded-xl hover:opacity-90 transition-all cursor-pointer shadow-xs">
          Đóng Panel
        </button>
      </div>
    </div>
  </div>
</div>
{/if}
