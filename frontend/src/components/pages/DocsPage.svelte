<script lang="ts">
  import { renderMarkdown } from '../../lib/markdown';
  import { activeTab } from '../../lib/stores/appState';

  let searchQuery = '';
  let selectedCategory: 'all' | 'quickstart' | 'agents' | 'kanban' | 'cli' | 'mcp' = 'all';

  const docSections = [
    {
      id: 'quickstart',
      title: 'Hướng dẫn nhanh và thiết lập Workspace',
      category: 'quickstart',
      icon: 'rocket_launch',
      content: `
### 1. Khởi tạo Dự án & Chọn Workspace
Khi mở ứng dụng, hãy chọn thư mục làm việc (Workspace Folder) bằng nút **Select Workspace** trên thanh Top Bar.

### 2. Phân rã Kế hoạch bằng AI
Vào tab **Task Board & Planning**, nhập yêu cầu dự án vào ô *Project Requirement* và nhấn **Decompose Requirement**.
AI (Claude 4.5 Sonnet hoặc Gemini 3.6 Flash) sẽ tự động phân tích và tạo danh sách các công việc cụ thể.

### 3. Thực thi Kế hoạch
Nhấn **Execute Plan** hoặc **Start Orchestrator**. Các Corporate Agent sẽ nhận công việc theo đúng chuyên môn và tiến hành lập trình tự động.
`
    },
    {
      id: 'agents',
      title: 'Quản lý agent roles và persona tuỳ biến',
      category: 'agents',
      icon: 'smart_toy',
      content: `
### Danh sách Agent Mặc định:
- **Tech Lead & Architect**: Đảm nhận thiết kế kiến trúc, database schema và giao công việc.
- **Business Analyst (BA)**: Phân tích yêu cầu bài toán, lập tài liệu spec.
- **Back-end Developer**: Lập trình API, xử lý database và logic backend (Go, Python, Node).
- **Front-end Developer**: Thiết kế giao diện UI/UX và lập trình Svelte / React.
- **QA/QC Specialist**: Viết unit test, chạy thử nghiệm kiểm thử tự động.
- **DevOps Engineer**: Cấu hình Docker, CI/CD và deployment.

### Chỉnh sửa System Prompt (Persona):
Vào tab **Studio & Settings** -> **Agents Registry**, chọn agent và chỉnh sửa *System Prompt / Persona* rồi nhấn **Lưu Agent** để lưu trực tiếp vào SQLite database.
`
    },
    {
      id: 'kanban',
      title: 'Kanban Board, ưu tiên và phụ thuộc (depends on)',
      category: 'kanban',
      icon: 'assignment',
      content: `
### Các Trạng Thái Task:
- **Backlog**: Task mới được tạo, chờ xếp lịch.
- **Queued**: Task sẵn sàng chờ Agent tiếp nhận.
- **Running**: Agent đang tiến hành xử lý trực tiếp.
- **Done**: Xử lý hoàn tất thành công.
- **Failed**: Thất bại (hỗ trợ tự động Retry & Fallback).

### Phụ thuộc Task (Ràng buộc Depends On):
Task con chứa tham số \`depends_on\` sẽ được giữ ở trạng thái chờ cho đến khi tất cả các Task cha hoàn tất (\`done\`).

### Xem chi tiết kết quả AI:
Click trực tiếp vào thẻ Task trên bảng Kanban để mở cửa sổ modal xem toàn bộ câu lệnh Prompt và kết quả thực thi chi tiết từ AI Agent.
`
    },
    {
      id: 'cli',
      title: 'CLI engines: Claude CLI và Antigravity CLI',
      category: 'cli',
      icon: 'terminal',
      content: `
### Hỗ trợ 2 Động cơ AI CLI chính:
1. **Claude CLI (\`claude\`)**: Tích hợp với Claude Opus 4.8, Sonnet 4.5, Haiku 4.5.
2. **Antigravity CLI (\`agy\` / \`antigravity\`)**: Tích hợp với Gemini 3.6 Flash, Gemini 3.1 Pro, Sonnet 4.6 Thinking.

### Smart Fallback khi hết Hạn ngạch (Quota 429):
Nếu Agent gặp lỗi hết Quota/Token limit 429 trên Claude, hệ thống sẽ tự động chuyển hướng (Fallback) sang Gemini 3.6 Flash trên Antigravity CLI để công việc không bị gián đoạn.
`
    },
    {
      id: 'mcp',
      title: 'Webhook và MCP (Model Context Protocol) API',
      category: 'mcp',
      icon: 'hub',
      content: `
### Webhook Server Nhận Task Tự Động:
Bật Webhook Server tại cổng \`9090\` trong **Settings -> Integrations**.
Gửi HTTP POST request dạng JSON:
\`\`\`json
{
  "title": "[CODE] Fix authentication bug",
  "prompt": "Fix JWT token expiration logic in auth.go",
  "priority": "high"
}
\`\`\`
Task sẽ tự động được thêm vào Backlog của hệ thống.
`
    }
  ];

  $: filteredDocs = docSections.filter(doc => {
    if (selectedCategory !== 'all' && doc.category !== selectedCategory) return false;
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      return doc.title.toLowerCase().includes(q) || doc.content.toLowerCase().includes(q);
    }
    return true;
  });
</script>

<div class="space-y-6 max-w-7xl mx-auto pb-12">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-lg font-semibold flex items-center gap-2 text-on-surface">
        <span class="material-symbols-outlined text-lg text-on-surface-variant">description</span>
        Tài liệu hướng dẫn
      </h1>
      <p class="text-on-surface-variant text-sm mt-0.5">Hướng dẫn chi tiết cách dùng Agent Center: orchestrator, CLI engine và các API tích hợp.</p>
    </div>

    <button
      type="button"
      on:click={() => activeTab.set('support')}
      class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium flex items-center gap-1.5 hover:bg-surface-container-high transition-colors cursor-pointer">
      <span class="material-symbols-outlined text-sm">help</span> Cần hỗ trợ kỹ thuật?
    </button>
  </div>

  <!-- Search & Category Filter. These controls used to sit inside their own
       filled, bordered panel — a box around a box; whitespace separates them. -->
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div class="relative flex-1 max-w-md">
      <span class="material-symbols-outlined absolute left-3 top-2 text-sm text-outline">search</span>
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Tìm kiếm tài liệu (ví dụ: Agent, Fallback, Webhook, Kanban)..."
        class="w-full bg-surface-container-lowest border border-outline-variant rounded-lg pl-9 pr-3 py-1.5 text-xs text-on-surface outline-none focus:border-primary"
      />
    </div>

    <div class="flex items-center gap-1 flex-wrap">
      {#each [
        { id: 'all', label: 'Tất cả' },
        { id: 'quickstart', label: 'Quick Start' },
        { id: 'agents', label: 'Agents' },
        { id: 'kanban', label: 'Kanban & Tasks' },
        { id: 'cli', label: 'CLI Engines' },
        { id: 'mcp', label: 'Webhook & MCP' }
      ] as cat}
        <button
          type="button"
          on:click={() => selectedCategory = cat.id as any}
          class="px-3 py-1.5 text-xs font-medium rounded-lg transition-colors cursor-pointer
          {selectedCategory === cat.id ? 'bg-secondary-container text-on-secondary-container' : 'text-on-surface-variant hover:bg-surface-container-high'}"
        >
          {cat.label}
        </button>
      {/each}
    </div>
  </div>

  <!-- Docs Articles Grid -->
  <div class="space-y-6">
    {#each filteredDocs as doc}
      <div class="bg-surface border border-outline-variant rounded-xl p-6 space-y-4">
        <div class="flex items-center gap-2.5 pb-3 border-b border-outline-variant">
          <span class="material-symbols-outlined text-base text-on-surface-variant">{doc.icon}</span>
          <h2 class="text-sm font-semibold text-on-surface">{doc.title}</h2>
        </div>

        <!-- Rendered, not printed. This block used to be whitespace-pre-wrap
             around raw markdown, so readers saw the ### and ** characters
             themselves. renderMarkdown escapes the source before formatting it. -->
        <div class="text-xs text-on-surface-variant leading-relaxed font-sans">
          {@html renderMarkdown(doc.content)}
        </div>
      </div>
    {:else}
      <div class="text-center py-12 rounded-xl border border-outline-variant">
        <p class="text-sm font-medium text-on-surface">Không tìm thấy tài liệu phù hợp</p>
        <p class="text-xs text-on-surface-variant mt-1">Vui lòng nhập từ khoá khác hoặc chuyển danh mục.</p>
      </div>
    {/each}
  </div>

  <!-- Keyboard shortcuts. Six individually bordered+filled boxes inside an
       already bordered card is a double border on every row; a plain list of
       label/key pairs reads faster. -->
  <div class="bg-surface border border-outline-variant rounded-xl p-6 space-y-3">
    <h3 class="text-sm font-semibold text-on-surface flex items-center gap-2">
      <span class="material-symbols-outlined text-base text-on-surface-variant">keyboard</span> Phím tắt
    </h3>
    <div class="grid grid-cols-1 md:grid-cols-3 gap-x-8">
      {#each [
        { label: 'Chuyển sang AI Cockpit', key: 'Ctrl + 1' },
        { label: 'Chuyển sang Task Board', key: 'Ctrl + 2' },
        { label: 'Chuyển sang Code Studio', key: 'Ctrl + 3' },
        { label: 'Chuyển sang Source Control', key: 'Ctrl + 4' },
        { label: 'Chuyển sang Scheduler', key: 'Ctrl + 5' },
        { label: 'Bảng lệnh (Command Palette)', key: 'Ctrl + K' }
      ] as sc}
        <div class="flex justify-between items-center gap-3 py-2 border-b border-outline-variant last:border-b-0">
          <span class="text-xs text-on-surface-variant">{sc.label}</span>
          <kbd class="text-[11px] font-mono text-on-surface-variant bg-surface-container-high border border-outline-variant px-1.5 py-0.5 rounded-lg whitespace-nowrap">{sc.key}</kbd>
        </div>
      {/each}
    </div>
  </div>
</div>
