<script lang="ts">
  import { onMount } from 'svelte';
  import type { Agent } from '../../lib/types';
  import { logs, addLog, addToast, agentsStore } from '../../lib/stores/appState';
  import AgentFormModal from '../ui/AgentFormModal.svelte';
  import MCPManager from '../ui/MCPManager.svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import Dropdown from '../ui/Dropdown.svelte';
  import ModelSelect, { loadModelCatalog } from '../ui/ModelSelect.svelte';
  import OAuthPoolDashboard from './OAuthPoolDashboard.svelte';

  // A named type, not `typeof subTab`: TypeScript answers a type query with the
  // variable's NARROWED type, which after the initializer is just 'oauth_pool',
  // so every other entry in the list below failed to typecheck.
  type SubTab = 'agents' | 'oauth_pool' | 'cli' | 'logs' | 'updates' | 'integrations' | 'memory';
  let subTab: SubTab = 'oauth_pool';

  // One list, one active treatment: the tabs used to be seven hand-written
  // buttons and the OAuth one had drifted into a filled pill with its own icon.
  const SUB_TABS: { id: SubTab; label: string }[] = [
    { id: 'oauth_pool', label: 'OAuth Pool' },
    { id: 'agents', label: 'Agents' },
    { id: 'cli', label: 'Quick CLI' },
    { id: 'logs', label: 'Log hệ thống' },
    { id: 'updates', label: 'Cập nhật' },
    { id: 'integrations', label: 'Webhook & MCP' },
    { id: 'memory', label: 'Memory' },
  ];
  // Use global store so agent list persists across tab switches
  let agents: Agent[] = [];
  const unsubscribeAgents = agentsStore.subscribe((v) => { agents = v as Agent[]; });

  // Quick CLI state
  let quickPrompt = '';
  let selectedAgentType = 'claude';
  let selectedModel = 'claude-opus-4-8';
  let quickOutput = '';
  let isQuickRunning = false;

  // Updater state
  let updateInfo: any = null;

  // Integrations state
  let webhookUrl = '';
  // No longer edited here — MCP servers are managed by MCPManager, which stores
  // them properly. Kept so saving integrations round-trips any value an older
  // build wrote instead of silently clearing it.
  let mcpConnectionString = '';
  let currentAppVersion = '';
  let isIntegrationsSaving = false;
  let isCheckingUpdate = false;
  let isUpdating = false;
  let updateStatusMessage = '';
  let updateStatusType: 'info' | 'success' | 'error' = 'info';

  // Daily spending cap: the app can spend money unattended, so the ceiling and
  // what has gone against it belong where the user can see them.
  let budget: { day: string; spent_usd: number; limit_usd: number } | null = null;
  let budgetLimit = 0;
  let isSavingBudget = false;

  let showCLIConsole = false;
  let antiAccountKeys: any[] = [];
  let newKeyName = '';
  let newKeyValue = '';

  // Memory subsystem settings (context pack, summaries, resume, promotion).
  let memoryCfg = { context_pack_max_chars: 12000, auto_summarize: true, session_resume: false, lesson_promotion: false };
  let isSavingMemoryCfg = false;

  async function loadMemoryCfg() {
    try {
      if ((AppBindings as any).GetMemoryConfig) {
        const cfg = await (AppBindings as any).GetMemoryConfig();
        if (cfg) memoryCfg = cfg;
      }
    } catch (e) {}
  }

  let isRefreshingModels = false;
  async function handleRefreshGeminiModels() {
    isRefreshingModels = true;
    try {
      const n = await (AppBindings as any).RefreshGeminiModels();
      await loadModelCatalog(true);
      addToast(`Đã tải danh sách model Gemini từ API: ${n} model.`, 'SUCCESS');
    } catch (e: any) {
      addToast('Tải danh sách Gemini thất bại: ' + String(e?.message ?? e), 'ERROR');
    } finally {
      isRefreshingModels = false;
    }
  }

  async function handleSaveMemoryCfg() {
    isSavingMemoryCfg = true;
    try {
      const budgetNum = Number(memoryCfg.context_pack_max_chars);
      memoryCfg.context_pack_max_chars = Number.isFinite(budgetNum) && budgetNum > 0 ? Math.floor(budgetNum) : 0;
      await (AppBindings as any).SaveMemoryConfig(memoryCfg);
      addToast(
        memoryCfg.context_pack_max_chars === 0
          ? 'Đã lưu — context pack đang tắt (budget = 0).'
          : `Đã lưu cài đặt memory (pack ≤ ${memoryCfg.context_pack_max_chars} ký tự).`,
        'SUCCESS'
      );
      await loadMemoryCfg();
    } catch (e: any) {
      addToast('Lưu cài đặt memory thất bại: ' + String(e?.message ?? e), 'ERROR');
    } finally {
      isSavingMemoryCfg = false;
    }
  }

  async function initSettings() {
    await loadAgents();
    await loadAntiKeys();
    await loadBudget();
    await loadMemoryCfg();
    try {
      if ((AppBindings as any).GetIntegrationsConfig) {
        const cfg = await (AppBindings as any).GetIntegrationsConfig();
        webhookUrl = cfg?.outbound_webhook_url || '';
        mcpConnectionString = cfg?.mcp_connection_string || '';
      }
    } catch (e) {}
    try {
      if ((AppBindings as any).GetAppVersion) {
        currentAppVersion = await (AppBindings as any).GetAppVersion();
      }
      if ((AppBindings as any).GetShowCLIConsole) {
        showCLIConsole = await (AppBindings as any).GetShowCLIConsole();
      }
      if ((window as any)?.runtime?.EventsOn) {
        // Captured for cleanup: the page is destroyed on every tab switch,
        // and a listener left behind fires once per past visit — k visits
        // meant k+1 duplicate toasts per OAuth login.
        offOAuthSuccess = (window as any).runtime.EventsOn("oauth_success", async (data: any) => {
          await loadAntiKeys();
          addLog(`Tự động đăng nhập và lưu OAuth Token cho ${data?.email || 'Google Account'} thành công.`, 'SUCCESS');
          addToast(`Tự động đăng nhập và lưu OAuth Token cho ${data?.email || 'Google Account'} thành công.`, 'SUCCESS');
        });
      }
    } catch (e) {}
  }

  let offOAuthSuccess: (() => void) | null = null;

  onMount(() => {
    initSettings();
    return () => {
      unsubscribeAgents();
      offOAuthSuccess?.();
    };
  });

  let newAuthType: 'api_key' | 'oauth_token' = 'api_key';

  async function loadAntiKeys() {
    try {
      if ((AppBindings as any).GetAntiAccountKeys) {
        antiAccountKeys = await (AppBindings as any).GetAntiAccountKeys();
      }
    } catch (e) {}
  }

  async function handleAddAntiKey() {
    if (!newKeyName.trim() || !newKeyValue.trim()) return;
    try {
      if (newAuthType === 'oauth_token' && (AppBindings as any).AddAntiOAuthAccountKey) {
        await (AppBindings as any).AddAntiOAuthAccountKey(newKeyName.trim(), newKeyValue.trim());
        addLog(`Đã thêm Antigravity Manager OAuth Token '${newKeyName}' vào Pool!`, 'SUCCESS');
        addToast(`Đã thêm Antigravity Manager OAuth Token '${newKeyName}' vào Pool!`, 'SUCCESS');
      } else if ((AppBindings as any).AddAntiAccountKey) {
        await (AppBindings as any).AddAntiAccountKey(newKeyName.trim(), newKeyValue.trim());
        addLog(`Đã thêm Backup API Key '${newKeyName}' vào Anti CLI Pool!`, 'SUCCESS');
        addToast(`Đã thêm Backup API Key '${newKeyName}' vào Anti CLI Pool!`, 'SUCCESS');
      }
      newKeyName = '';
      newKeyValue = '';
      await loadAntiKeys();
    } catch (e) {
      addLog(`Thêm tài khoản thất bại: ${e}`, 'ERROR');
      addToast(`Thêm tài khoản thất bại: ${e}`, 'ERROR');
    }
  }

  let customClientId: string = '';

  async function handleOpenGoogleLogin() {
    try {
      if ((AppBindings as any).OpenGoogleOAuthLogin) {
        await (AppBindings as any).OpenGoogleOAuthLogin(customClientId.trim());
        addLog('Đã mở trang Đăng nhập Google OAuth trên Trình duyệt!', 'INFO');
      } else {
        const id = customClientId.trim() || "32555940559-fa7440q2viic87nvi8nk5w0y86z.apps.googleusercontent.com";
        window.open(`https://accounts.google.com/o/oauth2/v2/auth?client_id=${id}&redirect_uri=http://localhost:8045/auth/callback&response_type=code&scope=https://www.googleapis.com/auth/cloud-platform%20https://www.googleapis.com/auth/userinfo.email`, '_blank');
      }
    } catch (e) {
      addLog(`Mở Google OAuth thất bại: ${e}`, 'ERROR');
      addToast(`Mở Google OAuth thất bại: ${e}`, 'ERROR');
    }
  }

  async function handleOpenExternalUrl(url: string) {
    try {
      if ((AppBindings as any).OpenURLInBrowser) {
        await (AppBindings as any).OpenURLInBrowser(url);
      } else {
        window.open(url, '_blank');
      }
    } catch (e) {}
  }

  async function loadAgents() {
    try {
      if (AppBindings && AppBindings.GetAgents) {
        let loaded = await AppBindings.GetAgents();
        // Auto-seed default agents if DB is empty
        if (!loaded || loaded.length === 0) {
          await AppBindings.ResetAgentsToDefaults();
          loaded = await AppBindings.GetAgents();
          addLog(`Chưa có agent nào — đã tạo ${(loaded || []).length} agent mặc định.`, 'INFO');
          addToast('Chưa có agent nào — đã tạo bộ agent mặc định.', 'INFO');
        }
        agentsStore.set(loaded || []);
      }
    } catch (e) {
      console.error(e);
    }
  }

  async function handleResetAgents() {
    await AppBindings.ResetAgentsToDefaults();
    const loaded = await AppBindings.GetAgents();
    agentsStore.set(loaded || []);
    addLog(`Đã khôi phục ${(loaded || []).length} agent mặc định.`, 'SUCCESS');
    addToast(`Đã khôi phục ${(loaded || []).length} agent mặc định.`, 'SUCCESS');
  }

  // In-progress persona edits live here, keyed by agent id, NOT on the store
  // objects: agent_updated replaces agentsStore whenever any task finishes
  // anywhere, and a bind:value on the store object silently discarded
  // whatever the user had typed.
  let personaDrafts: Record<string, string> = {};

  function setPersonaDraft(agentId: string, value: string) {
    personaDrafts = { ...personaDrafts, [agentId]: value };
  }

  async function handleSaveAgent(agent: Agent) {
    try {
      const system = personaDrafts[agent.agent_id] ?? agent.system;
      await AppBindings.SaveAgent({ ...agent, system } as any);
      const { [agent.agent_id]: _saved, ...rest } = personaDrafts;
      personaDrafts = rest;
      addLog(`Đã lưu Agent ${agent.name}.`, 'SUCCESS');
      addToast(`Đã lưu Agent ${agent.name}.`, 'SUCCESS');
      // agent_updated event refreshes agentsStore globally (3D office, kanban, cockpit).
    } catch (e) {
      addLog(`Lỗi lưu agent: ${e}`, 'ERROR');
      addToast(`Lỗi lưu agent: ${e}`, 'ERROR');
    }
  }

  // The two-input inline form is gone: it let the user set name and role while
  // the code decided provider, model, icon and system prompt — the fields that
  // actually determine which engine runs the task and how the agent behaves.
  let agentFormOpen = false;
  let agentBeingEdited: Agent | null = null;

  function openNewAgentForm() {
    agentBeingEdited = null;
    agentFormOpen = true;
  }

  function openEditAgentForm(agent: Agent) {
    agentBeingEdited = agent;
    agentFormOpen = true;
  }

  function duplicateAgent(agent: Agent) {
    // A copy with no id is a new agent; clearing the counters stops the clone
    // from inheriting the original's history.
    agentBeingEdited = {
      ...(agent as any),
      agent_id: '',
      name: `${agent.name} (bản sao)`,
      tasks_done: 0,
      tokens_used: 0,
    } as Agent;
    agentFormOpen = true;
  }

  async function handleAgentFormSave(e: CustomEvent<any>) {
    const agent = e.detail;
    try {
      await AppBindings.SaveAgent(agent);
      agentFormOpen = false;
      await loadAgents();
      addLog(`Đã lưu Agent: ${agent.name}.`, 'SUCCESS');
      addToast(`Đã lưu Agent: ${agent.name}.`, 'SUCCESS');
    } catch (err) {
      addLog(`Lỗi lưu agent: ${err}`, 'ERROR');
      addToast(`Lỗi lưu agent: ${err}`, 'ERROR');
    }
  }

  async function handleDeleteAgent(agent: Agent) {
    if (!(agent as any).agent_id) return;
    if (!confirm(`Xóa agent "${agent.name}"?`)) return;
    try {
      await AppBindings.DeleteAgent((agent as any).agent_id);
      addLog(`Đã xóa Agent ${agent.name}.`, 'WARN');
    } catch (e) {
      addLog(`Lỗi xóa agent: ${e}`, 'ERROR');
      addToast(`Lỗi xóa agent: ${e}`, 'ERROR');
    }
  }

  async function loadBudget() {
    try {
      budget = await (AppBindings as any).GetBudgetStatus();
      if (budget) budgetLimit = budget.limit_usd;
    } catch (e) {}
  }

  async function saveBudget() {
    isSavingBudget = true;
    try {
      await (AppBindings as any).SetDailyBudgetUSD(Number(budgetLimit) || 0);
      await loadBudget();
      const msg = Number(budgetLimit) > 0
        ? `Trần chi phí mỗi ngày: $${Number(budgetLimit).toFixed(2)}`
        : 'Đã bỏ trần chi phí mỗi ngày.';
      addLog(msg, 'SUCCESS');
      addToast(msg, 'SUCCESS');
    } catch (e) {
      addLog(`Lỗi lưu trần chi phí: ${e}`, 'ERROR');
      addToast(`Không lưu được trần chi phí: ${e}`, 'ERROR');
    } finally {
      isSavingBudget = false;
    }
  }

  async function handleSaveIntegrations() {
    isIntegrationsSaving = true;
    try {
      await (AppBindings as any).SaveIntegrationsConfig({
        outbound_webhook_url: webhookUrl.trim(),
        mcp_connection_string: mcpConnectionString.trim(),
      });
      addLog('Đã lưu cấu hình Webhook & MCP. Sự kiện task done/failed sẽ được gửi tới URL này.', 'SUCCESS');
      addToast('Đã lưu cấu hình Webhook & MCP. Sự kiện task done/failed sẽ được gửi tới URL này.', 'SUCCESS');
    } catch (e) {
      addLog(`Lỗi lưu cấu hình integrations: ${e}`, 'ERROR');
      addToast(`Lỗi lưu cấu hình integrations: ${e}`, 'ERROR');
    } finally {
      isIntegrationsSaving = false;
    }
  }

  async function handleRunQuickCLI() {
    if (!quickPrompt.trim()) return;
    isQuickRunning = true;
    quickOutput = 'Đang chạy CLI…';
    try {
      const res = await AppBindings.RunQuickCLI(quickPrompt, selectedModel, '', []);
      if (res && res.success) {
        quickOutput = res.output;
      } else if (res) {
        quickOutput = 'ERROR: ' + res.error;
      }
    } catch (e) {
      quickOutput = 'ERROR: ' + e;
    } finally {
      isQuickRunning = false;
    }
  }

  async function handleCheckUpdate() {
    isCheckingUpdate = true;
    updateStatusMessage = 'Đang kiểm tra bản cập nhật mới nhất từ GitHub...';
    updateStatusType = 'info';
    try {
      updateInfo = await AppBindings.CheckForUpdates();
      if (updateInfo) {
        if (!updateInfo.has_update) {
          updateStatusMessage = 'Bạn đang sử dụng phiên bản mới nhất!';
          updateStatusType = 'success';
        } else {
          updateStatusMessage = '';
        }
      }
    } catch (e: any) {
      updateStatusMessage = 'Lỗi kiểm tra cập nhật: ' + (e?.message || e);
      updateStatusType = 'error';
    } finally {
      isCheckingUpdate = false;
    }
  }

  async function handleAutoUpdate() {
    if (!updateInfo) return;
    if (!updateInfo.download_url) {
      updateStatusMessage = 'Không tìm thấy file .exe trong bản release mới nhất trên GitHub (v' + updateInfo.version + '). Vui lòng tải thủ công!';
      updateStatusType = 'error';
      addLog('Không tìm thấy file .exe trong release mới nhất của GitHub.', 'ERROR');
      addToast('Không tìm thấy file .exe trong release mới nhất của GitHub.', 'ERROR');
      return;
    }

    isUpdating = true;
    updateStatusMessage = 'Đang tải bản cập nhật từ GitHub...';
    updateStatusType = 'info';
    addLog('Đang tải bản cập nhật từ ' + updateInfo.download_url, 'INFO');

    try {
      // Show installing message before calling backend
      // App will call os.Exit(0) during this → IPC cut → catch fires. That is NORMAL.
      updateStatusMessage = 'Đang cài đặt... Bản cài đặt sẽ mở trình cài đặt (bấm Yes ở hộp thoại UAC); bản portable sẽ tự khởi động lại.';
      updateStatusType = 'success';

      const res = await (AppBindings as any).DownloadAndUpdate(updateInfo.download_url);
      // If we somehow get a response (unlikely), check for error
      if (res && !res.success) {
        updateStatusMessage = 'Cập nhật thất bại: ' + (res?.error || 'Không xác định');
        updateStatusType = 'error';
        addLog('Cập nhật thất bại: ' + (res?.error || 'Không xác định'), 'ERROR');
        addToast('Cập nhật thất bại: ' + (res?.error || 'Không xác định'), 'ERROR');
        isUpdating = false;
      }
    } catch (e: any) {
      // App called os.Exit(0) → IPC dropped → catch fires. This is expected.
      // Installed copy: the NSIS installer is opening (UAC prompt first).
      // Portable copy: the updater .bat swaps the exe and relaunches.
      updateStatusMessage = 'Đã tải xong! App sẽ đóng để cập nhật — nếu trình cài đặt hiện ra, làm theo các bước.';
      updateStatusType = 'success';
      addLog('Đã tải xong bản cập nhật — app sẽ đóng để cài đặt.', 'SUCCESS');
      addToast('Đã tải xong bản cập nhật — app sẽ đóng để cài đặt.', 'SUCCESS');
      // Do NOT set isUpdating = false — app is mid-restart
    }
  }

</script>

<div class="space-y-6 max-w-7xl mx-auto pb-12">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-lg font-semibold flex items-center gap-2 text-on-surface">
        <span class="material-symbols-outlined text-lg text-on-surface-variant">settings</span>
        Cài đặt hệ thống
      </h1>
      <p class="text-on-surface-variant text-xs mt-0.5">Quản lý agent, chạy nhanh CLI, xem log hệ thống và cập nhật.</p>
    </div>
  </div>

  <!-- Sub-Tabs Navigation -->
  <div class="flex justify-center overflow-x-auto">
    <div class="bg-surface-container-high p-1 rounded-xl flex flex-wrap justify-center gap-1 border border-outline-variant">
      {#each SUB_TABS as t}
        <button
          type="button"
          on:click={() => (subTab = t.id)}
          class="px-4 py-1.5 text-xs font-medium rounded-lg transition-colors cursor-pointer whitespace-nowrap
          {subTab === t.id ? 'bg-surface-container-lowest text-on-surface' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          {t.label}
        </button>
      {/each}
    </div>
  </div>

  {#if subTab === 'oauth_pool'}
    <OAuthPoolDashboard onOpenGoogleLogin={handleOpenGoogleLogin} />
  {:else if subTab === 'agents'}
    <!-- Agents Registry View -->
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="font-semibold text-sm text-on-surface">Agents ({agents.length})</h3>
        <button on:click={handleResetAgents} class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors cursor-pointer flex items-center gap-1">
          <span class="material-symbols-outlined text-sm">restart_alt</span> Khôi phục mặc định
        </button>
      </div>

      <!-- Add new agent (persists + syncs to 3D office, Kanban, Cockpit) -->
      <div class="flex flex-wrap items-center justify-between gap-2 bg-surface-container-low border border-outline-variant rounded-xl p-3">
        <span class="text-xs text-on-surface-variant flex items-center gap-2">
          <span class="material-symbols-outlined text-base text-on-surface-variant">person_add</span>
          Tạo agent mới với đầy đủ provider, model, persona và giới hạn token.
        </span>
        <button on:click={openNewAgentForm}
          class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity cursor-pointer flex items-center gap-1">
          <span class="material-symbols-outlined text-sm">add</span> Tạo agent
        </button>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        {#each agents as agent}
          <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3 hover:bg-surface-container-low transition-colors">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-3">
                <span class="material-symbols-outlined text-base text-on-surface-variant">{agent.icon || 'smart_toy'}</span>
                <div>
                  <h4 class="font-semibold text-sm text-on-surface">{agent.name}</h4>
                  <p class="text-xs text-on-surface-variant line-clamp-1">{agent.role}</p>
                </div>
              </div>
              <div class="flex items-center gap-1.5">
                <button
                  type="button"
                  on:click={() => openEditAgentForm(agent)}
                  title="Sửa đầy đủ"
                  class="p-1 rounded-lg text-on-surface-variant hover:text-primary hover:bg-surface-container-high cursor-pointer"
                >
                  <span class="material-symbols-outlined text-base">edit</span>
                </button>
                <button
                  type="button"
                  on:click={() => duplicateAgent(agent)}
                  title="Nhân bản agent này"
                  class="p-1 rounded-lg text-on-surface-variant hover:text-primary hover:bg-surface-container-high cursor-pointer"
                >
                  <span class="material-symbols-outlined text-base">content_copy</span>
                </button>
                <span class="w-2.5 h-2.5 rounded-full {agent.status === 'running' ? 'bg-primary' : agent.status === 'error' ? 'bg-error' : 'bg-outline'}"></span>
              </div>
            </div>

            <div class="text-xs space-y-3">
              <div class="flex flex-col gap-1">
                <span class="text-xs font-medium text-on-surface-variant">System prompt / persona</span>
                <textarea
                  value={personaDrafts[agent.agent_id] ?? agent.system}
                  on:input={(e) => setPersonaDraft(agent.agent_id, (e.currentTarget as HTMLTextAreaElement).value)}
                  class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2 h-16 resize-none focus:border-primary text-on-surface text-[11px]"
                  placeholder="Nhập system prompt cho agent…"
                ></textarea>
              </div>

              <div class="flex justify-between items-center text-on-surface-variant gap-2">
                <span class="font-medium">Loại agent</span>
                <div class="w-32">
                  <Dropdown
                    options={[
                      { value: 'claude_cli', label: 'Claude CLI' },
                      { value: 'anti_cli', label: 'Anti CLI' }
                    ]}
                    value={agent.provider || 'claude_cli'}
                    on:change={(e) => agent.provider = e.detail}
                  />
                </div>
              </div>

              <div class="flex justify-between items-center text-on-surface-variant gap-2">
                <span class="font-medium">Model</span>
                <div class="w-40">
                  <ModelSelect
                    bind:value={agent.model}
                    provider={agent.provider === 'anti_cli' ? 'anti' : 'claude'}
                  />
                </div>
              </div>

              <div class="flex justify-between text-on-surface-variant mt-2 pt-2 border-t border-outline-variant">
                <span class="font-medium">Token đã dùng</span>
                <span class="font-mono text-on-surface">{agent.tokens_used.toLocaleString()}</span>
              </div>
              <div class="flex justify-end gap-2 pt-2">
                <button on:click={() => handleDeleteAgent(agent)} class="border border-outline-variant text-error px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-error/10 transition-colors cursor-pointer flex items-center gap-1">
                  <span class="material-symbols-outlined text-sm">delete</span> Xoá
                </button>
                <button on:click={() => handleSaveAgent(agent)} class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity cursor-pointer">
                  Lưu
                </button>
              </div>
            </div>
          </div>
        {/each}
      </div>
    </div>
  {:else if subTab === 'cli'}
    <!-- Quick CLI View -->
    <div class="space-y-4">
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-4">
        <div class="flex items-center gap-4">
          <label for="agent-model-select" class="text-xs font-medium text-on-surface-variant whitespace-nowrap">Agent và model</label>
          <div class="flex gap-2 w-full max-w-sm">
            <select id="agent-model-select" bind:value={selectedAgentType}
              on:change={() => selectedModel = selectedAgentType === 'claude' ? 'claude-opus-4-8' : 'gemini-3.1-pro-high'}
              class="w-1/2 bg-surface-container-low border border-outline-variant px-3 py-2 rounded-lg text-xs text-on-surface focus:border-primary">
              <option value="claude">Claude</option>
              <option value="antigravity">Antigravity</option>
            </select>
            <div class="w-1/2">
              <ModelSelect
                bind:value={selectedModel}
                provider={selectedAgentType === 'claude' ? 'claude' : 'anti'}
              />
            </div>
          </div>
        </div>

        <!-- CLI Window Visibility Toggle Switch -->
        <div class="flex items-center justify-between gap-3 p-3 bg-surface-container-low rounded-xl border border-outline-variant">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-sm text-on-surface-variant">terminal</span>
            <div>
              <div class="text-xs font-medium text-on-surface">Hiển thị cửa sổ CMD khi chạy CLI</div>
              <div class="text-[11px] text-on-surface-variant">
                {showCLIConsole ? 'Đang bật — mở cửa sổ CMD để xem CLI làm việc trực tiếp.' : 'Đang tắt — chạy ngầm, không chiếm màn hình.'}
              </div>
            </div>
          </div>
          <button
            type="button"
            on:click={async () => {
              showCLIConsole = !showCLIConsole;
              if ((AppBindings as any).SetShowCLIConsole) {
                await (AppBindings as any).SetShowCLIConsole(showCLIConsole);
                addLog(`Chế độ cửa sổ CLI: ${showCLIConsole ? 'hiện cửa sổ CMD' : 'chạy ngầm'}.`, 'INFO');
              }
            }}
            class="px-3 py-1.5 rounded-lg text-xs font-medium transition-colors cursor-pointer flex items-center gap-1.5 flex-shrink-0 {showCLIConsole ? 'bg-primary text-on-primary hover:opacity-90' : 'border border-outline-variant text-on-surface-variant hover:bg-surface-container-high'}"
          >
            <span class="material-symbols-outlined text-sm">{showCLIConsole ? 'visibility' : 'visibility_off'}</span>
            {showCLIConsole ? 'Bật (hiện CMD)' : 'Tắt (ẩn ngầm)'}
          </button>
        </div>

        <textarea
          bind:value={quickPrompt}
          on:keydown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              handleRunQuickCLI();
            }
          }}
          class="w-full bg-surface-container-low h-32 rounded-xl p-4 font-mono text-xs text-on-surface border border-outline-variant focus:border-primary resize-none"
          placeholder="Nhập prompt chạy thử CLI trực tiếp… (Nhấn Enter để gửi, Shift+Enter xuống dòng)"
        ></textarea>

        <button on:click={handleRunQuickCLI} disabled={isQuickRunning}
          class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm {isQuickRunning ? 'animate-spin' : ''}">{isQuickRunning ? 'progress_activity' : 'play_arrow'}</span>
          {isQuickRunning ? 'Đang chạy…' : 'Chạy CLI trực tiếp'}
        </button>
      </div>

      {#if quickOutput}
        <div class="bg-surface-container-highest border border-outline-variant text-on-surface p-4 rounded-xl font-mono text-xs max-h-80 overflow-y-auto whitespace-pre-wrap">
          {quickOutput}
        </div>
      {/if}
    </div>
  {:else if subTab === 'logs'}
    <!-- Live Log View -->
    <div class="bg-surface-container-highest border border-outline-variant text-on-surface p-4 rounded-xl font-mono text-xs h-[480px] overflow-y-auto space-y-1">
      {#each $logs as log}
        <div class="flex gap-2">
          <span class="text-on-surface-variant">[{log.time}]</span>
          <span class="font-medium {log.level === 'ERROR' ? 'text-error' : log.level === 'WARN' ? 'text-warning' : log.level === 'SUCCESS' ? 'text-success' : 'text-on-surface-variant'}">{log.level}</span>
          <span class="text-on-surface">{log.message}</span>
        </div>
      {:else}
        <div class="text-on-surface-variant italic">Chưa có log entry nào.</div>
      {/each}
    </div>
  {:else if subTab === 'updates'}
    <!-- Info & Updates View -->
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 text-center space-y-4 max-w-xl mx-auto">
      <h3 class="font-semibold text-sm text-on-surface flex items-center justify-center gap-2">
        <span class="material-symbols-outlined text-base text-on-surface-variant">system_update</span>
        Claude Suite Control Center
      </h3>
      <p class="text-xs text-on-surface-variant">Phiên bản hiện tại: <span class="font-mono text-on-surface">{currentAppVersion}</span> (Go + Wails + Svelte)</p>

      <button
        on:click={handleCheckUpdate}
        disabled={isCheckingUpdate || isUpdating}
        class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer flex items-center justify-center gap-1.5 mx-auto"
      >
        <span class="material-symbols-outlined text-sm {isCheckingUpdate ? 'animate-spin' : ''}">refresh</span>
        {isCheckingUpdate ? 'Đang kiểm tra…' : 'Kiểm tra bản cập nhật'}
      </button>

      {#if updateStatusMessage}
        <div class="p-3 rounded-lg text-xs font-medium text-center
          {updateStatusType === 'error' ? 'bg-error/10 text-error border border-error/20' :
           updateStatusType === 'success' ? 'bg-success/10 text-success border border-success/20' :
           'bg-surface-container-high text-on-surface-variant border border-outline-variant'}"
        >
          {updateStatusMessage}
        </div>
      {/if}

      {#if isUpdating}
        <div class="flex items-center justify-center gap-2 pt-2 text-[11px] text-on-surface-variant">
          <span class="material-symbols-outlined text-sm animate-spin">progress_activity</span>
          Vui lòng không tắt ứng dụng trong quá trình cài đặt…
        </div>
      {/if}

      {#if updateInfo}
        <div class="p-4 bg-surface-container-low rounded-xl text-xs text-left space-y-3">
          {#if updateInfo.has_update}
            <p class="text-on-surface font-semibold mb-1">Có phiên bản mới: {updateInfo.version}</p>
            <p class="text-on-surface-variant mb-3">{updateInfo.body}</p>
            <button
              on:click={handleAutoUpdate}
              disabled={isUpdating}
              class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer flex items-center gap-1.5"
            >
              <span class="material-symbols-outlined text-sm {isUpdating ? 'animate-spin' : ''}">{isUpdating ? 'progress_activity' : 'download'}</span>
              {isUpdating ? 'Đang tải và cài đặt…' : 'Tự động cập nhật ngay'}
            </button>
          {:else if !updateStatusMessage}
            <p class="text-success font-medium">Bạn đang sử dụng phiên bản mới nhất!</p>
          {/if}
        </div>
      {/if}
    </div>
  {:else if subTab === 'integrations'}
    <!-- Daily spending cap -->
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 space-y-4 max-w-2xl mx-auto mb-4">
      <div class="flex items-center gap-2">
        <span class="material-symbols-outlined text-base text-on-surface-variant">savings</span>
        <h3 class="font-semibold text-sm text-on-surface">Trần chi phí mỗi ngày</h3>
      </div>
      <p class="text-xs text-on-surface-variant">
        Orchestrator tự thử lại task thất bại và lịch có thể khởi động nó lúc 2 giờ sáng. Đặt trần để một lỗi
        chạy lặp thành hàng đợi tạm dừng, thay vì một hoá đơn. Task đang chạy vẫn được chạy nốt.
      </p>

      {#if budget}
        <div class="bg-surface-container-low border border-outline-variant rounded-xl p-3 space-y-2">
          <div class="flex items-center justify-between text-xs">
            <span class="text-on-surface-variant">Đã dùng hôm nay</span>
            <span class="font-mono font-medium text-on-surface">
              ${budget.spent_usd.toFixed(2)}{#if budget.limit_usd > 0} / ${budget.limit_usd.toFixed(2)}{/if}
            </span>
          </div>
          {#if budget.limit_usd > 0}
            {@const pct = Math.min(100, (budget.spent_usd / budget.limit_usd) * 100)}
            <div class="w-full h-1.5 bg-surface-container-highest rounded-full overflow-hidden">
              <div class="h-full rounded-full transition-all duration-500 {pct >= 100 ? 'bg-error' : pct >= 80 ? 'bg-warning' : 'bg-primary'}"
                style="width: {pct}%"></div>
            </div>
            {#if pct >= 100}
              <p class="text-[11px] font-medium text-error">Đã chạm trần — không giao việc mới cho tới ngày mai.</p>
            {/if}
          {:else}
            <p class="text-[11px] text-on-surface-variant">Chưa đặt trần — agent chạy không giới hạn chi phí.</p>
          {/if}
        </div>
      {/if}

      <div class="flex items-end gap-2">
        <div class="flex-1 space-y-1">
          <label for="daily-budget" class="text-xs font-medium text-on-surface-variant">Giới hạn (USD/ngày)</label>
          <input id="daily-budget" type="number" min="0" step="0.5" bind:value={budgetLimit}
            placeholder="0 = không giới hạn"
            class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2.5 text-sm text-on-surface focus:border-primary" />
        </div>
        <button type="button" on:click={saveBudget} disabled={isSavingBudget}
          class="bg-primary text-on-primary px-4 py-2.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer">
          {isSavingBudget ? 'Đang lưu…' : 'Lưu'}
        </button>
      </div>
      <p class="text-[11px] text-on-surface-variant">
        Chỉ Claude CLI báo được chi phí thật (lấy từ usage event của chính nó). Lượt chạy không báo chi phí thì
        tính bằng 0 — trần này canh phần đo được, không đoán.
      </p>
    </div>

    <!-- Integrations View -->
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 space-y-6 max-w-2xl mx-auto">
      <div class="flex items-center gap-2 mb-4">
        <span class="material-symbols-outlined text-base text-on-surface-variant">cable</span>
        <h3 class="font-semibold text-sm text-on-surface">Webhook và MCP</h3>
      </div>

      <div class="space-y-2">
        <label for="global-webhook-url" class="text-xs font-medium text-on-surface-variant">Webhook thông báo (outbound)</label>
        <p class="text-xs text-on-surface-variant">Khi một task hoàn thành hoặc thất bại, hệ thống sẽ POST JSON tới URL này (Slack Incoming Webhook, Discord, hoặc endpoint tuỳ chỉnh).</p>
        <input
          id="global-webhook-url"
          type="text"
          bind:value={webhookUrl}
          placeholder="https://hooks.slack.com/services/..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2.5 text-sm text-on-surface focus:border-primary"
        />
      </div>

      <div class="pt-4 border-t border-outline-variant">
        <MCPManager />
      </div>

      <div class="pt-4 border-t border-outline-variant flex justify-end">
        <button
          on:click={handleSaveIntegrations}
          disabled={isIntegrationsSaving}
          class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer"
        >
          {isIntegrationsSaving ? 'Đang lưu…' : 'Lưu cấu hình'}
        </button>
      </div>


    </div>
  {:else if subTab === 'memory'}
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 space-y-5 max-w-2xl mx-auto">
      <div class="flex items-center gap-2">
        <span class="material-symbols-outlined text-base text-on-surface-variant">psychology</span>
        <h3 class="font-semibold text-sm text-on-surface">Memory và Context Pack</h3>
      </div>
      <p class="text-xs text-on-surface-variant">
        Điều khiển những gì được tiêm vào prompt của sub-agent. Dữ liệu xem tại trang <strong>Memory</strong> (nhóm Giám sát).
      </p>

      <div>
        <label class="text-xs font-medium text-on-surface-variant" for="pack-budget">Ngân sách context pack (ký tự)</label>
        <input id="pack-budget" type="number" min="0" step="500" bind:value={memoryCfg.context_pack_max_chars}
          class="w-full mt-1 px-3 py-2 rounded-lg bg-surface-container-low border border-outline-variant text-sm font-mono text-on-surface focus:border-primary" />
        <p class="text-[11px] text-on-surface-variant mt-1">
          Mặc định 12000 (~3k token). Đặt <strong>0</strong> để tắt hẳn việc tiêm memory vào prompt (kill switch).
        </p>
      </div>

      <label class="flex items-start gap-3 cursor-pointer">
        <input type="checkbox" bind:checked={memoryCfg.auto_summarize} class="mt-0.5 accent-primary cursor-pointer" />
        <span>
          <span class="text-sm font-semibold text-on-surface block">Tự làm mới summaries (LLM)</span>
          <span class="text-[11px] text-on-surface-variant">Sau mỗi task, các file đổi cấu trúc được model rẻ (Haiku) viết lại summary theo lô — nhỏ giọt, có cooldown 10 phút.</span>
        </span>
      </label>

      <label class="flex items-start gap-3 cursor-pointer">
        <input type="checkbox" bind:checked={memoryCfg.session_resume} class="mt-0.5 accent-primary cursor-pointer" />
        <span>
          <span class="text-sm font-semibold text-on-surface block">Session resume cho agent</span>
          <span class="text-[11px] text-on-surface-variant">Ghi SessionID của run thành công về agent để task sau tiếp tục cùng hội thoại CLI (--resume/--conversation). Context hội thoại sẽ lớn dần và CLI tự nén khó đoán — bật khi muốn đo thử.</span>
        </span>
      </label>

      <label class="flex items-start gap-3 cursor-pointer">
        <input type="checkbox" bind:checked={memoryCfg.lesson_promotion} class="mt-0.5 accent-primary cursor-pointer" />
        <span>
          <span class="text-sm font-semibold text-on-surface block">Promote lesson lên global</span>
          <span class="text-[11px] text-on-surface-variant">Lesson xuất hiện ở ≥2 workspace với confidence trung bình ≥0.8 được nhân bản sang mọi workspace. Tắt mặc định: lesson tồi ở phạm vi global gây hại ở mọi nơi cùng lúc.</span>
        </span>
      </label>

      <div class="pt-4 border-t border-outline-variant flex items-center justify-between gap-2">
        <button
          on:click={handleRefreshGeminiModels}
          disabled={isRefreshingModels}
          title="Gọi Generative Language API bằng API key trong pool để lấy danh sách model Gemini mới nhất"
          class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium disabled:opacity-50 cursor-pointer flex items-center gap-1.5 hover:bg-surface-container-high transition-colors"
        >
          <span class="material-symbols-outlined text-sm {isRefreshingModels ? 'animate-spin' : ''}">{isRefreshingModels ? 'progress_activity' : 'cloud_sync'}</span>
          Tải model Gemini từ API
        </button>
        <button
          on:click={handleSaveMemoryCfg}
          disabled={isSavingMemoryCfg}
          class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer flex items-center gap-1.5"
        >
          {#if isSavingMemoryCfg}<span class="material-symbols-outlined text-sm animate-spin">progress_activity</span>{/if}
          {isSavingMemoryCfg ? 'Đang lưu…' : 'Lưu cài đặt memory'}
        </button>
      </div>
    </div>
  {/if}
</div>

<AgentFormModal
  bind:open={agentFormOpen}
  agent={agentBeingEdited}
  existingNames={(agents || []).map((a) => a.name)}
  on:save={handleAgentFormSave}
  on:close={() => (agentFormOpen = false)}
/>
