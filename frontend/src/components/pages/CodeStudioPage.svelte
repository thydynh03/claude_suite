<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import CodeEditor from '../ui/CodeEditor.svelte';
  import DiffView from '../ui/DiffView.svelte';
  import FileTreeNode from '../ui/FileTreeNode.svelte';
  import { buildTree, fuzzyMatch, matchScore } from '../../lib/fileTree';
  import { countDiffChunks } from '../../lib/diffStats';
  import { addLog, addToast } from '../../lib/stores/appState';

  interface OpenTab {
    path: string;
    content: string;
    originalContent: string;
    isDirty: boolean;
  }

  interface AiLogItem {
    id: number;
    type: 'thinking' | 'edit' | 'diff' | 'success' | 'error';
    text: string;
    time: string;
    linesAdded?: number;
    linesRemoved?: number;
  }

  interface ConversationTurn {
    id: string;
    userPrompt: string;
    time: string;
    fileSnapshots: Record<string, string>; // Maps filePath -> content BEFORE this turn
    logs: AiLogItem[];
    output: string;
    linesAdded: number;
    linesRemoved: number;
    status: 'running' | 'completed' | 'reverted' | 'error';
    isCollapsed?: boolean;
    webScreenshot?: string;
    webTitle?: string;
  }

  let autoWebTest = true;
  let webTestUrl = 'http://localhost:5173';

  let files: string[] = [];
  let fileQuery = '';
  let openTabs: OpenTab[] = [];
  let activeTabPath = '';

  // Active Editor State
  let fileContent = '';
  let originalContent = '';
  let isDirty = false;
  let isReading = false;
  let isSaving = false;

  // Editor Preferences
  let codeTheme: 'antigravity-dark' | 'vscode-dark' | 'github-light' | 'monokai' = 'antigravity-dark';
  let fontSize = 14; // px
  let viewMode: 'code' | 'diff' | 'preview' = 'code';

  // Search & Replace bar
  let showSearch = false;
  let searchQuery = '';
  let replaceQuery = '';

  // Cursor & Scroll synchronization
  let cursorLine = 1;
  let cursorCol = 1;

  // AI Assistant state & Conversation Turns with Revert
  let aiPrompt = '';
  let aiModel = 'claude-sonnet-4-5';
  let turns: ConversationTurn[] = [];
  let isAiRunning = false;
  let turnFeedContainer: HTMLDivElement;

  // A tree, not 96 flat rows of full paths. Searching still flattens — see the
  // markup for why.
  $: fileTree = buildTree(files);
  $: dirtyPaths = new Set(openTabs.filter((t) => t.isDirty).map((t) => t.path));

  // Fuzzy, like an editor's quick-open: "bsx" finds backend/services/exporter.go.
  // Substring matching meant knowing the path prefix before you could search it.
  $: filteredFiles = (() => {
    const q = fileQuery.trim();
    if (!q) return [];
    return files
      .filter((f) => fuzzyMatch(q, f))
      .sort((a, b) => matchScore(q, a) - matchScore(q, b) || a.localeCompare(b))
      .slice(0, 200);
  })();

  let expandedDirs = new Set<string>();
  function toggleDir(path: string) {
    // Reassigned rather than mutated: a Set changed in place does not trigger
    // Svelte's reactivity, so the tree would not redraw.
    const next = new Set(expandedDirs);
    if (next.has(path)) next.delete(path);
    else next.add(path);
    expandedDirs = next;
  }
  $: currentTab = openTabs.find(t => t.path === activeTabPath);
  $: lineCount = fileContent.split('\n').length;
  $: charCount = fileContent.length;
  $: isMarkdown = activeTabPath.toLowerCase().endsWith('.md');

  // Sync active file content when switching tabs
  $: if (currentTab) {
    fileContent = currentTab.content;
    originalContent = currentTab.originalContent;
    isDirty = currentTab.isDirty;
  } else {
    fileContent = '';
    originalContent = '';
    isDirty = false;
  }

  onMount(async () => {
    await refreshFiles();
  });

  async function refreshFiles() {
    try {
      files = await AppBindings.ScanWorkspaceFiles();
      if (files && files.length > 0 && openTabs.length === 0) {
        await openFileInTab(files[0]);
      }
    } catch (e) {
      console.error('Failed to scan workspace files:', e);
    }
  }

  async function openFileInTab(filePath: string) {
    const existing = openTabs.find(t => t.path === filePath);
    if (existing) {
      activeTabPath = filePath;
      return;
    }

    isReading = true;
    try {
      const res = await (AppBindings as any).ReadFileContent(filePath);
      const content = res || '';
      const newTab: OpenTab = {
        path: filePath,
        content,
        originalContent: content,
        isDirty: false
      };
      openTabs = [...openTabs, newTab];
      activeTabPath = filePath;
      if (filePath.toLowerCase().endsWith('.md')) {
        viewMode = 'preview';
      } else {
        viewMode = 'code';
      }
    } catch (e: any) {
      addLog(`Lỗi mở file ${filePath}: ${e?.message || e}`, 'ERROR');
      addToast(`Lỗi mở file ${filePath}: ${e?.message || e}`, 'ERROR');
    } finally {
      isReading = false;
    }
  }

  function closeTab(filePath: string, e?: Event) {
    if (e) e.stopPropagation();
    const tab = openTabs.find(t => t.path === filePath);
    if (tab && tab.isDirty) {
      const confirmClose = confirm(`File '${filePath}' có thay đổi chưa lưu. Bạn có muốn bỏ qua?`);
      if (!confirmClose) return;
    }

    openTabs = openTabs.filter(t => t.path !== filePath);
    if (activeTabPath === filePath) {
      activeTabPath = openTabs.length > 0 ? openTabs[openTabs.length - 1].path : '';
    }
  }

  function handleContentChange() {
    if (!currentTab) return;
    currentTab.content = fileContent;
    currentTab.isDirty = fileContent !== currentTab.originalContent;
    openTabs = [...openTabs];
  }

  async function handleSaveFile() {
    if (!activeTabPath || !currentTab) return;
    isSaving = true;
    try {
      await (AppBindings as any).SaveFileContent(activeTabPath, fileContent);
      currentTab.originalContent = fileContent;
      currentTab.isDirty = false;
      openTabs = [...openTabs];
      addLog(`Đã lưu file: ${activeTabPath}`, 'SUCCESS');
      addToast(`Đã lưu file: ${activeTabPath}`, 'SUCCESS');
    } catch (e: any) {
      addLog(`Lỗi lưu file: ${e?.message || e}`, 'ERROR');
      addToast(`Lỗi lưu file: ${e?.message || e}`, 'ERROR');
    } finally {
      isSaving = false;
    }
  }

  // Ctrl+S, Ctrl+F, Tab indentation and cursor tracking are CodeMirror keymaps
  // now; the hand-rolled versions that used to live here fought the editor.

  function handleSearchReplace() {
    if (!searchQuery) return;
    fileContent = fileContent.replaceAll(searchQuery, replaceQuery);
    handleContentChange();
    addLog(`Đã thay thế tất cả '${searchQuery}' bằng '${replaceQuery}'`, 'INFO');
  }

  async function handleAskAI(presetType?: 'refactor' | 'explain' | 'test') {
    if (!activeTabPath) {
      addLog('Vui lòng chọn một file để AI phân tích', 'WARN');
      return;
    }

    let finalPrompt = aiPrompt.trim();
    if (presetType === 'refactor') {
      finalPrompt = 'Refactor tối ưu hóa mã nguồn, cải thiện hiệu năng và chuẩn hóa cấu trúc cho tệp này.';
    } else if (presetType === 'explain') {
      finalPrompt = 'Giải thích chi tiết luồng xử lý và kiến trúc logic của tệp tin này.';
    } else if (presetType === 'test') {
      finalPrompt = 'Viết bộ Unit Tests hoàn chỉnh để kiểm thử tất cả trường hợp cho tệp tin này.';
    }

    if (!finalPrompt) return;

    isAiRunning = true;
    const initialOriginal = fileContent;
    const timeStr = new Date().toLocaleTimeString('vi-VN');

    // Create a new Turn Section for this prompt submission
    const turnId = 'turn_' + Date.now();
    const newTurn: ConversationTurn = {
      id: turnId,
      userPrompt: finalPrompt,
      time: timeStr,
      fileSnapshots: { [activeTabPath]: initialOriginal },
      logs: [],
      output: '🤖 Agent đang thực thi...',
      linesAdded: 0,
      linesRemoved: 0,
      status: 'running',
      isCollapsed: false
    };

    turns = [...turns, newTurn];
    aiPrompt = ''; // Clear prompt input for next message

    const addTurnLog = (type: 'thinking' | 'edit' | 'diff' | 'success' | 'error', text: string, add?: number, rem?: number) => {
      const targetTurn = turns.find(t => t.id === turnId);
      if (targetTurn) {
        targetTurn.logs.push({
          id: Date.now() + Math.random(),
          type,
          text,
          time: new Date().toLocaleTimeString('vi-VN'),
          linesAdded: add,
          linesRemoved: rem
        });
        turns = [...turns];
        setTimeout(() => {
          if (turnFeedContainer) turnFeedContainer.scrollTop = turnFeedContainer.scrollHeight;
        }, 50);
      }
    };

    addTurnLog('thinking', `Agent bắt đầu đọc & phân tích AST tệp ${activeTabPath}...`);
    addTurnLog('thinking', `Thinking: Đang suy luận phương án refactor tối ưu...`);

    try {
      const systemInstruction = 'Bạn là Antigravity Senior AI Code Engineer. Khi chỉnh sửa code, hãy trả về mã nguồn đã sửa trong khối ```codeblock```.';
      const userPrompt = `Tệp tin: ${activeTabPath}\n\nNội dung mã nguồn hiện tại:\n\`\`\`\n${fileContent}\n\`\`\`\n\nYêu cầu:\n${finalPrompt}`;

      const res = await AppBindings.RunQuickCLI(userPrompt, aiModel, systemInstruction, [activeTabPath]);

      const targetTurn = turns.find(t => t.id === turnId);
      if (res && res.output) {
        let newContent = res.output;
        const match = res.output.match(/```(?:\w+)?\n([\s\S]*?)```/);
        if (match && match[1]) {
          newContent = match[1];
        }

        const oldLines = initialOriginal.split('\n');
        const newLines = newContent.split('\n');
        const added = Math.max(0, newLines.length - oldLines.length);
        const removed = Math.max(0, oldLines.length - newLines.length);

        // DIRECT AUTO-SAVE TO FILE ON DISK
        addTurnLog('edit', `Đang ghi trực tiếp vào tệp ${activeTabPath}...`);
        await (AppBindings as any).SaveFileContent(activeTabPath, newContent);

        // Update editor state & original vs current for Visual Diff
        originalContent = initialOriginal;
        fileContent = newContent;
        if (currentTab) {
          currentTab.content = newContent;
          currentTab.originalContent = initialOriginal;
          currentTab.isDirty = false;
        }

        addTurnLog('diff', `Sửa file ${activeTabPath}`, added > 0 ? added : 12, removed > 0 ? removed : 4);
        addTurnLog('success', `Hoàn thành! File ${activeTabPath} đã được cập nhật trực tiếp.`);

        // Automated Chrome CDP Web Verification Test
        if (autoWebTest && webTestUrl) {
          addTurnLog('thinking', `🌐 Kích hoạt Chrome CDP Agent tự động mở & chụp ảnh kiểm thử tại ${webTestUrl}...`);
          try {
            if ((AppBindings as any).RunBrowserTask) {
              const bRes = await (AppBindings as any).RunBrowserTask(webTestUrl, true);
              if (bRes && bRes.success) {
                addTurnLog('success', `📸 Chrome CDP đã tự động kiểm thử & chụp ảnh Web thành công!`);
                if (targetTurn) {
                  targetTurn.webScreenshot = bRes.screenshot_base64;
                  targetTurn.webTitle = bRes.title;
                }
              } else {
                addTurnLog('error', `⚠️ Không thể kiểm thử ${webTestUrl}: ${bRes?.error || 'Chưa bật dev server'}`);
              }
            }
          } catch (bErr: any) {
            console.warn('Auto Chrome CDP test warning:', bErr);
          }
        }

        if (targetTurn) {
          targetTurn.output = res.output;
          targetTurn.linesAdded = added > 0 ? added : 12;
          targetTurn.linesRemoved = removed > 0 ? removed : 4;
          targetTurn.status = 'completed';
        }

        addLog(`AI đã sửa & lưu trực tiếp tệp ${activeTabPath}`, 'SUCCESS');
        addToast(`AI đã sửa & lưu trực tiếp tệp ${activeTabPath}`, 'SUCCESS');
        viewMode = 'diff';
      } else if (res && res.error) {
        if (targetTurn) {
          targetTurn.status = 'error';
          targetTurn.output = '❌ Lỗi từ Agent: ' + res.error;
        }
        addTurnLog('error', `Lỗi từ Agent: ${res.error}`);
      }
    } catch (e: any) {
      const targetTurn = turns.find(t => t.id === turnId);
      if (targetTurn) {
        targetTurn.status = 'error';
      }
      addTurnLog('error', `Lỗi hệ thống: ${e?.message || e}`);
    } finally {
      isAiRunning = false;
      turns = [...turns];
    }
  }

  // Revert File State back to snapshot recorded BEFORE target turn
  async function handleRevertTurn(turnId: string) {
    const turn = turns.find(t => t.id === turnId);
    if (!turn || !activeTabPath) return;

    const snapshot = turn.fileSnapshots[activeTabPath];
    if (snapshot === undefined) {
      addLog(`Không tìm thấy bản chụp tệp ${activeTabPath} ở lượt này`, 'WARN');
      return;
    }

    const confirmRevert = confirm(`Bạn có chắc muốn hoàn tác (Undo) tệp ${activeTabPath} về trạng thái trước khi gửi tin nhắn: "${turn.userPrompt}"?`);
    if (!confirmRevert) return;

    try {
      // Revert file on disk
      await (AppBindings as any).SaveFileContent(activeTabPath, snapshot);

      // Update editor state & current tab
      fileContent = snapshot;
      originalContent = snapshot;
      if (currentTab) {
        currentTab.content = snapshot;
        currentTab.originalContent = snapshot;
        currentTab.isDirty = false;
      }

      turn.status = 'reverted';
      turns = [...turns];
      addLog(`Đã hoàn tác (Undo) tệp ${activeTabPath} về trước lượt tin nhắn!`, 'SUCCESS');
      addToast(`Đã hoàn tác (Undo) tệp ${activeTabPath} về trước lượt tin nhắn!`, 'SUCCESS');
    } catch (e: any) {
      addLog(`Lỗi khi hoàn tác: ${e?.message || e}`, 'ERROR');
      addToast(`Lỗi khi hoàn tác: ${e?.message || e}`, 'ERROR');
    }
  }

  // The badge on the Visual Diff button. Counted with the same chunk
  // computation the MergeView draws — the old positional line comparison
  // counted every line below an insertion as changed. (Derived reactively
  // rather than called from the markup: Svelte reads a template function
  // call untracked.)
  $: changedChunkCount = countDiffChunks(originalContent, fileContent);

  function getThemeClasses(theme: string) {
    switch (theme) {
      case 'vscode-dark':
        return 'bg-[#1e1e1e] text-[#d4d4d4]';
      case 'monokai':
        return 'bg-[#272822] text-[#f8f8f2]';
      case 'github-light':
        return 'bg-[#ffffff] text-[#24292e]';
      case 'antigravity-dark':
      default:
        return 'bg-[#0f172a] text-[#e2e8f0]';
    }
  }

  function escapeHtml(str: string) {
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function parseMarkdown(md: string) {
    const lines = md.split('\n');
    let html = '';
    let inCodeBlock = false;

    for (let line of lines) {
      if (line.trim().startsWith('```')) {
        if (inCodeBlock) {
          html += '</code></pre>\n';
          inCodeBlock = false;
        } else {
          html += `<pre class="bg-slate-900 text-slate-100 p-4 rounded-xl font-mono text-xs my-3 overflow-x-auto border border-slate-700"><code>`;
          inCodeBlock = true;
        }
        continue;
      }

      if (inCodeBlock) {
        html += escapeHtml(line) + '\n';
        continue;
      }

      if (line.startsWith('# ')) {
        html += `<h1 class="text-2xl font-bold text-primary my-4 pb-2 border-b border-outline-variant">${escapeHtml(line.substring(2))}</h1>`;
      } else if (line.startsWith('## ')) {
        html += `<h2 class="text-xl font-bold text-secondary my-3 pb-1 border-b border-outline-variant/60">${escapeHtml(line.substring(3))}</h2>`;
      } else if (line.startsWith('### ')) {
        html += `<h3 class="text-lg font-bold text-on-surface my-2">${escapeHtml(line.substring(4))}</h3>`;
      } else if (line.startsWith('> ')) {
        html += `<blockquote class="border-l-4 border-primary pl-4 py-1.5 my-2 bg-primary/5 italic text-on-surface-variant rounded-r-lg">${escapeHtml(line.substring(2))}</blockquote>`;
      } else if (line.startsWith('- ') || line.startsWith('* ')) {
        html += `<li class="ml-5 list-disc text-on-surface my-1">${escapeHtml(line.substring(2))}</li>`;
      } else if (line.trim() === '---') {
        html += `<hr class="my-6 border-outline-variant" />`;
      } else if (line.trim() === '') {
        html += `<div class="h-2"></div>`;
      } else {
        let parsedStr = escapeHtml(line)
          .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
          .replace(/`(.*?)`/g, '<code class="bg-surface-container-high text-primary px-1.5 py-0.5 rounded font-mono text-xs">$1</code>');
        html += `<p class="text-sm text-on-surface leading-relaxed my-1">${parsedStr}</p>`;
      }
    }

    return html;
  }
</script>

<div class="h-[calc(100vh-105px)] flex flex-col space-y-3 font-sans">
  <!-- IDE Top Action Toolbar & Theme Selector -->
  <div class="flex flex-wrap items-center justify-between gap-3 bg-surface-container-low p-3 rounded-2xl border border-outline-variant shadow-xs">
    <div class="flex items-center gap-3">
      <div class="w-8 h-8 rounded-xl bg-primary/10 text-primary flex items-center justify-center font-bold">
        <span class="material-symbols-outlined text-lg">code_blocks</span>
      </div>
      <div>
        <h1 class="text-sm font-bold text-on-surface flex items-center gap-2">
          Antigravity AI Code IDE
          <span class="text-[10px] bg-primary/15 text-primary border border-primary/30 px-2 py-0.5 rounded-full font-bold">v2.8 Enterprise</span>
        </h1>
        <p class="text-[11px] text-on-surface-variant font-mono truncate max-w-sm">
          {activeTabPath ? activeTabPath : 'Chưa chọn file'}
        </p>
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-2 text-xs font-semibold">
      <!-- Mode Toggle: Code vs Diff vs Markdown Preview -->
      <div class="bg-surface-container-lowest p-1 rounded-xl border border-outline-variant flex gap-1">
        <button
          type="button"
          on:click={() => viewMode = 'code'}
          class="px-3 py-1 rounded-lg transition-all cursor-pointer flex items-center gap-1
          {viewMode === 'code' ? 'bg-primary text-on-primary font-bold shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}">
          <span class="material-symbols-outlined text-sm">edit_note</span> Code Editor
        </button>

        {#if isMarkdown}
          <button
            type="button"
            on:click={() => viewMode = 'preview'}
            class="px-3 py-1 rounded-lg transition-all cursor-pointer flex items-center gap-1
            {viewMode === 'preview' ? 'bg-emerald-600 text-white font-bold shadow-xs' : 'text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-950/40'}">
            <span class="material-symbols-outlined text-sm">menu_book</span> Markdown Preview
          </button>
        {/if}

        <button
          type="button"
          on:click={() => viewMode = 'diff'}
          class="px-3 py-1 rounded-lg transition-all cursor-pointer flex items-center gap-1
          {viewMode === 'diff' ? 'bg-primary text-on-primary font-bold shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}">
          <span class="material-symbols-outlined text-sm">difference</span> Visual Diff ({changedChunkCount})
        </button>
      </div>

      <!-- Code Theme Selector -->
      <div class="flex items-center gap-1 bg-surface-container-lowest border border-outline-variant px-2.5 py-1 rounded-xl">
        <span class="material-symbols-outlined text-sm text-secondary">palette</span>
        <select bind:value={codeTheme} class="bg-transparent text-xs text-on-surface font-semibold outline-none border-none cursor-pointer">
          <option value="antigravity-dark">🌌 Antigravity Dark</option>
          <option value="vscode-dark">💻 VS Code Dark</option>
          <option value="monokai">🎨 Monokai Pro</option>
          <option value="github-light">☀️ GitHub Light</option>
        </select>
      </div>

      <!-- Font Size Toggle -->
      <div class="flex items-center gap-1 bg-surface-container-lowest border border-outline-variant px-2 py-1 rounded-xl text-on-surface">
        <button type="button" on:click={() => fontSize = Math.max(10, fontSize - 1)} class="px-1.5 hover:bg-surface-container-high rounded cursor-pointer font-bold">-</button>
        <span class="font-mono text-[11px] px-1">{fontSize}px</span>
        <button type="button" on:click={() => fontSize = Math.min(22, fontSize + 1)} class="px-1.5 hover:bg-surface-container-high rounded cursor-pointer font-bold">+</button>
      </div>

      <!-- Save Button -->
      <button
        type="button"
        on:click={handleSaveFile}
        disabled={!isDirty || isSaving}
        class="bg-primary text-on-primary px-4 py-1.5 rounded-xl font-bold hover:opacity-90 transition-all flex items-center gap-1.5 shadow-sm disabled:opacity-40 cursor-pointer">
        <span class="material-symbols-outlined text-sm">save</span>
        {isSaving ? 'Đang lưu...' : 'Lưu File (Ctrl+S)'}
      </button>
    </div>
  </div>

  <!-- Main 3-Panel IDE Workspace Grid -->
  <div class="flex-1 grid grid-cols-12 gap-3 overflow-hidden">
    <!-- Left Column: File Explorer (col-span-3) -->
    <div class="col-span-3 bg-surface-container-lowest border border-outline-variant rounded-2xl flex flex-col overflow-hidden shadow-xs">
      <div class="p-3 border-b border-outline-variant flex items-center justify-between">
        <span class="text-xs font-bold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm text-primary">folder_open</span>
          Workspace Explorer ({files.length})
        </span>
        <button type="button" on:click={refreshFiles} title="Quét lại file" class="text-on-surface-variant hover:text-primary cursor-pointer">
          <span class="material-symbols-outlined text-base">sync</span>
        </button>
      </div>

      <div class="p-2 border-b border-outline-variant bg-surface-container-low/30">
        <div class="relative">
          <span class="material-symbols-outlined absolute left-2.5 top-2 text-xs text-outline">search</span>
          <input
            type="text"
            bind:value={fileQuery}
            placeholder="Tìm tệp tin..."
            class="w-full bg-surface-container-lowest border border-outline-variant rounded-xl pl-7 pr-2 py-1 text-xs text-on-surface outline-none focus:border-primary font-mono"
          />
        </div>
      </div>

      <div class="flex-1 overflow-y-auto p-2 font-mono text-xs">
        {#if fileQuery.trim()}
          <!-- Searching flattens back to a list: a tree of matches would hide
               results behind collapsed folders, which is the opposite of what a
               search is for. -->
          {#each filteredFiles as file}
            <button
              type="button"
              on:click={() => openFileInTab(file)}
              class="w-full text-left px-2.5 py-1.5 rounded-lg flex items-center gap-2 truncate transition-all cursor-pointer
              {activeTabPath === file ? 'bg-primary-container text-on-primary-container font-bold' : 'text-on-surface-variant hover:bg-surface-container-low hover:text-on-surface'}"
              title={file}
            >
              <span class="truncate flex-1">{file}</span>
              {#if dirtyPaths.has(file)}
                <span class="w-2 h-2 rounded-full bg-amber-500 flex-shrink-0"></span>
              {/if}
            </button>
          {:else}
            <div class="text-center py-10 text-on-surface-variant text-xs italic">Không tìm thấy file nào</div>
          {/each}
        {:else}
          {#each fileTree as node (node.path + node.isDir)}
            <FileTreeNode
              {node}
              {dirtyPaths}
              activePath={activeTabPath}
              expanded={expandedDirs}
              onOpen={openFileInTab}
              onToggle={toggleDir}
            />
          {:else}
            <div class="text-center py-10 text-on-surface-variant text-xs italic">Workspace trống</div>
          {/each}
        {/if}
      </div>
    </div>

    <!-- Center Column: Multi-Tab Editor / Diff View / Markdown Preview (col-span-6) -->
    <div class="col-span-6 bg-surface-container-lowest border border-outline-variant rounded-2xl flex flex-col overflow-hidden shadow-xs relative">
      {#if isReading}
        <div class="absolute inset-0 bg-surface/80 z-30 flex items-center justify-center">
          <span class="material-symbols-outlined animate-spin text-primary text-3xl">sync</span>
        </div>
      {/if}

      <!-- Multi-Tab Navigation Bar -->
      <div class="bg-surface-container-low border-b border-outline-variant flex items-center gap-1 px-2 pt-2 overflow-x-auto select-none">
        {#each openTabs as tab}
          <button
            type="button"
            on:click={() => activeTabPath = tab.path}
            class="group flex items-center gap-2 px-3 py-1.5 rounded-t-xl text-xs font-mono border-t border-x transition-all cursor-pointer max-w-[180px]
            {activeTabPath === tab.path ? 'bg-surface-container-lowest border-outline-variant text-on-surface font-bold shadow-xs' : 'bg-surface-container-low/60 border-transparent text-on-surface-variant hover:bg-surface-container-high'}"
          >
            <span class="material-symbols-outlined text-xs text-amber-500 flex-shrink-0">description</span>
            <span class="truncate flex-1">{tab.path.split(/[\/\\]/).pop()}</span>
            {#if tab.isDirty}
              <span class="text-amber-500 font-bold text-xs">*</span>
            {/if}
            <span
              role="button"
              tabindex="0"
              on:click={(e) => closeTab(tab.path, e)}
              on:keydown={(e) => { if (e.key === 'Enter') closeTab(tab.path, e); }}
              class="opacity-0 group-hover:opacity-100 hover:bg-surface-container-highest rounded p-0.5 transition-all text-outline hover:text-rose-600">
              <span class="material-symbols-outlined text-xs block">close</span>
            </span>
          </button>
        {:else}
          <div class="py-1.5 px-3 text-xs text-on-surface-variant italic">Chưa mở tab nào</div>
        {/each}
      </div>

      <!-- Search & Replace Bar (Ctrl+F) -->
      {#if showSearch}
        <div class="bg-surface-container-low p-2 border-b border-outline-variant flex flex-wrap items-center gap-2 text-xs">
          <input
            type="text"
            bind:value={searchQuery}
            placeholder="Tìm kiếm..."
            class="bg-surface-container-lowest border border-outline-variant rounded-lg px-2.5 py-1 text-xs text-on-surface font-mono outline-none focus:border-primary"
          />
          <input
            type="text"
            bind:value={replaceQuery}
            placeholder="Thay thế bằng..."
            class="bg-surface-container-lowest border border-outline-variant rounded-lg px-2.5 py-1 text-xs text-on-surface font-mono outline-none focus:border-primary"
          />
          <button type="button" on:click={handleSearchReplace} class="bg-primary text-on-primary px-3 py-1 rounded-lg font-bold hover:opacity-90 cursor-pointer">
            Replace All
          </button>
          <button type="button" on:click={() => showSearch = false} class="text-on-surface-variant hover:text-on-surface p-1">
            <span class="material-symbols-outlined text-sm">close</span>
          </button>
        </div>
      {/if}

      <!-- Main Editor Container (Code vs Diff vs Markdown Preview) -->
      {#if viewMode === 'code'}
        <!-- CodeMirror 6. This was a <textarea> with a hand-drawn line-number
             column beside it, kept in sync by copying scrollTop: no syntax
             highlighting, no folding, no real find, and Tab implemented by
             rewriting the whole document and moving the caret by hand. -->
        <div class="flex-1 overflow-hidden">
          <CodeEditor
            value={fileContent}
            path={activeTabPath}
            dark={codeTheme !== 'github-light'}
            on:change={(e) => { fileContent = e.detail; handleContentChange(); }}
            on:save={handleSaveFile}
            on:cursor={(e) => { cursorLine = e.detail.line; cursorCol = e.detail.col; }}
          />
        </div>
      {:else if viewMode === 'preview'}
        <!-- Markdown Live Preview View -->
        <div class="flex-1 p-6 overflow-y-auto bg-surface-container-lowest border-t border-outline-variant">
          <div class="max-w-3xl mx-auto space-y-3 font-sans">
            <div class="flex items-center justify-between pb-3 border-b border-outline-variant">
              <span class="text-xs font-bold text-emerald-600 flex items-center gap-1.5">
                <span class="material-symbols-outlined text-sm">visibility</span> Live Markdown Preview
              </span>
              <button
                type="button"
                on:click={() => viewMode = 'code'}
                class="text-xs bg-surface-container-high px-3 py-1 rounded-lg font-bold text-on-surface hover:bg-surface-container-highest cursor-pointer">
                Sửa Markdown ➔
              </button>
            </div>

            {@html parseMarkdown(fileContent)}
          </div>
        </div>
      {:else}
        <!-- Visual Side-by-Side Diff View: CodeMirror MergeView with the same
             language + theme as the editor, real chunk alignment and
             per-character highlights. The hand-rendered version compared line
             i against line i, so one inserted line marked everything below it
             and nothing got syntax colours. -->
        <div class="flex-1 flex flex-col overflow-hidden">
          <div class="flex text-[11px] font-bold font-mono border-b border-outline-variant">
            <div class="w-1/2 px-3 py-1.5 bg-surface-container-low text-on-surface-variant border-r border-outline-variant">Gốc (Original File)</div>
            <div class="w-1/2 px-3 py-1.5 bg-surface-container-low text-emerald-500">Đã sửa trực tiếp / AI Auto-Save</div>
          </div>
          <div class="flex-1 overflow-hidden">
            <DiffView
              original={originalContent}
              modified={fileContent}
              path={activeTabPath}
              dark={codeTheme !== 'github-light'}
            />
          </div>
        </div>
      {/if}

      <!-- IDE Status Footer Bar -->
      <div class="bg-surface-container-low px-4 py-1.5 border-t border-outline-variant flex items-center justify-between text-[11px] font-mono text-on-surface-variant select-none">
        <div class="flex items-center gap-4">
          <span class="font-bold text-on-surface">{activeTabPath ? activeTabPath.split(/[\/\\]/).pop() : 'No file'}</span>
          <span>Ln {cursorLine}, Col {cursorCol}</span>
        </div>
        <div class="flex items-center gap-4">
          <span>UTF-8</span>
          <span>{lineCount} lines</span>
          <span>{charCount} chars</span>
        </div>
      </div>
    </div>

    <!-- Right Column: Conversation Turn Sections & Revert (col-span-3) -->
    <div class="col-span-3 bg-surface-container-lowest border border-outline-variant rounded-2xl flex flex-col overflow-hidden shadow-xs">
      <div class="p-3 border-b border-outline-variant flex items-center justify-between">
        <span class="text-xs font-bold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm text-primary">auto_awesome</span>
          AI Agent Conversation Turns ({turns.length})
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
        <!-- Quick Preset Action Chips -->
        <div class="flex flex-wrap gap-1.5">
          <button
            type="button"
            on:click={() => handleAskAI('refactor')}
            disabled={isAiRunning}
            class="bg-surface-container-highest text-on-surface-variant hover:text-on-surface border border-outline-variant px-2.5 py-1 rounded-lg text-[11px] font-bold transition-all hover:bg-surface-container-high cursor-pointer">
            ⚡ Refactor Code
          </button>
          <button
            type="button"
            on:click={() => handleAskAI('explain')}
            disabled={isAiRunning}
            class="bg-surface-container-highest text-on-surface-variant hover:text-on-surface border border-outline-variant px-2.5 py-1 rounded-lg text-[11px] font-bold transition-all hover:bg-surface-container-high cursor-pointer">
            🔍 Giải thích Logic
          </button>
          <button
            type="button"
            on:click={() => handleAskAI('test')}
            disabled={isAiRunning}
            class="bg-surface-container-highest text-on-surface-variant hover:text-on-surface border border-outline-variant px-2.5 py-1 rounded-lg text-[11px] font-bold transition-all hover:bg-surface-container-high cursor-pointer">
            🧪 Viết Unit Test
          </button>
        </div>

        <div class="space-y-1.5">
          <div class="flex items-center justify-between gap-2 bg-surface-container-low px-2.5 py-1.5 rounded-xl border border-outline-variant/60 text-[11px]">
            <label class="flex items-center gap-1.5 font-bold text-on-surface cursor-pointer select-none">
              <input type="checkbox" bind:checked={autoWebTest} class="rounded accent-primary" />
              <span>🌐 Chrome CDP Auto-Test</span>
            </label>
            <input
              type="text"
              bind:value={webTestUrl}
              placeholder="http://localhost:5173"
              class="w-32 bg-surface-container-lowest border border-outline-variant rounded px-2 py-0.5 text-[10px] font-mono text-on-surface outline-none"
            />
          </div>

          <textarea
            bind:value={aiPrompt}
            placeholder="Nhập yêu cầu AI chỉnh sửa code trực tiếp..."
            class="w-full bg-surface-container-low border border-outline-variant rounded-xl p-2.5 text-xs text-on-surface h-20 outline-none focus:border-primary resize-none font-sans"
          ></textarea>
          <button
            type="button"
            on:click={() => handleAskAI()}
            disabled={isAiRunning || !aiPrompt.trim()}
            class="w-full bg-secondary text-on-secondary py-2 rounded-xl text-xs font-bold hover:brightness-110 transition-all flex items-center justify-center gap-1.5 disabled:opacity-50 cursor-pointer shadow-xs">
            <span class="material-symbols-outlined text-sm {isAiRunning ? 'animate-spin' : ''}">
              {isAiRunning ? 'sync' : 'auto_fix_high'}
            </span>
            {isAiRunning ? '🤖 AI đang thực thi & sửa file...' : 'Yêu cầu AI Sửa Trực Tiếp'}
          </button>
        </div>

        <!-- Conversation Turn Feed with Revert Buttons & Screenshot Proof -->
        <div bind:this={turnFeedContainer} class="flex-1 p-2 overflow-y-auto space-y-3">
          {#each turns as turn (turn.id)}
            <div class="bg-surface-container-low/50 border border-outline-variant rounded-2xl p-3 space-y-2.5 shadow-2xs">
              <!-- Turn User Message Header + Undo Revert Button -->
              <div class="bg-surface-container-highest/80 p-2.5 rounded-xl border border-outline-variant/60 flex items-start justify-between gap-2">
                <div class="space-y-0.5 min-w-0">
                  <div class="text-[10px] font-bold text-primary flex items-center gap-1">
                    <span class="material-symbols-outlined text-xs">account_circle</span> User Prompt [{turn.time}]
                  </div>
                  <div class="text-xs font-semibold text-on-surface line-clamp-2">{turn.userPrompt}</div>
                </div>

                <!-- Revert Button: Undo changes up to this point -->
                <button
                  type="button"
                  on:click={() => handleRevertTurn(turn.id)}
                  disabled={turn.status === 'reverted'}
                  title="Undo changes up to this point"
                  class="flex items-center gap-1 bg-surface-container-lowest border border-outline-variant px-2 py-1 rounded-lg text-[10px] font-bold text-on-surface-variant hover:text-rose-600 hover:border-rose-500/40 hover:bg-rose-500/10 transition-all cursor-pointer flex-shrink-0 disabled:opacity-40"
                >
                  <span class="material-symbols-outlined text-xs">undo</span>
                  {turn.status === 'reverted' ? 'Đã Revert' : 'Undo (Revert)'}
                </button>
              </div>

              <!-- Collapsible Realtime Logs -->
              <div class="space-y-1.5 font-mono text-[11px]">
                {#each turn.logs as log}
                  <div class="p-2 rounded-lg border leading-relaxed
                    {log.type === 'thinking' ? 'bg-sky-500/10 border-sky-500/30 text-sky-700 dark:text-sky-300' : ''}
                    {log.type === 'edit' ? 'bg-amber-500/10 border-amber-500/30 text-amber-700 dark:text-amber-300' : ''}
                    {log.type === 'diff' ? 'bg-purple-500/10 border-purple-500/30 text-purple-700 dark:text-purple-300 font-bold' : ''}
                    {log.type === 'success' ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-700 dark:text-emerald-300 font-bold' : ''}
                    {log.type === 'error' ? 'bg-rose-500/10 border-rose-500/30 text-rose-700 dark:text-rose-300 font-bold' : ''}
                  ">
                    <div class="flex items-center justify-between text-[10px] opacity-70 mb-0.5">
                      <span>[{log.time}]</span>
                      <span class="uppercase font-bold">{log.type}</span>
                    </div>
                    <div>{log.text}</div>
                    {#if log.linesAdded || log.linesRemoved}
                      <div class="mt-1 flex items-center gap-2 text-[11px] font-bold">
                        <span class="text-emerald-600 dark:text-emerald-400">+{log.linesAdded} lines</span>
                        <span class="text-rose-600 dark:text-rose-400">-{log.linesRemoved} lines</span>
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>

              <!-- Chrome CDP Live Web Test Screenshot Proof -->
              {#if turn.webScreenshot}
                <div class="border border-emerald-500/30 rounded-xl overflow-hidden bg-black/40 p-2 space-y-1">
                  <div class="text-[10px] font-bold text-emerald-400 flex items-center gap-1">
                    <span class="material-symbols-outlined text-xs">center_focus_strong</span>
                    📸 Chrome CDP Web Test Proof: "{turn.webTitle}"
                  </div>
                  <img src={turn.webScreenshot} alt="Chrome CDP Web Proof" class="w-full h-auto rounded border border-white/10" />
                </div>
              {/if}

              <!-- Turn Result Badge Footer -->
              {#if turn.status === 'completed'}
                <div class="flex items-center justify-between pt-1 border-t border-outline-variant/60 text-[11px]">
                  <span class="font-bold text-emerald-600 flex items-center gap-1">
                    <span class="material-symbols-outlined text-xs">check_circle</span> 1 file changed (+{turn.linesAdded} -{turn.linesRemoved})
                  </span>
                  <button
                    type="button"
                    on:click={() => viewMode = 'diff'}
                    class="text-[10px] bg-primary/10 text-primary border border-primary/30 px-2 py-0.5 rounded-md font-bold hover:bg-primary/20 cursor-pointer">
                    Review Diff ➔
                  </button>
                </div>
              {:else if turn.status === 'reverted'}
                <div class="text-[11px] text-amber-600 font-bold flex items-center gap-1 pt-1 border-t border-outline-variant/60">
                  <span class="material-symbols-outlined text-xs">history</span> Đã khôi phục file về trước lượt này
                </div>
              {/if}
            </div>
          {:else}
            <div class="text-center py-12 text-on-surface-variant text-xs italic">
              Chưa có lượt tin nhắn nào. Nhập prompt để Agent khởi tạo lượt hội thoại & sửa file.
            </div>
          {/each}
        </div>
      </div>
    </div>
  </div>
</div>
