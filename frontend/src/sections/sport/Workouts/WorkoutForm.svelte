<script lang="ts">
  // Форма тренировки (§18.4, §18.5.7): название, дата, «как прошла?», данные с часов (минуты, ккал)
  // и упражнения строками «подходы × повторы × вес». Знакомое упражнение подсказывается из справочника,
  // а его числа подставляются «как в прошлый раз» — их можно поменять. «Добавить» — тренировка появляется в неделе.
  import { api } from '../../../lib/api/client'
  import type { Exercise, Workout } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import { formatAmount } from '../../../lib/habits'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { draftOf, entryTonnage, findExercise, newEntry, toInput, weightParts, withLast, type EntryDraft } from '../../../lib/sport'
  import { toasts } from '../../../lib/state/toasts.svelte'

  interface Props {
    workout: Workout | null // null — новая
    day: string // дата новой тренировки (выбранный в неделе день)
    today: string
    directory: Exercise[]
    titles: string[]
    onsaved: (w: Workout, created: boolean) => void
    ondeleted: () => void
    oncancel: () => void
  }
  let { workout, day, today, directory, titles, onsaved, ondeleted, oncancel }: Props = $props()

  let date = $state('')
  let title = $state('')
  let notes = $state('')
  let durationMin = $state('')
  let kcal = $state('')
  let entries = $state<EntryDraft[]>([])
  let saving = $state(false)
  let error = $state<string | null>(null)
  let confirmDelete = $state(false)

  // Заполнить форму, когда сменилась тренировка или выбранный день. Читаем только workout и day:
  // если бы эффект читал entries, каждое нажатие клавиши в упражнении перезапускало бы его и стирало ввод.
  $effect(() => {
    const w = workout
    date = w?.day ?? day
    title = w?.title ?? ''
    notes = w?.notes ?? ''
    durationMin = w?.durationMin?.toString() ?? ''
    kcal = w?.kcal?.toString() ?? ''
    entries = w ? draftOf(w) : [newEntry()]
    error = null
  })

  const tonnage = $derived(entries.reduce((sum, e) => sum + entryTonnage(e), 0))

  const kg = (n: number) => {
    const p = weightParts(n)
    return `${formatAmount(p.value, i18n.locale)} ${t(p.unit === 't' ? 'sport.t' : 'sport.kg')}`
  }
  const lastText = (ex: Exercise | undefined) =>
    ex?.last ? t('sport.lastTime', { text: `${ex.last.sets} × ${ex.last.reps}${ex.last.weight !== null ? ` × ${formatAmount(ex.last.weight, i18n.locale)} ${t('sport.kg')}` : ''}` }) : ''

  async function save(ev: SubmitEvent) {
    ev.preventDefault()
    if (saving) return
    const input = toInput({ day: date, title, notes, durationMin, kcal }, entries)
    if ('error' in input) {
      error = t(`errors.${input.error}`) + (input.entry !== undefined ? ` — ${entries[input.entry]?.name}` : '')
      return
    }
    saving = true
    error = null
    try {
      const saved = workout ? await api.sport.update(workout.id, input) : await api.sport.create(input)
      onsaved(saved, !workout)
    } catch (e) {
      error = errorText(e)
    } finally {
      saving = false
    }
  }

  async function remove() {
    if (!workout) return
    saving = true
    try {
      await api.sport.remove(workout.id)
      confirmDelete = false
      ondeleted()
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      saving = false
    }
  }
</script>

<datalist id="workout-titles">{#each titles as x (x)}<option value={x}></option>{/each}</datalist>
<datalist id="exercise-names">{#each directory as x (x.id)}<option value={x.name}></option>{/each}</datalist>

<Card>
  <form class="form" onsubmit={save}>
    <h2>{workout ? t('sport.edit') : t('sport.new')}</h2>

    <div class="row">
      <label class="grow">
        <span>{t('sport.title')}</span>
        <input class="input" list="workout-titles" bind:value={title} maxlength="100" placeholder={t('sport.titlePlaceholder')} required />
      </label>
      <label><span>{t('sport.day')}</span><input class="input" type="date" max={today} bind:value={date} required /></label>
    </div>

    <label>
      <span>{t('sport.notes')}</span>
      <textarea class="input" rows="2" bind:value={notes} placeholder={t('sport.notesPlaceholder')}></textarea>
    </label>

    <fieldset class="watch">
      <legend><Icon name="clock" size={14} /> {t('sport.watch')}</legend>
      <label><span>{t('sport.duration')}</span><input class="input num" inputmode="numeric" bind:value={durationMin} placeholder="60" /></label>
      <label><span>{t('sport.kcal')}</span><input class="input num" inputmode="numeric" bind:value={kcal} placeholder="450" /></label>
    </fieldset>

    <div class="exercises">
      <h3>{t('sport.exercises')}</h3>
      {#if entries.length > 0}
        <div class="grid head muted" aria-hidden="true">
          <span>{t('sport.exercise')}</span><span>{t('sport.sets')}</span><span>{t('sport.reps')}</span><span>{t('sport.weight')}</span><span></span>
        </div>
      {/if}
      {#each entries as e, i (i)}
        {@const known = findExercise(directory, e.name)}
        <div class="grid entry">
          <div class="name-cell">
            <input class="input" list="exercise-names" bind:value={e.name} maxlength="60" placeholder={t('sport.exercisePlaceholder')}
              aria-label={t('sport.exercise')} onchange={() => (entries[i] = withLast(e, findExercise(directory, e.name)))} />
            {#if known?.last}<small class="muted">{lastText(known)}</small>{/if}
          </div>
          <input class="input" inputmode="numeric" bind:value={e.sets} oninput={() => (e.touched = true)} aria-label={t('sport.sets')} />
          <input class="input" inputmode="numeric" bind:value={e.reps} oninput={() => (e.touched = true)} aria-label={t('sport.reps')} />
          <input class="input" inputmode="decimal" bind:value={e.weight} oninput={() => (e.touched = true)} aria-label={t('sport.weight')}
            placeholder={t('sport.noWeight')} />
          <button type="button" class="icon-btn" aria-label={t('sport.removeExercise')} title={t('sport.removeExercise')}
            onclick={() => entries.splice(i, 1)}><Icon name="trash" size={16} /></button>
        </div>
      {/each}
      <Button size="sm" icon="plus" onclick={() => entries.push(newEntry())}>{t('sport.addExercise')}</Button>
    </div>

    {#if error}<p class="error">{error}</p>{/if}

    <div class="foot">
      {#if tonnage > 0}<span class="muted">{t('sport.tonnage', { kg: kg(tonnage) })}</span>{/if}
      <span class="spacer"></span>
      {#if workout}
        <Button variant="ghost" icon="trash" onclick={() => (confirmDelete = true)} disabled={saving}>{t('common.delete')}</Button>
        <Button variant="ghost" onclick={oncancel} disabled={saving}>{t('common.cancel')}</Button>
      {/if}
      <Button type="submit" variant="primary" icon={workout ? 'check' : 'plus'} loading={saving}>{workout ? t('common.save') : t('sport.add')}</Button>
    </div>
  </form>
</Card>

<ConfirmDialog open={confirmDelete} title={t('sport.deleteTitle', { title: workout?.title ?? '' })} confirmText={t('common.delete')} danger
  loading={saving} onconfirm={remove} oncancel={() => (confirmDelete = false)}>
  {t('sport.deleteText')}
</ConfirmDialog>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  h2 {
    font-size: var(--fs-lg);
  }
  h3 {
    font-size: var(--fs-md);
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
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-3);
  }
  .grow {
    flex: 1;
    min-width: 200px;
  }
  textarea {
    resize: vertical;
  }
  .watch {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-3);
    margin: 0;
    padding: var(--s-2) var(--s-3) var(--s-3);
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
  }
  .watch legend {
    display: flex;
    align-items: center;
    gap: var(--s-1);
    padding: 0 var(--s-1);
    font-size: var(--fs-sm);
    font-weight: 700;
  }
  .num {
    width: 110px;
  }
  .exercises {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: var(--s-2);
  }
  .exercises > :global(button) {
    align-self: flex-start;
  }
  .grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 72px 72px 96px 32px;
    align-items: start;
    gap: var(--s-2);
  }
  .head {
    font-size: var(--fs-xs);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .name-cell {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .name-cell input {
    font-weight: 700;
  }
  small {
    font-size: var(--fs-xs);
  }
  .entry .input {
    min-width: 0;
  }
  .icon-btn {
    display: grid;
    place-items: center;
    width: 32px;
    height: 38px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--c-muted);
    cursor: pointer;
  }
  .icon-btn:hover {
    background: var(--c-surface-2);
    color: var(--c-text);
  }
  .foot {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-sm);
  }
  .spacer {
    flex: 1;
  }
  .error {
    margin: 0;
    color: var(--c-red);
    font-size: var(--fs-sm);
  }
  @media (max-width: 600px) {
    .grid {
      grid-template-columns: minmax(0, 1fr) 56px 56px 72px 32px;
    }
  }
</style>
