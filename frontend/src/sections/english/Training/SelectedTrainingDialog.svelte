<script lang="ts">
  import { api } from '../../../lib/api/client'
  import type { Group, TrainingDirection } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Modal from '../../../lib/components/Modal.svelte'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { route } from '../../../lib/state/route.svelte'
  import { session } from '../../../lib/state/session.svelte'
  import DirectionSelect from './DirectionSelect.svelte'

  let { group, wordIds, onclose, onstarted }: {
    group: Group; wordIds: number[]; onclose: () => void; onstarted: () => void
  } = $props()
  const formId = $props.id()
  // Deliberately selected words may not be due. Free mode practices every selected
  // word; ordinary mode remains available with the existing SRS/due/new-limit rules.
  let free = $state(true)
  let strategy = $state<TrainingDirection>(session.active?.strategy ?? 'en_ru')
  let starting = $state(false)
  let error = $state<string | null>(null)
  let nothingDue = $state(false)

  async function start(e?: SubmitEvent, asFree = free) {
    e?.preventDefault()
    if (starting || wordIds.length === 0 || session.phase === 'session') return
    starting = true
    error = null
    nothingDue = false
    try {
      const res = await api.training.start(group.id, strategy, asFree, wordIds)
      if (res.sessionId === null) { nothingDue = true; return }
      session.begin({ sessionId: res.sessionId, groupId: group.id, groupName: group.name, strategy, free: asFree }, res.cards)
      onstarted()
      route.go('english', 'training', String(group.id))
    } catch(e) {
      error = errorText(e)
    } finally {
      starting = false
    }
  }
</script>

<Modal open title={t('groups.selection.train')} onclose={() => !starting && onclose()}>
  <form id={formId} class="form" onsubmit={e => start(e)}>
    <p><strong>#{group.name}</strong> · {t('groups.selection.count',{count:wordIds.length})}</p>
    <DirectionSelect bind:value={strategy} disabled={starting} />
    <label class="free"><input type="checkbox" bind:checked={free} disabled={starting} onchange={() => nothingDue = false} /><span>{t('training.free')}</span></label>
    <p class="muted hint">{free ? t('groups.selection.freeHint') : t('groups.selection.normalHint')}</p>
    {#if session.phase === 'session'}
      <p class="hint">{t('groups.selection.active')}</p>
      <Button icon="play" onclick={() => route.go('english','training')}>{t('groups.selection.resume')}</Button>
    {/if}
    {#if nothingDue}<p role="status" class="hint">{t('groups.selection.nothingDue')}</p><Button onclick={() => start(undefined,true)} disabled={starting || wordIds.length===0}>{t('training.startFree')}</Button>{/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </form>
  {#snippet footer()}
    <Button variant="ghost" disabled={starting} onclick={onclose}>{t('common.cancel')}</Button>
    <Button type="submit" form={formId} variant="primary" icon="play" loading={starting} disabled={wordIds.length===0 || session.phase==='session'}>{t('groups.selection.start',{count:wordIds.length})}</Button>
  {/snippet}
</Modal>

<style>
  .form{display:flex;flex-direction:column;gap:var(--s-3)}
  label{display:flex;flex-direction:column;gap:var(--s-1);font-size:var(--fs-sm);font-weight:600}
  .free{flex-direction:row;align-items:center;gap:var(--s-2)}.free input{accent-color:var(--c-green)}
  .hint,.error{font-size:var(--fs-sm);line-height:1.5}.error{color:var(--c-red)}
  strong{overflow-wrap:anywhere}
</style>
