<script lang="ts">
  import type { Snippet } from 'svelte'
  import { t } from '../i18n/i18n.svelte'
  import Button from './Button.svelte'
  import Modal from './Modal.svelte'

  interface Props {
    open: boolean
    title: string
    confirmText?: string
    danger?: boolean
    loading?: boolean
    onconfirm: () => void
    oncancel: () => void
    children?: Snippet
  }
  let { open, title, confirmText, danger = false, loading = false, onconfirm, oncancel, children }: Props = $props()
</script>

<Modal {open} {title} onclose={() => !loading && oncancel()}>
  {#if children}<div class="muted">{@render children()}</div>{/if}
  {#snippet footer()}
    <Button variant="ghost" onclick={oncancel} disabled={loading}>{t('common.cancel')}</Button>
    <Button variant={danger ? 'danger' : 'primary'} onclick={onconfirm} {loading}>
      {confirmText ?? t('common.confirm')}
    </Button>
  {/snippet}
</Modal>
