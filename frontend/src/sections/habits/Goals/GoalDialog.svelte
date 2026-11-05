<script lang="ts">
  // Создание и правка цели на период (§17.4). goal = null — новая.
  import { api } from '../../../lib/api/client'
  import type { Goal, Habit } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Modal from '../../../lib/components/Modal.svelte'
  import { addDays, today } from '../../../lib/dates'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'

  interface Props {
    open: boolean
    goal: Goal | null
    habits: Habit[]
    onclose: () => void
    onsaved: (g: Goal) => void
  }
  let { open, goal, habits, onclose, onsaved }: Props = $props()

  let habitId = $state<number | null>(null)
  let title = $state('')
  let from = $state('')
  let to = $state('')
  let saving = $state(false)
  let error = $state<string | null>(null)

  // Выбор — активные привычки + та, к которой уже привязана цель (даже если в архиве).
  const options = $derived(habits.filter((h) => !h.archived || h.id === goal?.habitId))

  $effect(() => {
    if (!open) return
    const now = today()
    habitId = goal ? goal.habitId : (habits.find((h) => !h.archived)?.id ?? null)
    title = goal?.title ?? ''
    from = goal?.from ?? now
    to = goal?.to ?? addDays(now, 6)
    error = null
  })

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (saving) return
    saving = true
    error = null
    const body = { habitId, title, from, to }
    try {
      onsaved(goal ? await api.habits.updateGoal(goal.id, body) : await api.habits.createGoal(body))
    } catch (err) {
      error = errorText(err)
    } finally {
      saving = false
    }
  }
</script>

<Modal {open} title={goal ? t('habits.goalEdit') : t('habits.goalNew')} width={480} {onclose}>
  <form id="goal-form" class="form" onsubmit={save}>
    <label>
      <span>{t('habits.goalHabit')}</span>
      <select class="input" bind:value={habitId}>
        {#each options as h (h.id)}<option value={h.id}>{h.name}</option>{/each}
        <option value={null}>{t('habits.goalNoHabit')}</option>
      </select>
    </label>
    <label>
      <span>{habitId === null ? t('habits.goalTitle') : t('habits.goalTitleOptional')}</span>
      <input class="input" bind:value={title} maxlength="200" placeholder={t('habits.goalTitlePlaceholder')} required={habitId === null} />
    </label>
    <div class="dates">
      <label>
        <span>{t('habits.from')}</span>
        <input class="input" type="date" bind:value={from} required />
      </label>
      <label>
        <span>{t('habits.to')}</span>
        <input class="input" type="date" bind:value={to} min={from} required />
      </label>
    </div>
    {#if error}<p class="error">{error}</p>{/if}
  </form>

  {#snippet footer()}
    <Button variant="ghost" onclick={onclose} disabled={saving}>{t('common.cancel')}</Button>
    <Button type="submit" form="goal-form" variant="primary" loading={saving}>{goal ? t('common.save') : t('habits.create')}</Button>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }
  label {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  label > span {
    font-weight: 600;
    font-size: var(--fs-sm);
  }
  .dates {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-3);
  }
  .error {
    color: var(--c-red);
    font-size: var(--fs-sm);
  }
</style>
