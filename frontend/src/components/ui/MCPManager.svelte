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

  // In-app confirmation, replacing the two native confirm() popups. The OS
  // dialog was unstyled and flattened the command it was asking about onto one
  // truncated line — the one detail the question is actually about.
  type Confirm = { title: string; body: string; code: string; okLabel: string; danger: boolean; run: () => void };
  let pendingConfirm: Confirm | null = null;

  function confirmCancel() {
    pendingConfirm = null;
  }

  function confirmAccept() {
    const c = pendingConfirm;
    pendingConfirm = null;
    c?.run();
  }

  function onKeydown(e: KeyboardEvent) {
    if (pendingConfirm && e.key === 'Escape') {
      e.preventDefault();
      confirmCancel();
    }
  }

  onMount(() => {
    load();
    window.addEventListener('keydown', onKeydown);
    return () => window.removeEventListener('keydown', onKeydown);
  });

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

  function addFromCatalogue(entry: any) {
    // An MCP server runs commands on this machine with the user's permissions,
    // so adding one is always a decision they make explicitly.
    pendingConfirm = {
      title: `Thêm "${entry.name}"?`,
      body: 'MCP server chạy lệnh trên máy bạn, với quyền của bạn:',
      code: `${entry.command} ${(entry.args || []).join(' ')}`.trim(),
      okLabel: 'Thêm',
      danger: false,
      run: () => doAddFromCatalogue(entry),
    };
  }

  async function doAddFromCatalogue(entry: any) {
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

  // "Thêm" used to accept an empty name and an empty command/URL and post them
  // straight to the backend — a silent misconfiguration in the very screen whose
  // job is to prevent one.
  $: customValid =
    customName.trim() !== '' &&
    (mode === 'command' ? customCommand.trim() !== '' : customUrl.trim() !== '');

  async function addCustom() {
    if (!customValid) return;

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

  function remove(server: any) {
    pendingConfirm = {
      title: `Xoá MCP server "${server.name}"?`,
      body: 'Sub-agent sẽ không còn dùng được công cụ này khi chạy task.',
      code: server.url || `${server.command} ${(server.args || []).join(' ')}`.trim(),
      okLabel: 'Xoá',
      danger: true,
      run: () => doRemove(server),
    };
  }

  async function doRemove(server: any) {
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
      <h4 class="text-sm font-semibold text-on-surface">MCP Servers</h4>
      <p class="text-xs text-on-surface-variant mt-0.5">
        Công cụ mà sub-agent được dùng thêm khi chạy task.
      </p>
    </div>
    <div class="flex items-center gap-2">
      <button
        type="button"
        on:click={searchOnline}
        class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer flex items-center gap-1.5"
      >
        <span class="material-symbols-outlined text-sm">travel_explore</span> Tìm thêm trên web
      </button>
      <button
        type="button"
        on:click={() => (showCustom = !showCustom)}
        class="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary text-on-primary hover:opacity-90 transition-opacity cursor-pointer flex items-center gap-1.5"
      >
        <span class="material-symbols-outlined text-sm">add</span> Tự thêm
      </button>
    </div>
  </div>

  {#if showCustom}
    <div class="border border-outline-variant rounded-xl p-4 space-y-3 bg-surface-container-low">
      <div class="flex items-center gap-1">
        <button
          type="button"
          on:click={() => (mode = 'command')}
          class="px-2.5 py-1 rounded-lg text-[11px] font-medium transition-colors cursor-pointer {mode === 'command' ? 'bg-secondary-container text-on-secondary-container' : 'text-on-surface-variant hover:bg-surface-container-high'}"
        >
          Lệnh (stdio)
        </button>
        <button
          type="button"
          on:click={() => (mode = 'url')}
          class="px-2.5 py-1 rounded-lg text-[11px] font-medium transition-colors cursor-pointer {mode === 'url' ? 'bg-secondary-container text-on-secondary-container' : 'text-on-surface-variant hover:bg-surface-container-high'}"
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

      <div class="flex items-center justify-end gap-2">
        {#if !customValid}
          <p class="mr-auto text-[11px] text-on-surface-variant">
            Cần tên hiển thị và {mode === 'command' ? 'lệnh chạy' : 'URL'}.
          </p>
        {/if}
        <button
          type="button"
          on:click={() => (showCustom = false)}
          class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer"
        >
          Huỷ
        </button>
        <button
          type="button"
          on:click={addCustom}
          disabled={!customValid}
          class="px-3 py-1.5 rounded-lg text-xs font-medium bg-primary text-on-primary hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer"
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
          <!-- Was a bare 20px glyph with no padding, no switch semantics and a
               raw emerald tint: a sub-24px hit target announced as nothing. -->
          <button
            type="button"
            on:click={() => toggle(srv)}
            aria-pressed={srv.enabled}
            aria-label={`${srv.name}: ${srv.enabled ? 'đang bật' : 'đang tắt'}`}
            title={srv.enabled ? 'Đang bật' : 'Đang tắt'}
            class="flex-shrink-0 rounded-lg p-1 hover:bg-surface-container-high transition-colors cursor-pointer {srv.enabled ? 'text-primary' : 'text-outline'}"
          >
            <span class="material-symbols-outlined text-lg block">{srv.enabled ? 'toggle_on' : 'toggle_off'}</span>
          </button>

          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span class="text-xs font-medium text-on-surface truncate">{srv.name}</span>
              {#if srv.last_status === 'ok'}
                <span class="flex items-center gap-1 text-[11px] text-on-surface-variant whitespace-nowrap">
                  <span class="w-1.5 h-1.5 rounded-full bg-success"></span> Đã kiểm tra
                </span>
              {:else if srv.last_status === 'error'}
                <span class="flex items-center gap-1 text-[11px] text-error whitespace-nowrap">
                  <span class="w-1.5 h-1.5 rounded-full bg-error"></span> Lỗi
                </span>
              {:else}
                <span class="flex items-center gap-1 text-[11px] text-on-surface-variant whitespace-nowrap">
                  <span class="w-1.5 h-1.5 rounded-full bg-outline"></span> Chưa kiểm tra
                </span>
              {/if}
            </div>
            <p class="text-[11px] text-on-surface-variant font-mono truncate">
              {srv.url || `${srv.command} ${(srv.args || []).join(' ')}`}
            </p>
            {#if srv.last_error}
              <p class="text-[11px] text-error mt-0.5">{srv.last_error}</p>
            {/if}
          </div>

          <button
            type="button"
            on:click={() => test(srv)}
            disabled={busyId === srv.id}
            class="px-2.5 py-1 rounded-lg text-[11px] font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer"
          >
            {busyId === srv.id ? 'Đang kiểm tra…' : 'Kiểm tra'}
          </button>
          <button
            type="button"
            on:click={() => remove(srv)}
            aria-label={`Xoá ${srv.name}`}
            class="p-1 rounded-lg text-on-surface-variant hover:text-error hover:bg-error/10 transition-colors cursor-pointer"
          >
            <span class="material-symbols-outlined text-base block">delete</span>
          </button>
        </div>
      {/each}
    </div>
  {:else}
    <p class="text-xs text-on-surface-variant">Chưa cấu hình MCP server nào.</p>
  {/if}

  <!-- Catalogue -->
  <div class="pt-3 border-t border-outline-variant space-y-2">
    <p class="text-xs font-medium text-on-surface-variant">Có sẵn — bấm để thêm</p>
    <div class="grid grid-cols-1 md:grid-cols-2 gap-2">
      {#each catalogue as entry (entry.id)}
        {@const installed = installedNames.has((entry.name || '').toLowerCase())}
        <button
          type="button"
          on:click={() => addFromCatalogue(entry)}
          disabled={installed || busyId === entry.id}
          class="text-left border border-outline-variant rounded-xl px-3 py-2.5 transition-colors cursor-pointer
          {installed ? 'opacity-50 cursor-default' : 'hover:bg-surface-container-high'}"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-medium text-on-surface">{entry.name}</span>
            <span class="material-symbols-outlined text-sm {installed ? 'text-on-surface-variant' : 'text-primary'}">
              {installed ? 'check_circle' : 'add_circle'}
            </span>
          </div>
          <p class="text-[11px] text-on-surface-variant mt-0.5 leading-snug">{entry.description}</p>
        </button>
      {/each}
    </div>
  </div>
</div>

{#if pendingConfirm}
  <div class="fixed inset-0 z-[120] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
    <div
      class="w-full max-w-md bg-surface border border-outline-variant rounded-xl shadow-lg p-5"
      role="dialog"
      aria-modal="true"
      aria-labelledby="mcp-confirm-title"
    >
      <h4 id="mcp-confirm-title" class="text-sm font-semibold text-on-surface">{pendingConfirm.title}</h4>
      <p class="text-xs text-on-surface-variant mt-2">{pendingConfirm.body}</p>
      {#if pendingConfirm.code}
        <pre class="mt-3 max-h-40 overflow-auto rounded-lg bg-surface-container-highest border border-outline-variant p-3 font-mono text-[11px] text-on-surface whitespace-pre-wrap break-all">{pendingConfirm.code}</pre>
      {/if}
      <div class="flex justify-end gap-2 mt-5">
        <button
          type="button"
          on:click={confirmCancel}
          class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer"
        >
          Huỷ
        </button>
        <button
          type="button"
          on:click={confirmAccept}
          class="px-3 py-1.5 rounded-lg text-xs font-medium transition-colors cursor-pointer
          {pendingConfirm.danger
            ? 'border border-outline-variant text-error hover:bg-error/10'
            : 'bg-primary text-on-primary hover:opacity-90'}"
        >
          {pendingConfirm.okLabel}
        </button>
      </div>
    </div>
  </div>
{/if}
