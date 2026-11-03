<script lang="ts">
  import type { Snippet } from 'svelte'
  import { t } from '../i18n/i18n.svelte'
  import Icon from './Icon.svelte'

  // Модалка на нативном <dialog>: фокус-ловушка, Esc и затемнение даёт браузер.
  // open управляется снаружи; Esc/крестик/клик по фону зовут onclose.
  interface Props {
    open: boolean
    title: string
    width?: number
    onclose: () => void
    children: Snippet
    footer?: Snippet
  }
  let { open, title, width = 440, onclose, children, footer }: Props = $props()

  let dialog: HTMLDialogElement | undefined = $state()

  $effect(() => {
    if (!dialog) return
    if (open && !dialog.open) dialog.showModal()
    if (!open && dialog.open) dialog.close()
  })

  // Клик по затемнению: событие приходит на сам <dialog>, а не на его содержимое.
  function onclick(e: MouseEvent) {
    if (e.target === dialog) onclose()
  }
</script>

<dialog
  bind:this={dialog}
  style:--w="{width}px"
  oncancel={(e) => {
    e.preventDefault() // закрытие — решает родитель через open
    onclose()
  }}
  {onclick}
>
  <div class="inner">
    <header>
      <h2>{title}</h2>
      <button class="close" aria-label={t('common.close')} onclick={onclose}><Icon name="x" /></button>
    </header>
    <div class="body">{@render children()}</div>
    {#if footer}<footer>{@render footer()}</footer>{/if}
  </div>
</dialog>

<style>
  dialog {
    width: min(var(--w), calc(100vw - 32px));
    padding: 0;
    border: var(--bw-strong) solid var(--c-border);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
  }
  dialog[open] {
    animation: pop var(--dur) var(--ease-spring);
  }
  dialog::backdrop {
    background: rgb(0 0 0 / 0.35);
  }
  .inner {
    padding: var(--s-5);
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-3);
    margin-bottom: var(--s-3);
  }
  h2 {
    font-size: var(--fs-lg);
  }
  .close {
    display: grid;
    place-items: center;
    padding: var(--s-1);
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    cursor: pointer;
  }
  .close:hover {
    background: var(--c-surface-2);
  }
  footer {
    display: flex;
    justify-content: flex-end;
    gap: var(--s-2);
    margin-top: var(--s-5);
  }
  @keyframes pop {
    from {
      opacity: 0;
      transform: scale(0.96);
    }
  }
</style>
