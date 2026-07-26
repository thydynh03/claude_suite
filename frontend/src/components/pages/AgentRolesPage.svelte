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

  async function deleteRole(role: string) {
    if (!confirm(`Bạn có chắc muốn xóa file role ${role} không?`)) return;
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

<div class="h-full flex flex-col bg-background text-on-background p-6">
  <div class="flex items-center justify-between mb-6">
    <div>
      <h1 class="text-3xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-primary to-tertiary">
        Kho Agent Roles
      </h1>
      <p class="text-on-surface-variant text-sm mt-1">Quản lý file cấu hình Markdown (.md) cho các Role của Agent.</p>
    </div>
  </div>

  <div class="flex-1 flex gap-6 min-h-0">
    <!-- Sidebar -->
    <div class="w-64 flex flex-col gap-4">
      <div class="bg-surface border border-outline-variant rounded-2xl p-4 flex flex-col gap-4 h-full shadow-sm">
        <div class="text-lg font-semibold text-primary mb-2">Danh sách Roles</div>
        
        <!-- Create new -->
        <div class="flex gap-2">
          <input 
            type="text" 
            bind:value={newRoleName}
            placeholder="Tên role mới..."
            class="flex-1 min-w-0 bg-background text-on-background px-3 py-2 rounded-xl border border-outline-variant focus:border-primary focus:ring-1 focus:ring-primary outline-none transition-all text-sm"
            on:keydown={(e) => e.key === 'Enter' && createRole()}
          />
          <button 
            class="bg-primary text-on-primary p-2 rounded-xl hover:bg-primary/90 transition-colors disabled:opacity-50"
            on:click={createRole}
            disabled={!newRoleName.trim() || isCreating}
            title="Tạo Role mới"
          >
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
              class="group flex items-center justify-between px-3 py-2 rounded-xl cursor-pointer transition-all border {selectedRole === role ? 'bg-primary/10 border-primary text-primary' : 'border-transparent hover:bg-surface-variant text-on-surface'}"
              on:click={() => selectRole(role)}
              on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectRole(role); } }}
            >
              <div class="flex items-center gap-2 overflow-hidden">
                <svg class="w-4 h-4 flex-shrink-0 opacity-70" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
                </svg>
                <span class="text-sm font-medium truncate">{role}</span>
              </div>
              {#if !['agents.md', 'claude.md', 'antigravity.md'].includes(role)}
                <button 
                  class="opacity-0 group-hover:opacity-100 p-1 text-error hover:bg-error/10 rounded-md transition-all"
                  on:click|stopPropagation={() => deleteRole(role)}
                  title="Xóa role"
                >
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
                  </svg>
                </button>
              {/if}
            </div>
          {/each}
          {#if roles.length === 0}
            <div class="text-center py-4 text-on-surface-variant text-sm">Chưa có file role nào.</div>
          {/if}
        </div>
      </div>
    </div>

    <!-- Editor -->
    <div class="flex-1 bg-surface border border-outline-variant rounded-2xl flex flex-col shadow-sm min-w-0">
      {#if selectedRole}
        <div class="border-b border-outline-variant p-4 flex items-center justify-between">
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center text-primary">
              <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path>
              </svg>
            </div>
            <div>
              <h2 class="text-lg font-bold text-on-surface leading-tight">{selectedRole}</h2>
              <div class="text-xs text-on-surface-variant">Sửa file cấu hình System Prompt</div>
            </div>
          </div>
          <button 
            class="px-6 py-2 bg-primary text-on-primary font-medium rounded-xl hover:bg-primary/90 transition-colors disabled:opacity-50 shadow-md shadow-primary/20"
            on:click={saveRole}
            disabled={isSaving || isLoading}
          >
            {isSaving ? 'Đang lưu...' : 'Lưu Thay Đổi'}
          </button>
        </div>

        <div class="flex-1 p-4 relative">
          {#if isLoading}
            <div class="absolute inset-0 flex items-center justify-center bg-surface/50 backdrop-blur-sm z-10 rounded-b-2xl">
              <div class="w-8 h-8 border-4 border-primary border-t-transparent rounded-full animate-spin"></div>
            </div>
          {/if}
          <textarea 
            class="w-full h-full bg-background border border-outline-variant rounded-xl p-4 text-on-background font-mono text-sm resize-none focus:border-primary focus:ring-1 focus:ring-primary outline-none transition-all custom-scrollbar leading-relaxed"
            bind:value={roleContent}
            spellcheck="false"
            placeholder="Nhập nội dung markdown..."
          ></textarea>
        </div>
      {:else}
        <div class="flex-1 flex flex-col items-center justify-center text-on-surface-variant p-8 text-center">
          <svg class="w-16 h-16 opacity-20 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
          </svg>
          <p class="text-lg font-medium">Chưa chọn Role nào</p>
          <p class="text-sm mt-2 max-w-sm">Chọn một role bên cột trái hoặc tạo mới để xem và chỉnh sửa cấu hình System Prompt.</p>
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
  .custom-scrollbar::-webkit-scrollbar-thumb {
    background: rgba(156, 163, 175, 0.3);
    border-radius: 10px;
  }
  .custom-scrollbar::-webkit-scrollbar-thumb:hover {
    background: rgba(156, 163, 175, 0.5);
  }
</style>
