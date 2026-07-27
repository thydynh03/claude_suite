<script lang="ts">
  import { createEventDispatcher, onMount, onDestroy } from 'svelte';
  import { fade, fly, scale } from 'svelte/transition';
  import { cubicOut, quintOut } from 'svelte/easing';

  export let open = false;

  const dispatch = createEventDispatcher();

  type Step = {
    badge: string;
    icon: string;
    title: string;
    subtitle: string;
    points: { icon: string; text: string }[];
  };

  const steps: Step[] = [
    {
      badge: 'Chào mừng',
      icon: 'auto_awesome',
      title: 'Claude Suite — Trung tâm điều phối AI Agent',
      subtitle:
        'Mô tả mục tiêu dự án, AI sẽ tự phân rã thành tasks, giao cho các sub-agent chạy song song, tự kiểm thử và tự sửa lỗi.',
      points: [
        { icon: 'groups', text: 'Nhiều sub-agent (Claude & Gemini) chạy song song theo biểu đồ phụ thuộc' },
        { icon: 'terminal', text: 'Xem log realtime từng agent: tool calls, file đang sửa, kết quả' },
        { icon: 'verified', text: 'Tự verify build và kiểm thử E2E trước khi báo hoàn thành' },
      ],
    },
    {
      badge: 'Bước 1',
      icon: 'folder_open',
      title: 'Chọn thư mục Workspace',
      subtitle:
        'Đây là thư mục dự án mà các agent được phép đọc và chỉnh sửa trực tiếp. Mọi thay đổi đều nằm trong workspace này.',
      points: [
        { icon: 'drive_folder_upload', text: 'Bấm chọn workspace ở thanh trên, hoặc dùng Ctrl+K → “Chọn thư mục Workspace”' },
        { icon: 'history', text: 'App tự lưu workspace gần nhất và mở lại ở lần chạy sau' },
        { icon: 'backup', text: 'Trước mỗi task, app tự tạo git snapshot để bạn luôn khôi phục được' },
      ],
    },
    {
      badge: 'Bước 2',
      icon: 'auto_mode',
      title: 'AI Plan Builder — biến ý tưởng thành kế hoạch',
      subtitle:
        'Vào Task Board → AI Plan Builder, nhập mục tiêu dự án. AI sẽ brainstorm và tạo ra danh sách task chi tiết theo vai trò.',
      points: [
        { icon: 'label', text: 'Task được gắn vai trò [BA] [ARCH] [CODE] [QA] [DEVOPS] và độ ưu tiên' },
        { icon: 'checklist', text: 'Mỗi task có ACTION, REQUIREMENTS, TARGET FILES, EXPECTED OUTPUT rõ ràng' },
        { icon: 'swap_horiz', text: 'Chọn được model lập kế hoạch (Claude Opus/Sonnet hoặc Gemini)' },
      ],
    },
    {
      badge: 'Bước 3',
      icon: 'view_kanban',
      title: 'Kanban & Orchestrator — chạy thật',
      subtitle:
        'Bấm Execute Plan để orchestrator quét backlog và giao việc. Kéo thả task giữa các cột, mở task để theo dõi agent làm việc.',
      points: [
        { icon: 'groups', text: 'Chọn số agent chạy song song (1–6); task chỉ chạy khi phụ thuộc đã xong' },
        { icon: 'stop_circle', text: 'Dừng hoặc chạy lại từng task riêng lẻ bất cứ lúc nào' },
        { icon: 'monitor_heart', text: 'Task Inspector: log realtime, tool calls, Git diff, ảnh chụp E2E' },
      ],
    },
    {
      badge: 'Bước 4',
      icon: 'travel_explore',
      title: 'Kiểm thử E2E tự động & vòng tự sửa lỗi',
      subtitle:
        'Task test web sẽ tự chạy trên Chrome. Nếu thất bại, app tự tạo task sửa lỗi rồi kiểm thử lại — khép kín, không cần bạn can thiệp.',
      points: [
        { icon: 'rule', text: 'Đặt [E2E] ở tiêu đề, thêm URL và [EXPECT: nội dung] để app tự kiểm chứng' },
        { icon: 'bug_report', text: 'Bắt lỗi console và assertion sai, nạp thẳng vào prompt cho agent sửa' },
        { icon: 'build', text: 'Bật “Verify build” để chạy go build / npm build trước khi báo Done' },
      ],
    },
    {
      badge: 'Bước 5',
      icon: 'account_tree',
      title: 'Source Control — commit như một lập trình viên',
      subtitle:
        'Tab Source Control cho bạn xem thay đổi, commit, chuyển nhánh và hoàn tác — ngay trong app.',
      points: [
        { icon: 'auto_awesome', text: 'Nút “AI viết commit”: AI đọc diff và soạn commit message chuẩn' },
        { icon: 'difference', text: 'Diff tô màu, lịch sử commit kèm nút Revert từng commit' },
        { icon: 'terminal', text: 'Ô nhập lệnh git an toàn (các lệnh nguy hiểm bị chặn)' },
      ],
    },
    {
      badge: 'Tính năng phụ',
      icon: 'apps',
      title: 'Những thứ hữu ích khác',
      subtitle: 'Các công cụ bổ trợ giúp bạn làm việc nhanh và chủ động hơn.',
      points: [
        { icon: 'keyboard_command_key', text: 'Command Palette: nhấn Ctrl+K để đi tới mọi nơi và chạy lệnh nhanh' },
        { icon: 'schedule', text: 'Scheduler: hẹn giờ chạy prompt tự động, lặp lại theo lịch' },
        { icon: 'vpn_key', text: 'Key Pool đa tài khoản: tự xoay vòng khi gặp giới hạn 429' },
        { icon: 'webhook', text: 'Webhook: nhận task từ ngoài & bắn thông báo khi task xong' },
        { icon: 'apartment', text: 'Virtual Office 3D và Browser Agent để quan sát trực quan' },
      ],
    },
  ];

  let index = 0;
  let direction = 1;

  $: step = steps[index];
  $: isFirst = index === 0;
  $: isLast = index === steps.length - 1;
  $: progress = ((index + 1) / steps.length) * 100;

  function next() {
    if (isLast) {
      finish();
      return;
    }
    direction = 1;
    index += 1;
  }

  function prev() {
    if (isFirst) return;
    direction = -1;
    index -= 1;
  }

  function goTo(i: number) {
    if (i === index) return;
    direction = i > index ? 1 : -1;
    index = i;
  }

  function finish() {
    dispatch('close');
  }

  function onKeydown(e: KeyboardEvent) {
    if (!open) return;
    if (e.key === 'Escape') { e.preventDefault(); finish(); }
    else if (e.key === 'ArrowRight' || e.key === 'Enter') { e.preventDefault(); next(); }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); prev(); }
  }

  onMount(() => window.addEventListener('keydown', onKeydown));
  onDestroy(() => window.removeEventListener('keydown', onKeydown));
</script>

{#if open}
  <!-- Backdrop: plain scrim. This used to carry three drifting gradient orbs
       (#4f7cff / #22d3ee / #a855f7), an animated grid veil, a shimmer sweeping
       the dialog edge and a pulsing halo — colours outside the token system and
       continuous decorative motion on the app's first screen. -->
  <div
    class="fixed inset-0 z-[400] flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
    transition:fade={{ duration: 200 }}
    role="presentation"
    on:click|self={finish}
  >
    <div
      class="relative w-full max-w-2xl rounded-xl border border-outline-variant bg-surface shadow-lg overflow-hidden"
      in:scale={{ duration: 260, start: 0.97, opacity: 0, easing: quintOut }}
      out:scale={{ duration: 160, start: 0.98, opacity: 0, easing: cubicOut }}
      role="dialog"
      aria-modal="true"
      aria-label="Hướng dẫn sử dụng"
    >
      <!-- Header -->
      <div class="relative flex items-start justify-between gap-4 px-7 pt-6 pb-4">
        <div class="flex items-center gap-3 min-w-0">
          <span class="material-symbols-outlined text-lg text-on-surface-variant">{step.icon}</span>
          <div class="min-w-0">
            <span class="text-xs font-medium text-primary">
              {step.badge}
            </span>
            <p class="mt-0.5 text-[11px] text-on-surface-variant">
              {index + 1} / {steps.length}
            </p>
          </div>
        </div>
        <button
          type="button"
          on:click={finish}
          title="Bỏ qua (Esc)"
          class="flex-shrink-0 rounded-lg p-1.5 text-on-surface-variant hover:bg-surface-container-high hover:text-on-surface transition-colors cursor-pointer"
        >
          <span class="material-symbols-outlined text-lg">close</span>
        </button>
      </div>

      <!-- Body -->
      <div class="relative px-7 pb-2 min-h-[268px]">
        {#key index}
          <div
            in:fly={{ x: direction * 26, y: 0, duration: 340, delay: 80, easing: quintOut, opacity: 0 }}
            out:fly={{ x: direction * -18, duration: 160, easing: cubicOut, opacity: 0 }}
            class="space-y-4"
          >
            <h2 class="text-lg font-semibold leading-snug text-on-surface">{step.title}</h2>
            <p class="text-sm leading-relaxed text-on-surface-variant">{step.subtitle}</p>

            <ul class="space-y-2.5 pt-1">
              {#each step.points as p, i}
                <li
                  class="flex items-start gap-3 rounded-xl border border-outline-variant bg-surface-container-low px-3.5 py-2.5"
                  in:fly={{ y: 10, duration: 320, delay: 150 + i * 70, easing: quintOut, opacity: 0 }}
                >
                  <span class="material-symbols-outlined mt-0.5 text-base text-on-surface-variant">{p.icon}</span>
                  <span class="text-[13px] leading-relaxed text-on-surface">{p.text}</span>
                </li>
              {/each}
            </ul>
          </div>
        {/key}
      </div>

      <!-- Progress rail -->
      <div class="px-7 pt-4">
        <div class="h-1 w-full overflow-hidden rounded-full bg-surface-container-highest">
          <div
            class="h-full rounded-full bg-primary transition-all duration-500 ease-out"
            style="width: {progress}%"
          ></div>
        </div>
      </div>

      <!-- Footer -->
      <div class="flex items-center justify-between gap-3 px-7 py-5">
        <div class="flex items-center gap-1.5">
          {#each steps as _, i}
            <button
              type="button"
              on:click={() => goTo(i)}
              aria-label={`Bước ${i + 1}`}
              class="h-1.5 rounded-full transition-all duration-300 cursor-pointer
              {i === index ? 'w-6 bg-primary' : 'w-1.5 bg-outline-variant hover:bg-outline'}"
            ></button>
          {/each}
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            on:click={finish}
            class="rounded-lg px-3 py-1.5 text-xs font-medium text-on-surface-variant hover:bg-surface-container-high hover:text-on-surface transition-colors cursor-pointer"
          >
            Bỏ qua
          </button>
          {#if !isFirst}
            <button
              type="button"
              on:click={prev}
              class="flex items-center gap-1.5 rounded-lg border border-outline-variant px-3 py-1.5 text-xs font-medium text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer"
            >
              <span class="material-symbols-outlined text-sm">arrow_back</span> Quay lại
            </button>
          {/if}
          <button
            type="button"
            on:click={next}
            class="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-on-primary hover:opacity-90 transition-opacity cursor-pointer"
          >
            {isLast ? 'Bắt đầu ngay' : 'Tiếp tục'}
            <span class="material-symbols-outlined text-sm">{isLast ? 'rocket_launch' : 'arrow_forward'}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}

