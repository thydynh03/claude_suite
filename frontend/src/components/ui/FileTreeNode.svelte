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
  class="w-full text-left pr-2 py-1 rounded-lg flex items-center gap-1.5 truncate transition-all cursor-pointer
  {activePath === node.path
    ? 'bg-primary-container text-on-primary-container font-bold'
    : 'text-on-surface-variant hover:bg-surface-container-low hover:text-on-surface'}"
  title={node.path}
>
  {#if node.isDir}
    <span class="material-symbols-outlined text-[15px] flex-shrink-0 text-on-surface-variant">
      {isOpen ? 'expand_more' : 'chevron_right'}
    </span>
    <span class="material-symbols-outlined text-[15px] flex-shrink-0 text-amber-500">
      {isOpen ? 'folder_open' : 'folder'}
    </span>
  {:else}
    <span class="w-[15px] flex-shrink-0"></span>
    <span class="material-symbols-outlined text-[15px] flex-shrink-0 text-primary">{iconFor(node.name)}</span>
  {/if}

  <span class="truncate flex-1">{node.name}</span>

  {#if !node.isDir && dirtyPaths.has(node.path)}
    <span class="w-2 h-2 rounded-full bg-amber-500 flex-shrink-0" title="Chưa lưu"></span>
  {/if}
</button>

{#if node.isDir && isOpen}
  {#each node.children as child (child.path + child.isDir)}
    <Self node={child} depth={depth + 1} {activePath} {dirtyPaths} {expanded} {onOpen} {onToggle} />
  {/each}
{/if}
