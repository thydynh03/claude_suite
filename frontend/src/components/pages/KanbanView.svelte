<script lang="ts">
  import type { Task } from '../../lib/types';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog } from '../../lib/stores/appState';

  export let tasks: Task[] = [];
  export let onRefresh: () => void;

  let newTaskTitle = '';

  const columns = [
    { key: 'backlog', title: 'Backlog', icon: 'inventory_2', color: 'text-outline' },
    { key: 'queued', title: 'Queued', icon: 'hourglass_empty', color: 'text-secondary' },
    { key: 'running', title: 'Running', icon: 'bolt', color: 'text-primary' },
    { key: 'done', title: 'Done', icon: 'check_circle', color: 'text-emerald-600' },
    { key: 'failed', title: 'Failed', icon: 'cancel', color: 'text-rose-600' },
  ];

  function getTasksByStatus(status: string): Task[] {
    return tasks.filter((t) => t.status === status);
  }

  async function handleAddTask() {
    if (!newTaskTitle.trim()) return;
    await AppBindings.CreateTask({
      task_id: '',
      title: newTaskTitle,
      description: '',
      prompt: newTaskTitle,
      priority: 'normal',
      status: 'backlog',
      assigned_to: '',
      depends_on: [],
      retry_count: 0,
      max_retries: 3,
      result: '',
      session_id: '',
      parent_id: '',
    });
    newTaskTitle = '';
    onRefresh();
  }

  async function handleMoveStatus(taskID: string, newStatus: string) {
    await AppBindings.UpdateTaskStatus(taskID, newStatus);
    onRefresh();
  }

  async function handleClearDone() {
    await AppBindings.DeleteDoneTasks();
    onRefresh();
  }
</script>

<div class="space-y-4">
  <!-- Actions Bar -->
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div class="flex items-center gap-2">
      <input
        type="text"
        bind:value={newTaskTitle}
        on:keydown={(e) => e.key === 'Enter' && handleAddTask()}
        placeholder="Tạo Task mới (nhấn Enter)..."
        class="bg-surface-container-lowest border border-outline-variant px-3 py-1.5 rounded-lg text-xs w-72 focus:ring-2 focus:ring-primary outline-none"
      />
      <button on:click={handleAddTask} class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-bold flex items-center gap-1">
        <span class="material-symbols-outlined text-sm">add</span> Add Task
      </button>
    </div>

    <div class="flex items-center gap-2">
      <button on:click={handleClearDone} class="bg-surface-container-highest border border-outline-variant px-3 py-1.5 rounded-lg text-xs font-semibold hover:bg-surface-container-high transition-all flex items-center gap-1">
        <span class="material-symbols-outlined text-sm">delete_sweep</span> Clear Done
      </button>
    </div>
  </div>

  <!-- 5 Columns Kanban Grid -->
  <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
    {#each columns as col}
      {@const colTasks = getTasksByStatus(col.key)}
      <div class="flex flex-col gap-2">
        <!-- Column Header -->
        <div class="flex items-center justify-between px-1">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined {col.color}">{col.icon}</span>
            <h3 class="text-xs font-bold uppercase text-on-surface-variant">{col.title} ({colTasks.length})</h3>
          </div>
        </div>

        <!-- Column Card Body -->
        <div class="min-h-[420px] bg-surface-container-low/40 border border-outline-variant border-dashed rounded-xl p-3 space-y-3 overflow-y-auto max-h-[500px]">
          {#each colTasks as task}
            <div class="bg-surface-container-lowest border border-outline-variant border-l-4 border-l-primary rounded-xl p-3 space-y-2 shadow-sm hover:shadow-md transition-all">
              <div class="font-semibold text-xs text-on-surface">{task.title}</div>
              {#if task.prompt}
                <p class="text-[11px] text-on-surface-variant line-clamp-2">{task.prompt}</p>
              {/if}
              <div class="flex items-center justify-between pt-1 border-t border-outline-variant/40 text-[10px]">
                <span class="px-2 py-0.5 rounded-full bg-secondary-container text-on-secondary-container font-mono font-bold uppercase">{task.priority}</span>

                <!-- Quick status switcher -->
                <select
                  value={task.status}
                  on:change={(e) => handleMoveStatus(task.task_id, e.currentTarget.value)}
                  class="bg-surface-container-low border border-outline-variant rounded px-1 text-[10px] outline-none"
                >
                  <option value="backlog">Backlog</option>
                  <option value="queued">Queued</option>
                  <option value="running">Running</option>
                  <option value="done">Done</option>
                  <option value="failed">Failed</option>
                </select>
              </div>
            </div>
          {:else}
            <div class="flex flex-col items-center justify-center h-48 text-center text-on-surface-variant opacity-50">
              <span class="material-symbols-outlined text-3xl mb-1">{col.icon}</span>
              <p class="text-xs">Empty</p>
            </div>
          {/each}
        </div>
      </div>
    {/each}
  </div>
</div>
