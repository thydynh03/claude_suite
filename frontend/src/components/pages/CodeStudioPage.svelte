<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog, workspaceFolder } from '../../lib/stores/appState';

  let files: string[] = [];
  let fileQuery = '';
  let activeFile = '';
  let fileContent = '';
  let originalContent = '';
  let isDirty = false;
  let isReading = false;
  let isSaving = false;

  // AI Assistant state
  let aiPrompt = '';
  let aiModel = 'claude-sonnet-4-5';
  let aiOutput = '';
  let isAiRunning = false;

  $: filteredFiles = files.filter(f => f.toLowerCase().includes(fileQuery.toLowerCase().trim()));
  $: lineCount = fileContent.split('\n').length;
  $: charCount = fileContent.length;

  onMount(async () => {
    await refreshFiles();
  });

  async function refreshFiles() {
    try {
      files = await AppBindings.ScanWorkspaceFiles();
      if (files && files.length > 0 && !activeFile) {
        openFile(files[0]);
      }
    } catch (e) {
      console.error('Failed to scan workspace files:', e);
    }
  }

  async function openFile(filePath: string) {
    if (isDirty) {
      const confirmLeave = confirm(`File '${activeFile}' có thay đổi chưa lưu. Bạn có muốn bỏ qua?`);
      if (!confirmLeave) return;
    }

    isReading = true;
    activeFile = filePath;
    try {
      const res = await (AppBindings as any).ReadFileContent(filePath);
      fileContent = res || '';
      originalContent = fileContent;
      isDirty = false;
    } catch (e: any) {
      fileContent = `// Lỗi đọc file: ${e?.message || e}`;
    } finally {
      isReading = false;
    }
  }

  function handleContentChange() {
    isDirty = fileContent !== originalContent;
  }

  async function handleSaveFile() {
    if (!activeFile) return;
    isSaving = true;
    try {
      await (AppBindings as any).SaveFileContent(activeFile, fileContent);
      originalContent = fileContent;
      isDirty = false;
      addLog(`Saved file: ${activeFile}`, 'SUCCESS');
    } catch (e: any) {
      addLog(`Failed to save file: ${e?.message || e}`, 'ERROR');
    } finally {
      isSaving = false;
    }
  }

  async function handleAskAI() {
    if (!aiPrompt.trim() || !activeFile) return;
    isAiRunning = true;
    aiOutput = '🤖 AI Agent đang đọc mã nguồn và phân tích...';
    try {
      const fullPrompt = `Tệp tin: ${activeFile}\n\nNội dung mã nguồn hiện tại:\n\`\`\`\n${fileContent}\n\`\`\`\n\nYêu cầu chỉnh sửa / cải tiến:\n${aiPrompt}`;
      const res = await AppBindings.RunQuickCLI(fullPrompt, aiModel, 'Bạn là một Chuyên gia Lập trình Senior Agent IDE.', [activeFile]);
      if (res && res.output) {
        aiOutput = res.output;
      } else if (res && res.error) {
        aiOutput = '❌ Lỗi từ Agent: ' + res.error;
      }
    } catch (e: any) {
      aiOutput = '❌ Lỗi hệ thống: ' + (e?.message || e);
    } finally {
      isAiRunning = false;
    }
  }

  function handleApplyAiToEditor() {
    if (!aiOutput) return;
    // Extract codeblock if present
    const match = aiOutput.match(/```(?:\w+)?\n([\s\S]*?)```/);
    if (match && match[1]) {
      fileContent = match[1];
    } else {
      fileContent = aiOutput;
    }
    handleContentChange();
    addLog(`Đã áp dụng gợi ý AI vào tệp ${activeFile}`, 'INFO');
  }

  function handleKeyDown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault();
      handleSaveFile();
    }
  }
</script>

<div class="h-[calc(100vh-105px)] flex flex-col space-y-4">
  <!-- Header / Action Toolbar -->
  <div class="flex flex-wrap items-center justify-between gap-4 bg-surface-container-low p-3.5 rounded-2xl border border-outline-variant">
    <div class="flex items-center gap-3">
      <div class="w-8 h-8 rounded-xl bg-primary/10 text-primary flex items-center justify-center font-bold">
        <span class="material-symbols-outlined text-lg">code</span>
      </div>
      <div>
        <h1 class="text-base font-bold text-on-surface flex items-center gap-2">
          Agent IDE & Live Code Studio
          {#if isDirty}<span class="text-xs bg-amber-500/20 text-amber-600 border border-amber-500/40 px-2 py-0.5 rounded-full font-bold">* Chưa lưu</span>{/if}
        </h1>
        <p class="text-[11px] text-on-surface-variant font-mono truncate max-w-md">
          {activeFile ? activeFile : 'Vui lòng chọn một tệp tin từ cây thư mục'}
        </p>
      </div>
    </div>

    <div class="flex items-center gap-2">
      <button
        type="button"
        on:click={refreshFiles}
        class="bg-surface-container-highest text-on-surface border border-outline-variant px-3 py-1.5 rounded-xl text-xs font-bold hover:bg-surface-container-high transition-all flex items-center gap-1 cursor-pointer">
        <span class="material-symbols-outlined text-sm">refresh</span> Quét lại Files
      </button>

      <button
        type="button"
        on:click={handleSaveFile}
        disabled={!isDirty || isSaving}
        class="bg-primary text-on-primary px-4 py-1.5 rounded-xl text-xs font-bold hover:opacity-90 transition-all flex items-center gap-1.5 shadow-sm disabled:opacity-40 cursor-pointer">
        <span class="material-symbols-outlined text-sm">save</span>
        {isSaving ? 'Đang lưu...' : 'Lưu File (Ctrl+S)'}
      </button>
    </div>
  </div>

  <!-- 3-Column IDE Layout Grid -->
  <div class="flex-1 grid grid-cols-12 gap-4 overflow-hidden">
    <!-- Left Column: File Explorer Tree (col-span-3) -->
    <div class="col-span-3 bg-surface-container-lowest border border-outline-variant rounded-2xl flex flex-col overflow-hidden shadow-sm">
      <div class="p-3 border-b border-outline-variant flex flex-col gap-2">
        <span class="text-xs font-bold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm text-primary">folder_open</span>
          Workspace Explorer ({files.length})
        </span>
        <div class="relative">
          <span class="material-symbols-outlined absolute left-2.5 top-2 text-xs text-outline">search</span>
          <input
            type="text"
            bind:value={fileQuery}
            placeholder="Lọc tên file..."
            class="w-full bg-surface-container-low border border-outline-variant rounded-lg pl-7 pr-2 py-1 text-xs text-on-surface outline-none focus:border-primary font-mono"
          />
        </div>
      </div>

      <div class="flex-1 overflow-y-auto p-2 space-y-1 font-mono text-xs">
        {#each filteredFiles as file}
          <button
            type="button"
            on:click={() => openFile(file)}
            class="w-full text-left px-2.5 py-1.5 rounded-lg flex items-center gap-2 truncate transition-all cursor-pointer
            {activeFile === file ? 'bg-primary-container text-on-primary-container font-bold shadow-xs' : 'text-on-surface-variant hover:bg-surface-container-low hover:text-on-surface'}"
          >
            <span class="material-symbols-outlined text-sm flex-shrink-0 text-secondary">
              {file.endsWith('.go') ? 'data_object' : file.endsWith('.svelte') || file.endsWith('.html') ? 'web' : file.endsWith('.ts') || file.endsWith('.js') ? 'javascript' : 'description'}
            </span>
            <span class="truncate">{file}</span>
          </button>
        {:else}
          <div class="text-center py-8 text-on-surface-variant text-xs">Không tìm thấy file nào</div>
        {/each}
      </div>
    </div>

    <!-- Center Column: Code Editor (col-span-6) -->
    <div class="col-span-6 bg-surface-container-lowest border border-outline-variant rounded-2xl flex flex-col overflow-hidden shadow-sm relative">
      {#if isReading}
        <div class="absolute inset-0 bg-surface/80 z-20 flex items-center justify-center">
          <span class="material-symbols-outlined animate-spin text-primary text-2xl">sync</span>
        </div>
      {/if}

      <!-- Editor Status Bar -->
      <div class="bg-surface-container-low px-4 py-2 border-b border-outline-variant flex items-center justify-between text-[11px] font-mono text-on-surface-variant">
        <span class="font-bold text-on-surface truncate">{activeFile || 'Chưa mở file'}</span>
        <span>{lineCount} dòng | {charCount} ký tự</span>
      </div>

      <!-- Editor Textarea with Line Numbers -->
      <div class="flex-1 flex overflow-hidden font-mono text-xs">
        <!-- Line Numbers Column -->
        <div class="bg-surface-container-low/60 text-outline px-2 py-3 border-r border-outline-variant select-none text-right font-mono min-w-[40px]">
          {#each Array(lineCount) as _, i}
            <div>{i + 1}</div>
          {/each}
        </div>

        <!-- Code Input Textarea -->
        <textarea
          bind:value={fileContent}
          on:input={handleContentChange}
          on:keydown={handleKeyDown}
          spellcheck="false"
          class="flex-1 bg-surface-container-lowest text-on-surface p-3 outline-none resize-none font-mono text-xs leading-relaxed border-none whitespace-pre"
          placeholder="Mã nguồn sẽ hiển thị ở đây..."
        ></textarea>
      </div>
    </div>

    <!-- Right Column: AI Agent Assistant Panel (col-span-3) -->
    <div class="col-span-3 bg-surface-container-lowest border border-outline-variant rounded-2xl flex flex-col overflow-hidden shadow-sm">
      <div class="p-3 border-b border-outline-variant flex items-center justify-between">
        <span class="text-xs font-bold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm text-primary">auto_awesome</span>
          AI Code Assistant
        </span>
        <select
          bind:value={aiModel}
          class="bg-surface-container-low border border-outline-variant rounded px-2 py-1 text-[11px] font-mono text-on-surface outline-none"
        >
          <option value="claude-sonnet-4-5">Sonnet 4.5</option>
          <option value="claude-opus-4-8">Opus 4.8</option>
          <option value="gemini-3.6-flash-high">Gemini 3.6 Flash</option>
        </select>
      </div>

      <div class="p-3 flex-1 flex flex-col gap-3 overflow-hidden">
        <div>
          <label for="ai-prompt-input" class="text-[11px] font-bold text-on-surface-variant block mb-1">Yêu cầu AI Sửa Code / Refactor:</label>
          <textarea
            id="ai-prompt-input"
            bind:value={aiPrompt}
            placeholder="Ví dụ: Refactor hàm này, thêm kiểm tra lỗi, hoặc viết unit test..."
            class="w-full bg-surface-container-low border border-outline-variant rounded-xl p-2.5 text-xs text-on-surface h-24 outline-none focus:border-primary resize-none font-sans"
          ></textarea>
          <button
            type="button"
            on:click={handleAskAI}
            disabled={isAiRunning || !aiPrompt.trim()}
            class="mt-2 w-full bg-secondary text-on-secondary py-2 rounded-xl text-xs font-bold hover:brightness-110 transition-all flex items-center justify-center gap-1.5 disabled:opacity-50 cursor-pointer">
            <span class="material-symbols-outlined text-sm">{isAiRunning ? 'sync' : 'auto_fix_high'}</span>
            {isAiRunning ? 'Đang phân tích...' : 'Yêu cầu AI Sửa Code'}
          </button>
        </div>

        <div class="flex-1 flex flex-col overflow-hidden border border-outline-variant rounded-xl bg-surface-container-low/30">
          <div class="bg-surface-container-low p-2 text-[11px] font-bold text-on-surface flex items-center justify-between border-b border-outline-variant">
            <span>Phản hồi từ AI Agent:</span>
            {#if aiOutput && !isAiRunning}
              <button
                type="button"
                on:click={handleApplyAiToEditor}
                class="text-[10px] bg-emerald-600 text-white px-2 py-0.5 rounded font-bold hover:opacity-90 transition-all cursor-pointer">
                Áp dụng vào Editor ➔
              </button>
            {/if}
          </div>
          <div class="flex-1 p-2.5 overflow-y-auto font-mono text-[11px] text-on-surface-variant whitespace-pre-wrap leading-relaxed">
            {aiOutput || 'Kết quả gợi ý từ AI Agent sẽ xuất hiện ở đây...'}
          </div>
        </div>
      </div>
    </div>
  </div>
</div>
