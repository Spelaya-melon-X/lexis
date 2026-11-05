<script lang="ts">
  // Колонка доски: шапка (имя, число дел, объём работы), карточки, быстрое добавление.
  // Принимает перетаскиваемое дело: dropBefore по серединам карточек решает, куда встать.
  import { fade } from 'svelte/transition'
  import type { Task, TaskType } from '../../../lib/api/types'
  import { t } from '../../../lib/i18n/i18n.svelte'
  import { dropBefore, type BoardColumn } from '../../../lib/tasks'
  import { blocksText } from '../format'
  import TaskCard from './TaskCard.svelte'

  interface Props {
    col: BoardColumn
    types: Map<number, TaskType>
    today: string
    dragId: number | null
    first: boolean
    last: boolean
    pathOf: (t: Task) => Task[]
    onopen: (t: Task) => void
    ongraph: (t: Task) => void
    ontoggle: (t: Task) => void
    onshift: (t: Task, dir: -1 | 1) => void
    onside: (t: Task, dir: -1 | 1) => void
    onquickadd: (title: string) => Promise<boolean>
    ondragstart: (t: Task) => void
    ondragend: () => void
    ondrop: (beforeId: number | null) => void
  }
  let { col, types, today, dragId, first, last, pathOf, onopen, ongraph, ontoggle, onshift, onside, onquickadd, ondragstart, ondragend, ondrop }: Props = $props()

  let list: HTMLElement | undefined = $state()
  // Куда упадёт перетаскиваемое: id карточки, перед которой, null — в конец, undefined — не над колонкой.
  let dropAt = $state<number | null | undefined>(undefined)
  let draft = $state('')
  let adding = $state(false)

  function onDragOver(e: DragEvent) {
    if (dragId === null || !list) return
    e.preventDefault()
    const cards = [...list.querySelectorAll<HTMLElement>('[data-id]')].map((el) => {
      const r = el.getBoundingClientRect()
      return { id: Number(el.dataset.id), mid: r.top + r.height / 2 }
    })
    dropAt = dropBefore(cards, e.clientY, dragId)
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    const at = dropAt
    dropAt = undefined
    if (at !== undefined) ondrop(at)
  }

  function onDragLeave(e: DragEvent) {
    // dragleave приходит и при переходе на дочерний элемент — сбрасываем, только когда курсор ушёл из колонки
    if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node | null)) dropAt = undefined
  }

  async function quickAdd(e: SubmitEvent) {
    e.preventDefault()
    const title = draft.trim()
    if (!title || adding) return
    adding = true
    if (await onquickadd(title)) draft = ''
    adding = false
  }
</script>

<section class="column" class:over={dropAt !== undefined} aria-label={col.column.name} ondragover={onDragOver} ondrop={onDrop} ondragleave={onDragLeave}>
  <header>
    <h2>{col.column.name}</h2>
    <span class="count">{col.tasks.length}</span>
    {#if col.blocks > 0}<span class="hours muted">{blocksText(col.blocks)}</span>{/if}
  </header>

  <div class="cards scroll" bind:this={list}>
    {#each col.tasks as task, i (task.id)}
      {#if dropAt === task.id}<div class="marker" in:fade={{ duration: 100 }}></div>{/if}
      <div in:fade={{ duration: 200 }}>
        <TaskCard
          {task}
          path={pathOf(task)}
          type={task.typeId === null ? undefined : types.get(task.typeId)}
          {today}
          dragging={task.id === dragId}
          canUp={i > 0}
          canDown={i < col.tasks.length - 1}
          canLeft={!first}
          canRight={!last}
          onopen={() => onopen(task)}
          ongraph={() => ongraph(task)}
          ontoggle={() => ontoggle(task)}
          onshift={(dir) => onshift(task, dir)}
          onside={(dir) => onside(task, dir)}
          ondragstart={(e) => {
            e.dataTransfer?.setData('text/plain', String(task.id))
            if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
            ondragstart(task)
          }}
          {ondragend}
        />
      </div>
    {:else}
      <p class="empty muted">{t('game.emptyColumn')}</p>
    {/each}
    {#if dropAt === null && col.tasks.length > 0}<div class="marker" in:fade={{ duration: 100 }}></div>{/if}
  </div>

  <form class="quick" onsubmit={quickAdd}>
    <input class="input" bind:value={draft} placeholder={t('game.quickAddPlaceholder')} aria-label={t('game.quickAdd')} maxlength="200" disabled={adding} />
  </form>
</section>

<style>
  .column {
    flex: none;
    width: 290px;
    max-height: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding: var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface-2);
    transition: background var(--dur-fast) var(--ease-out);
  }
  .column.over {
    background: var(--c-green-soft);
  }
  header {
    display: flex;
    align-items: baseline;
    gap: var(--s-2);
  }
  h2 {
    font-size: var(--fs-md);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .count {
    padding: 0 var(--s-2);
    border: 1px solid var(--c-border);
    border-radius: 999px;
    font-size: var(--fs-xs);
    font-weight: 700;
  }
  .hours {
    margin-left: auto;
    font-size: var(--fs-xs);
    white-space: nowrap;
  }
  .cards {
    flex: 1;
    min-height: 40px;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding: 2px 4px 4px 2px;
  }
  .marker {
    flex: none;
    height: var(--bw-strong);
    border-radius: 2px;
    background: var(--c-green);
  }
  .empty {
    margin: 0;
    padding: var(--s-3);
    border: var(--bw) dashed var(--c-line);
    border-radius: var(--radius-sm);
    font-size: var(--fs-sm);
    text-align: center;
  }
  .quick .input {
    width: 100%;
    background: var(--c-surface);
  }
</style>
