<script lang="ts">
  import { onMount } from 'svelte';
  import type { Agent } from '../../lib/types';
  import { logs, addLog, agentsStore } from '../../lib/stores/appState';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import Dropdown from '../ui/Dropdown.svelte';
  import OAuthPoolDashboard from './OAuthPoolDashboard.svelte';

  let subTab: 'agents' | 'oauth_pool' | 'cli' | 'logs' | 'updates' | 'integrations' = 'oauth_pool';
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

  async function initSettings() {
    await loadAgents();
    await loadAntiKeys();
    await loadBudget();
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
        (window as any).runtime.EventsOn("oauth_success", async (data: any) => {
          await loadAntiKeys();
          addLog(`🎉 Tự động đăng nhập và lưu OAuth Token cho ${data?.email || 'Google Account'} thành công!`, 'SUCCESS');
        });
      }
    } catch (e) {}
  }

  onMount(() => {
    initSettings();
    return () => { unsubscribeAgents(); };
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
      } else if ((AppBindings as any).AddAntiAccountKey) {
        await (AppBindings as any).AddAntiAccountKey(newKeyName.trim(), newKeyValue.trim());
        addLog(`Đã thêm Backup API Key '${newKeyName}' vào Anti CLI Pool!`, 'SUCCESS');
      }
      newKeyName = '';
      newKeyValue = '';
      await loadAntiKeys();
    } catch (e) {
      addLog(`Thêm tài khoản thất bại: ${e}`, 'ERROR');
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
          addLog('Auto-seeded 8 default corporate agents.', 'INFO');
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
    addLog(`Reset agents to ${(loaded || []).length} default corporate roles`, 'SUCCESS');
  }

  async function handleSaveAgent(agent: Agent) {
    try {
      await AppBindings.SaveAgent(agent as any);
      addLog(`Đã lưu Agent ${agent.name}.`, 'SUCCESS');
      // agent_updated event refreshes agentsStore globally (3D office, kanban, cockpit).
    } catch (e) {
      addLog(`Lỗi lưu agent: ${e}`, 'ERROR');
    }
  }

  let newAgentName = '';
  let newAgentRole = '';
  async function handleAddAgent() {
    const name = newAgentName.trim();
    if (!name) return;
    const agent: any = {
      agent_id: '',
      name,
      role: newAgentRole.trim() || 'Custom Agent',
      provider: 'claude_cli',
      model: 'claude-sonnet-4-5',
      system: `You are ${name}, ${newAgentRole.trim() || 'a specialized AI agent'}.`,
      icon: 'smart_toy',
      status: 'idle',
      tasks_done: 0,
      tokens_used: 0,
      token_limit: 0,
      token_remaining: 0,
    };
    try {
      await AppBindings.SaveAgent(agent);
      newAgentName = '';
      newAgentRole = '';
      addLog(`Đã tạo Agent mới: ${name}.`, 'SUCCESS');
    } catch (e) {
      addLog(`Lỗi tạo agent: ${e}`, 'ERROR');
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
      addLog(Number(budgetLimit) > 0 ? `Trần chi phí mỗi ngày: $${Number(budgetLimit).toFixed(2)}` : "Đã bỏ trần chi phí mỗi ngày.", "SUCCESS");
    } catch (e) {
      addLog(`Lỗi lưu trần chi phí: ${e}`, "ERROR");
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
    } catch (e) {
      addLog(`Lỗi lưu cấu hình integrations: ${e}`, 'ERROR');
    } finally {
      isIntegrationsSaving = false;
    }
  }

  async function handleRunQuickCLI() {
    if (!quickPrompt.trim()) return;
    isQuickRunning = true;
    quickOutput = '⏳ Running CLI...';
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
      return;
    }

    isUpdating = true;
    updateStatusMessage = '⬇️ Đang tải bản cập nhật từ GitHub...';
    updateStatusType = 'info';
    addLog('Downloading update from ' + updateInfo.download_url, 'INFO');

    try {
      // Show installing message before calling backend
      // App will call os.Exit(0) during this → IPC cut → catch fires. That is NORMAL.
      updateStatusMessage = '⚙️ Đang cài đặt... Ứng dụng sẽ tự động khởi động lại sau vài giây.';
      updateStatusType = 'success';

      const res = await (AppBindings as any).DownloadAndUpdate(updateInfo.download_url);
      // If we somehow get a response (unlikely), check for error
      if (res && !res.success) {
        updateStatusMessage = '❌ Cập nhật thất bại: ' + (res?.error || 'Không xác định');
        updateStatusType = 'error';
        addLog('Update failed: ' + (res?.error || 'Unknown error'), 'ERROR');
        isUpdating = false;
      }
    } catch (e: any) {
      // App called os.Exit(0) → IPC dropped → catch fires. This is expected.
      // The updater.bat is running in background and will relaunch the app.
      updateStatusMessage = '✅ Đã tải xong! Ứng dụng đang được khởi động lại tự động...';
      updateStatusType = 'success';
      addLog('App is restarting with new version. Please wait...', 'SUCCESS');
      // Do NOT set isUpdating = false — app is mid-restart
    }
  }

</script>

<div class="space-y-6 max-w-7xl mx-auto pb-12">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-bold flex items-center gap-2 text-on-surface">
        <span class="material-symbols-outlined text-primary">settings</span>
        Studio & Settings — Cài đặt hệ thống
      </h1>
      <p class="text-on-surface-variant text-sm mt-0.5">Manage agents, run quick CLI, view system logs & updates.</p>
    </div>
  </div>

  <!-- Sub-Tabs Navigation -->
  <div class="flex justify-center">
    <div class="bg-surface-container-high p-1 rounded-xl flex gap-1 border border-outline-variant">
      <button
        on:click={() => (subTab = 'oauth_pool')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all flex items-center gap-1.5
        {subTab === 'oauth_pool' ? 'bg-primary text-on-primary shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        <span class="material-symbols-outlined text-sm">key</span> Multi-Account OAuth Pool
      </button>
      <button
        on:click={() => (subTab = 'agents')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all
        {subTab === 'agents' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Agents Registry
      </button>
      <button
        on:click={() => (subTab = 'cli')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all
        {subTab === 'cli' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Quick CLI
      </button>
      <button
        on:click={() => (subTab = 'logs')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all
        {subTab === 'logs' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Live System Log
      </button>
      <button
        on:click={() => (subTab = 'updates')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all
        {subTab === 'updates' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Info & Updates
      </button>
      <button
        on:click={() => (subTab = 'integrations')}
        class="px-5 py-1.5 text-xs font-bold rounded-lg transition-all
        {subTab === 'integrations' ? 'bg-surface-container-lowest text-on-surface shadow-sm' : 'text-on-surface-variant hover:text-on-surface'}"
      >
        Integrations (Webhook/MCP)
      </button>
    </div>
  </div>

  {#if subTab === 'oauth_pool'}
    <OAuthPoolDashboard onOpenGoogleLogin={handleOpenGoogleLogin} />
  {:else if subTab === 'agents'}
    <!-- Agents Registry View -->
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="font-bold text-sm text-on-surface">Corporate Agent Roles ({agents.length})</h3>
        <button on:click={handleResetAgents} class="bg-surface-container-highest border border-outline-variant px-3 py-1.5 rounded-lg text-xs font-semibold hover:bg-surface-container-high transition-all flex items-center gap-1">
          <span class="material-symbols-outlined text-sm">restart_alt</span> Reset Defaults
        </button>
      </div>

      <!-- Add new agent (persists + syncs to 3D office, Kanban, Cockpit) -->
      <div class="flex flex-wrap items-center gap-2 bg-surface-container-low/50 border border-outline-variant rounded-xl p-3">
        <span class="material-symbols-outlined text-primary text-lg">person_add</span>
        <input bind:value={newAgentName} placeholder="Tên agent mới..."
          on:keydown={(e) => { if (e.key === 'Enter') handleAddAgent(); }}
          class="flex-1 min-w-[140px] bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-1.5 text-xs text-on-surface outline-none focus:border-primary" />
        <input bind:value={newAgentRole} placeholder="Vai trò (role)..."
          on:keydown={(e) => { if (e.key === 'Enter') handleAddAgent(); }}
          class="flex-1 min-w-[140px] bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-1.5 text-xs text-on-surface outline-none focus:border-primary" />
        <button on:click={handleAddAgent} disabled={!newAgentName.trim()}
          class="bg-primary text-on-primary px-4 py-1.5 rounded-lg text-xs font-bold hover:opacity-90 transition-all disabled:opacity-40 cursor-pointer flex items-center gap-1">
          <span class="material-symbols-outlined text-sm">add</span> Tạo Agent
        </button>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        {#each agents as agent}
          <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3 shadow-sm hover:shadow-md transition-all">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-3">
                <span class="material-symbols-outlined text-2xl text-primary">{agent.icon || 'smart_toy'}</span>
                <div>
                  <h4 class="font-bold text-sm text-on-surface">{agent.name}</h4>
                  <p class="text-xs text-on-surface-variant line-clamp-1">{agent.role}</p>
                </div>
              </div>
              <span class="w-2.5 h-2.5 rounded-full {agent.status === 'running' ? 'bg-primary animate-pulse' : agent.status === 'error' ? 'bg-rose-500' : 'bg-slate-400'}"></span>
            </div>

            <div class="bg-surface-container-low/50 p-3 rounded-lg text-xs space-y-3 font-mono">
              <div class="flex flex-col gap-1">
                <div class="flex items-center justify-between">
                  <span class="text-on-surface-variant font-bold">System Prompt / Persona:</span>
                  <button 
                    type="button"
                    on:click={() => handleSaveAgent(agent)}
                    class="text-[10px] bg-primary text-on-primary px-2 py-0.5 rounded font-bold hover:opacity-90 transition-all cursor-pointer">
                    Lưu Agent
                  </button>
                </div>
                <textarea
                  bind:value={agent.system}
                  class="w-full bg-surface-container-lowest border border-outline-variant rounded p-2 h-16 resize-none focus:ring-1 focus:ring-primary outline-none text-on-surface text-[11px]"
                  placeholder="Enter system prompt for agent..."
                ></textarea>
              </div>

              <div class="flex justify-between items-center text-on-surface-variant gap-2">
                <span>Agent Type:</span>
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
                <span>Model:</span>
                <div class="w-32">
                  <Dropdown
                    options={[
                      { value: 'claude-opus-4-8', label: 'Opus 4.8' },
                      { value: 'claude-sonnet-4-5', label: 'Sonnet 4.5' },
                      { value: 'gemini-3.6-flash-high', label: 'Gemini Flash' }
                    ]}
                    value={agent.model}
                    on:change={(e) => agent.model = e.detail}
                  />
                </div>
              </div>

              <div class="flex justify-between text-on-surface-variant mt-2 pt-2 border-t border-outline-variant">
                <span>Tokens Used:</span>
                <span>{agent.tokens_used.toLocaleString()}</span>
              </div>
              <div class="flex justify-end gap-2 pt-2">
                <button on:click={() => handleDeleteAgent(agent)} class="bg-rose-500/10 text-rose-600 border border-rose-500/30 px-3 py-1.5 rounded-lg text-xs font-bold hover:bg-rose-500/20 transition-all cursor-pointer flex items-center gap-1">
                  <span class="material-symbols-outlined text-sm">delete</span> Xóa
                </button>
                <button on:click={() => handleSaveAgent(agent)} class="bg-primary text-on-primary px-4 py-1.5 rounded-lg text-xs font-bold hover:opacity-90 cursor-pointer">
                  Save
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
      <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-4 shadow-sm">
        <div class="flex items-center gap-4">
          <label for="agent-model-select" class="text-xs font-bold text-on-surface whitespace-nowrap">Agent & Model:</label>
          <div class="flex gap-2 w-full max-w-sm">
            <select id="agent-model-select" bind:value={selectedAgentType} 
              on:change={() => selectedModel = selectedAgentType === 'claude' ? 'claude-opus-4-8' : 'gemini-3.1-pro-high'}
              class="w-1/2 bg-surface-container-low border border-outline-variant p-2 rounded-lg text-xs outline-none focus:border-primary">
              <option value="claude">Claude</option>
              <option value="antigravity">Antigravity</option>
            </select>
            <select bind:value={selectedModel} 
              class="w-1/2 bg-surface-container-low border border-outline-variant p-2 rounded-lg text-xs outline-none focus:border-primary">
              {#if selectedAgentType === 'claude'}
                <option value="claude-opus-4-8">Opus 4.8</option>
                <option value="claude-sonnet-4-5">Sonnet 4.5</option>
                <option value="claude-haiku-4-5">Haiku 4.5</option>
                <option value="fable-5">Fable 5</option>
              {:else}
                <option value="gemini-3.6-flash-high">Gemini 3.6 Flash (High)</option>
                <option value="gemini-3.6-flash-medium">Gemini 3.6 Flash (Medium)</option>
                <option value="gemini-3.6-flash-low">Gemini 3.6 Flash (Low)</option>
                <option value="gemini-3.5-flash-high">Gemini 3.5 Flash (High)</option>
                <option value="gemini-3.1-pro-high">Gemini 3.1 Pro (High)</option>
                <option value="gpt-oss-120b">GPT-OSS 120B (Medium)</option>
              {/if}
            </select>
          </div>
        </div>

        <!-- CLI Window Visibility Toggle Switch -->
        <div class="flex items-center justify-between p-3 bg-surface-container-low/50 rounded-xl border border-outline-variant/60">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-primary text-sm">terminal</span>
            <div>
              <div class="text-xs font-bold text-on-surface">Hiển thị Cửa Sổ CMD Khi Chạy CLI (Console Window)</div>
              <div class="text-[11px] text-on-surface-variant">
                {showCLIConsole ? '🟢 ĐANG BẬT: Mở cửa sổ CMD để xem CLI làm việc trực tiếp' : '⚫ ĐANG TẮT: Chạy ngầm 100% không làm phiền màn hình'}
              </div>
            </div>
          </div>
          <button
            type="button"
            on:click={async () => {
              showCLIConsole = !showCLIConsole;
              if ((AppBindings as any).SetShowCLIConsole) {
                await (AppBindings as any).SetShowCLIConsole(showCLIConsole);
                addLog(`Chế độ cửa sổ CLI: ${showCLIConsole ? 'HIỆN (BẬT CMD)' : 'ẨN (TẮT CMD)'}`, 'INFO');
              }
            }}
            class="px-4 py-1.5 rounded-full text-xs font-bold transition-all flex items-center gap-1.5 {showCLIConsole ? 'bg-emerald-600 text-white shadow-md' : 'bg-surface-container-highest text-on-surface-variant border border-outline-variant'}"
          >
            <span class="material-symbols-outlined text-sm">{showCLIConsole ? 'visibility' : 'visibility_off'}</span>
            {showCLIConsole ? 'BẬT (Hiện CMD)' : 'TẮT (Ẩn ngầm)'}
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
          class="w-full bg-surface-container-low/30 h-32 rounded-xl p-4 font-mono text-xs text-on-surface border border-outline-variant focus:ring-2 focus:ring-primary outline-none resize-none"
          placeholder="Nhập prompt chạy thử CLI trực tiếp... (Nhấn Enter để gửi, Shift+Enter xuống dòng)"
        ></textarea>

        <button on:click={handleRunQuickCLI} disabled={isQuickRunning} class="bg-primary text-on-primary px-6 py-2 rounded-xl text-xs font-bold disabled:opacity-50">
          {isQuickRunning ? 'Đang chạy...' : '▶ Chạy CLI Trực Tiếp'}
        </button>
      </div>

      {#if quickOutput}
        <div class="bg-slate-900 text-slate-100 p-4 rounded-xl font-mono text-xs max-h-80 overflow-y-auto whitespace-pre-wrap">
          {quickOutput}
        </div>
      {/if}
    </div>
  {:else if subTab === 'logs'}
    <!-- Live Log View -->
    <div class="bg-slate-900 text-slate-100 p-4 rounded-xl font-mono text-xs h-[480px] overflow-y-auto space-y-1">
      {#each $logs as log}
        <div class="flex gap-2">
          <span class="text-slate-400">[{log.time}]</span>
          <span class="font-bold">{log.level}</span>
          <span class="text-slate-200">{log.message}</span>
        </div>
      {:else}
        <div class="text-slate-500 italic">Chưa có log entry nào.</div>
      {/each}
    </div>
  {:else if subTab === 'updates'}
    <!-- Info & Updates View -->
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 text-center space-y-4 max-w-xl mx-auto shadow-sm">
      <div class="w-12 h-12 bg-primary-container text-on-primary-container rounded-full flex items-center justify-center mx-auto">
        <span class="material-symbols-outlined text-2xl">system_update</span>
      </div>
      <h3 class="font-bold text-lg text-on-surface">Claude Suite Control Center</h3>
      <p class="text-xs text-on-surface-variant">Phiên bản hiện tại: <strong>{currentAppVersion} (Go + Wails + Svelte)</strong></p>

      <button 
        on:click={handleCheckUpdate} 
        disabled={isCheckingUpdate || isUpdating}
        class="bg-primary text-on-primary px-6 py-2 rounded-xl text-xs font-bold disabled:opacity-50 flex items-center justify-center gap-2 mx-auto transition-all"
      >
        {#if isCheckingUpdate}
          <span class="material-symbols-outlined text-sm animate-spin">refresh</span>
          Đang kiểm tra...
        {:else}
          🔄 Kiểm tra bản cập nhật
        {/if}
      </button>

      {#if updateStatusMessage}
        <div class="p-3 rounded-xl text-xs font-semibold text-center transition-all
          {updateStatusType === 'error' ? 'bg-red-500/10 text-red-500 border border-red-500/20' : 
           updateStatusType === 'success' ? 'bg-emerald-500/10 text-emerald-500 border border-emerald-500/20' : 
           'bg-blue-500/10 text-blue-500 border border-blue-500/20'}"
        >
          {updateStatusMessage}
        </div>
      {/if}

      {#if isUpdating}
        <div class="space-y-2 pt-2">
          <div class="w-full bg-surface-container-high h-2 rounded-full overflow-hidden">
            <div class="bg-primary h-full animate-pulse w-full"></div>
          </div>
          <p class="text-[11px] text-on-surface-variant animate-pulse">Vui lòng không tắt ứng dụng trong quá trình cài đặt...</p>
        </div>
      {/if}

      {#if updateInfo}
        <div class="p-4 bg-surface-container-low rounded-xl text-xs text-left space-y-3">
          {#if updateInfo.has_update}
            <p class="text-emerald-600 font-bold mb-1">Có phiên bản mới: {updateInfo.version}</p>
            <p class="text-on-surface-variant mb-3">{updateInfo.body}</p>
            <button 
              on:click={handleAutoUpdate} 
              disabled={isUpdating}
              class="bg-emerald-600 hover:bg-emerald-700 disabled:opacity-50 text-white px-6 py-2 rounded-xl text-xs font-bold flex items-center gap-2 transition-all"
            >
              {#if isUpdating}
                <span class="material-symbols-outlined text-sm animate-spin">sync</span>
                Đang tải & cài đặt...
              {:else}
                🚀 Tự động cập nhật ngay
              {/if}
            </button>
          {:else if !updateStatusMessage}
            <p class="text-emerald-600 font-bold">Bạn đang sử dụng phiên bản mới nhất!</p>
          {/if}
        </div>
      {/if}
    </div>
  {:else if subTab === 'integrations'}
    <!-- Daily spending cap -->
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 space-y-4 max-w-2xl mx-auto shadow-sm mb-4">
      <div class="flex items-center gap-2">
        <span class="material-symbols-outlined text-primary text-2xl">savings</span>
        <h3 class="font-bold text-lg text-on-surface">Trần chi phí mỗi ngày</h3>
      </div>
      <p class="text-xs text-on-surface-variant">
        Orchestrator tự thử lại task thất bại và lịch có thể khởi động nó lúc 2 giờ sáng. Đặt trần để một lỗi
        chạy lặp thành hàng đợi tạm dừng, thay vì một hoá đơn. Task đang chạy vẫn được chạy nốt.
      </p>

      {#if budget}
        <div class="bg-surface-container-low border border-outline-variant rounded-xl p-3 space-y-2">
          <div class="flex items-center justify-between text-xs">
            <span class="text-on-surface-variant">Đã dùng hôm nay</span>
            <span class="font-mono font-bold text-on-surface">
              ${budget.spent_usd.toFixed(2)}{#if budget.limit_usd > 0} / ${budget.limit_usd.toFixed(2)}{/if}
            </span>
          </div>
          {#if budget.limit_usd > 0}
            {@const pct = Math.min(100, (budget.spent_usd / budget.limit_usd) * 100)}
            <div class="w-full h-1.5 bg-surface-container-highest rounded-full overflow-hidden">
              <div class="h-full rounded-full transition-all duration-500 {pct >= 100 ? 'bg-rose-500' : pct >= 80 ? 'bg-amber-500' : 'bg-emerald-500'}"
                style="width: {pct}%"></div>
            </div>
            {#if pct >= 100}
              <p class="text-[11px] font-bold text-rose-600">Đã chạm trần — không giao việc mới cho tới ngày mai.</p>
            {/if}
          {:else}
            <p class="text-[11px] text-on-surface-variant">Chưa đặt trần — agent chạy không giới hạn chi phí.</p>
          {/if}
        </div>
      {/if}

      <div class="flex items-end gap-2">
        <div class="flex-1 space-y-1">
          <label for="daily-budget" class="text-xs font-bold text-on-surface">Giới hạn (USD/ngày)</label>
          <input id="daily-budget" type="number" min="0" step="0.5" bind:value={budgetLimit}
            placeholder="0 = không giới hạn"
            class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-2.5 text-sm text-on-surface outline-none focus:border-primary" />
        </div>
        <button type="button" on:click={saveBudget} disabled={isSavingBudget}
          class="bg-primary text-on-primary px-5 py-2.5 rounded-lg text-xs font-bold hover:opacity-90 disabled:opacity-50 cursor-pointer">
          {isSavingBudget ? 'Đang lưu...' : 'Lưu'}
        </button>
      </div>
      <p class="text-[11px] text-on-surface-variant">
        Chỉ Claude CLI báo được chi phí thật (lấy từ usage event của chính nó). Lượt chạy không báo chi phí thì
        tính bằng 0 — trần này canh phần đo được, không đoán.
      </p>
    </div>

    <!-- Integrations View -->
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 space-y-6 max-w-2xl mx-auto shadow-sm">
      <div class="flex items-center gap-2 mb-4">
        <span class="material-symbols-outlined text-primary text-2xl">cable</span>
        <h3 class="font-bold text-lg text-on-surface">Webhooks & MCP Configuration</h3>
      </div>
      
      <div class="space-y-2">
        <label for="global-webhook-url" class="text-sm font-bold text-on-surface">Outbound Notification Webhook</label>
        <p class="text-xs text-on-surface-variant">Khi một task hoàn thành hoặc thất bại, hệ thống sẽ POST JSON tới URL này (Slack Incoming Webhook, Discord, hoặc endpoint tùy chỉnh).</p>
        <input 
          id="global-webhook-url"
          type="text" 
          bind:value={webhookUrl}
          placeholder="https://hooks.slack.com/services/..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-3 text-sm focus:ring-2 focus:ring-primary outline-none"
        />
      </div>

      <div class="space-y-2">
        <label for="mcp-connection-string" class="text-sm font-bold text-on-surface">MCP Connection String</label>
        <p class="text-xs text-on-surface-variant">Configure the Model Context Protocol endpoint for advanced external integrations and agent memories.</p>
        <input 
          id="mcp-connection-string"
          type="text" 
          bind:value={mcpConnectionString}
          placeholder="mcp://localhost:8000/v1"
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-3 text-sm focus:ring-2 focus:ring-primary outline-none font-mono"
        />
      </div>

      <div class="pt-4 border-t border-outline-variant flex justify-end">
        <button 
          on:click={handleSaveIntegrations} 
          disabled={isIntegrationsSaving}
          class="bg-primary text-on-primary px-6 py-2 rounded-xl text-sm font-bold disabled:opacity-50"
        >
          {isIntegrationsSaving ? 'Saving...' : 'Save Configuration'}
        </button>
      </div>


    </div>
  {/if}
</div>
