<script lang="ts">
  import type { TreeNode } from '../../lib/fileTree';
  import Self from './FileTreeNode.svelte';

  export let node: TreeNode;
  export let depth = 0;
  export let activePath = '';
  export let dirtyPaths: Set<string> = new Set();
  export let expanded: Set<string>;
  export let onOpen: (path: string) => void;
  export let onToggle: (path: string) => void;

  // Expansion state lives in the parent so it survives a filter changing the
  // tree shape; a Set is not reactive by identity, so the parent reassigns it.
  $: isOpen = expanded.has(node.path);

  function iconFor(name: string): string {
    const ext = (name.split('.').pop() || '').toLowerCase();
    if (ext === 'go') return 'data_object';
    if (ext === 'svelte' || ext === 'html' || ext === 'vue') return 'web';
    if (ext === 'ts' || ext === 'js' || ext === 'tsx' || ext === 'jsx') return 'javascript';
    if (ext === 'json' || ext === 'yml' || ext === 'yaml' || ext === 'toml') return 'settings';
    if (ext === 'md') return 'article';
    if (ext === 'css' || ext === 'scss') return 'palette';
    if (ext === 'png' || ext === 'jpg' || ext === 'svg' || ext === 'ico') return 'image';
    return 'description';
  }
</script>

<button
  type="button"
  on:click={() => (node.isDir ? onToggle(node.path) : onOpen(node.path))}
  style="padding-left: {depth * 12 + 8}px"
  class="w-full text-left pr-2 py-1 rounded-lg flex items-center gap-1.5 truncate transition-colors cursor-pointer
  {activePath === node.path
    ? 'bg-secondary-container text-on-secondary-container font-medium'
    : 'text-on-surface-variant hover:bg-surface-container-low hover:text-on-surface'}"
  title={node.path}
>
  {#if node.isDir}
    <span class="material-symbols-outlined text-sm flex-shrink-0">
      {isOpen ? 'expand_more' : 'chevron_right'}
    </span>
    <span class="material-symbols-outlined text-sm flex-shrink-0">
      {isOpen ? 'folder_open' : 'folder'}
    </span>
  {:else}
    <span class="w-3.5 flex-shrink-0"></span>
    <span class="material-symbols-outlined text-sm flex-shrink-0">{iconFor(node.name)}</span>
  {/if}

  <span class="truncate flex-1">{node.name}</span>

  {#if !node.isDir && dirtyPaths.has(node.path)}
    <span class="w-1.5 h-1.5 rounded-full bg-warning flex-shrink-0" title="Chưa lưu"></span>
  {/if}
</button>

{#if node.isDir && isOpen}
  {#each node.children as child (child.path + child.isDir)}
    <Self node={child} depth={depth + 1} {activePath} {dirtyPaths} {expanded} {onOpen} {onToggle} />
  {/each}
{/if}
