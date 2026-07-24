<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte';
  
  export let options: { value: string; label: string }[] = [];
  export let value: string = '';
  export let placeholder: string = 'Select an option';
  export let disabled: boolean = false;
  
  let isOpen = false;
  let dropdownRef: HTMLDivElement;
  
  const dispatch = createEventDispatcher();
  
  $: selectedOption = options.find(o => o.value === value);
  
  function toggle() {
    if (!disabled) {
      isOpen = !isOpen;
    }
  }
  
  function selectOption(optionValue: string) {
    value = optionValue;
    isOpen = false;
    dispatch('change', value);
  }
  
  function handleClickOutside(event: MouseEvent) {
    if (dropdownRef && !dropdownRef.contains(event.target as Node)) {
      isOpen = false;
    }
  }
  
  onMount(() => {
    document.addEventListener('click', handleClickOutside);
    return () => {
      document.removeEventListener('click', handleClickOutside);
    };
  });
</script>

<div class="relative inline-block w-full" bind:this={dropdownRef}>
  <button
    type="button"
    class="flex items-center justify-between w-full bg-surface-container-low border border-outline-variant rounded-lg px-3 py-1.5 text-xs font-mono outline-none transition-all {disabled ? 'opacity-50 cursor-not-allowed' : 'hover:bg-surface-container hover:border-outline focus:ring-2 focus:ring-primary'}"
    on:click={toggle}
    {disabled}
  >
    <span class="truncate text-on-surface">
      {selectedOption ? selectedOption.label : placeholder}
    </span>
    <span class="material-symbols-outlined text-base text-on-surface-variant transition-transform {isOpen ? 'rotate-180' : ''}">
      expand_more
    </span>
  </button>

  {#if isOpen}
    <div class="absolute z-50 w-full mt-1 bg-surface-container-lowest border border-outline-variant rounded-lg shadow-lg overflow-hidden py-1 max-h-60 overflow-y-auto">
      {#each options as option}
        <button
          type="button"
          class="w-full text-left px-3 py-2 text-xs font-mono transition-colors {value === option.value ? 'bg-primary-container text-on-primary-container font-bold' : 'text-on-surface hover:bg-surface-container-low'}"
          on:click={() => selectOption(option.value)}
        >
          {option.label}
        </button>
      {/each}
    </div>
  {/if}
</div>
