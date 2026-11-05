<script lang="ts">
  // Поля записи ленты: тип, привычка (или общая), текст. Общий для новой записи и правки.
  import { untrack } from 'svelte'
  import type { Habit, NoteInput, NoteKind } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Segmented from '../../../lib/components/Segmented.svelte'
  import { t } from '../../../lib/i18n/i18n.svelte'

  interface Props {
    habits: Habit[]
    initial?: NoteInput
    submitText: string
    oncancel?: () => void
    /** true — успешно; форма новой записи после этого очищается. */
    onsubmit: (n: NoteInput) => Promise<boolean>
  }
  let { habits, initial, submitText, oncancel, onsubmit }: Props = $props()

  const init = untrack(() => initial)
  let kind = $state<NoteKind>(init?.kind ?? 'observation')
  let habitId = $state<number | null>(init?.habitId ?? null)
  let body = $state(init?.body ?? '')
  let saving = $state(false)

  // В выборе — активные привычки и та, к которой запись уже привязана (даже архивная).
  const options = $derived(habits.filter((h) => !h.archived || h.id === init?.habitId))

  async function submit(e?: Event) {
    e?.preventDefault()
    if (!body.trim() || saving) return
    saving = true
    try {
      if ((await onsubmit({ kind, habitId, body })) && !init) body = ''
    } finally {
      saving = false
    }
  }
</script>

<form class="editor" onsubmit={submit}>
  <div class="row">
    <Segmented
      label={t('habits.filterKind')}
      value={kind}
      options={[
        { value: 'observation', label: t('habits.noteObservation') },
        { value: 'insight', label: t('habits.noteInsight') },
      ]}
      onchange={(v) => (kind = v)}
    />
    <select class="input habit" bind:value={habitId} aria-label={t('habits.filterHabit')}>
      <option value={null}>{t('habits.noteGeneral')}</option>
      {#each options as h (h.id)}<option value={h.id}>{h.name}</option>{/each}
    </select>
  </div>
  <textarea
    class="input"
    bind:value={body}
    placeholder={t('habits.notePlaceholder')}
    rows="3"
    onkeydown={(e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') void submit()
      if (e.key === 'Escape' && oncancel) oncancel()
    }}
  ></textarea>
  <div class="row end">
    {#if oncancel}<Button variant="ghost" size="sm" onclick={oncancel} disabled={saving}>{t('common.cancel')}</Button>{/if}
    <Button type="submit" variant="primary" size="sm" icon="check" loading={saving} disabled={!body.trim()} title="⌘/Ctrl + Enter">{submitText}</Button>
  </div>
</form>

<style>
  .editor {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--s-3);
  }
  .end {
    justify-content: flex-end;
  }
  .habit {
    width: auto;
    min-width: 180px;
  }
  textarea {
    resize: vertical;
    line-height: 1.5;
    field-sizing: content;
    min-height: 72px;
  }
</style>
