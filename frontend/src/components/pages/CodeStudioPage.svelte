<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import CodeEditor from '../ui/CodeEditor.svelte';
  import DiffView from '../ui/DiffView.svelte';
  import FileTreeNode from '../ui/FileTreeNode.svelte';
  import ModelSelect from '../ui/ModelSelect.svelte';
  import { buildTree, fuzzyMatch, matchScore } from '../../lib/fileTree';
  import { countDiffChunks } from '../../lib/diffStats';
  import { addLog, addToast } from '../../lib/stores/appState';
  import { theme } from '../../lib/stores/theme';

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

  // Editor Preferences. The editor follows the app's light/dark theme by
  // default — a light shell used to render a dark editor, splitting the window
  // into two colour worlds. The dropdown is an explicit override, nothing more.
  let codeTheme: 'auto' | 'dark' | 'light' = 'auto';
  $: editorDark = codeTheme === 'auto' ? $theme === 'dark' : codeTheme === 'dark';
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

  // largestCodeBlock returns the biggest fenced block in a response. The
  // first block is the wrong one to take: models often lead with a small
  // "before/after" excerpt and put the full file later — grabbing the first
  // is how a 727-line stylesheet became its own 12-line snippet on disk.
  function largestCodeBlock(s: string): string | null {
    const re = /```[^\n]*\n([\s\S]*?)```/g;
    let best: string | null = null;
    for (let m; (m = re.exec(s)); ) {
      if (best === null || m[1].length > best.length) best = m[1];
    }
    return best;
  }

  // snippetSuspicion answers "is this a fragment rather than the whole
  // file?" before the content is allowed to overwrite the real file.
  // Returns a human-readable reason, or null when the content looks whole.
  function snippetSuspicion(candidate: string, original: string): string | null {
    const oldN = original.split('\n').length;
    const newN = candidate.split('\n').length;
    // A refactor can legitimately shrink a file, but halving a large one is
    // almost always an excerpt.
    if (oldN >= 40 && newN < oldN * 0.5) {
      return `AI chỉ trả về ${newN} dòng cho tệp gốc ${oldN} dòng`;
    }
    // Ellipsis-as-omission: a bare "..." line, or a comment starting with
    // "..." — as opposed to legitimate spread syntax inside an expression.
    if (newN < oldN && /^\s*(\.\.\.|…)\s*$|^\s*(\/\/|#|\/\*|<!--)\s*(\.\.\.|…)/m.test(candidate)) {
      return 'nội dung chứa dấu "..." thay cho phần bị lược bỏ';
    }
    return null;
  }

  async function handleAskAI(presetType?: 'refactor' | 'explain' | 'test') {
    if (!activeTabPath) {
      addLog('Vui lòng chọn một file để AI phân tích', 'WARN');
      return;
    }

    // Two very different contracts: an edit turn may overwrite the file, an
    // analysis turn must never touch it. "Giải thích logic" used to run
    // through the edit path — any code excerpt in the explanation replaced
    // the whole file on disk. "Viết Unit Test" likewise overwrote the SOURCE
    // file with test code, so both are analysis now.
    const mode: 'edit' | 'analyze' =
      presetType === 'explain' || presetType === 'test' ? 'analyze' : 'edit';

    let finalPrompt = aiPrompt.trim();
    if (presetType === 'refactor') {
      finalPrompt = 'Refactor tối ưu hóa mã nguồn, cải thiện hiệu năng và chuẩn hóa cấu trúc cho tệp này.';
    } else if (presetType === 'explain') {
      finalPrompt = 'Giải thích chi tiết luồng xử lý và kiến trúc logic của tệp tin này.';
    } else if (presetType === 'test') {
      finalPrompt = 'Viết bộ Unit Tests hoàn chỉnh để kiểm thử tất cả trường hợp cho tệp tin này. Ghi rõ tên tệp test nên đặt là gì.';
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
      output: 'Agent đang thực thi...',
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

    addTurnLog('thinking', `Agent bắt đầu đọc & phân tích tệp ${activeTabPath}...`);

    try {
      // The contract in the system prompt is what the save path then verifies:
      // an edit must come back as the COMPLETE file, because whatever is in
      // that block replaces the file verbatim.
      const systemInstruction =
        mode === 'edit'
          ? 'Bạn là Antigravity Senior AI Code Engineer. Khi chỉnh sửa, bạn PHẢI trả về TOÀN BỘ nội dung tệp sau khi sửa — từ dòng đầu đến dòng cuối — trong đúng MỘT khối ```...```. Nội dung khối đó sẽ GHI ĐÈ NGUYÊN VĂN lên tệp, nên tuyệt đối không rút gọn, không bỏ sót phần chưa sửa, không dùng "..." hay ghi chú kiểu "phần còn lại giữ nguyên". Giải thích ngắn (nếu cần) đặt bên ngoài khối code.'
          : 'Bạn là Antigravity Senior AI Code Engineer. Đây là yêu cầu phân tích: trả lời bằng văn bản, KHÔNG chỉnh sửa tệp. Cứ dùng khối code để minh hoạ — hệ thống sẽ không ghi đè tệp trong chế độ này.';
      const userPrompt = `Tệp tin: ${activeTabPath}\n\nNội dung mã nguồn hiện tại:\n\`\`\`\n${fileContent}\n\`\`\`\n\nYêu cầu:\n${finalPrompt}`;

      const res = await AppBindings.RunQuickCLI(userPrompt, aiModel, systemInstruction, [activeTabPath]);

      const targetTurn = turns.find(t => t.id === turnId);
      if (res && res.output) {
        if (targetTurn) targetTurn.output = res.output;

        const block = mode === 'edit' ? largestCodeBlock(res.output) : null;

        if (mode === 'analyze' || block === null) {
          // Nothing gets written: either the user asked for analysis, or the
          // model answered an edit request with prose only.
          if (mode === 'edit') {
            addTurnLog('error', 'AI không trả về khối code nào — tệp được giữ nguyên. Đọc câu trả lời bên dưới rồi thử lại với yêu cầu cụ thể hơn.');
          } else {
            addTurnLog('success', `Phân tích xong. Tệp ${activeTabPath} không bị thay đổi.`);
          }
          if (targetTurn) targetTurn.status = 'completed';
          addLog(`AI đã trả lời về ${activeTabPath} (không sửa tệp)`, 'SUCCESS');
        } else {
          const newContent = block;
          const reason = snippetSuspicion(newContent, initialOriginal);
          if (reason) {
            // The defect this guard exists for: the model suggested a few
            // spots, the app replaced the whole file with the suggestion.
            // Refuse the write and hand the user the suggestion instead.
            addTurnLog('error', `KHÔNG ghi đè ${activeTabPath}: ${reason}. Đề xuất của AI nằm trong output của lượt này — áp dụng tay từng đoạn, hoặc chạy lại và yêu cầu "trả về toàn bộ file".`);
            if (targetTurn) targetTurn.status = 'error';
            addToast(`Đã chặn ghi đè ${activeTabPath}: AI trả về đoạn trích, không phải cả file.`, 'WARN', 9000);
          } else {
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

            // Real numbers only — this line used to invent "+12 −4" whenever
            // the file length happened not to change.
            addTurnLog('diff', `Sửa file ${activeTabPath} — ${countDiffChunks(initialOriginal, newContent)} vùng thay đổi`, added, removed);
            addTurnLog('success', `Hoàn thành! File ${activeTabPath} đã được cập nhật trực tiếp.`);

            // Automated Chrome CDP Web Verification Test
            if (autoWebTest && webTestUrl) {
              addTurnLog('thinking', `Kích hoạt Chrome CDP Agent tự động mở & chụp ảnh kiểm thử tại ${webTestUrl}...`);
              try {
                if ((AppBindings as any).RunBrowserTask) {
                  const bRes = await (AppBindings as any).RunBrowserTask(webTestUrl, true);
                  if (bRes && bRes.success) {
                    addTurnLog('success', `Chrome CDP đã tự động kiểm thử & chụp ảnh Web thành công.`);
                    if (targetTurn) {
                      targetTurn.webScreenshot = bRes.screenshot_base64;
                      targetTurn.webTitle = bRes.title;
                    }
                  } else {
                    addTurnLog('error', `Không thể kiểm thử ${webTestUrl}: ${bRes?.error || 'Chưa bật dev server'}`);
                  }
                }
              } catch (bErr: any) {
                console.warn('Auto Chrome CDP test warning:', bErr);
              }
            }

            if (targetTurn) {
              targetTurn.linesAdded = added;
              targetTurn.linesRemoved = removed;
              targetTurn.status = 'completed';
            }

            addLog(`AI đã sửa & lưu trực tiếp tệp ${activeTabPath}`, 'SUCCESS');
            addToast(`AI đã sửa & lưu trực tiếp tệp ${activeTabPath}`, 'SUCCESS');
            viewMode = 'diff';
          }
        }
      } else if (res && res.error) {
        if (targetTurn) {
          targetTurn.status = 'error';
          targetTurn.output = 'Lỗi từ Agent: ' + res.error;
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

  // The badge on the Visual Diff button. Counted with the same diff the
  // MergeView draws — the old positional line comparison counted every line
  // below an insertion as changed. (Derived reactively rather than called
  // from the markup: Svelte reads a template function call untracked.)
  $: changedChunkCount = countDiffChunks(originalContent, fileContent);

  // One neutral glyph per log kind. The rows used to carry a five-hue tinted
  // box each; the icon says the same thing without repainting the panel.
  function logIcon(type: AiLogItem['type']): string {
    switch (type) {
      case 'edit': return 'edit';
      case 'diff': return 'difference';
      case 'success': return 'check_circle';
      case 'error': return 'error';
      default: return 'neurology';
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
          html += `<pre class="bg-surface-container-highest text-on-surface p-4 rounded-xl font-mono text-xs my-3 overflow-x-auto border border-outline-variant"><code>`;
          inCodeBlock = true;
        }
        continue;
      }

      if (inCodeBlock) {
        html += escapeHtml(line) + '\n';
        continue;
      }

      if (line.startsWith('# ')) {
        html += `<h1 class="text-lg font-semibold text-on-surface my-4 pb-2 border-b border-outline-variant">${escapeHtml(line.substring(2))}</h1>`;
      } else if (line.startsWith('## ')) {
        html += `<h2 class="text-base font-semibold text-on-surface my-3 pb-1 border-b border-outline-variant">${escapeHtml(line.substring(3))}</h2>`;
      } else if (line.startsWith('### ')) {
        html += `<h3 class="text-sm font-semibold text-on-surface my-2">${escapeHtml(line.substring(4))}</h3>`;
      } else if (line.startsWith('> ')) {
        html += `<blockquote class="border-l-2 border-outline-variant pl-4 py-1.5 my-2 italic text-on-surface-variant">${escapeHtml(line.substring(2))}</blockquote>`;
      } else if (line.startsWith('- ') || line.startsWith('* ')) {
        html += `<li class="ml-5 list-disc text-on-surface my-1">${escapeHtml(line.substring(2))}</li>`;
      } else if (line.trim() === '---') {
        html += `<hr class="my-6 border-outline-variant" />`;
      } else if (line.trim() === '') {
        html += `<div class="h-2"></div>`;
      } else {
        let parsedStr = escapeHtml(line)
          .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
          .replace(/`(.*?)`/g, '<code class="bg-surface-container-high text-on-surface px-1.5 py-0.5 rounded-lg font-mono text-xs">$1</code>');
        html += `<p class="text-sm text-on-surface leading-relaxed my-1">${parsedStr}</p>`;
      }
    }

    return html;
  }
</script>

<div class="h-[calc(100vh-105px)] flex flex-col space-y-3 font-sans">
  <!-- IDE Top Action Toolbar & Theme Selector -->
  <div class="flex flex-wrap items-center justify-between gap-3 bg-surface-container-low p-3 rounded-xl border border-outline-variant">
    <div class="flex items-center gap-3">
      <span class="material-symbols-outlined text-lg text-on-surface-variant">code_blocks</span>
      <div>
        <h1 class="text-lg font-semibold text-on-surface">Code Studio</h1>
        <p class="text-[11px] text-on-surface-variant font-mono truncate max-w-sm">
          {activeTabPath ? activeTabPath : 'Chưa chọn file'}
        </p>
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-2 text-xs">
      <!-- Mode Toggle: Code vs Diff vs Markdown Preview -->
      <div class="bg-surface-container-lowest p-1 rounded-lg border border-outline-variant flex gap-1">
        <button
          type="button"
          on:click={() => viewMode = 'code'}
          class="px-3 py-1.5 rounded-lg font-medium transition-colors cursor-pointer flex items-center gap-1.5
          {viewMode === 'code' ? 'bg-secondary-container text-on-secondary-container' : 'text-on-surface-variant hover:bg-surface-container-high'}">
          <span class="material-symbols-outlined text-sm">edit_note</span> Editor
        </button>

        {#if isMarkdown}
          <button
            type="button"
            on:click={() => viewMode = 'preview'}
            class="px-3 py-1.5 rounded-lg font-medium transition-colors cursor-pointer flex items-center gap-1.5
            {viewMode === 'preview' ? 'bg-secondary-container text-on-secondary-container' : 'text-on-surface-variant hover:bg-surface-container-high'}">
            <span class="material-symbols-outlined text-sm">menu_book</span> Preview
          </button>
        {/if}

        <button
          type="button"
          on:click={() => viewMode = 'diff'}
          class="px-3 py-1.5 rounded-lg font-medium transition-colors cursor-pointer flex items-center gap-1.5
          {viewMode === 'diff' ? 'bg-secondary-container text-on-secondary-container' : 'text-on-surface-variant hover:bg-surface-container-high'}">
          <span class="material-symbols-outlined text-sm">difference</span> Diff ({changedChunkCount})
        </button>
      </div>

      <!-- Editor theme override: 'auto' follows the app theme -->
      <div class="flex items-center gap-1.5 bg-surface-container-lowest border border-outline-variant px-2.5 py-1.5 rounded-lg">
        <span class="material-symbols-outlined text-sm text-on-surface-variant">palette</span>
        <select bind:value={codeTheme} title="Giao diện editor" class="bg-transparent text-xs text-on-surface font-medium border-none cursor-pointer">
          <option value="auto">Theo giao diện app</option>
          <option value="dark">Nền tối</option>
          <option value="light">Nền sáng</option>
        </select>
      </div>

      <!-- Font Size Toggle -->
      <div class="flex items-center gap-1 bg-surface-container-lowest border border-outline-variant px-2 py-1 rounded-lg text-on-surface">
        <button type="button" on:click={() => fontSize = Math.max(10, fontSize - 1)} title="Giảm cỡ chữ" class="px-1.5 hover:bg-surface-container-high rounded-lg cursor-pointer font-medium">-</button>
        <span class="font-mono text-[11px] px-1">{fontSize}px</span>
        <button type="button" on:click={() => fontSize = Math.min(22, fontSize + 1)} title="Tăng cỡ chữ" class="px-1.5 hover:bg-surface-container-high rounded-lg cursor-pointer font-medium">+</button>
      </div>

      <!-- Save Button -->
      <button
        type="button"
        on:click={handleSaveFile}
        disabled={!isDirty || isSaving}
        class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg font-medium hover:bg-surface-container-high transition-colors flex items-center gap-1.5 disabled:opacity-50 cursor-pointer">
        <span class="material-symbols-outlined text-sm">save</span>
        {isSaving ? 'Đang lưu...' : 'Lưu File (Ctrl+S)'}
      </button>
    </div>
  </div>

  <!-- Main 3-Panel IDE Workspace Grid -->
  <div class="flex-1 grid grid-cols-12 gap-3 overflow-hidden">
    <!-- Left Column: File Explorer (col-span-3) -->
    <div class="col-span-3 bg-surface-container-lowest border border-outline-variant rounded-xl flex flex-col overflow-hidden">
      <div class="p-3 border-b border-outline-variant flex items-center justify-between">
        <span class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm text-on-surface-variant">folder_open</span>
          Workspace ({files.length})
        </span>
        <button type="button" on:click={refreshFiles} title="Quét lại file" class="text-on-surface-variant hover:text-on-surface transition-colors cursor-pointer">
          <span class="material-symbols-outlined text-base">sync</span>
        </button>
      </div>

      <div class="p-2 border-b border-outline-variant">
        <div class="relative">
          <span class="material-symbols-outlined absolute left-2.5 top-1.5 text-sm text-outline">search</span>
          <input
            type="text"
            bind:value={fileQuery}
            placeholder="Tìm tệp tin..."
            class="w-full bg-surface-container-lowest border border-outline-variant rounded-lg pl-8 pr-2 py-1 text-xs text-on-surface outline-none focus:border-primary font-mono"
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
              class="w-full text-left px-2.5 py-1.5 rounded-lg flex items-center gap-2 truncate transition-colors cursor-pointer
              {activeTabPath === file ? 'bg-secondary-container text-on-secondary-container font-medium' : 'text-on-surface-variant hover:bg-surface-container-low hover:text-on-surface'}"
              title={file}
            >
              <span class="truncate flex-1">{file}</span>
              {#if dirtyPaths.has(file)}
                <span class="w-1.5 h-1.5 rounded-full bg-warning flex-shrink-0" title="Chưa lưu"></span>
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
    <div class="col-span-6 bg-surface-container-lowest border border-outline-variant rounded-xl flex flex-col overflow-hidden relative">
      {#if isReading}
        <div class="absolute inset-0 bg-surface/80 z-30 flex items-center justify-center">
          <span class="material-symbols-outlined animate-spin text-on-surface-variant text-lg">sync</span>
        </div>
      {/if}

      <!-- Multi-Tab Navigation Bar. The close control is a sibling button, not a
           span nested inside the tab button: an interactive element inside a
           <button> is invalid, and the keyboard could focus a control that was
           invisible until the pointer hovered it. -->
      <div class="bg-surface-container-low border-b border-outline-variant flex items-center gap-1 px-2 pt-2 overflow-x-auto select-none">
        {#each openTabs as tab}
          <div
            class="group flex items-center flex-shrink-0 rounded-t-xl border-t border-x transition-colors max-w-[200px]
            {activeTabPath === tab.path ? 'bg-surface-container-lowest border-outline-variant text-on-surface' : 'border-transparent text-on-surface-variant hover:bg-surface-container-high'}"
          >
            <button
              type="button"
              on:click={() => activeTabPath = tab.path}
              class="flex items-center gap-2 min-w-0 pl-3 pr-1.5 py-1.5 text-xs font-mono cursor-pointer
              {activeTabPath === tab.path ? 'font-medium' : ''}"
              title={tab.path}
            >
              <span class="material-symbols-outlined text-sm flex-shrink-0">description</span>
              <span class="truncate">{tab.path.split(/[\/\\]/).pop()}</span>
              {#if tab.isDirty}
                <span class="w-1.5 h-1.5 rounded-full bg-warning flex-shrink-0" title="Chưa lưu"></span>
              {/if}
            </button>
            <button
              type="button"
              on:click={(e) => closeTab(tab.path, e)}
              title="Đóng tab"
              aria-label="Đóng tab {tab.path}"
              class="w-6 h-6 mr-1 flex items-center justify-center flex-shrink-0 rounded-lg opacity-0 group-hover:opacity-100 focus-visible:opacity-100 text-on-surface-variant hover:bg-surface-container-highest hover:text-error transition-opacity cursor-pointer"
            >
              <span class="material-symbols-outlined text-sm">close</span>
            </button>
          </div>
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
          <button type="button" on:click={handleSearchReplace} class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg font-medium hover:bg-surface-container-high transition-colors cursor-pointer">
            Thay tất cả
          </button>
          <button type="button" on:click={() => showSearch = false} title="Đóng" class="text-on-surface-variant hover:text-on-surface p-1 rounded-lg cursor-pointer">
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
            dark={editorDark}
            {fontSize}
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
              <span class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
                <span class="material-symbols-outlined text-sm text-on-surface-variant">visibility</span> Markdown preview
              </span>
              <button
                type="button"
                on:click={() => viewMode = 'code'}
                class="text-xs border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg font-medium hover:bg-surface-container-high transition-colors cursor-pointer">
                Sửa Markdown
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
          <div class="flex text-[11px] font-medium font-mono border-b border-outline-variant">
            <div class="w-1/2 px-3 py-1.5 bg-surface-container-low text-on-surface-variant border-r border-outline-variant">Bản gốc</div>
            <div class="w-1/2 px-3 py-1.5 bg-surface-container-low text-on-surface-variant">Sau khi sửa</div>
          </div>
          <div class="flex-1 overflow-hidden">
            <DiffView
              original={originalContent}
              modified={fileContent}
              path={activeTabPath}
              dark={editorDark}
            />
          </div>
        </div>
      {/if}

      <!-- IDE Status Footer Bar -->
      <div class="bg-surface-container-low px-4 py-1.5 border-t border-outline-variant flex items-center justify-between text-[11px] font-mono text-on-surface-variant select-none">
        <div class="flex items-center gap-4">
          <span class="font-medium text-on-surface">{activeTabPath ? activeTabPath.split(/[\/\\]/).pop() : 'Chưa mở file'}</span>
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
    <div class="col-span-3 bg-surface-container-lowest border border-outline-variant rounded-xl flex flex-col overflow-hidden">
      <div class="p-3 border-b border-outline-variant flex items-center justify-between gap-2">
        <span class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm text-on-surface-variant">auto_awesome</span>
          AI Assistant ({turns.length})
        </span>
        <ModelSelect
          bind:value={aiModel}
          selectClass="bg-surface-container-low border border-outline-variant rounded-lg px-2 py-1 text-[11px] font-mono text-on-surface"
        />
      </div>

      <div class="p-3 flex-1 flex flex-col gap-3 overflow-hidden">
        <!-- Quick Preset Action Chips -->
        <div class="flex flex-wrap gap-1.5">
          <button
            type="button"
            on:click={() => handleAskAI('refactor')}
            disabled={isAiRunning}
            class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm">build</span> Refactor Code
          </button>
          <button
            type="button"
            on:click={() => handleAskAI('explain')}
            disabled={isAiRunning}
            class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm">help</span> Giải thích Logic
          </button>
          <button
            type="button"
            on:click={() => handleAskAI('test')}
            disabled={isAiRunning}
            class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm">science</span> Viết Unit Test
          </button>
        </div>

        <div class="space-y-1.5">
          <div class="flex items-center justify-between gap-2 bg-surface-container-low px-2.5 py-1.5 rounded-lg border border-outline-variant text-[11px]">
            <label class="flex items-center gap-1.5 font-medium text-on-surface cursor-pointer select-none">
              <input type="checkbox" bind:checked={autoWebTest} class="accent-primary" />
              <span>Chrome CDP Auto-Test</span>
            </label>
            <input
              type="text"
              bind:value={webTestUrl}
              placeholder="http://localhost:5173"
              aria-label="URL kiểm thử"
              class="w-32 bg-surface-container-lowest border border-outline-variant rounded-lg px-2 py-0.5 text-[11px] font-mono text-on-surface outline-none focus:border-primary"
            />
          </div>

          <textarea
            bind:value={aiPrompt}
            placeholder="Nhập yêu cầu AI chỉnh sửa code trực tiếp..."
            class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2.5 text-xs text-on-surface h-20 outline-none focus:border-primary resize-none font-sans"
          ></textarea>
          <button
            type="button"
            on:click={() => handleAskAI()}
            disabled={isAiRunning || !aiPrompt.trim()}
            class="w-full bg-primary text-on-primary py-2 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity flex items-center justify-center gap-1.5 disabled:opacity-50 cursor-pointer">
            <span class="material-symbols-outlined text-sm {isAiRunning ? 'animate-spin' : ''}">
              {isAiRunning ? 'sync' : 'auto_fix_high'}
            </span>
            {isAiRunning ? 'AI đang thực thi & sửa file...' : 'Yêu cầu AI sửa trực tiếp'}
          </button>
        </div>

        <!-- Conversation Turn Feed with Revert Buttons & Screenshot Proof -->
        <div bind:this={turnFeedContainer} class="flex-1 p-2 overflow-y-auto space-y-3">
          {#each turns as turn (turn.id)}
            <div class="bg-surface-container-low border border-outline-variant rounded-xl p-3 space-y-2.5">
              <!-- Turn header: the prompt and the revert action, separated from
                   the logs by a hairline instead of a second nested box. -->
              <div class="flex items-start justify-between gap-2">
                <div class="space-y-0.5 min-w-0">
                  <div class="text-[11px] text-on-surface-variant">Prompt · {turn.time}</div>
                  <div class="text-xs font-medium text-on-surface line-clamp-2">{turn.userPrompt}</div>
                </div>

                <!-- Revert Button: Undo changes up to this point -->
                <button
                  type="button"
                  on:click={() => handleRevertTurn(turn.id)}
                  disabled={turn.status === 'reverted'}
                  title="Hoàn tác file về trước lượt này"
                  class="flex items-center gap-1 border border-outline-variant px-2 py-1 rounded-lg text-[11px] font-medium text-error hover:bg-error/10 transition-colors cursor-pointer flex-shrink-0 disabled:opacity-50"
                >
                  <span class="material-symbols-outlined text-sm">undo</span>
                  {turn.status === 'reverted' ? 'Đã revert' : 'Revert'}
                </button>
              </div>

              <!-- Realtime logs: plain rows on the panel surface, colour only on
                   the leading icon for success/error. -->
              {#if turn.logs.length > 0}
                <div class="space-y-1 font-mono text-[11px] pt-2 border-t border-outline-variant">
                  {#each turn.logs as log}
                    <div class="flex items-start gap-1.5 leading-relaxed text-on-surface-variant">
                      <span class="material-symbols-outlined text-sm flex-shrink-0 mt-px
                        {log.type === 'success' ? 'text-success' : log.type === 'error' ? 'text-error' : ''}">
                        {logIcon(log.type)}
                      </span>
                      <div class="min-w-0 flex-1">
                        <div>{log.text}</div>
                        {#if log.linesAdded || log.linesRemoved}
                          <div class="mt-0.5 flex items-center gap-2">
                            <span class="text-success">+{log.linesAdded}</span>
                            <span class="text-error">-{log.linesRemoved}</span>
                          </div>
                        {/if}
                      </div>
                      <span class="text-outline flex-shrink-0">{log.time}</span>
                    </div>
                  {/each}
                </div>
              {/if}

              <!-- Chrome CDP Live Web Test Screenshot Proof -->
              {#if turn.webScreenshot}
                <div class="border border-outline-variant rounded-xl overflow-hidden bg-surface-container p-2 space-y-1">
                  <div class="text-[11px] text-on-surface-variant flex items-center gap-1.5">
                    <span class="material-symbols-outlined text-sm">center_focus_strong</span>
                    Chrome CDP · {turn.webTitle}
                  </div>
                  <img src={turn.webScreenshot} alt="Ảnh chụp kiểm thử Chrome CDP" class="w-full h-auto rounded-lg border border-outline-variant" />
                </div>
              {/if}

              <!-- Turn Result Badge Footer -->
              {#if turn.status === 'completed'}
                <div class="flex items-center justify-between gap-2 pt-2 border-t border-outline-variant text-[11px]">
                  <span class="text-on-surface-variant flex items-center gap-1.5">
                    <span class="material-symbols-outlined text-sm text-success">check_circle</span>
                    Đã sửa 1 file (+{turn.linesAdded} -{turn.linesRemoved})
                  </span>
                  <button
                    type="button"
                    on:click={() => viewMode = 'diff'}
                    class="text-[11px] border border-outline-variant text-on-surface-variant px-2 py-1 rounded-lg font-medium hover:bg-surface-container-high transition-colors cursor-pointer flex-shrink-0">
                    Xem diff
                  </button>
                </div>
              {:else if turn.status === 'reverted'}
                <div class="text-[11px] text-on-surface-variant flex items-center gap-1.5 pt-2 border-t border-outline-variant">
                  <span class="material-symbols-outlined text-sm text-warning">history</span> Đã khôi phục file về trước lượt này
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
