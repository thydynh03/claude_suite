<script lang="ts">
  import { onMount } from 'svelte';
  import * as AppBindings from '../../../wailsjs/go/main/App';
  import { cli } from '../../../wailsjs/go/models';
  import { addLog, addToast } from '../../lib/stores/appState';

  export let onOpenGoogleLogin: () => void = () => {};

  // The generated binding type, not a hand-copied one. This file used to declare
  // its own AntiAccountKey interface alongside the Go struct; the two drifted the
  // moment a field was added on the backend, and the missing field was invisible
  // until a value silently read as undefined at runtime.
  type AntiAccountKey = cli.AntiAccountKey;

  let accounts: AntiAccountKey[] = [];
  let searchQuery = '';
  let activeFilter: 'all' | 'PRO' | 'ULTRA' | 'FREE' | 'UNKNOWN' = 'all';
  let isLoading = false;
  let selectedIds: string[] = [];
  let isWarming = false;
  let bulkBusy = false;
  // Xác nhận xoá bằng modal trong app. confirm() là hộp thoại của hệ điều hành:
  // nó không theo giao diện app và khoá luôn luồng WebView2 trong lúc mở.
  let confirmDelete: { title: string; body: string; run: () => void } | null = null;

  // Every account currently visible under the search box and tier filter. Bulk
  // actions operate on the intersection with selectedIds, never on rows the user
  // cannot see — selecting all, then filtering, then deleting used to take the
  // hidden ones with it.
  $: visibleIds = filteredAccounts.map((a: any) => a.id).filter(Boolean);
  $: selectedVisible = selectedIds.filter((id) => visibleIds.includes(id));
  $: allVisibleSelected = visibleIds.length > 0 && selectedVisible.length === visibleIds.length;
  $: someVisibleSelected = selectedVisible.length > 0 && !allVisibleSelected;

  function toggleSelectAll() {
    selectedIds = allVisibleSelected ? [] : [...visibleIds];
  }

  function toggleOne(id: string) {
    selectedIds = selectedIds.includes(id)
      ? selectedIds.filter((x) => x !== id)
      : [...selectedIds, id];
  }

  // GCP Credentials State
  let showGcpSettings = false;
  let gcpClientId = '';
  let gcpClientSecret = '';
  let isSavingGcp = false;
  let isImportingCreds = false;
  
  // Custom API Key / OAuth Token form state
  let showAddForm = false;
  let newAuthType: 'api_key' | 'oauth_token' = 'oauth_token';
  let newKeyName = '';
  let newKeyValue = '';

  let unoff: any;

  async function initOAuthDashboard() {
    // Register before the first await. onMount captures its cleanup closure
    // synchronously, so a listener attached after an await would leak whenever
    // the component unmounts while the initial load is still in flight.
    if ((window as any)?.runtime?.EventsOn) {
      unoff = (window as any).runtime.EventsOn('oauth_success', async (data: any) => {
        addLog(`Đăng nhập Google thành công: ${data.email || ''}`, 'SUCCESS');
        addToast(`Đăng nhập Google thành công: ${data.email || ''}`, 'SUCCESS');
        await loadAccounts();
      });
    }
    await loadAccounts();
    await loadGcpCreds();
  }

  onMount(() => {
    initOAuthDashboard();
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
      {
        await (AppBindings as any).SaveGCPOAuthCredentials(gcpClientId, gcpClientSecret);
        addLog('Đã lưu GCP OAuth Credentials vào file config!', 'SUCCESS');
        addToast('Đã lưu GCP OAuth Credentials vào file config!', 'SUCCESS');
        addToast('Đã lưu GCP OAuth Credentials — nút Đăng Nhập Google đã sẵn sàng.', 'SUCCESS');
        showGcpSettings = false;
      }
    } catch (e) {
      addLog(`Lỗi lưu GCP Creds: ${e}`, 'ERROR');
      addToast(`Lỗi lưu GCP Creds: ${e}`, 'ERROR');
      addToast(`Lưu GCP Credentials thất bại: ${e}`, 'ERROR');
    } finally {
      isSavingGcp = false;
    }
  }

  async function handleAddAntiKey() {
    if (!newKeyName.trim() || !newKeyValue.trim()) {
      addLog('Vui lòng nhập đầy đủ tên và giá trị (Key/Token).', 'ERROR');
      addToast('Vui lòng nhập đầy đủ tên và giá trị (Key/Token).', 'ERROR');
      return;
    }
    try {
      if (newAuthType === 'api_key') {
        if ((AppBindings as any).AddAntiAccountKey) {
          await (AppBindings as any).AddAntiAccountKey(newKeyName, newKeyValue);
          addLog(`Đã thêm API Key: ${newKeyName}`, 'SUCCESS');
          addToast(`Đã thêm API Key: ${newKeyName}`, 'SUCCESS');
        }
      } else {
        if ((AppBindings as any).AddAntiOAuthAccountKey) {
          await (AppBindings as any).AddAntiOAuthAccountKey(newKeyName, newKeyValue);
          addLog(`Đã thêm OAuth Token: ${newKeyName}`, 'SUCCESS');
          addToast(`Đã thêm OAuth Token: ${newKeyName}`, 'SUCCESS');
        }
      }
      newKeyName = '';
      newKeyValue = '';
      showAddForm = false;
      await loadAccounts();
    } catch (e) {
      addLog(`Lỗi thêm key: ${e}`, 'ERROR');
      addToast(`Lỗi thêm key: ${e}`, 'ERROR');
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
        addToast('Đã chuyển tài khoản hiện tại!', 'SUCCESS');
        await loadAccounts();
      }
    } catch (e) {
      addLog(`Lỗi chọn tài khoản: ${e}`, 'ERROR');
      addToast(`Lỗi chọn tài khoản: ${e}`, 'ERROR');
    }
  }

  function handleDelete(id: string, name: string) {
    confirmDelete = {
      title: 'Xoá tài khoản khỏi pool?',
      body: `Tài khoản "${name}" sẽ bị gỡ khỏi pool. Thao tác này không hoàn tác được.`,
      run: () => doDelete(id, name),
    };
  }

  async function doDelete(id: string, name: string) {
    try {
      if ((AppBindings as any).DeleteAntiAccountKey) {
        await (AppBindings as any).DeleteAntiAccountKey(id);
        addLog(`Đã xoá tài khoản: ${name}`, 'INFO');
        addToast(`Đã xoá tài khoản: ${name}`, 'SUCCESS');
        await loadAccounts();
      }
    } catch (e) {
      addLog(`Lỗi xoá tài khoản: ${e}`, 'ERROR');
      addToast(`Lỗi xoá tài khoản: ${e}`, 'ERROR');
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
      addToast(`Lỗi đổi trạng thái: ${e}`, 'ERROR');
    }
  }

  // Reports per-account outcome. Announcing a flat "thành công" while three
  // accounts failed to refresh is how a revoked token stayed invisible until a
  // task died on it.
  async function handleWarmupAll() {
    if (isWarming) return;
    isWarming = true;
    try {
      const r = await (AppBindings as any).WarmupAntiAccountKeys();
      await loadAccounts();

      const refreshed = r?.refreshed ?? 0;
      const failed = r?.failed ?? 0;
      const skipped = r?.skipped ?? 0;

      if (failed > 0) {
        const detail = (r?.errors || []).slice(0, 3).join(' · ');
        addToast(`Làm nóng: ${refreshed} thành công, ${failed} lỗi. ${detail}`, 'ERROR', 9000);
        (r?.errors || []).forEach((e: string) => addLog(`Warmup lỗi — ${e}`, 'ERROR'));
      } else if (refreshed === 0) {
        addToast(`Không có tài khoản nào cần làm mới (${skipped} token vẫn còn hạn).`, 'INFO');
      } else {
        addToast(`Đã làm nóng ${refreshed} tài khoản.`, 'SUCCESS');
      }
      addLog(`Warmup: ${refreshed} refreshed, ${failed} failed, ${skipped} skipped`, 'INFO');
    } catch (e) {
      addLog(`Lỗi Warmup: ${e}`, 'ERROR');
      addToast(`Lỗi Warmup: ${e}`, 'ERROR');
      addToast(`Làm nóng thất bại: ${e}`, 'ERROR');
    } finally {
      isWarming = false;
    }
  }

  // Reading the real plan and quota from Google's Code Assist endpoints.
  //
  // Until this existed the tier was whatever a human typed and the quota table
  // was blank, because the numbers that used to fill it were a compile-time
  // constant nobody had ever fetched. The backend refuses to write anything
  // Google did not send, so a failure here leaves a stated reason rather than a
  // plausible-looking bar.
  let fetchingQuota: Record<string, boolean> = {};
  let fetchingAllQuota = false;

  async function fetchQuota(id: string) {
    if (fetchingQuota[id]) return;
    fetchingQuota = { ...fetchingQuota, [id]: true };
    try {
      const r = await (AppBindings as any).RefreshAntiAccountQuota(id);
      await loadAccounts();
      if (r?.ok) {
        addToast(`Gói ${r.tier}${r.raw_tier ? ` (${r.raw_tier})` : ''} · đọc được hạn mức ${r.models} model.`, 'SUCCESS');
      } else {
        addToast(`Chưa đọc được hạn mức: ${r?.error || 'không rõ lý do'}`, 'ERROR', 9000);
      }
    } catch (e) {
      addLog(`Lỗi đọc hạn mức: ${e}`, 'ERROR');
      addToast(`Lỗi đọc hạn mức: ${e}`, 'ERROR');
    } finally {
      const next = { ...fetchingQuota };
      delete next[id];
      fetchingQuota = next;
    }
  }

  async function fetchAllQuota() {
    if (fetchingAllQuota) return;
    fetchingAllQuota = true;
    try {
      const r = await (AppBindings as any).RefreshAllAntiAccountQuota();
      await loadAccounts();
      // Per-account outcome, not a flat "done" — the same reason handleWarmupAll
      // reports its failures: a silent failure hides a revoked account until a
      // task dies on it.
      if ((r?.failed ?? 0) > 0) {
        addToast(`Đọc hạn mức: ${r.ok}/${r.total} tài khoản có dữ liệu, ${r.failed} không.`, 'WARN', 9000);
      } else {
        addToast(`Đã đọc gói và hạn mức của ${r?.ok ?? 0} tài khoản.`, 'SUCCESS');
      }
    } catch (e) {
      addLog(`Lỗi đọc hạn mức hàng loạt: ${e}`, 'ERROR');
      addToast(`Lỗi đọc hạn mức: ${e}`, 'ERROR');
    } finally {
      fetchingAllQuota = false;
    }
  }

  // A model Google reported on. usage_pct is -1 when it said nothing, and that
  // stays a blank rather than an empty bar that reads as "0% used".
  function knownQuotas(acc: any) {
    return (acc?.model_quotas || []).filter((q: any) => (q?.usage_pct ?? -1) >= 0);
  }

  function quotaBarClass(status: string) {
    if (status === 'exceeded') return 'bg-error';
    if (status === 'warning') return 'bg-warning';
    return 'bg-primary';
  }

  function fetchedAgo(iso: string) {
    if (!iso || String(iso).startsWith('0001')) return '';
    const secs = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
    if (secs < 60) return 'vừa xong';
    if (secs < 3600) return `${Math.round(secs / 60)} phút trước`;
    return `${Math.round(secs / 3600)} giờ trước`;
  }

  function openExternal(url: string) {
    try {
      (AppBindings as any).OpenURLInBrowser(url);
    } catch {
      window.open(url, '_blank');
    }
  }

  async function importCredsFile() {
    if (isImportingCreds) return;
    isImportingCreds = true;
    try {
      const clientId = await (AppBindings as any).ImportGCPCredentialsFile();
      // An empty result means the file dialog was dismissed, which is not a
      // failure and should not be reported as one.
      if (!clientId) return;
      await loadGcpCreds();
      showGcpSettings = false;
      addToast('Đã nhập GCP Credentials từ tệp — có thể đăng nhập Google ngay.', 'SUCCESS');
    } catch (e) {
      addToast(`Không đọc được tệp: ${e}`, 'ERROR', 9000);
    } finally {
      isImportingCreds = false;
    }
  }

  async function handleSetTier(id: string, tier: string) {
    try {
      const ok = await (AppBindings as any).SetAntiAccountTier(id, tier);
      if (!ok) throw new Error('gói không hợp lệ');
      await loadAccounts();
      addToast(`Đã gán gói ${tier === 'UNKNOWN' ? 'chưa rõ' : tier} cho tài khoản.`, 'SUCCESS');
    } catch (e) {
      addToast(`Không gán được gói: ${e}`, 'ERROR');
      await loadAccounts();
    }
  }

  async function handleManualRefresh() {
    await loadAccounts();
    addToast(`Đã tải lại — ${accounts.length} tài khoản trong pool.`, 'INFO');
  }

  // Bulk actions run one account at a time and count both outcomes, so a partial
  // failure is reported as a partial failure.
  async function runBulk(label: string, fn: (id: string) => Promise<any>) {
    if (bulkBusy || selectedVisible.length === 0) return;
    bulkBusy = true;
    let ok = 0;
    let failed = 0;
    try {
      for (const id of selectedVisible) {
        try {
          await fn(id);
          ok++;
        } catch (e) {
          failed++;
          addLog(`${label} thất bại cho ${id}: ${e}`, 'ERROR');
          addToast(`${label} thất bại cho ${id}: ${e}`, 'ERROR');
        }
      }
      await loadAccounts();
      selectedIds = [];
      addToast(
        failed === 0 ? `${label}: ${ok} tài khoản.` : `${label}: ${ok} thành công, ${failed} lỗi.`,
        failed === 0 ? 'SUCCESS' : 'ERROR'
      );
    } finally {
      bulkBusy = false;
    }
  }

  function bulkToggleStatus() {
    return runBulk('Đổi trạng thái', (id) => (AppBindings as any).ToggleAntiAccountKeyStatus(id));
  }

  function bulkDelete() {
    // Deleting accounts cannot be undone from the UI, and the pool is the only
    // copy of an imported refresh token.
    const count = selectedVisible.length;
    if (count === 0) return;
    confirmDelete = {
      title: `Xoá ${count} tài khoản khỏi pool?`,
      body: 'Pool là bản duy nhất giữ refresh token đã nhập — xoá rồi không lấy lại được.',
      run: () => runBulk('Xoá', (id) => (AppBindings as any).DeleteAntiAccountKey(id)),
    };
  }

  function copyText(text: string, label: string) {
    if (!text) return;
    navigator.clipboard.writeText(text);
    addLog(`Đã sao chép ${label}!`, 'SUCCESS');
    addToast(`Đã sao chép ${label}!`, 'SUCCESS');
  }

  $: filteredAccounts = accounts.filter(acc => {
    // An account with no tier set counts as UNKNOWN rather than falling through
    // every filter and appearing only under "Tất cả".
    const tier = acc.tier || 'UNKNOWN';
    if (activeFilter !== 'all' && tier !== activeFilter) return false;
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
  $: unknownTierCount = accounts.filter(a => !a.tier || a.tier === 'UNKNOWN').length;
</script>

<!-- Escape cancels the confirm dialog, as confirm() always did. Outside the
     {#if} because Svelte does not allow this tag inside a block. -->
<svelte:window on:keydown={(e) => { if (e.key === 'Escape' && confirmDelete) confirmDelete = null; }} />

<div class="space-y-4">
  <!-- Top Control Banner -->
  <div class="bg-surface-container-lowest border border-outline-variant rounded-xl p-5 space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-sm font-semibold text-on-surface flex items-center gap-2">
          <span class="material-symbols-outlined text-base text-on-surface-variant">key</span> Pool tài khoản Google OAuth &amp; API key
        </h3>
        <p class="text-xs text-on-surface-variant mt-0.5">
          Đăng nhập Google để tự động thêm tài khoản vào pool. App tự xoay vòng sang tài khoản
          khác khi tài khoản đang dùng bị rate limit 429.
        </p>
      </div>

      <div class="flex items-center gap-2">
        <button
          type="button"
          on:click={onOpenGoogleLogin}
          class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity cursor-pointer flex items-center gap-1.5"
        >
          <span class="material-symbols-outlined text-sm">login</span> Đăng nhập Google
        </button>
        <button
          type="button"
          on:click={handleWarmupAll}
          disabled={isWarming}
          class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5"
        >
          <span class="material-symbols-outlined text-sm {isWarming ? 'animate-spin' : ''}">{isWarming ? 'progress_activity' : 'local_fire_department'}</span>
          {isWarming ? 'Đang làm nóng…' : 'Làm nóng tất cả'}
        </button>
        <button
          type="button"
          on:click={fetchAllQuota}
          disabled={fetchingAllQuota}
          title="Hỏi Google gói và hạn mức còn lại của từng tài khoản"
          class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5"
        >
          <span class="material-symbols-outlined text-sm {fetchingAllQuota ? 'animate-spin' : ''}">{fetchingAllQuota ? 'progress_activity' : 'cloud_sync'}</span>
          {fetchingAllQuota ? 'Đang đọc hạn mức…' : 'Lấy gói & hạn mức'}
        </button>
        <button
          type="button"
          on:click={handleManualRefresh}
          disabled={isLoading}
          class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5"
        >
          <span class="material-symbols-outlined text-sm {isLoading ? 'animate-spin' : ''}">refresh</span> Làm mới
        </button>
      </div>
    </div>

    {#if !gcpClientId || showGcpSettings}
      <div class="p-4 bg-surface-container-low border border-warning/30 rounded-xl space-y-3 mt-4">
        <div class="flex items-center justify-between">
          <h4 class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
            <span class="material-symbols-outlined text-sm text-warning">warning</span>
            {gcpClientId ? 'Cấu hình GCP OAuth credentials' : 'Thiếu GCP OAuth credentials'}
          </h4>
          <button type="button" on:click={() => showGcpSettings = false} class="text-on-surface-variant hover:text-on-surface cursor-pointer">
            <span class="material-symbols-outlined text-sm">close</span>
          </button>
        </div>
        <!-- The credentials cannot ship with the app: they would be identical in
             every download, extractable by anyone, and revoked by Google once
             noticed. So each user creates their own, and these are the steps. -->
        <p class="text-xs text-on-surface-variant">
          App không kèm sẵn Client ID/Secret — nếu nhúng vào bản cài thì ai tải về
          cũng rút ra được và Google sẽ thu hồi. Mỗi người tự tạo một bộ riêng,
          làm một lần là xong:
        </p>

        <ol class="text-xs text-on-surface-variant space-y-2 list-decimal pl-4">
          <li>
            Mở
            <button type="button" on:click={() => openExternal('https://console.cloud.google.com/apis/credentials')} class="text-primary hover:underline font-medium cursor-pointer">Google Cloud Console → Credentials</button>,
            tạo project mới nếu chưa có.
          </li>
          <li>
            Bấm <b>Create Credentials → OAuth client ID</b>, chọn loại
            <b>Desktop app</b>. Chọn <i>Web application</i> sẽ không đăng nhập được.
          </li>
          <li>Đặt tên bất kỳ rồi bấm <b>Create</b>.</li>
          <li>
            Bấm <b>Download JSON</b> rồi nhập tệp đó vào đây — khỏi phải copy tay:
            <button
              type="button"
              on:click={importCredsFile}
              disabled={isImportingCreds}
              class="ml-1 px-3 py-1.5 rounded-lg border border-outline-variant text-on-surface-variant font-medium hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer"
            >
              {isImportingCreds ? 'Đang đọc tệp…' : 'Nhập tệp client_secret.json'}
            </button>
          </li>
        </ol>

        <p class="text-xs text-on-surface-variant pt-1 border-t border-outline-variant/50">
          Hoặc dán thủ công hai giá trị bên dưới:
        </p>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div>
            <label for="gcp-client-id" class="text-xs font-medium text-on-surface-variant block mb-1">GCP Client ID</label>
            <input id="gcp-client-id" type="text" bind:value={gcpClientId} class="w-full bg-surface-container-highest border border-outline-variant rounded-lg p-2 text-xs font-mono text-on-surface" placeholder="1234...apps.googleusercontent.com" />
          </div>
          <div>
            <label for="gcp-client-secret" class="text-xs font-medium text-on-surface-variant block mb-1">GCP Client Secret</label>
            <input id="gcp-client-secret" type="text" bind:value={gcpClientSecret} class="w-full bg-surface-container-highest border border-outline-variant rounded-lg p-2 text-xs font-mono text-on-surface" placeholder="GOCSPX-..." />
          </div>
        </div>
        <div class="flex justify-end pt-1">
          <button type="button" on:click={saveGcpCreds} class="bg-primary text-on-primary px-3 py-1.5 rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer" disabled={isSavingGcp}>
            {isSavingGcp ? 'Đang lưu…' : 'Lưu cấu hình'}
          </button>
        </div>
      </div>
    {:else}
      <div class="flex justify-end mt-2">
        <button type="button" on:click={() => showGcpSettings = true} class="text-xs text-on-surface-variant hover:text-on-surface px-2 py-1 rounded-lg hover:bg-surface-container-high transition-colors cursor-pointer flex items-center gap-1">
          <span class="material-symbols-outlined text-sm">settings</span> Đổi GCP credentials
        </button>
      </div>
    {/if}

    <div class="mt-4 pt-4 border-t border-outline-variant">
      <div class="flex items-center justify-between mb-3">
        <h5 class="text-sm font-semibold text-on-surface flex items-center gap-1.5">
          <span class="material-symbols-outlined text-base text-on-surface-variant">add_circle</span> Thêm tài khoản dự phòng
        </h5>
        <div class="flex bg-surface-container-high p-0.5 rounded-lg border border-outline-variant text-xs">
          <button type="button" on:click={() => newAuthType = 'api_key'} class="px-2.5 py-1 rounded-lg font-medium transition-colors cursor-pointer {newAuthType === 'api_key' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}">
            API key
          </button>
          <button type="button" on:click={() => newAuthType = 'oauth_token'} class="px-2.5 py-1 rounded-lg font-medium transition-colors cursor-pointer {newAuthType === 'oauth_token' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}">
            OAuth token
          </button>
        </div>
      </div>
      <div class="flex gap-2">
        <input type="text" bind:value={newKeyName} placeholder={newAuthType === 'oauth_token' ? 'Tên tài khoản OAuth' : 'Tên API key'} class="flex-1 bg-surface-container-low border border-outline-variant rounded-lg p-2 text-xs text-on-surface" />
        <input type="text" bind:value={newKeyValue} placeholder={newAuthType === 'oauth_token' ? 'OAuth session / access token' : 'API key'} class="flex-2 bg-surface-container-low border border-outline-variant rounded-lg p-2 text-xs font-mono text-on-surface" />
        <button type="button" on:click={handleAddAntiKey} class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors cursor-pointer whitespace-nowrap">Thêm</button>
      </div>
    </div>

    <!-- Filters and Search Bar -->
    <div class="flex flex-wrap items-center justify-between gap-3 pt-3 border-t border-outline-variant text-xs mt-4">
      <!-- Một trạng thái active duy nhất cho cả bộ lọc: gói của tài khoản là dữ
           liệu trong bảng, không phải màu của thanh lọc. -->
      <div class="flex items-center gap-1.5 bg-surface-container-low p-1 rounded-lg border border-outline-variant">
        <button
          type="button"
          on:click={() => activeFilter = 'all'}
          class="px-3 py-1 rounded-lg font-medium transition-colors cursor-pointer {activeFilter === 'all' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          Tất cả <span class="ml-1 opacity-80 font-mono">({accounts.length})</span>
        </button>
        <button
          type="button"
          on:click={() => activeFilter = 'PRO'}
          class="px-3 py-1 rounded-lg font-medium transition-colors cursor-pointer {activeFilter === 'PRO' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          PRO <span class="ml-1 opacity-80 font-mono">({proCount})</span>
        </button>
        <button
          type="button"
          on:click={() => activeFilter = 'ULTRA'}
          class="px-3 py-1 rounded-lg font-medium transition-colors cursor-pointer {activeFilter === 'ULTRA' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          ULTRA <span class="ml-1 opacity-80 font-mono">({ultraCount})</span>
        </button>
        <button
          type="button"
          on:click={() => activeFilter = 'FREE'}
          class="px-3 py-1 rounded-lg font-medium transition-colors cursor-pointer {activeFilter === 'FREE' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          FREE <span class="ml-1 opacity-80 font-mono">({freeCount})</span>
        </button>
        <button
          type="button"
          on:click={() => activeFilter = 'UNKNOWN'}
          class="px-3 py-1 rounded-lg font-medium transition-colors cursor-pointer {activeFilter === 'UNKNOWN' ? 'bg-primary text-on-primary' : 'text-on-surface-variant hover:text-on-surface'}"
        >
          Chưa rõ <span class="ml-1 opacity-80 font-mono">({unknownTierCount})</span>
        </button>
      </div>

      <div class="relative min-w-[240px]">
        <span class="material-symbols-outlined absolute left-3 top-2.5 text-on-surface-variant text-sm">search</span>
        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Tìm theo email tài khoản…"
          class="w-full bg-surface-container-low border border-outline-variant rounded-lg pl-9 pr-3 py-2 text-xs text-on-surface outline-none focus:border-primary"
        />
      </div>
    </div>
  </div>

  <!-- Bulk action bar: only rendered when something is selected, so it never
       occupies space it is not using. -->
  {#if selectedVisible.length > 0}
    <div class="flex items-center justify-between gap-3 px-4 py-2.5 bg-primary/5 border border-primary/30 rounded-xl">
      <span class="text-xs font-medium text-on-surface">
        Đã chọn {selectedVisible.length} tài khoản
      </span>
      <div class="flex items-center gap-2">
        <button
          type="button"
          on:click={bulkToggleStatus}
          disabled={bulkBusy}
          class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5"
        >
          <span class="material-symbols-outlined text-sm">toggle_on</span> Bật / tắt
        </button>
        <button
          type="button"
          on:click={handleWarmupAll}
          disabled={bulkBusy || isWarming}
          class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5"
        >
          <span class="material-symbols-outlined text-sm">local_fire_department</span> Làm nóng
        </button>
        <button
          type="button"
          on:click={bulkDelete}
          disabled={bulkBusy}
          class="px-3 py-1.5 rounded-lg text-xs font-medium border border-outline-variant text-error hover:bg-error/10 transition-colors disabled:opacity-50 cursor-pointer flex items-center gap-1.5"
        >
          <span class="material-symbols-outlined text-sm">delete</span> Xoá
        </button>
        <button
          type="button"
          on:click={() => (selectedIds = [])}
          class="px-3 py-1.5 rounded-lg text-xs font-medium text-on-surface-variant hover:bg-surface-container-high transition-colors cursor-pointer"
        >
          Bỏ chọn
        </button>
      </div>
    </div>
  {/if}

  <!-- Accounts & Quotas Main Table -->
  <div class="bg-surface-container-lowest border border-outline-variant rounded-xl overflow-hidden">
    <div class="overflow-x-auto">
      <table class="w-full text-left text-xs border-collapse">
        <thead>
          <tr class="bg-surface-container-low text-on-surface-variant font-medium border-b border-outline-variant text-xs">
            <th class="p-3 w-10 text-center">
              <input
                type="checkbox"
                class="accent-primary cursor-pointer"
                checked={allVisibleSelected}
                indeterminate={someVisibleSelected}
                on:change={toggleSelectAll}
                title="Chọn tất cả tài khoản đang hiển thị"
              />
            </th>
            <th class="p-3 min-w-[200px] font-medium">Email &amp; trạng thái</th>
            <th class="p-3 min-w-[420px] font-medium">Mức dùng</th>
            <th class="p-3 w-36 whitespace-nowrap font-medium">Dùng lần cuối</th>
            <th class="p-3 w-36 text-right font-medium">Thao tác</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-outline-variant/60">
          {#each filteredAccounts as acc, i (acc.id || acc.email || i)}
            <tr class="hover:bg-surface-container-low/50 transition-colors {acc.is_current ? 'bg-primary/5' : ''}">
              <!-- Checkbox -->
              <td class="p-3 text-center">
                <input
                  type="checkbox"
                  class="accent-primary cursor-pointer"
                  checked={selectedIds.includes(acc.id)}
                  on:change={() => toggleOne(acc.id)}
                />
              </td>

              <!-- Email & Badges -->
              <td class="p-3 align-top space-y-1.5">
                <div class="flex items-center gap-2">
                  <span class="font-medium text-sm text-on-surface truncate max-w-[180px]" title={acc.email || acc.name}>
                    {acc.email || acc.name}
                  </span>
                  {#if acc.is_current}
                    <span class="px-2 py-0.5 rounded-full text-[11px] font-medium bg-primary/10 text-primary border border-primary/20">
                      Hiện tại
                    </span>
                  {/if}
                  <!-- Editable, because the tier is not fetched from anywhere.
                       Showing "PRO" for every account made the tier filters count
                       a label this code had invented. -->
                  <select
                    value={acc.tier || 'UNKNOWN'}
                    on:click|stopPropagation
                    on:change={(e) => handleSetTier(acc.id, (e.target as HTMLSelectElement).value)}
                    title={acc.tier_source === 'user' ? 'Bạn đã gán gói này' : 'Chưa xác định — chọn để gán'}
                    class="px-1.5 py-0.5 rounded-lg text-[11px] font-medium border border-outline-variant bg-surface-container-low text-on-surface-variant cursor-pointer"
                  >
                    <option value="UNKNOWN">Chưa rõ</option>
                    <option value="FREE">FREE</option>
                    <option value="PRO">PRO</option>
                    <option value="ULTRA">ULTRA</option>
                  </select>
                </div>

                <div class="flex items-center gap-2 text-[11px] text-on-surface-variant">
                  <span class="px-1.5 py-0.5 rounded-full text-[11px] font-medium border border-outline-variant text-on-surface-variant">
                    {acc.type === 'oauth_token' ? 'OAuth token' : 'API key'}
                  </span>
                  {#if acc.oauth_token || acc.api_key}
                    <span class="truncate max-w-[120px] font-mono text-on-surface-variant">{(acc.oauth_token || acc.api_key).substring(0, 10)}…</span>
                  {/if}
                </div>
              </td>

              <!-- Usage this app measured, and an explicit blank for what it
                   does not know. The progress bars here used to be filled from a
                   compile-time constant: every account showed the same 53%, and
                   nothing had ever asked Google for a quota. -->
              <td class="p-3 align-top">
                <div class="flex flex-wrap items-center gap-2 text-[11px] mb-2">
                  <span class="px-2 py-1 rounded-lg bg-surface-container-low border border-outline-variant/50">
                    <span class="text-on-surface-variant">Request đã gửi:</span>
                    <span class="font-medium font-mono text-on-surface">{acc.usage?.requests ?? 0}</span>
                  </span>
                  <span class="px-2 py-1 rounded-lg border
                    {(acc.usage?.rate_limit_hits ?? 0) > 0
                      ? 'bg-error/10 border-error/30 text-error'
                      : 'bg-surface-container-low border-outline-variant/50 text-on-surface-variant'}">
                    Dính 429: <span class="font-medium font-mono">{acc.usage?.rate_limit_hits ?? 0}</span>
                    {#if acc.usage?.last_rate_limit_at && !String(acc.usage.last_rate_limit_at).startsWith('0001')}
                      <span class="opacity-70">· {new Date(acc.usage.last_rate_limit_at).toLocaleString('vi-VN')}</span>
                    {/if}
                  </span>
                </div>

                <!-- Real per-model quota, when Google has actually been asked.
                     usage_pct is -1 until then, and -1 renders as the note
                     below rather than as an empty bar. -->
                {#if knownQuotas(acc).length > 0}
                  <div class="space-y-1.5">
                    {#each knownQuotas(acc) as q (q.name)}
                      <div class="space-y-0.5">
                        <div class="flex items-center justify-between text-[11px]">
                          <span class="text-on-surface-variant truncate max-w-[190px]" title={q.name}>{q.name}</span>
                          <span class="font-mono {q.status === 'exceeded' ? 'text-error' : q.status === 'warning' ? 'text-warning' : 'text-on-surface-variant'}">
                            còn {100 - q.usage_pct}%{q.reset_time ? ` · reset ${q.reset_time}` : ''}
                          </span>
                        </div>
                        <div class="h-1 rounded-full bg-surface-container-high overflow-hidden">
                          <div class="h-full rounded-full {quotaBarClass(q.status)}" style="width: {100 - q.usage_pct}%"></div>
                        </div>
                      </div>
                    {/each}
                    <div class="text-[11px] text-outline">
                      Đọc từ Google {fetchedAgo(acc.quota_fetched_at)}{acc.raw_tier ? ` · gói "${acc.raw_tier}"` : ''}
                    </div>
                  </div>
                {:else}
                  <div class="flex items-start gap-1.5 text-[11px] {acc.quota_error ? 'text-warning' : 'text-on-surface-variant'}">
                    <span class="material-symbols-outlined text-sm">{acc.quota_error ? 'warning' : 'info'}</span>
                    <span>
                      {#if acc.quota_error}
                        Chưa đọc được hạn mức: {acc.quota_error}
                      {:else}
                        Chưa hỏi Google về hạn mức — bấm nút bên phải để lấy. Số ở trên là do app tự đo trong lúc chạy.
                      {/if}
                    </span>
                  </div>
                {/if}
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
                      title="Đặt làm tài khoản hiện tại"
                      class="p-1.5 text-on-surface-variant hover:bg-surface-container-high rounded-lg transition-colors cursor-pointer"
                    >
                      <span class="material-symbols-outlined text-sm">check_circle</span>
                    </button>
                  {/if}

                  <!-- Only a Google sign-in has a plan to read; an API key
                       would just come back 401, so it does not get the button. -->
                  {#if acc.type === 'oauth_token' || acc.refresh_token}
                    <button
                      type="button"
                      on:click={() => fetchQuota(acc.id)}
                      disabled={fetchingQuota[acc.id]}
                      title="Lấy gói và hạn mức từ Google"
                      aria-label="Lấy gói và hạn mức từ Google cho {acc.email || acc.name}"
                      class="p-1.5 text-on-surface-variant hover:bg-surface-container-high rounded-lg transition-colors cursor-pointer disabled:opacity-50"
                    >
                      <span class="material-symbols-outlined text-sm {fetchingQuota[acc.id] ? 'animate-spin' : ''}">
                        {fetchingQuota[acc.id] ? 'progress_activity' : 'cloud_sync'}
                      </span>
                    </button>
                  {/if}

                  <button
                    type="button"
                    on:click={() => copyText(acc.oauth_token || acc.api_key, acc.type === 'oauth_token' ? 'OAuth token' : 'API key')}
                    title="Sao chép token/key"
                    class="p-1.5 text-on-surface-variant hover:bg-surface-container-high rounded-lg transition-colors cursor-pointer"
                  >
                    <span class="material-symbols-outlined text-sm">content_copy</span>
                  </button>

                  <button
                    type="button"
                    on:click={() => handleToggleStatus(acc.id)}
                    title={acc.status === 'active' ? 'Đang bật — bấm để tắt' : 'Đang tắt — bấm để bật'}
                    class="p-1.5 text-on-surface-variant hover:bg-surface-container-high rounded-lg transition-colors cursor-pointer"
                  >
                    <span class="material-symbols-outlined text-sm {acc.status === 'active' ? 'text-success' : ''}">
                      {acc.status === 'active' ? 'toggle_on' : 'toggle_off'}
                    </span>
                  </button>

                  <button
                    type="button"
                    on:click={() => handleDelete(acc.id, acc.name)}
                    title="Xoá khỏi pool"
                    class="p-1.5 text-error hover:bg-error/10 rounded-lg transition-colors cursor-pointer"
                  >
                    <span class="material-symbols-outlined text-sm">delete</span>
                  </button>
                </div>
              </td>
            </tr>
          {:else}
            <!-- Lúc đang tải mà hiện "chưa có tài khoản nào" là nói sai với
                 người dùng: pool có thể đang đầy, chỉ là chưa đọc xong. -->
            {#if isLoading}
              <tr>
                <td colspan="5" class="p-8 text-center text-on-surface-variant">
                  <span class="inline-flex items-center gap-2">
                    <span class="material-symbols-outlined text-sm animate-spin">progress_activity</span>
                    Đang tải danh sách tài khoản…
                  </span>
                </td>
              </tr>
            {:else}
              <tr>
                <td colspan="5" class="p-8 text-center text-on-surface-variant">
                  <div class="space-y-3">
                    <p>
                      {accounts.length === 0
                        ? 'Chưa có tài khoản nào trong pool.'
                        : 'Không có tài khoản nào khớp bộ lọc hiện tại.'}
                    </p>
                    {#if accounts.length === 0}
                      <button
                        type="button"
                        on:click={onOpenGoogleLogin}
                        class="inline-flex items-center gap-1.5 border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors cursor-pointer"
                      >
                        <span class="material-symbols-outlined text-sm">login</span> Đăng nhập Google
                      </button>
                    {/if}
                  </div>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Bảng không phân trang, nên chỉ hiện số lượng thật thay vì bộ nút trang
         bấm không ăn. -->
    <div class="p-3 bg-surface-container-low border-t border-outline-variant text-xs text-on-surface-variant">
      {#if accounts.length === 0}
        Hiển thị 0 mục
      {:else}
        Hiển thị <strong class="font-medium text-on-surface">{filteredAccounts.length}</strong>
        trong tổng số <strong class="font-medium text-on-surface">{accounts.length}</strong> mục
      {/if}
    </div>
  </div>
</div>

<!-- Hỏi lại trước khi xoá, bằng modal của app thay cho confirm() của webview. -->
{#if confirmDelete}
  {@const cd = confirmDelete}
  <div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
    <!-- tabindex="-1": see ClaimsPage — an invisible full-viewport button is
         otherwise the dialog's first tab stop. -->
    <button
      type="button"
      tabindex="-1"
      aria-hidden="true"
      class="absolute inset-0 cursor-default"
      on:click={() => (confirmDelete = null)}
    ></button>
    <div class="relative w-full max-w-sm bg-surface-container-lowest border border-outline-variant rounded-xl p-4 space-y-3 shadow-lg">
      <h4 class="text-sm font-semibold text-on-surface">{cd.title}</h4>
      <p class="text-xs text-on-surface-variant">{cd.body}</p>
      <div class="flex justify-end gap-2 pt-1">
        <button
          type="button"
          on:click={() => (confirmDelete = null)}
          class="border border-outline-variant text-on-surface-variant px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-surface-container-high transition-colors cursor-pointer"
        >
          Huỷ
        </button>
        <button
          type="button"
          on:click={() => { confirmDelete = null; cd.run(); }}
          class="border border-outline-variant text-error px-3 py-1.5 rounded-lg text-xs font-medium hover:bg-error/10 transition-colors cursor-pointer"
        >
          Xoá
        </button>
      </div>
    </div>
  </div>
{/if}
