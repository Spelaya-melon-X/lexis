<script lang="ts">
  // «Задачи» (§18.3): доска со своими колонками, горизонтальный скролл, DnD дел между колонками.
  // Изменения порядка — оптимистично (lib/tasks.moveTask), ошибка → тост и перечитывание.
  import { onDestroy, onMount } from 'svelte'
  import { api } from '../../../lib/api/client'
  import type { Task, TaskColumn, TaskType } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { today as todayOf } from '../../../lib/dates'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import { ancestors, board, moveTask, shiftBefore, type TypeFilter } from '../../../lib/tasks'
  import Column from './Column.svelte'
  import SplitDialog from './SplitDialog.svelte'
  import TaskDialog from './TaskDialog.svelte'
  import TaskGraph from './TaskGraph.svelte'

  const PREFS_KEY = 'lexis.board'

  let columns = $state<TaskColumn[] | null>(null)
  let types = $state<TaskType[]>([])
  let tasks = $state<Task[]>([])
  let error = $state<string | null>(null)
  const prefs = savedPrefs()
  let filter = $state<TypeFilter>(prefs.filter)
  let showDone = $state(prefs.showDone)

  let dialogOpen = $state(false)
  let editing = $state<Task | null>(null)
  let newIn = $state<number | null>(null)
  let deleting = $state<Task | null>(null)
  let busy = $state(false)
  let firstColumn = $state(t('game.defaultColumn'))
  let dragId = $state<number | null>(null)
  let boardEl: HTMLDivElement | undefined = $state()
  let scrollFrame: number | null = null
  let lastFrame: number | null = null
  let scrollSpeed = 0
  let splitting = $state<Task | null>(null)
  let graphFor = $state<number | null>(null)
  let graphVersion = $state(0) // +1 после правок — граф перечитывает дерево
  let deleteSteps = $state(0) // сколько шагов удалится вместе с делом

  const today = todayOf()
  const typeMap = $derived(new Map(types.map((x) => [x.id, x])))
  const cols = $derived(columns ? board(columns, tasks, filter, showDone) : [])

  function savedPrefs(): { filter: TypeFilter; showDone: boolean } {
    try {
      const v = JSON.parse(localStorage.getItem(PREFS_KEY) ?? '{}') as Record<string, unknown>
      const f = v.filter
      return {
        filter: f === 'none' || typeof f === 'number' ? f : 'all',
        showDone: v.showDone === true,
      }
    } catch {
      return { filter: 'all', showDone: false }
    }
  }

  $effect(() => {
    try {
      localStorage.setItem(PREFS_KEY, JSON.stringify({ filter, showDone }))
    } catch {
      // не страшно: просто не запомнится
    }
  })

  async function load() {
    try {
      ;[columns, types, tasks] = await Promise.all([
        api.tasks.columns.list(),
        api.tasks.types.list(),
        api.tasks.list(showDone ? 'all' : 'false'),
      ])
      // Выбранного типа больше нет (удалили в настройках) — показываем все.
      if (typeof filter === 'number' && !types.some((x) => x.id === filter)) filter = 'all'
      error = null
    } catch (e) {
      error = errorText(e)
    }
  }
  onMount(load)

  // Открыть дело по ссылке #/game/board/<id> (например, из распорядка — в следующих фазах).
  $effect(() => {
    const id = /^\d+$/.test(route.rest) ? Number(route.rest) : null
    const found = id === null ? undefined : tasks.find((x) => x.id === id)
    if (found && !dialogOpen) openEdit(found)
  })

  async function toggleShowDone() {
    showDone = !showDone
    await load()
  }

  function openNew(column: number | null) {
    editing = null
    newIn = column
    dialogOpen = true
  }
  function openEdit(task: Task) {
    editing = task
    dialogOpen = true
  }
  function closeDialog() {
    dialogOpen = false
    if (route.rest) route.go('game', 'board') // убрать id из адреса, чтобы диалог не открылся снова
  }

  function upsert(task: Task) {
    tasks = tasks.some((x) => x.id === task.id) ? tasks.map((x) => (x.id === task.id ? task : x)) : [...tasks, task]
  }

  async function saved(task: Task) {
    closeDialog()
    toasts.success(t('game.saved'))
    graphVersion++
    if ((editing && editing.columnId !== task.columnId) || !task.leaf) {
      await load() // перенос из диалога — позиции считает сервер; у родителя меняются суммы
      return
    }
    upsert(task)
  }

  function openSplit(task: Task) {
    closeDialog()
    splitting = task
  }

  function openGraph(task: Task) {
    closeDialog()
    graphFor = task.id
  }

  async function afterSplit() {
    splitting = null
    toasts.success(t('game.splitDone'))
    graphVersion++
    await load()
  }

  async function askDelete(task: Task) {
    deleteSteps = 0
    if (!task.leaf) {
      // Шаги могут быть скрыты (выполненные) — считаем по полному дереву.
      try {
        const tree = await api.tasks.tree(task.id)
        const below = new Set([task.id])
        for (const x of tree) if (x.parentId !== null && below.has(x.parentId)) below.add(x.id) // дерево — в порядке обхода
        deleteSteps = below.size - 1
      } catch (e) {
        toasts.error(errorText(e))
        return
      }
    }
    deleting = task
  }

  async function quickAdd(columnId: number, title: string): Promise<boolean> {
    try {
      upsert(await api.tasks.create({ title, blocks: 2, importance: 5, columnId }))
      return true
    } catch (e) {
      toasts.error(errorText(e))
      return false
    }
  }

  async function toggle(task: Task) {
    try {
      const next = await api.tasks.update(task.id, { done: !task.done })
      upsert(next)
      if (next.done) toasts.success(t('game.doneToast'))
      if (task.parentId === null) return
      // У шага меняются суммы и «выполнено» предков — перечитываем; закрыли последний шаг — празднуем.
      graphVersion++
      await load()
      if (next.done) {
        const root = (await api.tasks.tree(task.id))[0]
        if (root?.done) toasts.success(t('game.treeDone', { title: root.title }))
      }
    } catch (e) {
      toasts.error(errorText(e))
    }
  }

  async function move(id: number, columnId: number, beforeId: number | null) {
    const prev = tasks
    tasks = moveTask(tasks, id, columnId, beforeId) // сразу — интерфейс не ждёт сеть
    try {
      await api.tasks.move(id, columnId, beforeId)
    } catch (e) {
      tasks = prev
      toasts.error(errorText(e))
      await load()
    }
  }

  function shift(task: Task, dir: -1 | 1) {
    const col = cols.find((c) => c.column.id === task.columnId)
    const before = col && shiftBefore(col.tasks.map((x) => x.id), task.id, dir)
    if (before !== undefined) void move(task.id, task.columnId, before)
  }

  function side(task: Task, dir: -1 | 1) {
    const i = cols.findIndex((c) => c.column.id === task.columnId)
    const target = cols[i + dir]
    if (target) void move(task.id, target.column.id, null)
  }

  function stopScroll() {
    if (scrollFrame !== null) cancelAnimationFrame(scrollFrame)
    scrollFrame = null
    lastFrame = null
    scrollSpeed = 0
  }
  onDestroy(stopScroll)

  function endDrag() {
    dragId = null
    stopScroll()
  }

  function scrollBoard(time: number) {
    scrollFrame = null
    if (!boardEl || dragId === null || scrollSpeed === 0) return
    const elapsed = lastFrame === null ? 16 : Math.min(time - lastFrame, 32)
    lastFrame = time
    const limit = boardEl.scrollWidth - boardEl.clientWidth
    const next = Math.max(0, Math.min(limit, boardEl.scrollLeft + scrollSpeed * elapsed / 1000))
    boardEl.scrollLeft = next
    if (scrollSpeed > 0 ? next < limit : next > 0) {
      scrollFrame = requestAnimationFrame(scrollBoard)
    } else {
      stopScroll()
    }
  }

  function trackDrag(e: DragEvent) {
    if (dragId === null || !boardEl) return
    if (!(e.target instanceof Node) || !boardEl.contains(e.target) || boardEl.scrollWidth <= boardEl.clientWidth) {
      stopScroll()
      return
    }
    const { left, right } = boardEl.getBoundingClientRect()
    const zone = Math.min(72, (right - left) / 3)
    const distance = Math.min(e.clientX - left, right - e.clientX)
    const direction = e.clientX < (left + right) / 2 ? -1 : 1
    if (distance < 0 || distance >= zone) {
      stopScroll()
      return
    }
    // dragover может приходить редко, поэтому держим прокрутку между событиями.
    scrollSpeed = direction * (120 + 840 * (1 - distance / zone))
    if (scrollFrame === null) scrollFrame = requestAnimationFrame(scrollBoard)
  }

  function leaveBoard(e: DragEvent) {
    // При переходе между вложенными элементами доски dragleave тоже всплывает.
    const target = e.relatedTarget ?? document.elementFromPoint(e.clientX, e.clientY)
    if (!(target instanceof Node) || !boardEl?.contains(target)) stopScroll()
  }

  async function remove() {
    if (!deleting) return
    busy = true
    try {
      await api.tasks.remove(deleting.id)
      deleting = null
      graphFor = null
      await load() // вместе с делом ушли шаги, а у его родителя поменялись суммы
      closeDialog()
      toasts.success(t('game.deleted'))
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
    }
  }

  async function createFirstColumn(e: SubmitEvent) {
    e.preventDefault()
    try {
      await api.tasks.columns.create(firstColumn)
      await load()
    } catch (err) {
      toasts.error(errorText(err))
    }
  }

  const filterValue = $derived(String(filter))
  function setFilter(v: string) {
    filter = v === 'all' || v === 'none' ? v : Number(v)
  }
</script>

<svelte:window ondragover={trackDrag} ondragend={endDrag} ondrop={endDrag} />

<div class="page">
  <header>
    <h1>{t('nav.game.board')}</h1>
    {#if types.length > 0}
      <div class="filter" role="group" aria-label={t('game.filter')}>
        {#each [{ v: 'all', label: t('game.filterAll'), color: null }, ...types.map((x) => ({ v: String(x.id), label: x.name, color: x.color })), { v: 'none', label: t('game.filterNone'), color: null }] as o (o.v)}
          <button class="chip" class:active={filterValue === o.v} aria-pressed={filterValue === o.v} onclick={() => setFilter(o.v)}>
            {#if o.color}<span class="dot" style:background={o.color}></span>{/if}{o.label}
          </button>
        {/each}
      </div>
    {/if}
    <label class="toggle"><input type="checkbox" checked={showDone} onchange={toggleShowDone} /> {t('game.showDone')}</label>
    <Button variant="primary" icon="plus" disabled={!columns?.length} onclick={() => openNew(null)}>{t('game.newTask')}</Button>
  </header>

  {#if error && !columns}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={load}>{t('common.retry')}</Button></div>
  {:else if !columns}
    <div class="state"><Spinner size={28} /></div>
  {:else if columns.length === 0}
    <form class="state" onsubmit={createFirstColumn}>
      <p class="muted">{t('game.noColumns')}</p>
      <input class="input" bind:value={firstColumn} maxlength="40" aria-label={t('game.settings.columnName')} required />
      <Button type="submit" variant="primary" icon="plus">{t('game.createColumn')}</Button>
    </form>
  {:else}
    <div class="board" role="region" aria-label={t('nav.game.board')} bind:this={boardEl} ondragleave={leaveBoard}>
      {#each cols as col, i (col.column.id)}
        <Column
          {col}
          types={typeMap}
          {today}
          {dragId}
          first={i === 0}
          last={i === cols.length - 1}
          pathOf={(task) => ancestors(tasks, task.id)}
          onopen={openEdit}
          ongraph={openGraph}
          ontoggle={toggle}
          onshift={shift}
          onside={side}
          onquickadd={(title) => quickAdd(col.column.id, title)}
          ondragstart={(task) => (dragId = task.id)}
          ondragend={endDrag}
          ondrop={(before) => {
            const id = dragId
            endDrag()
            if (id !== null) void move(id, col.column.id, before)
          }}
        />
      {/each}
    </div>
    <p class="hint muted">{t('game.boardHint')}</p>
  {/if}
</div>

<TaskDialog
  open={dialogOpen}
  task={editing}
  column={newIn}
  columns={columns ?? []}
  {types}
  onclose={closeDialog}
  onsaved={saved}
  ondelete={askDelete}
  onsplit={openSplit}
  ongraph={openGraph}
/>

<SplitDialog task={splitting} onclose={() => (splitting = null)} onsplit={afterSplit} />

<TaskGraph taskId={graphFor} version={graphVersion} onclose={() => (graphFor = null)} onopen={openEdit} />

<ConfirmDialog
  open={deleting !== null}
  title={t('game.deleteTitle', { title: deleting?.title ?? '' })}
  confirmText={t('common.delete')}
  danger
  loading={busy}
  onconfirm={remove}
  oncancel={() => (deleting = null)}
>
  {deleteSteps > 0 ? t('game.deleteTreeText', { count: deleteSteps }) : t('game.deleteText')}
</ConfirmDialog>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    min-height: 0;
    min-width: 0;
    flex: 1;
  }
  header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s-3) var(--s-4);
  }
  h1 {
    flex: 1;
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
  }
  .filter {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-1);
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--s-1);
    padding: 2px var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: 999px;
    background: var(--c-surface);
    font: inherit;
    font-size: var(--fs-sm);
    font-weight: 600;
    cursor: pointer;
  }
  .chip.active {
    background: var(--c-text);
    color: var(--c-bg);
  }
  .dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-sm);
    color: var(--c-muted);
    cursor: pointer;
  }
  .state {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
    text-align: center;
  }
  /* Колонки уходят вправо — скроллится доска, а не страница */
  .board {
    flex: 1;
    min-height: 0;
    display: flex;
    align-items: flex-start;
    gap: var(--s-3);
    overflow-x: auto;
    padding-bottom: var(--s-2);
  }
  .hint {
    margin: 0;
    font-size: var(--fs-xs);
  }
</style>
