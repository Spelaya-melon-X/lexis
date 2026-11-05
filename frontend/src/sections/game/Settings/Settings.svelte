<script lang="ts">
  // Настройки «Игры» (§18.3): шкала и лимит дня (B3), ритуал фокуса (B4), колонки доски и типы дел.
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { api } from '../../../lib/api/client'
  import type { TaskColumn, TaskType } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import ColorPicker from '../../../lib/components/ColorPicker.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Icon, { type IconName } from '../../../lib/components/Icon.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { splitLines } from '../../../lib/form'
  import { formatTime, parseTime } from '../../../lib/plan'
  import { toasts } from '../../../lib/state/toasts.svelte'

  let types = $state<TaskType[] | null>(null)
  let columns = $state<TaskColumn[] | null>(null)
  let counts = $state(new Map<number, number>()) // дел в колонке (и выполненных) — для диалога удаления
  let error = $state<string | null>(null)

  let newType = $state('')
  let newColumn = $state('')
  let renaming = $state<{ kind: 'type' | 'column'; id: number; name: string } | null>(null)
  let deletingType = $state<TaskType | null>(null)
  let deletingColumn = $state<TaskColumn | null>(null)
  let moveTo = $state<number | null>(null)
  let busy = $state(false)

  // Распорядок дня: поля формы — строки «ЧЧ:ММ» и часы; в API — минуты и блоки.
  let dayStart = $state('')
  let dayEnd = $state('')
  let dayLimitHours = $state(9)
  let savingDay = $state(false)

  function pad(min: number): string {
    return formatTime(min).padStart(5, '0') // <input type=time> ждёт "09:00"
  }

  async function loadDay() {
    try {
      const st = await api.tasks.settings()
      dayStart = pad(st.dayStart)
      dayEnd = pad(st.dayEnd)
      dayLimitHours = st.dayLimit / 2
    } catch (e) {
      toasts.error(errorText(e))
    }
  }
  onMount(loadDay)

  // Ритуал фокуса: по пункту на строку; и сколько минут даётся на настройку.
  let ritualText = $state('')
  let setupMin = $state(10)
  let savingRitual = $state(false)

  onMount(async () => {
    try {
      const [items, st] = await Promise.all([api.focus.ritual(), api.focus.settings()])
      ritualText = items.join('\n')
      setupMin = st.setupMin
    } catch (e) {
      toasts.error(errorText(e))
    }
  })

  async function saveRitual(e: SubmitEvent) {
    e.preventDefault()
    savingRitual = true
    try {
      const [items] = await Promise.all([api.focus.saveRitual(splitLines(ritualText)), api.focus.updateSettings(setupMin)])
      ritualText = items.join('\n')
      toasts.success(t('game.plan.settingsSaved'))
    } catch (err) {
      toasts.error(errorText(err))
    } finally {
      savingRitual = false
    }
  }

  async function saveDay(e: SubmitEvent) {
    e.preventDefault()
    const start = parseTime(dayStart)
    const end = parseTime(dayEnd)
    if (start === null || end === null) return
    savingDay = true
    try {
      await api.tasks.updateSettings({ dayStart: start, dayEnd: end, dayLimit: Math.round(dayLimitHours * 2) })
      toasts.success(t('game.plan.settingsSaved'))
      await loadDay()
    } catch (err) {
      toasts.error(errorText(err))
    } finally {
      savingDay = false
    }
  }

  const deleteCount = $derived(deletingColumn ? (counts.get(deletingColumn.id) ?? 0) : 0)
  const moveTargets = $derived((columns ?? []).filter((c) => c.id !== deletingColumn?.id))

  async function load() {
    try {
      const [ts, cs, tasks] = await Promise.all([api.tasks.types.list(), api.tasks.columns.list(), api.tasks.list('all')])
      types = ts
      columns = cs
      const m = new Map<number, number>()
      for (const task of tasks) m.set(task.columnId, (m.get(task.columnId) ?? 0) + 1)
      counts = m
      error = null
    } catch (e) {
      error = errorText(e)
    }
  }
  onMount(load)

  async function run(action: () => Promise<unknown>) {
    try {
      await action()
      await load()
      return true
    } catch (e) {
      toasts.error(errorText(e))
      return false
    }
  }

  async function addType(e: SubmitEvent) {
    e.preventDefault()
    if (await run(() => api.tasks.types.create(newType, null))) newType = ''
  }
  async function addColumn(e: SubmitEvent) {
    e.preventDefault()
    if (await run(() => api.tasks.columns.create(newColumn))) newColumn = ''
  }

  async function saveRename(e: SubmitEvent) {
    e.preventDefault()
    const r = renaming
    if (!r) return
    const ok = await run(() => (r.kind === 'type' ? api.tasks.types.update(r.id, { name: r.name }) : api.tasks.columns.rename(r.id, r.name)))
    if (ok) renaming = null
  }

  /** Сдвиг на одну позицию: порядок шлём полным списком id. */
  function shift<T extends { id: number }>(list: T[], item: T, dir: -1 | 1, save: (ids: number[]) => Promise<void>) {
    const i = list.indexOf(item)
    const j = i + dir
    if (j < 0 || j >= list.length) return
    const ids = list.map((x) => x.id)
    ;[ids[i], ids[j]] = [ids[j]!, ids[i]!]
    void run(() => save(ids))
  }

  async function removeType() {
    const ty = deletingType
    if (!ty) return
    busy = true
    if (await run(() => api.tasks.types.remove(ty.id))) deletingType = null
    busy = false
  }

  function askDeleteColumn(c: TaskColumn) {
    deletingColumn = c
    moveTo = (columns ?? []).find((x) => x.id !== c.id)?.id ?? null
  }

  async function removeColumn() {
    const c = deletingColumn
    if (!c) return
    if (deleteCount > 0 && moveTo === null) return // переносить некуда — кнопка и так неактивна
    busy = true
    if (await run(() => api.tasks.columns.remove(c.id, deleteCount > 0 ? (moveTo ?? undefined) : undefined))) deletingColumn = null
    busy = false
  }
</script>

{#snippet act(icon: IconName, label: string, onclick: () => void, disabled = false)}
  <button class="act" aria-label={label} title={label} {disabled} {onclick}><Icon name={icon} size={16} /></button>
{/snippet}

{#snippet nameCell(kind: 'type' | 'column', id: number, name: string)}
  {#if renaming?.kind === kind && renaming.id === id}
    <form class="rename" onsubmit={saveRename}>
      <!-- svelte-ignore a11y_autofocus -->
      <input class="input" bind:value={renaming.name} maxlength="40" autofocus aria-label={t('game.settings.rename')} />
      <Button type="submit" size="sm" variant="primary" icon="check" aria-label={t('common.save')} />
      <Button size="sm" variant="ghost" icon="x" aria-label={t('common.cancel')} onclick={() => (renaming = null)} />
    </form>
  {:else}
    <button class="name" title={t('game.settings.rename')} onclick={() => (renaming = { kind, id, name })}>{name}</button>
  {/if}
{/snippet}

<div class="page scroll">
  <h1>{t('nav.game.settings')}</h1>

  {#if error && !types}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={load}>{t('common.retry')}</Button></div>
  {:else if !types || !columns}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <Card>
      <h2>{t('game.plan.settings')}</h2>
      <p class="muted hint">{t('game.plan.settingsHint')}</p>
      <form class="day" onsubmit={saveDay}>
        <label><span>{t('game.plan.dayStart')}</span><input class="input" type="time" step="1800" bind:value={dayStart} required /></label>
        <label><span>{t('game.plan.dayEnd')}</span><input class="input" type="time" step="1800" bind:value={dayEnd} required /></label>
        <label><span>{t('game.plan.dayLimit')}</span><input class="input" type="number" min="0.5" max="24" step="0.5" bind:value={dayLimitHours} required /></label>
        <Button type="submit" variant="primary" icon="check" loading={savingDay}>{t('common.save')}</Button>
      </form>
    </Card>

    <Card>
      <h2>{t('game.focus.ritualTitle')}</h2>
      <p class="muted hint">{t('game.focus.ritualHint')}</p>
      <form class="ritual" onsubmit={saveRitual}>
        <textarea class="input" rows="5" bind:value={ritualText} placeholder={t('game.focus.ritualPlaceholder')} aria-label={t('game.focus.ritualTitle')}></textarea>
        <div class="day">
          <label><span>{t('game.focus.setupMin')}</span><input class="input" type="number" min="1" max="60" bind:value={setupMin} required /></label>
          <Button type="submit" variant="primary" icon="check" loading={savingRitual}>{t('common.save')}</Button>
        </div>
      </form>
    </Card>

    <Card>
      <h2>{t('game.settings.columns')}</h2>
      <p class="muted hint">{t('game.settings.columnsHint')}</p>
      {#if columns.length === 0}<p class="muted">{t('game.settings.noColumns')}</p>{/if}
      <ul>
        {#each columns as c, i (c.id)}
          <li in:fade={{ duration: 200 }}>
            {@render nameCell('column', c.id, c.name)}
            <span class="muted count">{counts.get(c.id) ?? 0}</span>
            <span class="acts">
              {@render act('chevronLeft', t('game.moveUp'), () => shift(columns!, c, -1, api.tasks.columns.reorder), i === 0)}
              {@render act('chevronRight', t('game.moveDown'), () => shift(columns!, c, 1, api.tasks.columns.reorder), i === columns.length - 1)}
              {@render act('trash', t('common.delete'), () => askDeleteColumn(c))}
            </span>
          </li>
        {/each}
      </ul>
      <form class="add" onsubmit={addColumn}>
        <input class="input" bind:value={newColumn} maxlength="40" placeholder={t('game.settings.newColumn')} aria-label={t('game.settings.columnName')} required />
        <Button type="submit" icon="plus">{t('game.settings.add')}</Button>
      </form>
    </Card>

    <Card>
      <h2>{t('game.settings.types')}</h2>
      <p class="muted hint">{t('game.settings.typesHint')}</p>
      {#if types.length === 0}<p class="muted">{t('game.settings.noTypes')}</p>{/if}
      <ul>
        {#each types as ty, i (ty.id)}
          <li in:fade={{ duration: 200 }}>
            <span class="dot" style:background={ty.color ?? 'var(--c-surface)'}></span>
            {@render nameCell('type', ty.id, ty.name)}
            <ColorPicker value={ty.color} onchange={(color) => run(() => api.tasks.types.update(ty.id, { color }))} />
            <span class="acts">
              {@render act('chevronLeft', t('game.moveUp'), () => shift(types!, ty, -1, api.tasks.types.reorder), i === 0)}
              {@render act('chevronRight', t('game.moveDown'), () => shift(types!, ty, 1, api.tasks.types.reorder), i === types.length - 1)}
              {@render act('trash', t('common.delete'), () => (deletingType = ty))}
            </span>
          </li>
        {/each}
      </ul>
      <form class="add" onsubmit={addType}>
        <input class="input" bind:value={newType} maxlength="40" placeholder={t('game.settings.newType')} aria-label={t('game.settings.typeName')} required />
        <Button type="submit" icon="plus">{t('game.settings.add')}</Button>
      </form>
    </Card>
  {/if}
</div>

<ConfirmDialog
  open={deletingType !== null}
  title={t('game.settings.deleteTypeTitle', { name: deletingType?.name ?? '' })}
  confirmText={t('common.delete')}
  danger
  loading={busy}
  onconfirm={removeType}
  oncancel={() => (deletingType = null)}
>
  {t('game.settings.deleteTypeText')}
</ConfirmDialog>

<ConfirmDialog
  open={deletingColumn !== null}
  title={t('game.settings.deleteColumnTitle', { name: deletingColumn?.name ?? '' })}
  confirmText={t('common.delete')}
  danger
  loading={busy}
  onconfirm={removeColumn}
  oncancel={() => (deletingColumn = null)}
>
  {#if deleteCount === 0}
    {t('game.settings.deleteColumnEmpty')}
  {:else if moveTargets.length === 0}
    {t('game.settings.deleteColumnOnly')}
  {:else}
    <label class="move">
      <span>{t('game.settings.deleteColumnMove', { count: deleteCount })}</span>
      <select class="input" bind:value={moveTo}>
        {#each moveTargets as c (c.id)}<option value={c.id}>{c.name}</option>{/each}
      </select>
    </label>
  {/if}
</ConfirmDialog>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    max-width: 760px;
    min-height: 0;
    padding: 2px 6px var(--s-4) 2px;
  }
  h1 {
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
  }
  h2 {
    font-size: var(--fs-lg);
  }
  .hint {
    margin: var(--s-1) 0 var(--s-3);
    font-size: var(--fs-sm);
  }
  .state {
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
    padding: var(--s-6);
  }
  ul {
    list-style: none;
    margin: 0 0 var(--s-3);
    padding: 0;
  }
  li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s-2) var(--s-3);
    padding: var(--s-2) 0;
    border-bottom: 1px solid var(--c-line);
  }
  .dot {
    flex: none;
    width: 14px;
    height: 14px;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
  .name {
    flex: 1;
    min-width: 120px;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    font-weight: 700;
    text-align: left;
    cursor: pointer;
  }
  .name:hover {
    text-decoration: underline;
  }
  .rename {
    flex: 1;
    display: flex;
    gap: var(--s-1);
    min-width: 200px;
  }
  .rename .input {
    flex: 1;
  }
  .count {
    font-size: var(--fs-sm);
  }
  .acts {
    display: flex;
    gap: 2px;
  }
  .act {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--c-muted);
    cursor: pointer;
  }
  .act:nth-child(1) :global(svg),
  .act:nth-child(2) :global(svg) {
    transform: rotate(90deg);
  }
  .act:hover:not(:disabled) {
    background: var(--c-surface-2);
    color: var(--c-text);
  }
  .act:disabled {
    opacity: 0.3;
    cursor: default;
  }
  .add {
    display: flex;
    gap: var(--s-2);
  }
  .add .input {
    flex: 1;
  }
  .day {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--s-3);
  }
  .day label {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    font-size: var(--fs-sm);
    font-weight: 600;
  }
  .day .input {
    width: 130px;
  }
  .ritual {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  .ritual textarea {
    resize: vertical;
  }
  .move {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    color: var(--c-text);
  }
</style>
