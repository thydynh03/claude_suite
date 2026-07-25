<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { addLog } from '../../lib/stores/appState';

  export let onOpenGoogleLogin: () => void = () => {};

  interface ModelQuota {
    name: string;
    category: string;
    reset_time: string;
    usage_pct: number;
    status: string;
  }

  interface AntiAccountKey {
    id: string;
    name: string;
    email: string;
    type: string;
    api_key: string;
    oauth_token: string;
    refresh_token: string;
    status: string;
    tier: string;
    is_current: boolean;
    last_used: string;
    model_quotas: ModelQuota[];
  }

  let accounts: AntiAccountKey[] = [];
  let searchQuery = '';
  let activeFilter: 'all' | 'PRO' | 'ULTRA' | 'FREE' = 'all';
  let isLoading = false;
  let selectedIds: string[] = [];

  // GCP Credentials State
  let showGcpSettings = false;
  let gcpClientId = '';
  let gcpClientSecret = '';
  let isSavingGcp = false;
  
  // Custom API Key / OAuth Token form state
  let showAddForm = false;
  let newAuthType: 'api_key' | 'oauth_token' = 'oauth_token';
  let newKeyName = '';
  let newKeyValue = '';

  onMount(async () => {
    await loadAccounts();
    await loadGcpCreds();
    let unoff: any;
    if ((window as any)?.runtime?.EventsOn) {
      unoff = (window as any).runtime.EventsOn('oauth_success', async (data: any) => {
        addLog(`Đăng nhập Google thành công: ${data.email || ''}`, 'SUCCESS');
        await loadAccounts();
      });
    }
    return () => {
      if (unoff && typeof unoff === 'function') unoff();
    };
  });

  async function loadGcpCreds() {
    try {
      if ((AppBindings as any).GetGCPOAuthCredentials) {
        const creds = await (AppBindings as any).GetGCPOAuthCredentials();
        gcpClientId = creds.client_id || '';
        gcpClientSecret = creds.client_secret || '';
      }
    } catch (e) {
      console.error('Error loading GCP creds:', e);
    }
  }

  async function saveGcpCreds() {
    isSavingGcp = true;
    try {
      if ((AppBindings as any).SaveGCPOAuthCredentials) {
        await (AppBindings as any).SaveGCPOAuthCredentials(gcpClientId, gcpClientSecret);
        addLog('Đã lưu GCP OAuth Credentials vào file config!', 'SUCCESS');
        showGcpSettings = false;
      }
    } catch (e) {
      addLog(`Lỗi lưu GCP Creds: ${e}`, 'ERROR');
    } finally {
      isSavingGcp = false;
    }
  }

  async function handleAddAntiKey() {
    if (!newKeyName.trim() || !newKeyValue.trim()) {
      addLog('Vui lòng nhập đầy đủ tên và giá trị (Key/Token).', 'ERROR');
      return;
    }
    try {
      if (newAuthType === 'api_key') {
        if ((AppBindings as any).AddAntiAccountKey) {
          await (AppBindings as any).AddAntiAccountKey(newKeyName, newKeyValue);
          addLog(`Đã thêm API Key: ${newKeyName}`, 'SUCCESS');
        }
      } else {
        if ((AppBindings as any).AddAntiOAuthAccountKey) {
          await (AppBindings as any).AddAntiOAuthAccountKey(newKeyName, newKeyValue);
          addLog(`Đã thêm OAuth Token: ${newKeyName}`, 'SUCCESS');
        }
      }
      newKeyName = '';
      newKeyValue = '';
      showAddForm = false;
      await loadAccounts();
    } catch (e) {
      addLog(`Lỗi thêm key: ${e}`, 'ERROR');
    }
  }

  async function loadAccounts() {
    isLoading = true;
    try {
      if ((AppBindings as any).GetAntiAccountKeys) {
        const res = await (AppBindings as any).GetAntiAccountKeys();
        accounts = Array.isArray(res) ? res : [];
      }
    } catch (e) {
      console.error('Error loading OAuth keys:', e);
    } finally {
      isLoading = false;
    }
  }

  async function handleSetCurrent(id: string) {
    try {
      if ((AppBindings as any).SetCurrentAntiAccountKey) {
        await (AppBindings as any).SetCurrentAntiAccountKey(id);
        addLog('Đã chuyển tài khoản hiện tại!', 'SUCCESS');
        await loadAccounts();
      }
    } catch (e) {
      addLog(`Lỗi chọn tài khoản: ${e}`, 'ERROR');
    }
  }

  async function handleDelete(id: string, name: string) {
    if (!confirm(`Bạn có chắc chắn muốn xóa tài khoản "${name}" khỏi Pool?`)) return;
    try {
      if ((AppBindings as any).DeleteAntiAccountKey) {
        await (AppBindings as any).DeleteAntiAccountKey(id);
        addLog(`Đã xóa tài khoản: ${name}`, 'INFO');
        await loadAccounts();
      }
    } catch (e) {
      addLog(`Lỗi xóa tài khoản: ${e}`, 'ERROR');
    }
  }

  async function handleToggleStatus(id: string) {
    try {
      if ((AppBindings as any).ToggleAntiAccountKeyStatus) {
        const status = await (AppBindings as any).ToggleAntiAccountKeyStatus(id);
        addLog(`Trạng thái tài khoản: ${status}`, 'INFO');
        await loadAccounts();
      }
    } catch (e) {
      addLog(`Lỗi đổi trạng thái: ${e}`, 'ERROR');
    }
  }

  async function handleWarmupAll() {
    try {
      if ((AppBindings as any).WarmupAntiAccountKeys) {
        await (AppBindings as any).WarmupAntiAccountKeys();
        addLog('🔥 Đã làm nóng (Warmup) tất cả tài khoản trong Pool!', 'SUCCESS');
        await loadAccounts();
      }
    } catch (e) {
      addLog(`Lỗi Warmup: ${e}`, 'ERROR');
    }
  }

  function copyText(text: string, label: string) {
    if (!text) return;
    navigator.clipboard.writeText(text);
    addLog(`Đã sao chép ${label}!`, 'SUCCESS');
  }

  $: filteredAccounts = accounts.filter(acc => {
    if (activeFilter !== 'all' && acc.tier !== activeFilter) return false;
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      const matchEmail = acc.email?.toLowerCase().includes(q);
      const matchName = acc.name?.toLowerCase().includes(q);
      return matchEmail || matchName;
    }
    return true;
  });

  $: proCount = accounts.filter(a => a.tier === 'PRO').length;
  $: ultraCount = accounts.filter(a => a.tier === 'ULTRA').length;
  $: freeCount = accounts.filter(a => a.tier === 'FREE').length;
</script>

<div class="space-y-4">
  <!-- Top Control Banner -->
  <div class="bg-surface-container-lowest border border-outline-variant rounded-2xl p-5 shadow-sm space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-base font-bold text-on-surface flex items-center gap-2">
          <span class="material-symbols-outlined text-primary">key</span> Multi-Account Google OAuth Pool & Hạn Mức Model
        </h3>
        <p class="text-xs text-on-surface-variant mt-0.5">
          Quản lý danh sách tài khoản Google OAuth, API Keys, hạn mức từng Model (Gemini 3.1/3.6, Claude, GPT) và tự động xoay vòng khi dính Rate Limit 429.
        </p>
      </div>

      <div class="flex items-center gap-2">
        <button
          type="button"
          on:click={onOpenGoogleLogin}
          class="bg-gradient-to-r from-blue-600 to-indigo-600 text-white px-4 py-2 rounded-xl text-xs font-bold shadow-sm hover:opacity-90 transition-all cursor-pointer flex items-center gap-1.5"
        >
          <span class="material-symbols-outlined text-base">login</span> 🌐 + Đăng Nhập Google (Tự Động Lấy Key)
        </button>
        <button
          type="button"
          on:click={handleWarmupAll}
          class="bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/30 px-3.5 py-2 rounded-xl text-xs font-bold hover:bg-amber-500/20 transition-all cursor-pointer flex items-center gap-1.5"
        >
          <span class="material-symbols-outlined text-base">local_fire_department</span> 🔥 Làm Nóng (Warmup) Tất Cả
        </button>
        <button
          type="button"
          on:click={loadAccounts}
          class="bg-surface-container-high text-on-surface hover:bg-surface-container-highest px-3.5 py-2 rounded-xl text-xs font-bold border border-outline-variant transition-all cursor-pointer flex items-center gap-1.5"
        >
          <span class="material-symbols-outlined text-base {isLoading ? 'animate-spin' : ''}">refresh</span> Làm Mới
        </button>
      </div>
    </div>

    {#if !gcpClientId || showGcpSettings}
      <div class="p-4 bg-surface-container-low border border-amber-500/30 rounded-xl space-y-3 mt-4">
        <div class="flex items-center justify-between">
          <h4 class="text-xs font-bold text-amber-500 flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm">warning</span> 
            {gcpClientId ? 'Cấu hình GCP OAuth Credentials' : 'Thiếu GCP OAuth Credentials'}
          </h4>
          <button type="button" on:click={() => showGcpSettings = false} class="text-on-surface-variant hover:text-on-surface">
            <span class="material-symbols-outlined text-sm">close</span>
          </button>
        </div>
        <p class="text-[11px] text-on-surface-variant">Để tính năng tự động đăng nhập Google hoạt động, bạn cần cấu hình Client ID và Client Secret. Nếu chưa có, xem <a href="https://console.cloud.google.com/apis/credentials" target="_blank" class="text-primary hover:underline">GCP Console</a>.</p>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div>
            <label class="text-[10px] font-bold text-on-surface block mb-1">GCP Client ID</label>
            <input type="text" bind:value={gcpClientId} class="w-full bg-surface-container-highest border border-outline-variant rounded p-2 text-xs font-mono" placeholder="1234...apps.googleusercontent.com" />
          </div>
          <div>
            <label class="text-[10px] font-bold text-on-surface block mb-1">GCP Client Secret</label>
            <input type="text" bind:value={gcpClientSecret} class="w-full bg-surface-container-highest border border-outline-variant rounded p-2 text-xs font-mono" placeholder="GOCSPX-..." />
          </div>
        </div>
        <div class="flex justify-end pt-1">
          <button type="button" on:click={saveGcpCreds} class="bg-primary text-on-primary px-4 py-1.5 rounded-lg text-xs font-bold shadow-sm" disabled={isSavingGcp}>
            {isSavingGcp ? 'Đang lưu...' : 'Lưu Cấu Hình'}
          </button>
        </div>
      </div>
    {:else}
      <div class="flex justify-end mt-2">
        <button type="button" on:click={() => showGcpSettings = true} class="text-[10px] text-on-surface-variant hover:text-primary flex items-center gap-1">
          <span class="material-symbols-outlined text-[12px]">settings</span> Đổi GCP Credentials
        </button>
      </div>
    {/if}

    <div class="mt-4 pt-4 border-t border-outline-variant">
      <div class="flex items-center justify-between mb-3">
        <h5 class="text-xs font-bold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-sm">add_circle</span> Thêm Tài Khoản Dự Phòng
        </h5>
        <div class="flex bg-surface-container-high p-0.5 rounded-lg border border-outline-variant text-[11px]">
          <button type="button" on:click={() => newAuthType = 'api_key'} class="px-2.5 py-1 rounded font-bold transition-all {newAuthType === 'api_key' ? 'bg-primary text-on-primary shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}">
            🔑 API Key
          </button>
          <button type="button" on:click={() => newAuthType = 'oauth_token'} class="px-2.5 py-1 rounded font-bold transition-all {newAuthType === 'oauth_token' ? 'bg-primary text-on-primary shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}">
            🔐 OAuth Token
          </button>
        </div>
      </div>
      <div class="flex gap-2">
        <input type="text" bind:value={newKeyName} placeholder={newAuthType === 'oauth_token' ? 'Tên Tài khoản OAuth' : 'Tên API Key'} class="flex-1 bg-surface-container-low border border-outline-variant rounded-lg p-2 text-xs" />
        <input type="text" bind:value={newKeyValue} placeholder={newAuthType === 'oauth_token' ? 'OAuth Session / Access Token' : 'API Key'} class="flex-2 bg-surface-container-low border border-outline-variant rounded-lg p-2 text-xs font-mono" />
        <button type="button" on:click={handleAddAntiKey} class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-bold whitespace-nowrap">Thêm</button>
      </div>
    </div>

    <!-- Filters and Search Bar -->
    <div class="flex flex-wrap items-center justify-between gap-3 pt-3 border-t border-outline-variant text-xs mt-4">
      <div class="flex items-center gap-1.5 bg-surface-container-low p-1 rounded-xl border border-outline-variant">
        <button
          type="button"
          on:click={() => activeFilter = 'all'}
          class="px-3 py-1 rounded-lg font-bold transition-all cursor-pointer {activeFilter === 'all' ? 'bg-primary text-on-primary shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          Tất cả <span class="ml-1 opacity-80 font-mono">({accounts.length})</span>
        </button>
        <button
          type="button"
          on:click={() => activeFilter = 'PRO'}
          class="px-3 py-1 rounded-lg font-bold transition-all cursor-pointer {activeFilter === 'PRO' ? 'bg-purple-600 text-white shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          PRO <span class="ml-1 opacity-80 font-mono">({proCount})</span>
        </button>
        <button
          type="button"
          on:click={() => activeFilter = 'ULTRA'}
          class="px-3 py-1 rounded-lg font-bold transition-all cursor-pointer {activeFilter === 'ULTRA' ? 'bg-blue-600 text-white shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          ULTRA <span class="ml-1 opacity-80 font-mono">({ultraCount})</span>
        </button>
        <button
          type="button"
          on:click={() => activeFilter = 'FREE'}
          class="px-3 py-1 rounded-lg font-bold transition-all cursor-pointer {activeFilter === 'FREE' ? 'bg-slate-600 text-white shadow-xs' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          MIỄN PHÍ <span class="ml-1 opacity-80 font-mono">({freeCount})</span>
        </button>
      </div>

      <div class="relative min-w-[240px]">
        <span class="material-symbols-outlined absolute left-3 top-2.5 text-on-surface-variant text-sm">search</span>
        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Tìm kiếm email tài khoản..."
          class="w-full bg-surface-container-low border border-outline-variant rounded-xl pl-9 pr-3 py-2 text-xs text-on-surface outline-none focus:border-primary"
        />
      </div>
    </div>
  </div>

  <!-- Accounts & Quotas Main Table -->
  <div class="bg-surface-container-lowest border border-outline-variant rounded-2xl shadow-sm overflow-hidden">
    <div class="overflow-x-auto">
      <table class="w-full text-left text-xs border-collapse">
        <thead>
          <tr class="bg-surface-container-low text-on-surface-variant font-bold border-b border-outline-variant uppercase text-[11px] tracking-wider">
            <th class="p-3 w-10 text-center">
              <input type="checkbox" class="rounded accent-primary" />
            </th>
            <th class="p-3 min-w-[200px]">EMAIL & TRẠNG THÁI</th>
            <th class="p-3 min-w-[420px]">HẠN MỨC MODEL (MODEL QUOTA)</th>
            <th class="p-3 w-36 whitespace-nowrap">DÙNG LẦN CUỐI</th>
            <th class="p-3 w-36 text-right">THAO TÁC</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-outline-variant/60 font-mono">
          {#each filteredAccounts as acc (acc.id)}
            <tr class="hover:bg-surface-container-low/50 transition-all {acc.is_current ? 'bg-primary/5' : ''}">
              <!-- Checkbox -->
              <td class="p-3 text-center">
                <input type="checkbox" class="rounded accent-primary" />
              </td>

              <!-- Email & Badges -->
              <td class="p-3 align-top space-y-1.5 font-sans">
                <div class="flex items-center gap-2">
                  <span class="font-bold text-sm text-on-surface truncate max-w-[180px]" title={acc.email || acc.name}>
                    {acc.email || acc.name}
                  </span>
                  {#if acc.is_current}
                    <span class="px-2 py-0.5 rounded text-[10px] font-extrabold uppercase bg-emerald-500/20 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30">
                      HIỆN TẠI
                    </span>
                  {/if}
                  <span class="px-2 py-0.5 rounded text-[10px] font-bold uppercase {acc.tier === 'PRO' ? 'bg-purple-500/20 text-purple-600 dark:text-purple-400' : acc.tier === 'ULTRA' ? 'bg-blue-500/20 text-blue-600 dark:text-blue-400' : 'bg-slate-500/20 text-slate-400'}">
                    {acc.tier || 'FREE'}
                  </span>
                </div>

                <div class="flex items-center gap-2 text-[11px] text-on-surface-variant font-mono">
                  <span class="px-1.5 py-0.2 rounded text-[9px] font-bold uppercase {acc.type === 'oauth_token' ? 'bg-purple-500/10 text-purple-600 border border-purple-500/20' : 'bg-blue-500/10 text-blue-600 border border-blue-500/20'}">
                    {acc.type === 'oauth_token' ? 'OAuth Token' : 'API Key'}
                  </span>
                  {#if acc.oauth_token || acc.api_key}
                    <span class="truncate max-w-[120px] text-slate-400">{(acc.oauth_token || acc.api_key).substring(0, 10)}...</span>
                  {/if}
                </div>
              </td>

              <!-- Model Quotas Grid -->
              <td class="p-3 align-top">
                <div class="grid grid-cols-2 gap-2 text-[10px]">
                  {#each (acc.model_quotas || []) as q}
                    <div class="bg-surface-container-low/80 p-1.5 rounded-lg border border-outline-variant/50 space-y-1">
                      <div class="flex items-center justify-between">
                        <span class="font-bold text-on-surface truncate max-w-[120px]" title={q.name}>
                          ⚡ {q.name}
                        </span>
                        <span class="text-slate-400 text-[9px] font-mono">⏱ {q.reset_time}</span>
                      </div>
                      <!-- Percentage Progress Bar -->
                      <div class="flex items-center gap-1.5">
                        <div class="flex-1 bg-surface-container-high h-1.5 rounded-full overflow-hidden">
                          <div
                            class="h-full rounded-full transition-all {q.usage_pct > 80 ? 'bg-rose-500' : q.usage_pct > 30 ? 'bg-emerald-500' : 'bg-amber-500'}"
                            style="width: {q.usage_pct}%"
                          ></div>
                        </div>
                        <span class="font-bold text-[9px] {q.usage_pct > 80 ? 'text-rose-500' : 'text-emerald-500'}">
                          {q.usage_pct}%
                        </span>
                      </div>
                    </div>
                  {/each}
                </div>
              </td>

              <!-- Last Used -->
              <td class="p-3 align-top font-mono text-[11px] text-on-surface-variant">
                <div>{acc.last_used || 'Vừa xong'}</div>
              </td>

              <!-- Actions -->
              <td class="p-3 align-top text-right space-y-1">
                <div class="flex items-center justify-end gap-1">
                  {#if !acc.is_current}
                    <button
                      type="button"
                      on:click={() => handleSetCurrent(acc.id)}
                      title="Đặt làm Tài khoản Hiện Tại"
                      class="p-1.5 bg-primary/10 text-primary hover:bg-primary/20 rounded-lg transition-all cursor-pointer"
                    >
                      <span class="material-symbols-outlined text-sm">check_circle</span>
                    </button>
                  {/if}

                  <button
                    type="button"
                    on:click={() => copyText(acc.oauth_token || acc.api_key, acc.type === 'oauth_token' ? 'OAuth Token' : 'API Key')}
                    title="Sao chép Token/Key"
                    class="p-1.5 bg-surface-container-high text-on-surface hover:bg-surface-container-highest rounded-lg transition-all cursor-pointer"
                  >
                    <span class="material-symbols-outlined text-sm">content_copy</span>
                  </button>

                  <button
                    type="button"
                    on:click={() => handleToggleStatus(acc.id)}
                    title="Đổi trạng thái Active/Disabled"
                    class="p-1.5 bg-surface-container-high text-on-surface hover:bg-surface-container-highest rounded-lg transition-all cursor-pointer"
                  >
                    <span class="material-symbols-outlined text-sm {acc.status === 'active' ? 'text-emerald-500' : 'text-amber-500'}">
                      {acc.status === 'active' ? 'toggle_on' : 'toggle_off'}
                    </span>
                  </button>

                  <button
                    type="button"
                    on:click={() => handleDelete(acc.id, acc.name)}
                    title="Xóa khỏi Pool"
                    class="p-1.5 bg-rose-500/10 text-rose-500 hover:bg-rose-500/20 rounded-lg transition-all cursor-pointer"
                  >
                    <span class="material-symbols-outlined text-sm">delete</span>
                  </button>
                </div>
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="5" class="p-8 text-center text-on-surface-variant font-sans italic">
                Chưa có tài khoản nào trong Key Pool. Hãy bấm <strong>"🌐 + Đăng Nhập Google"</strong> trên để tự động thêm tài khoản!
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Table Footer Pagination -->
    <div class="p-3 bg-surface-container-low border-t border-outline-variant flex items-center justify-between text-xs text-on-surface-variant font-sans">
      <div>
        Hiển thị <strong>1 đến {filteredAccounts.length}</strong> trong tổng số <strong>{accounts.length}</strong> mục
      </div>
      <div class="flex items-center gap-1 font-mono text-[11px]">
        <button class="px-2 py-1 rounded bg-surface-container-high text-on-surface cursor-pointer">&lt;</button>
        <span class="px-3 py-1 rounded bg-primary text-on-primary font-bold">1</span>
        <button class="px-2 py-1 rounded bg-surface-container-high text-on-surface cursor-pointer">&gt;</button>
      </div>
    </div>
  </div>
</div>
