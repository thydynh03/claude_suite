<script lang="ts">
  import { onMount } from 'svelte';
  import { addLog, addToast } from '../../lib/stores/appState';
  import * as AppBindings from '../../../wailsjs/go/main/App';

  let roles: string[] = [];
  let selectedRole = '';
  let roleContent = '';
  let isLoading = false;
  let isSaving = false;
  let isCreating = false;
  let newRoleName = '';

  onMount(async () => {
    await loadRoles();
  });

  async function loadRoles() {
    try {
      if ((AppBindings as any).ListRoles) {
        roles = await (AppBindings as any).ListRoles() || [];
        if (roles.length > 0 && !selectedRole) {
          selectRole(roles[0]);
        }
      }
    } catch (e: any) {
      addLog(`Failed to load roles: ${e.message}`, 'ERROR');
      addToast(`Failed to load roles: ${e.message}`, 'ERROR');
    }
  }

  async function selectRole(role: string) {
    if (!role) return;
    // Moving to another role cancels a delete that was awaiting confirmation.
    confirmingDelete = '';
    selectedRole = role;
    isLoading = true;
    try {
      if ((AppBindings as any).GetRoleContent) {
        roleContent = await (AppBindings as any).GetRoleContent(role);
      }
    } catch (e: any) {
      addLog(`Failed to read role ${role}: ${e.message}`, 'ERROR');
      addToast(`Failed to read role ${role}: ${e.message}`, 'ERROR');
      roleContent = '';
    } finally {
      isLoading = false;
    }
  }

  async function saveRole() {
    if (!selectedRole) return;
    isSaving = true;
    try {
      if ((AppBindings as any).SaveRoleContent) {
        await (AppBindings as any).SaveRoleContent(selectedRole, roleContent);
        addLog(`Đã lưu cấu hình role: ${selectedRole}`, 'SUCCESS');
        addToast(`Đã lưu cấu hình role: ${selectedRole}`, 'SUCCESS');
      }
    } catch (e: any) {
      addLog(`Lỗi khi lưu role ${selectedRole}: ${e.message}`, 'ERROR');
      addToast(`Lỗi khi lưu role ${selectedRole}: ${e.message}`, 'ERROR');
    } finally {
      isSaving = false;
    }
  }

  async function createRole() {
    if (!newRoleName.trim()) return;
    let name = newRoleName.trim();
    if (!name.endsWith('.md')) {
      name += '.md';
    }
    isCreating = true;
    try {
      if ((AppBindings as any).SaveRoleContent) {
        await (AppBindings as any).SaveRoleContent(name, `# ${name}\n\nNhập quy tắc và system prompt cho role này tại đây.`);
        addLog(`Đã tạo role mới: ${name}`, 'SUCCESS');
        addToast(`Đã tạo role mới: ${name}`, 'SUCCESS');
        await loadRoles();
        selectRole(name);
        newRoleName = '';
      }
    } catch (e: any) {
      addLog(`Lỗi khi tạo role: ${e.message}`, 'ERROR');
      addToast(`Lỗi khi tạo role: ${e.message}`, 'ERROR');
    } finally {
      isCreating = false;
    }
  }

  let confirmingDelete = '';

  async function deleteRole(role: string) {
    // In-app two-step confirm instead of the native confirm() popup.
    if (confirmingDelete !== role) {
      confirmingDelete = role;
      return;
    }
    confirmingDelete = '';
    try {
      if ((AppBindings as any).DeleteRole) {
        await (AppBindings as any).DeleteRole(role);
        addLog(`Đã xóa role: ${role}`, 'SUCCESS');
        addToast(`Đã xóa role: ${role}`, 'SUCCESS');
        if (selectedRole === role) {
          selectedRole = '';
          roleContent = '';
        }
        await loadRoles();
      }
    } catch (e: any) {
      addLog(`Lỗi khi xóa role ${role}: ${e.message}`, 'ERROR');
      addToast(`Lỗi khi xóa role ${role}: ${e.message}`, 'ERROR');
    }
  }
</script>

<div class="h-full flex flex-col bg-background text-on-surface p-6">
  <div class="flex items-center justify-between mb-6">
    <div>
      <h1 class="text-lg font-semibold text-on-surface">Kho Agent Roles</h1>
      <p class="text-on-surface-variant text-sm mt-0.5">Quản lý file cấu hình Markdown (.md) cho các role của agent.</p>
    </div>
  </div>

  <div class="flex-1 flex gap-6 min-h-0">
    <!-- Sidebar -->
    <div class="w-64 flex flex-col gap-4">
      <div class="bg-surface border border-outline-variant rounded-xl p-4 flex flex-col gap-4 h-full">
        <div class="text-sm font-semibold text-on-surface">Danh sách roles</div>

        <!-- Create new -->
        <div class="flex gap-2">
          <input
            type="text"
            bind:value={newRoleName}
            placeholder="Tên role mới..."
            class="flex-1 min-w-0 bg-background text-on-surface px-3 py-1.5 rounded-lg border border-outline-variant focus:border-primary outline-none transition-colors text-xs"
            on:keydown={(e) => e.key === 'Enter' && createRole()}
          />
          <button
            class="bg-primary text-on-primary p-1.5 rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer"
            on:click={createRole}
            disabled={!newRoleName.trim() || isCreating}
            title="Tạo role mới"
            aria-label="Tạo role mới"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"></path>
            </svg>
          </button>
        </div>

        <div class="h-px bg-outline-variant w-full"></div>

        <!-- Role list -->
        <div class="flex-1 overflow-y-auto pr-2 space-y-1 custom-scrollbar">
          {#each roles as role}
            <div
              role="button"
              tabindex="0"
              class="group flex items-center justify-between gap-2 px-3 py-2 rounded-lg cursor-pointer transition-colors border {selectedRole === role ? 'bg-secondary-container border-transparent text-on-secondary-container' : 'border-transparent hover:bg-surface-container-high text-on-surface'}"
              on:click={() => selectRole(role)}
              on:keydown={(e) => {
                // Only the row itself. The delete button lives inside this
                // element, so its Enter keydown bubbled up here first and
                // selectRole cleared confirmingDelete before the click even
                // fired — arming and disarming forever, with the second step
                // unreachable from the keyboard.
                if (e.target !== e.currentTarget) return;
                if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectRole(role); }
              }}
            >
              <div class="flex items-center gap-2 overflow-hidden">
                <span class="material-symbols-outlined text-sm flex-shrink-0 text-on-surface-variant">description</span>
                <span class="text-xs font-medium truncate">{role}</span>
              </div>
              {#if !['agents.md', 'claude.md', 'antigravity.md'].includes(role)}
                <!-- Two-step confirm, in place of the native confirm() popup.
                     The first click has to *say* it armed something, or the
                     button just looks broken. -->
                {#if confirmingDelete === role}
                  <button
                    class="flex-shrink-0 px-2 py-0.5 rounded-lg text-[11px] font-medium text-error hover:bg-error/10 transition-colors cursor-pointer whitespace-nowrap"
                    on:click|stopPropagation={() => deleteRole(role)}
                    title="Bấm lần nữa để xoá"
                  >
                    Xoá?
                  </button>
                {:else}
                  <button
                    class="flex-shrink-0 opacity-0 group-hover:opacity-100 focus:opacity-100 p-1 text-on-surface-variant hover:text-error hover:bg-error/10 rounded-lg transition-colors cursor-pointer"
                    on:click|stopPropagation={() => deleteRole(role)}
                    title="Xoá role"
                    aria-label={`Xoá role ${role}`}
                  >
                    <span class="material-symbols-outlined text-sm block">delete</span>
                  </button>
                {/if}
              {/if}
            </div>
          {/each}
          {#if roles.length === 0}
            <div class="text-center py-4 text-on-surface-variant text-xs">Chưa có file role nào.</div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Editor -->
    <div class="flex-1 bg-surface border border-outline-variant rounded-xl flex flex-col min-w-0">
      {#if selectedRole}
        <div class="border-b border-outline-variant p-4 flex items-center justify-between gap-4">
          <div class="min-w-0">
            <h2 class="text-sm font-semibold text-on-surface leading-tight truncate">{selectedRole}</h2>
            <div class="text-xs text-on-surface-variant mt-0.5">Sửa file cấu hình system prompt</div>
          </div>
          <button
            class="flex-shrink-0 bg-primary text-on-primary text-xs font-medium px-3 py-1.5 rounded-lg hover:opacity-90 transition-opacity disabled:opacity-50 cursor-pointer"
            on:click={saveRole}
            disabled={isSaving || isLoading}
          >
            {isSaving ? 'Đang lưu...' : 'Lưu thay đổi'}
          </button>
        </div>

        <div class="flex-1 p-4 relative">
          {#if isLoading}
            <div class="absolute inset-0 flex items-center justify-center bg-surface/70 z-10 rounded-b-xl">
              <div class="w-6 h-6 border-2 border-primary border-t-transparent rounded-full animate-spin"></div>
            </div>
          {/if}
          <textarea
            class="w-full h-full bg-background border border-outline-variant rounded-lg p-4 text-on-surface font-mono text-xs resize-none focus:border-primary outline-none transition-colors custom-scrollbar leading-relaxed"
            bind:value={roleContent}
            spellcheck="false"
            placeholder="Nhập nội dung markdown..."
          ></textarea>
        </div>
      {:else}
        <div class="flex-1 flex flex-col items-center justify-center text-on-surface-variant p-8 text-center">
          <p class="text-sm font-medium text-on-surface">Chưa chọn role nào</p>
          <p class="text-xs mt-2 max-w-sm">Chọn một role bên cột trái hoặc tạo mới để xem và chỉnh sửa cấu hình system prompt.</p>
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .custom-scrollbar::-webkit-scrollbar {
    width: 6px;
  }
  .custom-scrollbar::-webkit-scrollbar-track {
    background: transparent;
  }
  /* Token-driven, so the thumb follows the theme; it used to be a hardcoded
     gray-400 rgba that stayed light-grey in dark mode. */
  .custom-scrollbar::-webkit-scrollbar-thumb {
    background: var(--outline-variant);
    border-radius: 10px;
  }
  .custom-scrollbar::-webkit-scrollbar-thumb:hover {
    background: var(--outline);
  }
</style>
