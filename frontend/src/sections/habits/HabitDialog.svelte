<script lang="ts">
  // Создание и правка привычки (§17.4). habit = null — новая; open управляет видимостью.
  import { api } from '../../lib/api/client'
  import type { Habit, HabitKind } from '../../lib/api/types'
  import Button from '../../lib/components/Button.svelte'
  import ColorPicker from '../../lib/components/ColorPicker.svelte'
  import Modal from '../../lib/components/Modal.svelte'
  import Segmented from '../../lib/components/Segmented.svelte'
  import { errorText, t } from '../../lib/i18n/i18n.svelte'

  interface Props {
    open: boolean
    habit: Habit | null
    onclose: () => void
    onsaved: (h: Habit) => void
  }
  let { open, habit, onclose, onsaved }: Props = $props()

  let name = $state('')
  let kind = $state<HabitKind>('positive')
  let color = $state<string | null>(null)
  let trackTrigger = $state(false)
  let trackRating = $state(false)
  let trackAmount = $state(false)
  let unit = $state('')
  let saving = $state(false)
  let error = $state<string | null>(null)

  // При каждом открытии — заново заполнить форму (новая или копия правимой).
  $effect(() => {
    if (!open) return
    name = habit?.name ?? ''
    kind = habit?.kind ?? 'positive'
    color = habit?.color ?? null
    trackTrigger = habit?.trackTrigger ?? false
    trackRating = habit?.trackRating ?? false
    trackAmount = habit?.trackAmount ?? false
    unit = habit?.unit ?? ''
    error = null
  })

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (saving) return
    saving = true
    error = null
    const fields = { name, color, trackTrigger, trackRating, trackAmount, unit }
    try {
      onsaved(habit ? await api.habits.update(habit.id, fields) : await api.habits.create({ ...fields, kind }))
    } catch (err) {
      error = errorText(err) // ошибка в форме: диалог не закрывается, можно поправить
    } finally {
      saving = false
    }
  }
</script>

<Modal {open} title={habit ? t('habits.editTitle') : t('habits.new')} width={520} {onclose}>
  <form id="habit-form" class="form" onsubmit={save}>
    <label>
      <span>{t('habits.name')}</span>
      <!-- диалог открыт ради этого поля — фокус сразу в него -->
      <!-- svelte-ignore a11y_autofocus -->
      <input class="input" bind:value={name} maxlength="60" placeholder={t('habits.namePlaceholder')} required autofocus />
    </label>

    <div class="field">
      <span>{t('habits.kind')}</span>
      {#if habit}
        <p class="muted small">{t(habit.kind === 'positive' ? 'habits.positive' : 'habits.negative')} · {t('habits.kindLocked')}</p>
      {:else}
        <Segmented
          label={t('habits.kind')}
          value={kind}
          options={[
            { value: 'positive', label: t('habits.positive') },
            { value: 'negative', label: t('habits.negative') },
          ]}
          onchange={(v) => (kind = v)}
        />
        <p class="muted small">{t(kind === 'positive' ? 'habits.positiveHint' : 'habits.negativeHint')}</p>
      {/if}
    </div>

    <div class="field">
      <span>{t('habits.color')}</span>
      <ColorPicker value={color} onchange={(c) => (color = c)} />
    </div>

    <fieldset>
      <legend>{t('habits.fields')}</legend>
      <label class="check"><input type="checkbox" bind:checked={trackTrigger} /> {t('habits.trackTrigger')} <small class="muted">— {t('habits.trackTriggerHint')}</small></label>
      <label class="check"><input type="checkbox" bind:checked={trackRating} /> {t('habits.trackRating')} <small class="muted">— {t('habits.trackRatingHint')}</small></label>
      <label class="check"><input type="checkbox" bind:checked={trackAmount} /> {t('habits.trackAmount')} <small class="muted">— {t('habits.trackAmountHint')}</small></label>
      {#if trackAmount}
        <label class="unit">
          <span>{t('habits.unit')}</span>
          <input class="input" bind:value={unit} maxlength="20" placeholder={t('habits.unitPlaceholder')} />
        </label>
      {/if}
    </fieldset>

    {#if error}<p class="error">{error}</p>{/if}
  </form>

  {#snippet footer()}
    <Button variant="ghost" onclick={onclose} disabled={saving}>{t('common.cancel')}</Button>
    <Button type="submit" form="habit-form" variant="primary" loading={saving}>{habit ? t('common.save') : t('habits.create')}</Button>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }
  label,
  .field {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  label > span,
  .field > span,
  legend {
    font-weight: 600;
    font-size: var(--fs-sm);
  }
  .small {
    font-size: var(--fs-sm);
  }
  fieldset {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    margin: 0;
    padding: var(--s-3);
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
  }
  legend {
    padding: 0 var(--s-1);
  }
  .check {
    flex-direction: row;
    align-items: center;
    gap: var(--s-2);
    cursor: pointer;
  }
  .check input {
    width: 18px;
    height: 18px;
    accent-color: var(--c-green);
  }
  .unit {
    margin-left: 26px;
  }
  .error {
    color: var(--c-red);
    font-size: var(--fs-sm);
  }
</style>
