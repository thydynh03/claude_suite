<script context="module" lang="ts">
  import * as AppBindings from '../../../wailsjs/go/main/App';

  export type CatalogModel = { id: string; label: string; provider: string; source: string };

  // One fetch shared by every dropdown on screen. The catalog is the single
  // source of truth (backend/modelcatalog) — the nine hand-copied lists this
  // component replaced each drifted their own way.
  let cache: CatalogModel[] | null = null;
  let inflight: Promise<CatalogModel[]> | null = null;

  export function loadModelCatalog(force = false): Promise<CatalogModel[]> {
    if (cache && !force) return Promise.resolve(cache);
    if (!inflight || force) {
      inflight = Promise.resolve()
        .then(() => (AppBindings as any).GetAvailableModels())
        .then((r: any) => {
          cache = Array.isArray(r) ? r : [];
          return cache!;
        })
        .catch(() => cache ?? []);
    }
    return inflight;
  }

  export function providerOfModel(models: CatalogModel[], id: string): 'claude' | 'anti' {
    const hit = models.find((m) => m.id === id);
    if (hit) return hit.provider === 'anti' ? 'anti' : 'claude';
    const low = (id || '').toLowerCase();
    // Mirror of backend/provider.ResolveProvider's marker rule.
    return low.includes('gemini') || low.includes('thinking') || low.includes('antigravity') ? 'anti' : 'claude';
  }
</script>

<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte';

  /** Selected model id (two-way). Unknown ids are legal — they open the custom input. */
  export let value = '';
  /** Limit the list to one provider ('' shows both, grouped). */
  export let provider: '' | 'claude' | 'anti' = '';
  /**
   * Shared input recipe. Call sites used to pass their own string each time and
   * the same control ended up a different size and a different border on every
   * page, so overriding this is a last resort.
   */
  export let selectClass =
    'w-full px-3 py-2 rounded-lg bg-surface-container-low border border-outline-variant text-xs text-on-surface focus:border-primary';
  /**
   * Accessible name for the control. Call sites used to write a <label for>
   * pointing at an id this component never rendered, so the label was attached
   * to nothing and the dropdown announced itself as unlabelled.
   */
  export let ariaLabel = 'Chọn model';

  const CUSTOM = '__custom__';
  const dispatch = createEventDispatcher<{ change: { value: string; provider: 'claude' | 'anti' } }>();

  let models: CatalogModel[] = [];
  let useCustom = false;
  let customText = '';

  onMount(async () => {
    models = await loadModelCatalog();
    if (value && !models.some((m) => m.id === value)) {
      useCustom = true;
      customText = value;
    }
  });

  $: claudeModels = models.filter((m) => m.provider === 'claude');
  $: antiModels = models.filter((m) => m.provider === 'anti');
  $: visible = provider === 'claude' ? claudeModels : provider === 'anti' ? antiModels : models;

  function emit(v: string) {
    value = v;
    dispatch('change', { value: v, provider: providerOfModel(models, v) });
  }

  function onSelect(e: Event) {
    const v = (e.currentTarget as HTMLSelectElement).value;
    if (v === CUSTOM) {
      useCustom = true;
      customText = value;
      return;
    }
    emit(v);
  }

  function onCustomChange() {
    const v = customText.trim();
    if (v) emit(v);
  }
</script>

{#if !useCustom}
  <select class={selectClass} {value} aria-label={ariaLabel} on:change={onSelect}>
    {#if provider === ''}
      <optgroup label="Claude Agent">
        {#each claudeModels as m (m.id)}
          <option value={m.id}>{m.label}</option>
        {/each}
      </optgroup>
      <optgroup label="Antigravity Agent (Gemini)">
        {#each antiModels as m (m.id)}
          <option value={m.id}>{m.label}</option>
        {/each}
      </optgroup>
    {:else}
      {#each visible as m (m.id)}
        <option value={m.id}>{m.label}</option>
      {/each}
    {/if}
    <option value={CUSTOM}>Khác — tự nhập model id…</option>
  </select>
{:else}
  <div class="flex items-center gap-1">
    <input
      class={selectClass}
      bind:value={customText}
      on:change={onCustomChange}
      aria-label={ariaLabel}
      placeholder="model id (vd: claude-opus-5)"
      spellcheck="false"
    />
    <button
      type="button"
      class="p-2 rounded-lg text-on-surface-variant hover:bg-surface-container-high cursor-pointer flex-shrink-0"
      title="Chọn từ danh sách"
      on:click={() => (useCustom = false)}
    >
      <span class="material-symbols-outlined text-base">list</span>
    </button>
  </div>
{/if}
