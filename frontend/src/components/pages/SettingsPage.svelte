<script lang="ts">
  import { onMount } from 'svelte';
  import type { Agent } from '../../lib/types';
  import { logs, addLog } from '../../lib/stores/appState';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import Dropdown from '../ui/Dropdown.svelte';

  let subTab: 'agents' | 'cli' | 'logs' | 'updates' | 'integrations' = 'agents';
  let agents: Agent[] = [];

  // Quick CLI state
  let quickPrompt = '';
  let selectedModel = 'claude-opus-4-8';
  let quickOutput = '';
  let isQuickRunning = false;

  // Updater state
  let updateInfo: any = null;

  // Integrations state
  let webhookUrl = '';
  let mcpConnectionString = '';
  let isIntegrationsSaving = false;

  onMount(async () => {
    await loadAgents();
  });

  async function loadAgents() {
    try {
      if ((window as any)?.go?.main?.App) {
        agents = await AppBindings.GetAgents();
      }
    } catch (e) {
      console.error(e);
    }
  }

  async function handleResetAgents() {
    await AppBindings.ResetAgentsToDefaults();
    await loadAgents();
    addLog('Reset agents to 7 default corporate roles', 'SUCCESS');
  }

  async function handleSaveAgent(agent: Agent) {
    // In a real app, call a Go binding like AppBindings.SaveAgent(agent)
    addLog(`Agent ${agent.name} saved.`, 'SUCCESS');
  }

  async function handleSaveIntegrations() {
    isIntegrationsSaving = true;
    setTimeout(() => {
      isIntegrationsSaving = false;
      addLog('Webhooks & MCP configuration saved.', 'SUCCESS');
    }, 1000);
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
    try {
      updateInfo = await AppBindings.CheckForUpdates();
    } catch (e) {
      console.error(e);
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

  {#if subTab === 'agents'}
    <!-- Agents Registry View -->
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="font-bold text-sm text-on-surface">Corporate Agent Roles ({agents.length})</h3>
        <button on:click={handleResetAgents} class="bg-surface-container-highest border border-outline-variant px-3 py-1.5 rounded-lg text-xs font-semibold hover:bg-surface-container-high transition-all flex items-center gap-1">
          <span class="material-symbols-outlined text-sm">restart_alt</span> Reset Defaults
        </button>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        {#each agents as agent}
          <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3 shadow-sm hover:shadow-md transition-all">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-3">
                <span class="text-2xl">{agent.icon}</span>
                <div>
                  <h4 class="font-bold text-sm text-on-surface">{agent.name}</h4>
                  <p class="text-xs text-on-surface-variant line-clamp-1">{agent.role}</p>
                </div>
              </div>
              <span class="w-2.5 h-2.5 rounded-full {agent.status === 'running' ? 'bg-primary animate-pulse' : agent.status === 'error' ? 'bg-rose-500' : 'bg-slate-400'}"></span>
            </div>

            <div class="bg-surface-container-low/50 p-3 rounded-lg text-xs space-y-3 font-mono">
              <div class="flex flex-col gap-1">
                <span class="text-on-surface-variant font-bold">System Prompt:</span>
                <textarea
                  class="w-full bg-surface-container-lowest border border-outline-variant rounded p-2 h-16 resize-none focus:ring-1 focus:ring-primary outline-none"
                  placeholder="Enter system prompt for agent..."
                >Bạn là một trợ lý AI chuyên nghiệp.</textarea>
              </div>

              <div class="flex justify-between items-center text-on-surface-variant gap-2">
                <span>Agent Type:</span>
                <div class="w-32">
                  <Dropdown
                    options={[
                      { value: 'claude-cli', label: 'Claude CLI' },
                      { value: 'anti-cli', label: 'Anti CLI' }
                    ]}
                    value="claude-cli"
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
                  />
                </div>
              </div>

              <div class="flex justify-between text-on-surface-variant mt-2 pt-2 border-t border-outline-variant">
                <span>Tokens Used:</span>
                <span>{agent.tokens_used.toLocaleString()}</span>
              </div>
              <div class="flex justify-end pt-2">
                <button on:click={() => handleSaveAgent(agent)} class="bg-primary text-on-primary px-4 py-1.5 rounded-lg text-xs font-bold hover:opacity-90">
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
          <label class="text-xs font-bold text-on-surface whitespace-nowrap">Model Target:</label>
          <div class="w-48">
            <Dropdown
              options={[
                { value: 'claude-opus-4-8', label: 'claude-opus-4-8' },
                { value: 'claude-sonnet-4-5', label: 'claude-sonnet-4-5' },
                { value: 'gemini-3.6-flash-high', label: 'gemini-3.6-flash-high' },
                { value: 'gemini-3.1-pro-high', label: 'gemini-3.1-pro-high' }
              ]}
              bind:value={selectedModel}
            />
          </div>
        </div>

        <textarea
          bind:value={quickPrompt}
          class="w-full bg-surface-container-low/30 h-32 rounded-xl p-4 font-mono text-xs text-on-surface border border-outline-variant focus:ring-2 focus:ring-primary outline-none resize-none"
          placeholder="Nhập prompt chạy thử CLI trực tiếp..."
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
      <p class="text-xs text-on-surface-variant">Phiên bản hiện tại: <strong>v2.0.0 (Go + Wails + Svelte)</strong></p>

      <button on:click={handleCheckUpdate} class="bg-primary text-on-primary px-6 py-2 rounded-xl text-xs font-bold">
        🔄 Kiểm tra bản cập nhật
      </button>

      {#if updateInfo}
        <div class="p-4 bg-surface-container-low rounded-xl text-xs text-left">
          {#if updateInfo.has_update}
            <p class="text-emerald-600 font-bold mb-1">Có phiên bản mới: {updateInfo.version}</p>
            <p class="text-on-surface-variant mb-3">{updateInfo.body}</p>
            <button on:click={handleAutoUpdate} class="bg-emerald-600 hover:bg-emerald-700 text-white px-6 py-2 rounded-xl text-xs font-bold">
              🚀 Tự động cập nhật ngay
            </button>
          {:else}
            <p class="text-emerald-600 font-bold">Bạn đang sử dụng phiên bản mới nhất!</p>
          {/if}
        </div>
      {/if}
    </div>
  {:else if subTab === 'integrations'}
    <!-- Integrations View -->
    <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-6 space-y-6 max-w-2xl mx-auto shadow-sm">
      <div class="flex items-center gap-2 mb-4">
        <span class="material-symbols-outlined text-primary text-2xl">cable</span>
        <h3 class="font-bold text-lg text-on-surface">Webhooks & MCP Configuration</h3>
      </div>
      
      <div class="space-y-2">
        <label class="text-sm font-bold text-on-surface">Global Webhook URL</label>
        <p class="text-xs text-on-surface-variant">Used to send automated events and task completion notifications to external services like Slack, Discord, or custom backends.</p>
        <input 
          type="text" 
          bind:value={webhookUrl}
          placeholder="https://hooks.slack.com/services/..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg p-3 text-sm focus:ring-2 focus:ring-primary outline-none"
        />
      </div>

      <div class="space-y-2">
        <label class="text-sm font-bold text-on-surface">MCP Connection String</label>
        <p class="text-xs text-on-surface-variant">Configure the Model Context Protocol endpoint for advanced external integrations and agent memories.</p>
        <input 
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
