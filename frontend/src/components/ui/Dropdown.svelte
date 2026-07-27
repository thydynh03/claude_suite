<script lang="ts">
  import { onMount, tick, createEventDispatcher } from 'svelte';

  export let options: { value: string; label: string }[] = [];
  export let value: string = '';
  export let placeholder: string = 'Chọn một mục';
  export let disabled: boolean = false;

  let isOpen = false;
  let dropdownRef: HTMLDivElement;
  let triggerRef: HTMLButtonElement;
  // Filled by the keyed list below; used only to move focus between options.
  let optionRefs: HTMLButtonElement[] = [];

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
    triggerRef?.focus();
  }

  function close() {
    isOpen = false;
    triggerRef?.focus();
  }

  function focusOption(index: number) {
    const count = options.length;
    if (count === 0) return;
    const next = ((index % count) + count) % count;
    optionRefs[next]?.focus();
  }

  // The list only exists once `isOpen` has been flushed to the DOM, so opening
  // and focusing in the same keystroke has to wait for the update.
  async function openAndFocus(index: number) {
    if (disabled) return;
    isOpen = true;
    await tick();
    focusOption(index);
  }

  function onTriggerKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      isOpen = false;
      return;
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      openAndFocus(0);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      openAndFocus(options.length - 1);
    }
  }

  function onOptionKeydown(event: KeyboardEvent, index: number) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      focusOption(index + 1);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      focusOption(index - 1);
    } else if (event.key === 'Home') {
      event.preventDefault();
      focusOption(0);
    } else if (event.key === 'End') {
      event.preventDefault();
      focusOption(options.length - 1);
    } else if (event.key === 'Escape') {
      event.preventDefault();
      close();
    }
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
    bind:this={triggerRef}
    aria-haspopup="listbox"
    aria-expanded={isOpen}
    class="flex items-center justify-between w-full bg-surface-container-low border border-outline-variant rounded-lg px-3 py-1.5 text-xs transition-colors {disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer hover:bg-surface-container-high'}"
    on:click={toggle}
    on:keydown={onTriggerKeydown}
    {disabled}
  >
    <span class="truncate text-on-surface">
      {selectedOption ? selectedOption.label : placeholder}
    </span>
    <span class="material-symbols-outlined text-sm text-on-surface-variant transition-transform {isOpen ? 'rotate-180' : ''}">
      expand_more
    </span>
  </button>

  {#if isOpen}
    <!-- shadow-lg stays: the no-shadow rule is for cards, and this list floats
         over them. Stripped, it carried the same surface and border as the
         Kanban card underneath and read as part of it. -->
    <div
      role="listbox"
      class="absolute z-50 w-full mt-1 bg-surface-container-lowest border border-outline-variant rounded-lg shadow-lg overflow-hidden py-1 max-h-60 overflow-y-auto"
    >
      {#each options as option, i}
        <button
          type="button"
          role="option"
          aria-selected={value === option.value}
          bind:this={optionRefs[i]}
          class="w-full text-left px-3 py-2 text-xs transition-colors cursor-pointer {value === option.value ? 'bg-secondary-container text-on-secondary-container font-medium' : 'text-on-surface hover:bg-surface-container-low'}"
          on:click={() => selectOption(option.value)}
          on:keydown={(e) => onOptionKeydown(e, i)}
        >
          {option.label}
        </button>
      {/each}
    </div>
  {/if}
</div>
