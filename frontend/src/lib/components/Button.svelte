<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLButtonAttributes } from 'svelte/elements'
  import Icon, { type IconName } from './Icon.svelte'
  import Spinner from './Spinner.svelte'

  interface Props extends HTMLButtonAttributes {
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
    size?: 'sm' | 'md' | 'lg'
    icon?: IconName
    loading?: boolean
    children?: Snippet
  }
  let {
    variant = 'secondary',
    size = 'md',
    icon,
    loading = false,
    disabled,
    type = 'button',
    children,
    ...rest
  }: Props = $props()
</script>

<!-- loading блокирует повторный клик («Создать карточки» дважды — два bulk-запроса) -->
<button
  {type}
  class="btn {variant} {size}"
  disabled={disabled || loading}
  aria-busy={loading || undefined}
  {...rest}
>
  {#if loading}
    <Spinner size={size === 'sm' ? 14 : 18} />
  {:else if icon}
    <Icon name={icon} size={size === 'sm' ? 16 : 20} />
  {/if}
  {#if children}<span>{@render children()}</span>{/if}
</button>

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-2);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    font-weight: 600;
    cursor: pointer;
    white-space: nowrap;
    box-shadow: var(--shadow-sm);
    transition:
      transform var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out),
      background var(--dur-fast) var(--ease-out);
  }
  /* neo-brutalism: при наведении кнопка «приподнимается», при нажатии «вдавливается» */
  .btn:hover:not(:disabled) {
    transform: translate(-1px, -1px);
    box-shadow: 3px 3px 0 var(--c-border);
  }
  .btn:active:not(:disabled) {
    transform: translate(2px, 2px);
    box-shadow: none;
  }
  .btn:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }

  .sm {
    padding: var(--s-1) var(--s-2);
    font-size: var(--fs-sm);
  }
  .md {
    padding: var(--s-2) var(--s-4);
  }
  .lg {
    padding: var(--s-3) var(--s-5);
    font-size: var(--fs-lg);
  }

  .primary {
    background: var(--c-green);
    color: #fff;
  }
  .danger {
    background: var(--c-red);
    color: #fff;
  }
  .ghost {
    border-color: transparent;
    box-shadow: none;
    background: transparent;
  }
  .ghost:hover:not(:disabled) {
    background: var(--c-surface-2);
    box-shadow: none;
    transform: none;
  }
</style>
