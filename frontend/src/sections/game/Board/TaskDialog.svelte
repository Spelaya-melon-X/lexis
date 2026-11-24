<script lang="ts">
  // Создание и правка дела (§18.3). task = null — новое (в колонку column); open управляет видимостью.
  import { api } from '../../../lib/api/client'
  import type { Task, TaskColumn, TaskInput, TaskType } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Modal from '../../../lib/components/Modal.svelte'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { focus } from '../../../lib/state/focus.svelte'
  import { blocksText } from '../format'
  import TaskTags from './TaskTags.svelte'

  interface Props {
    open: boolean
    task: Task | null
    column: number | null // для нового дела
    columns: TaskColumn[]
    types: TaskType[]
    onclose: () => void
    oncreate?: (input: TaskInput) => Promise<void>
    onsaved?: (t: Task) => void
    ondelete?: (t: Task) => void
    onsplit?: (t: Task) => void
    ongraph?: (t: Task) => void
  }
  let { open, task, column, columns, types, onclose, oncreate, onsaved, ondelete, onsplit, ongraph }: Props = $props()

  // У дела с шагами объём и «выполнено» считаются по шагам: правим только первоначальную оценку.
  const parent = $derived(task !== null && !task.leaf)

  let title = $state('')
  let blocks = $state(2)
  let importance = $state(5)
  let deadline = $state('')
  let typeId = $state<number | null>(null)
  let columnId = $state<number | null>(null)
  let note = $state('')
  let saving = $state(false)
  let error = $state<string | null>(null)

  $effect(() => {
    if (!open) return
    title = task?.title ?? ''
    blocks = task?.ownBlocks ?? 2
    importance = task?.importance ?? 5
    deadline = task?.deadline ?? ''
    typeId = task?.typeId ?? null
    columnId = task?.columnId ?? column ?? columns[0]?.id ?? null
    note = task?.note ?? ''
    error = null
  })

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (saving || columnId === null) return
    saving = true
    error = null
    const fields = { title, blocks, importance, deadline: deadline || null, typeId, note }
    try {
      if (!task) {
        const input = { ...fields, columnId }
        if (oncreate) await oncreate(input)
        else {
          const saved = await api.tasks.create(input)
          onsaved?.(saved)
        }
        return
      }
      let saved = await api.tasks.update(task.id, fields)
      if (columnId !== task.columnId) {
        await api.tasks.move(task.id, columnId, null) // в другую колонку — в конец
        saved = { ...saved, columnId }
      }
      onsaved?.(saved)
    } catch (err) {
      error = errorText(err) // ошибка в форме: диалог не закрывается, можно поправить
    } finally {
      saving = false
    }
  }
</script>

<Modal {open} title={task ? t('game.editTask') : t('game.newTask')} width={560} onclose={() => !saving && onclose()}>
  <form id="task-form" class="form" onsubmit={save}>
    <label>
      <span>{t('game.title')}</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input class="input" bind:value={title} maxlength="200" placeholder={t('game.titlePlaceholder')} required autofocus />
    </label>

    <div class="row">
      <label>
        <span>{parent ? t('game.ownEstimate') : t('game.blocks')} · <strong>{blocksText(blocks)}</strong></span>
        <input class="input" type="number" min="1" max="40" bind:value={blocks} required />
        <small class="muted">{parent && task ? t('game.parentHint', { sum: blocksText(task.blocks) }) : t('game.blocksHint')}</small>
      </label>
      <label>
        <span>{t('game.importance')} · <strong>{importance}</strong></span>
        <input class="range" type="range" min="1" max="10" bind:value={importance} />
        <small class="muted">{t('game.importanceHint')}</small>
      </label>
    </div>

    <div class="row">
      <label>
        <span>{t('game.deadline')}</span>
        <input class="input" type="date" bind:value={deadline} />
      </label>
      <label>
        <span>{t('game.type')}</span>
        <select class="input" bind:value={typeId}>
          <option value={null}>{t('game.noType')}</option>
          {#each types as ty (ty.id)}<option value={ty.id}>{ty.name}</option>{/each}
        </select>
      </label>
      {#if !parent}
        <label>
          <span>{t('game.column')}</span>
          <select class="input" bind:value={columnId}>
            {#each columns as c (c.id)}<option value={c.id}>{c.name}</option>{/each}
          </select>
        </label>
      {/if}
    </div>

    <label>
      <span>{t('game.note')}</span>
      <textarea class="input" rows="4" bind:value={note} placeholder={t('game.notePlaceholder')}></textarea>
    </label>
    <TaskTags {note} />

    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </form>

  {#snippet footer()}
    {#if task}
      <Button variant="ghost" icon="trash" aria-label={t('common.delete')} title={t('common.delete')} onclick={() => ondelete?.(task)} disabled={saving} />
      {#if task.leaf && !task.done}
        <Button variant="ghost" icon="play" disabled={saving || focus.session !== null} onclick={() => (onclose(), focus.begin(task.id))}>
          {t('game.focus.begin')}
        </Button>
      {/if}
      {#if !(task.leaf && task.done)}
        <Button variant="ghost" icon="steps" onclick={() => onsplit?.(task)} disabled={saving}>{parent ? t('game.splitMore') : t('game.split')}</Button>
      {/if}
      {#if parent || task.parentId !== null}
        <Button variant="ghost" icon="graph" onclick={() => ongraph?.(task)} disabled={saving}>{t('game.graph')}</Button>
      {/if}
      <span class="spacer"></span>
    {/if}
    <Button variant="ghost" onclick={onclose} disabled={saving}>{t('common.cancel')}</Button>
    <Button type="submit" form="task-form" variant="primary" loading={saving}>{task ? t('common.save') : t('game.create')}</Button>
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
    min-width: 0;
  }
  label > span {
    font-weight: 600;
    font-size: var(--fs-sm);
  }
  small {
    font-size: var(--fs-xs);
  }
  .row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: var(--s-3);
  }
  .range {
    accent-color: var(--c-green);
    height: 38px;
  }
  textarea {
    resize: vertical;
  }
  .spacer {
    flex: 1;
  }
  .error {
    color: var(--c-red);
    font-size: var(--fs-sm);
  }
</style>
