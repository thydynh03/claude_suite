<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { Agent } from '../../lib/types';

  /**
   * Full agent editor.
   *
   * Creating an agent used to be two inline text boxes — name and role — with
   * every other field written by the code: provider always claude_cli, model
   * always claude-sonnet-4-5, icon always smart_toy, and a system prompt
   * generated as `You are {name}, {role}.`. Those are the fields that decide
   * which engine runs the task and how the agent behaves, so the two things a
   * user could set were the two that mattered least.
   */

  export let open = false;
  /** Editing an existing agent when set; creating a new one when null. */
  export let agent: Agent | null = null;
  /** Existing names, for the duplicate check. */
  export let existingNames: string[] = [];

  const dispatch = createEventDispatcher<{ save: any; close: void }>();

  const PROVIDERS = [
    { value: 'claude_cli', label: 'Claude CLI' },
    { value: 'anti_cli', label: 'Antigravity / Gemini CLI' },
  ];

  // Kept next to the provider they belong to: routing is by model name, so a
  // Gemini model under claude_cli sends the task to the wrong engine.
  const MODELS: Record<string, { value: string; label: string }[]> = {
    claude_cli: [
      { value: 'claude-opus-4-8', label: 'Claude Opus 4.8' },
      { value: 'claude-sonnet-4-5', label: 'Claude Sonnet 4.5' },
      { value: 'claude-haiku-4-5', label: 'Claude Haiku 4.5' },
    ],
    anti_cli: [
      { value: 'gemini-3.6-flash-high', label: 'Gemini 3.6 Flash (High)' },
      { value: 'gemini-3.6-flash-medium', label: 'Gemini 3.6 Flash (Medium)' },
      { value: 'gemini-3.1-pro-high', label: 'Gemini 3.1 Pro (High)' },
      { value: 'claude-sonnet-4.6-thinking', label: 'Claude Sonnet 4.6 (Thinking)' },
    ],
  };

  const ICONS = [
    'smart_toy', 'engineering', 'architecture', 'code', 'terminal',
    'bug_report', 'science', 'design_services', 'cloud', 'security',
    'analytics', 'psychology', 'support_agent', 'manage_accounts',
  ];

  // Starting points, not the finished prompt: a persona the user never edits is
  // still better than `You are {name}, {role}.`, which told the model nothing.
  const TEMPLATES: Record<string, string> = {
    'Tech Lead & Architect':
      'Bạn là Tech Lead. Thiết kế kiến trúc hệ thống, database schema và API contract trước khi viết code. Nêu rõ đánh đổi của mỗi lựa chọn. Không tự ý mở rộng phạm vi ngoài yêu cầu.',
    'Business Analyst':
      'Bạn là Business Analyst. Làm rõ yêu cầu, viết đặc tả và tiêu chí nghiệm thu. Hỏi lại khi yêu cầu mơ hồ thay vì đoán.',
    'Back-end Developer':
      'Bạn là lập trình viên back-end. Viết API, xử lý dữ liệu và logic nghiệp vụ. Luôn xử lý lỗi tường minh, không nuốt lỗi. Viết test cho phần bạn thêm.',
    'Front-end Developer':
      'Bạn là lập trình viên front-end. Viết giao diện theo đúng hệ thống design token sẵn có của dự án. Mọi hành động người dùng bấm phải có phản hồi nhìn thấy được.',
    'QA/QC Specialist':
      'Bạn là kỹ sư kiểm thử. Viết test chạy được và thất bại đúng khi có lỗi. Trước khi khẳng định đã sửa, hãy revert bản sửa và xác nhận test đỏ.',
    'DevOps Engineer':
      'Bạn là kỹ sư DevOps. Cấu hình build, CI/CD và triển khai. Mọi bước phải kiểm chứng được bằng lệnh, không dựa vào giả định.',
  };

  let name = '';
  let role = '';
  let provider = 'claude_cli';
  let model = 'claude-sonnet-4-5';
  let system = '';
  let icon = 'smart_toy';
  let tokenLimit = 0;
  let notes = '';
  let error = '';

  let lastLoadedId: string | null = null;

  // Reset when the dialog opens, so a cancelled edit does not leak into the next
  // one. Keyed on the agent id rather than on `open` alone: re-running on every
  // reactive tick would wipe what the user is typing.
  $: if (open) {
    const id = (agent as any)?.agent_id || '__new__';
    if (id !== lastLoadedId) {
      lastLoadedId = id;
      name = agent?.name || '';
      role = (agent as any)?.role || '';
      provider = (agent as any)?.provider || 'claude_cli';
      model = (agent as any)?.model || MODELS[provider][0].value;
      system = (agent as any)?.system || '';
      icon = (agent as any)?.icon || 'smart_toy';
      tokenLimit = (agent as any)?.token_limit || 0;
      notes = (agent as any)?.notes || '';
      error = '';
    }
  } else {
    lastLoadedId = null;
  }

  // Switching engine must switch the model too, or the agent ends up pointed at a
  // model its provider cannot run.
  function onProviderChange(next: string) {
    provider = next;
    if (!MODELS[next].some((m) => m.value === model)) {
      model = MODELS[next][0].value;
    }
  }

  function applyTemplate(key: string) {
    role = key;
    system = TEMPLATES[key];
  }

  $: isEditing = !!(agent as any)?.agent_id;
  $: duplicate =
    !isEditing &&
    name.trim() !== '' &&
    existingNames.some((n) => n.toLowerCase() === name.trim().toLowerCase());

  function submit() {
    error = '';
    if (!name.trim()) {
      error = 'Cần đặt tên cho agent.';
      return;
    }
    if (duplicate) {
      error = `Đã có agent tên "${name.trim()}".`;
      return;
    }

    dispatch('save', {
      agent_id: (agent as any)?.agent_id || '',
      name: name.trim(),
      role: role.trim() || 'Custom Agent',
      provider,
      model,
      // Falls back to something usable rather than to an empty prompt, but the
      // user's own text always wins.
      system: system.trim() || `Bạn là ${name.trim()}, ${role.trim() || 'một AI agent chuyên biệt'}.`,
      icon,
      notes: notes.trim(),
      token_limit: Number(tokenLimit) || 0,
      status: (agent as any)?.status || 'idle',
      tasks_done: (agent as any)?.tasks_done || 0,
      tokens_used: (agent as any)?.tokens_used || 0,
      token_remaining: (agent as any)?.token_remaining || 0,
    });
  }
</script>

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div
    class="fixed inset-0 z-[80] bg-black/50 backdrop-blur-sm flex items-center justify-center p-4"
    on:click={() => dispatch('close')}
  >
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div
      class="bg-surface-container-lowest border border-outline-variant rounded-2xl shadow-xl w-full max-w-2xl max-h-[88vh] overflow-y-auto"
      on:click|stopPropagation
    >
      <div class="flex items-center justify-between px-5 py-4 border-b border-outline-variant sticky top-0 bg-surface-container-lowest z-10">
        <h3 class="font-bold text-base text-on-surface flex items-center gap-2">
          <span class="material-symbols-outlined text-primary">{icon}</span>
          {isEditing ? `Sửa agent: ${agent?.name}` : 'Tạo agent mới'}
        </h3>
        <button
          type="button"
          on:click={() => dispatch('close')}
          class="text-on-surface-variant hover:text-on-surface cursor-pointer"
        >
          <span class="material-symbols-outlined">close</span>
        </button>
      </div>

      <div class="p-5 space-y-5">
        <!-- Identity -->
        <section class="space-y-3">
          <h4 class="text-[11px] font-bold uppercase tracking-wider text-on-surface-variant">Danh tính</h4>
          <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label for="agent-name" class="text-[10px] font-bold text-on-surface block mb-1">Tên agent</label>
              <input
                id="agent-name"
                bind:value={name}
                placeholder="VD: Back-end Developer"
                class="w-full bg-surface-container-low border rounded-lg px-3 py-2 text-xs outline-none focus:border-primary
                {duplicate ? 'border-red-500' : 'border-outline-variant'}"
              />
              {#if duplicate}
                <p class="text-[10px] text-red-500 mt-1">Tên này đã tồn tại.</p>
              {/if}
            </div>
            <div>
              <label for="agent-role" class="text-[10px] font-bold text-on-surface block mb-1">Vai trò</label>
              <input
                id="agent-role"
                bind:value={role}
                placeholder="VD: Lập trình API và database"
                class="w-full bg-surface-container-low border border-outline-variant rounded-lg px-3 py-2 text-xs outline-none focus:border-primary"
              />
            </div>
          </div>

          <div>
            <span class="text-[10px] font-bold text-on-surface block mb-1.5">Biểu tượng</span>
            <div class="flex flex-wrap gap-1.5">
              {#each ICONS as ic}
                <button
                  type="button"
                  on:click={() => (icon = ic)}
                  title={ic}
                  class="w-9 h-9 rounded-lg border flex items-center justify-center transition-all cursor-pointer
                  {icon === ic
                    ? 'border-primary bg-primary/10 text-primary'
                    : 'border-outline-variant text-on-surface-variant hover:bg-surface-container-high'}"
                >
                  <span class="material-symbols-outlined text-lg">{ic}</span>
                </button>
              {/each}
            </div>
          </div>
        </section>

        <!-- Engine -->
        <section class="space-y-3 pt-1 border-t border-outline-variant">
          <h4 class="text-[11px] font-bold uppercase tracking-wider text-on-surface-variant pt-3">Bộ máy</h4>
          <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label for="agent-provider" class="text-[10px] font-bold text-on-surface block mb-1">Provider</label>
              <select
                id="agent-provider"
                value={provider}
                on:change={(e) => onProviderChange((e.target as HTMLSelectElement).value)}
                class="w-full bg-surface-container-low border border-outline-variant rounded-lg px-3 py-2 text-xs cursor-pointer"
              >
                {#each PROVIDERS as p}
                  <option value={p.value}>{p.label}</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="agent-model" class="text-[10px] font-bold text-on-surface block mb-1">Model</label>
              <select
                id="agent-model"
                bind:value={model}
                class="w-full bg-surface-container-low border border-outline-variant rounded-lg px-3 py-2 text-xs cursor-pointer"
              >
                {#each MODELS[provider] as m}
                  <option value={m.value}>{m.label}</option>
                {/each}
              </select>
            </div>
            <div>
              <label for="agent-token-limit" class="text-[10px] font-bold text-on-surface block mb-1">
                Giới hạn token (0 = mặc định)
              </label>
              <input
                id="agent-token-limit"
                type="number"
                min="0"
                bind:value={tokenLimit}
                class="w-full bg-surface-container-low border border-outline-variant rounded-lg px-3 py-2 text-xs font-mono outline-none focus:border-primary"
              />
            </div>
          </div>
        </section>

        <!-- Behaviour -->
        <section class="space-y-3 pt-1 border-t border-outline-variant">
          <div class="flex items-center justify-between pt-3">
            <h4 class="text-[11px] font-bold uppercase tracking-wider text-on-surface-variant">Hành vi</h4>
            <div class="flex flex-wrap gap-1 justify-end">
              {#each Object.keys(TEMPLATES) as key}
                <button
                  type="button"
                  on:click={() => applyTemplate(key)}
                  class="px-2 py-0.5 rounded-md text-[10px] font-bold border border-outline-variant text-on-surface-variant hover:bg-surface-container-high cursor-pointer"
                >
                  {key}
                </button>
              {/each}
            </div>
          </div>
          <div>
            <label for="agent-system" class="text-[10px] font-bold text-on-surface block mb-1">
              System prompt (persona)
            </label>
            <textarea
              id="agent-system"
              bind:value={system}
              rows="6"
              placeholder="Mô tả cách agent này làm việc, giới hạn của nó, và cái nó không được tự ý làm."
              class="w-full bg-surface-container-low border border-outline-variant rounded-lg px-3 py-2 text-xs leading-relaxed outline-none focus:border-primary resize-y"
            ></textarea>
            <p class="text-[10px] text-on-surface-variant mt-1">
              Bỏ trống thì app tự sinh một câu mô tả tối thiểu — nên viết rõ hơn để agent làm đúng việc.
            </p>
          </div>
          <div>
            <label for="agent-notes" class="text-[10px] font-bold text-on-surface block mb-1">Ghi chú</label>
            <input
              id="agent-notes"
              bind:value={notes}
              placeholder="Ghi chú nội bộ, không gửi cho model"
              class="w-full bg-surface-container-low border border-outline-variant rounded-lg px-3 py-2 text-xs outline-none focus:border-primary"
            />
          </div>
        </section>

        {#if error}
          <p class="text-xs text-red-500 font-bold">{error}</p>
        {/if}
      </div>

      <div class="flex items-center justify-end gap-2 px-5 py-4 border-t border-outline-variant sticky bottom-0 bg-surface-container-lowest">
        <button
          type="button"
          on:click={() => dispatch('close')}
          class="px-4 py-2 rounded-lg text-xs font-bold text-on-surface-variant hover:bg-surface-container-high cursor-pointer"
        >
          Huỷ
        </button>
        <button
          type="button"
          on:click={submit}
          disabled={!name.trim() || duplicate}
          class="px-4 py-2 rounded-lg text-xs font-bold bg-primary text-on-primary disabled:opacity-40 cursor-pointer"
        >
          {isEditing ? 'Lưu thay đổi' : 'Tạo agent'}
        </button>
      </div>
    </div>
  </div>
{/if}
