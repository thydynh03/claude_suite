<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { EventsOn } from '../../../wailsjs/runtime/runtime';

  let mode = 'time'; // 'time' or 'countdown'
  
  let timeHour = '05';
  let timeMin = '40';

  let cdHour = '00';
  let cdMin = '30';
  let cdSec = '00';

  let prompt = 'continue';
  let repeat = false;
  
  let jobs: any[] = [];
  let schedulerLogs: {msg: string, level: string, time: string}[] = [];

  onMount(async () => {
    refreshJobs();
    
    // @ts-ignore
    if (window.runtime?.EventsOn) {
      // @ts-ignore
      window.runtime.EventsOn('scheduler_updated', () => {
        refreshJobs();
      });
      // @ts-ignore
      window.runtime.EventsOn('scheduler_log', (data: any) => {
        if (data) {
          const now = new Date();
          schedulerLogs = [...schedulerLogs, {
            msg: data.msg || data, 
            level: data.level || 'INFO', 
            time: `${now.getHours()}:${now.getMinutes()}:${now.getSeconds()}`
          }];
        }
      });
    }
  });

  async function refreshJobs() {
    try {
      // @ts-ignore
      if (window.go?.main?.App?.GetScheduledJobs) {
        // @ts-ignore
        jobs = await window.go.main.App.GetScheduledJobs();
      }
    } catch (e) {
      console.error(e);
    }
  }

  async function schedulePrompt() {
    if (!prompt.trim()) {
      alert("Vui lòng nhập prompt!");
      return;
    }

    let targetDate = new Date();
    
    if (mode === 'time') {
      const h = parseInt(timeHour);
      const m = parseInt(timeMin);
      targetDate.setHours(h, m, 0, 0);
      if (targetDate <= new Date()) {
        targetDate.setDate(targetDate.getDate() + 1);
      }
    } else {
      const h = parseInt(cdHour);
      const m = parseInt(cdMin);
      const s = parseInt(cdSec);
      const totalMs = (h * 3600 + m * 60 + s) * 1000;
      targetDate = new Date(Date.now() + totalMs);
    }

    try {
      // @ts-ignore
      if (window.go?.main?.App?.SchedulePrompt) {
        // @ts-ignore
        await window.go.main.App.SchedulePrompt(prompt, targetDate.toISOString(), repeat);
        refreshJobs();
      }
    } catch (e) {
      console.error(e);
      alert("Lỗi khi đặt lịch: " + e);
    }
  }

  async function cancelJob(id: string) {
    try {
      // @ts-ignore
      if (window.go?.main?.App?.CancelScheduledJob) {
        // @ts-ignore
        await window.go.main.App.CancelScheduledJob(id);
        refreshJobs();
      }
    } catch (e) {
      console.error(e);
    }
  }

  function setPreset(val: string) {
    prompt = val;
  }
</script>

<div class="flex flex-col h-full bg-background gap-6">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-3xl font-bold font-display text-on-surface mb-2">Claude Prompt Scheduler</h1>
      <p class="text-on-surface-muted">Hẹn giờ gửi prompt tự động vào Claude CLI</p>
    </div>
  </div>

  <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 h-full min-h-0">
    <!-- Cột trái: Cài đặt -->
    <div class="flex flex-col gap-6 overflow-y-auto pr-2">
      <!-- Mode -->
      <div class="bg-surface border border-border rounded-xl p-5 shadow-sm">
        <h3 class="text-sm font-bold text-on-surface-muted mb-4 tracking-wider uppercase">Chế độ</h3>
        <div class="flex gap-4">
          <label class="flex items-center gap-2 cursor-pointer">
            <input type="radio" bind:group={mode} value="time" class="accent-primary" />
            <span class="text-on-surface font-medium">Theo giờ cụ thể</span>
          </label>
          <label class="flex items-center gap-2 cursor-pointer">
            <input type="radio" bind:group={mode} value="countdown" class="accent-primary" />
            <span class="text-on-surface font-medium">Đếm ngược</span>
          </label>
        </div>
      </div>

      <!-- Time Input -->
      <div class="bg-surface border border-border rounded-xl p-5 shadow-sm">
        <h3 class="text-sm font-bold text-on-surface-muted mb-4 tracking-wider uppercase">Thời gian</h3>
        
        {#if mode === 'time'}
          <div class="flex items-center gap-4">
            <div class="flex items-center gap-2">
              <span class="text-on-surface">Giờ:</span>
              <input type="number" min="0" max="23" bind:value={timeHour} 
                class="w-16 h-12 bg-background border border-border rounded-lg text-center font-mono text-xl font-bold text-primary focus:outline-none focus:border-primary transition-colors" />
            </div>
            <span class="text-2xl font-bold text-on-surface">:</span>
            <div class="flex items-center gap-2">
              <span class="text-on-surface">Phút:</span>
              <input type="number" min="0" max="59" bind:value={timeMin} 
                class="w-16 h-12 bg-background border border-border rounded-lg text-center font-mono text-xl font-bold text-primary focus:outline-none focus:border-primary transition-colors" />
            </div>
          </div>
          <div class="flex gap-2 mt-4">
            {#each ['05:40', '09:00', '12:00'] as preset}
              <button class="px-3 py-1 text-xs font-bold rounded bg-surface2 text-on-surface-muted hover:bg-border transition-colors"
                on:click={() => { timeHour = preset.split(':')[0]; timeMin = preset.split(':')[1]; }}>
                Reset {preset}
              </button>
            {/each}
          </div>
        {:else}
          <div class="flex items-center gap-4">
            <div class="flex flex-col gap-1">
              <span class="text-xs text-on-surface-muted">Giờ</span>
              <input type="number" min="0" max="99" bind:value={cdHour} 
                class="w-16 h-12 bg-background border border-border rounded-lg text-center font-mono text-xl font-bold text-primary" />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs text-on-surface-muted">Phút</span>
              <input type="number" min="0" max="59" bind:value={cdMin} 
                class="w-16 h-12 bg-background border border-border rounded-lg text-center font-mono text-xl font-bold text-primary" />
            </div>
            <div class="flex flex-col gap-1">
              <span class="text-xs text-on-surface-muted">Giây</span>
              <input type="number" min="0" max="59" bind:value={cdSec} 
                class="w-16 h-12 bg-background border border-border rounded-lg text-center font-mono text-xl font-bold text-primary" />
            </div>
          </div>
        {/if}
      </div>

      <!-- Prompt Input -->
      <div class="bg-surface border border-border rounded-xl p-5 shadow-sm">
        <h3 class="text-sm font-bold text-on-surface-muted mb-4 tracking-wider uppercase">Nội dung Prompt</h3>
        <textarea bind:value={prompt} rows="4" 
          class="w-full bg-background border border-border rounded-lg p-3 text-on-surface focus:outline-none focus:border-primary transition-colors resize-none font-mono text-sm"
          placeholder="Nhập prompt muốn gửi..."></textarea>
        
        <div class="flex flex-wrap gap-2 mt-3">
          {#each ['continue', 'tiếp tục', 'làm tiếp đi', 'go ahead'] as p}
            <button class="px-3 py-1 text-xs font-bold rounded bg-surface2 text-on-surface-muted hover:bg-border transition-colors"
              on:click={() => setPreset(p)}>
              {p}
            </button>
          {/each}
        </div>
      </div>

      <!-- Options & Action -->
      <div class="bg-surface border border-border rounded-xl p-5 shadow-sm flex flex-col gap-4">
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="checkbox" bind:checked={repeat} class="accent-primary w-4 h-4" />
          <span class="text-on-surface font-medium">Lặp lại mỗi ngày (Daily)</span>
        </label>
        
        <button class="w-full py-4 rounded-xl font-bold text-lg bg-primary hover:bg-primary-hover text-white transition-colors shadow-lg shadow-primary/20 flex items-center justify-center gap-2"
          on:click={schedulePrompt}>
          <span class="text-xl">⏰</span>
          Bắt đầu Hẹn giờ
        </button>
      </div>
    </div>

    <!-- Cột phải: Log & Jobs -->
    <div class="flex flex-col gap-6 h-full min-h-0">
      <!-- Active Jobs -->
      <div class="bg-surface border border-border rounded-xl p-5 shadow-sm flex-[0.4] min-h-0 flex flex-col">
        <h3 class="text-sm font-bold text-on-surface-muted mb-4 tracking-wider uppercase flex items-center gap-2">
          <span>Tiến trình đang chạy</span>
          <span class="bg-primary/10 text-primary px-2 py-0.5 rounded-full text-xs">{jobs.length}</span>
        </h3>
        
        <div class="flex-1 overflow-y-auto flex flex-col gap-3">
          {#if jobs.length === 0}
            <div class="flex-1 flex items-center justify-center text-on-surface-muted italic">
              Không có lịch hẹn nào
            </div>
          {:else}
            {#each jobs as job}
              <div class="bg-background border border-border rounded-lg p-3 flex items-center justify-between group">
                <div class="flex flex-col">
                  <span class="text-accent font-mono font-bold text-lg">
                    {new Date(job.target_time).toLocaleTimeString()}
                  </span>
                  <span class="text-xs text-on-surface-muted truncate max-w-[200px]" title={job.prompt}>
                    "{job.prompt}"
                  </span>
                  {#if job.repeat}
                    <span class="text-[10px] text-success uppercase mt-1 font-bold">↻ Lặp lại</span>
                  {/if}
                </div>
                <button class="w-8 h-8 rounded-full bg-error/10 text-error flex items-center justify-center hover:bg-error/20 transition-colors opacity-0 group-hover:opacity-100"
                  on:click={() => cancelJob(job.id)} title="Hủy">
                  ✕
                </button>
              </div>
            {/each}
          {/if}
        </div>
      </div>

      <!-- Logs -->
      <div class="bg-surface border border-border rounded-xl p-5 shadow-sm flex-[0.6] min-h-0 flex flex-col">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-sm font-bold text-on-surface-muted tracking-wider uppercase">Log Hoạt động</h3>
          <button class="text-xs text-on-surface-muted hover:text-on-surface transition-colors"
            on:click={() => schedulerLogs = []}>
            Xóa log
          </button>
        </div>
        
        <div class="flex-1 overflow-y-auto bg-background rounded-lg p-3 border border-border font-mono text-xs flex flex-col gap-1">
          {#each schedulerLogs as log}
            <div class="flex gap-2">
              <span class="text-on-surface-muted shrink-0">[{log.time}]</span>
              <span class="shrink-0 
                {log.level === 'SUCCESS' ? 'text-success' : 
                 log.level === 'ERROR' ? 'text-error' : 
                 log.level === 'WARN' ? 'text-warning' : 'text-accent'}">
                {log.level}
              </span>
              <span class="text-on-surface break-words">{log.msg}</span>
            </div>
          {/each}
        </div>
      </div>
    </div>
  </div>
</div>
