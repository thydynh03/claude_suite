<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addToast, addLog } from '../../lib/stores/appState';

  /**
   * Manage the MCP servers the sub-agents may use.
   *
   * This replaced a single free-text "MCP Connection String" box: one field, no
   * list, no validation, and no way to find out whether what you typed worked.
   * A wrong value produced no error here and none later either — agents simply
   * ran without the tools you thought you had given them.
   */

  let servers: any[] = [];
  let catalogue: any[] = [];
  let busyId = '';
  let showCustom = false;

  let customName = '';
  let customCommand = '';
  let customArgs = '';
  let customUrl = '';
  let customDesc = '';
  let mode: 'command' | 'url' = 'command';

  onMount(load);

  async function load() {
    try {
      catalogue = (await (AppBindings as any).GetMCPCatalogue()) || [];
      servers = (await (AppBindings as any).GetMCPServers()) || [];
    } catch (e) {
      addLog(`Không tải được danh sách MCP: ${e}`, 'ERROR');
      addToast(`Không tải được danh sách MCP: ${e}`, 'ERROR');
    }
  }

  $: installedNames = new Set(servers.map((s) => (s.name || '').toLowerCase()));

  async function addFromCatalogue(entry: any) {
    // An MCP server runs commands on this machine with the user's permissions,
    // so adding one is always a decision they make explicitly.
    if (!confirm(`Thêm "${entry.name}"?\n\nMCP server chạy lệnh trên máy bạn với quyền của bạn:\n${entry.command} ${(entry.args || []).join(' ')}`)) {
      return;
    }
    busyId = entry.id;
    try {
      await (AppBindings as any).AddMCPServer({ ...entry, id: '', enabled: true });
      await load();
      addToast(`Đã thêm MCP server "${entry.name}".`, 'SUCCESS');
    } catch (e) {
      addToast(`Không thêm được: ${e}`, 'ERROR');
    } finally {
      busyId = '';
    }
  }

  async function addCustom() {
    const server: any = {
      id: '',
      name: customName.trim(),
      description: customDesc.trim(),
      enabled: true,
      builtin: false,
      args: [],
      env: {},
      command: '',
      url: '',
    };

    if (mode === 'command') {
      server.command = customCommand.trim();
      // Split on whitespace: quoting rules would be a source of silent
      // misconfiguration, and anything needing them belongs in a wrapper script.
      server.args = customArgs.trim() ? customArgs.trim().split(/\s+/) : [];
    } else {
      server.url = customUrl.trim();
    }

    try {
      await (AppBindings as any).AddMCPServer(server);
      await load();
      customName = customCommand = customArgs = customUrl = customDesc = '';
      showCustom = false;
      addToast(`Đã thêm MCP server "${server.name}".`, 'SUCCESS');
    } catch (e) {
      addToast(`Không thêm được: ${e}`, 'ERROR', 8000);
    }
  }

  async function test(server: any) {
    busyId = server.id;
    try {
      const msg = await (AppBindings as any).TestMCPServer(server.id);
      await load();
      addToast(`${server.name}: ${msg}`, 'SUCCESS', 7000);
    } catch (e) {
      await load();
      addToast(`${server.name}: ${e}`, 'ERROR', 9000);
    } finally {
      busyId = '';
    }
  }

  async function toggle(server: any) {
    try {
      await (AppBindings as any).SetMCPServerEnabled(server.id, !server.enabled);
      await load();
    } catch (e) {
      addToast(`Không đổi được trạng thái: ${e}`, 'ERROR');
    }
  }

  async function remove(server: any) {
    if (!confirm(`Xoá MCP server "${server.name}"?`)) return;
    try {
      await (AppBindings as any).RemoveMCPServer(server.id);
      await load();
      addToast(`Đã xoá "${server.name}".`, 'INFO');
    } catch (e) {
      addToast(`Không xoá được: ${e}`, 'ERROR');
    }
  }

  function searchOnline() {
    (AppBindings as any).OpenURLInBrowser('https://github.com/modelcontextprotocol/servers');
  }
</script>

<div class="space-y-5">
  <div class="flex items-center justify-between">
    <div>
      <h4 class="text-sm font-bold text-on-surface">MCP Servers</h4>
      <p class="text-xs text-on-surface-variant mt-0.5">
        Công cụ mà sub-agent được dùng thêm khi chạy task.
      </p>
    </div>
    <div class="flex items-center gap-2">
      <button
        type="button"
        on:click={searchOnline}
        class="px-3 py-1.5 rounded-lg text-xs font-bold border border-outline-variant text-on-surface-variant hover:bg-surface-container-high cursor-pointer flex items-center gap-1.5"
      >
        <span class="material-symbols-outlined text-sm">travel_explore</span> Tìm thêm trên web
      </button>
      <button
        type="button"
        on:click={() => (showCustom = !showCustom)}
        class="px-3 py-1.5 rounded-lg text-xs font-bold bg-primary text-on-primary cursor-pointer flex items-center gap-1.5"
      >
        <span class="material-symbols-outlined text-sm">add</span> Tự thêm
      </button>
    </div>
  </div>

  {#if showCustom}
    <div class="border border-outline-variant rounded-xl p-4 space-y-3 bg-surface-container-low/40">
      <div class="flex items-center gap-2">
        <button
          type="button"
          on:click={() => (mode = 'command')}
          class="px-2.5 py-1 rounded-lg text-[11px] font-bold cursor-pointer {mode === 'command' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:bg-surface-container-high'}"
        >
          Lệnh (stdio)
        </button>
        <button
          type="button"
          on:click={() => (mode = 'url')}
          class="px-2.5 py-1 rounded-lg text-[11px] font-bold cursor-pointer {mode === 'url' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:bg-surface-container-high'}"
        >
          URL (HTTP)
        </button>
      </div>

      <input
        bind:value={customName}
        placeholder="Tên hiển thị"
        class="w-full bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-2 text-xs outline-none focus:border-primary"
      />
      <input
        bind:value={customDesc}
        placeholder="Mô tả ngắn (tuỳ chọn)"
        class="w-full bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-2 text-xs outline-none focus:border-primary"
      />

      {#if mode === 'command'}
        <div class="grid grid-cols-1 md:grid-cols-2 gap-2">
          <input
            bind:value={customCommand}
            placeholder="Lệnh, VD: npx"
            class="bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-2 text-xs font-mono outline-none focus:border-primary"
          />
          <input
            bind:value={customArgs}
            placeholder="Tham số, VD: -y @scope/server"
            class="bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-2 text-xs font-mono outline-none focus:border-primary"
          />
        </div>
      {:else}
        <input
          bind:value={customUrl}
          placeholder="https://host/mcp"
          class="w-full bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-2 text-xs font-mono outline-none focus:border-primary"
        />
      {/if}

      <div class="flex justify-end gap-2">
        <button
          type="button"
          on:click={() => (showCustom = false)}
          class="px-3 py-1.5 rounded-lg text-xs font-bold text-on-surface-variant hover:bg-surface-container-high cursor-pointer"
        >
          Huỷ
        </button>
        <button
          type="button"
          on:click={addCustom}
          class="px-3 py-1.5 rounded-lg text-xs font-bold bg-primary text-on-primary cursor-pointer"
        >
          Thêm
        </button>
      </div>
    </div>
  {/if}

  <!-- Installed -->
  {#if servers.length > 0}
    <div class="space-y-2">
      {#each servers as srv (srv.id)}
        <div class="flex items-center gap-3 border border-outline-variant rounded-xl px-3 py-2.5">
          <button
            type="button"
            on:click={() => toggle(srv)}
            title={srv.enabled ? 'Đang bật' : 'Đang tắt'}
            class="material-symbols-outlined text-xl cursor-pointer {srv.enabled ? 'text-emerald-500' : 'text-outline'}"
          >
            {srv.enabled ? 'toggle_on' : 'toggle_off'}
          </button>

          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span class="text-xs font-bold text-on-surface truncate">{srv.name}</span>
              {#if srv.last_status === 'ok'}
                <span class="text-[9px] font-bold px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-600 dark:text-emerald-400">ĐÃ KIỂM TRA</span>
              {:else if srv.last_status === 'error'}
                <span class="text-[9px] font-bold px-1.5 py-0.5 rounded bg-red-500/15 text-red-600 dark:text-red-400">LỖI</span>
              {:else}
                <span class="text-[9px] font-bold px-1.5 py-0.5 rounded bg-surface-container-high text-on-surface-variant">CHƯA KIỂM TRA</span>
              {/if}
            </div>
            <p class="text-[10px] text-on-surface-variant font-mono truncate">
              {srv.url || `${srv.command} ${(srv.args || []).join(' ')}`}
            </p>
            {#if srv.last_error}
              <p class="text-[10px] text-red-500 mt-0.5">{srv.last_error}</p>
            {/if}
          </div>

          <button
            type="button"
            on:click={() => test(srv)}
            disabled={busyId === srv.id}
            class="px-2.5 py-1 rounded-lg text-[11px] font-bold border border-outline-variant hover:bg-surface-container-high disabled:opacity-50 cursor-pointer"
          >
            {busyId === srv.id ? 'Đang kiểm tra…' : 'Kiểm tra'}
          </button>
          <button
            type="button"
            on:click={() => remove(srv)}
            class="p-1 rounded-lg text-on-surface-variant hover:text-red-500 hover:bg-red-500/10 cursor-pointer"
          >
            <span class="material-symbols-outlined text-base">delete</span>
          </button>
        </div>
      {/each}
    </div>
  {:else}
    <p class="text-xs text-on-surface-variant italic">Chưa cấu hình MCP server nào.</p>
  {/if}

  <!-- Catalogue -->
  <div class="pt-3 border-t border-outline-variant space-y-2">
    <p class="text-[11px] font-bold uppercase tracking-wider text-on-surface-variant">Có sẵn — bấm để thêm</p>
    <div class="grid grid-cols-1 md:grid-cols-2 gap-2">
      {#each catalogue as entry (entry.id)}
        {@const installed = installedNames.has((entry.name || '').toLowerCase())}
        <button
          type="button"
          on:click={() => addFromCatalogue(entry)}
          disabled={installed || busyId === entry.id}
          class="text-left border border-outline-variant rounded-xl px-3 py-2.5 transition-all cursor-pointer
          {installed ? 'opacity-50 cursor-default' : 'hover:border-primary hover:bg-primary/5'}"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-bold text-on-surface">{entry.name}</span>
            <span class="material-symbols-outlined text-sm {installed ? 'text-emerald-500' : 'text-primary'}">
              {installed ? 'check_circle' : 'add_circle'}
            </span>
          </div>
          <p class="text-[10px] text-on-surface-variant mt-0.5 leading-snug">{entry.description}</p>
        </button>
      {/each}
    </div>
  </div>
</div>
