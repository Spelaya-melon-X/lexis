<script lang="ts">
  import { fly } from 'svelte/transition'
  import { toasts } from '../state/toasts.svelte'
  import Icon from './Icon.svelte'

  const icons = { success: 'check', error: 'alert', info: 'check' } as const
</script>

<div class="toaster" aria-live="polite">
  {#each toasts.items as toast (toast.id)}
    <button class="toast {toast.kind}" transition:fly={{ y: 16, duration: 220 }} onclick={() => toasts.dismiss(toast.id)}>
      <Icon name={icons[toast.kind]} size={18} />
      <span>{toast.text}</span>
    </button>
  {/each}
</div>

<style>
  .toaster {
    position: fixed;
    right: var(--s-5);
    bottom: var(--s-5);
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    z-index: 100;
  }
  .toast {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    max-width: 360px;
    padding: var(--s-3) var(--s-4);
    background: var(--c-surface);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow-sm);
    text-align: left;
    cursor: pointer;
  }
  .success {
    background: var(--c-green-soft);
  }
  .error {
    background: var(--c-red-soft);
  }
</style>
