<script lang="ts">
  // «Распорядок» (§18.3, B3): слева шкала дня — дело можно поставить в любое время (по получасам),
  // наложилось на другое — меняются местами (lib/plan.place); без мыши — стрелки ↑/↓ на выделенном деле.
  // Справа — «не доделал», топ-5 по формуле и «+ любое дело». План сохраняется целиком (PUT) и оптимистично.
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { Plan, PlanItem, Task, TaskColumn, TaskInput, TaskType } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import DatePager from '../../../lib/components/DatePager.svelte'
  import Icon, { type IconName } from '../../../lib/components/Icon.svelte'
  import ProgressBar from '../../../lib/components/ProgressBar.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { isDay } from '../../../lib/dates'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { BLOCK_MIN, clampStart, endOf, formatTime, hourTicks, place, resize, snap, type Slot } from '../../../lib/plan'
  import { route } from '../../../lib/state/route.svelte'
  import { focus } from '../../../lib/state/focus.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import { ancestors } from '../../../lib/tasks'
  import { blocksText } from '../format'
  import TaskDialog from '../Board/TaskDialog.svelte'
  import TaskPicker from './TaskPicker.svelte'

  const PX_PER_BLOCK = 30 // высота получаса на шкале

  const requestedDay = $derived(isDay(route.rest) ? route.rest : 'today')
  let today = $state<string | undefined>()
  const day = $derived(requestedDay === 'today' ? today : requestedDay)

  let plan = $state<Plan | null>(null)
  let error = $state<string | null>(null)
  let types = $state(new Map<number, TaskType>())
  let openTasks = $state<Task[]>([]) // для хлебных крошек «Родитель › шаг»: родители открытых шагов тоже открыты
  let picking = $state(false)
  let creating = $state(false)
  let columns = $state<TaskColumn[]>([])
  let loadingColumns = $state(false)
  let writing = $state(false)
  // Перетаскивание: какое дело, где его ухватили (смещение курсора от верха карточки) и куда оно встанет.
  let drag = $state<{ id: number; grab: number } | null>(null)
  let preview = $state<Slot | null>(null)
  let scaleEl: HTMLElement | undefined = $state()

  const fetchPlan = latest(api.tasks.plan)

  function load(d: string) {
    error = null
    void fetchPlan(d)
      .then((r) => {
        if (r.stale) return
        plan = r.value
        today = plan.today
      })
      .catch((e: unknown) => (error = errorText(e)))
  }

  $effect(() => {
    plan = null // не показывать прошлый день под новой датой, пока грузится
    picking = false
    creating = false
    load(requestedDay)
    return fetchPlan.cancel
  })

  onMount(() => {
    // Не критично для страницы: без типов — без цветных полос, без списка — без хлебных крошек.
    api.tasks.types
      .list()
      .then((ts) => (types = new Map(ts.map((x) => [x.id, x]))))
      .catch(() => {})
    api.tasks
      .list('false')
      .then((ts) => (openTasks = ts))
      .catch(() => {})
  })

  function go(d: string) {
    route.go('game', 'plan', d === today ? '' : d)
  }

  const s = $derived(plan?.settings ?? { dayStart: 540, dayEnd: 1380, dayLimit: 18 })
  const slots = $derived<Slot[]>(plan ? plan.items.map((x) => ({ id: x.task.id, start: x.start, blocks: x.blocks })) : [])
  // Дело вне рамок дня (поставили на 7:00 или вылезло за конец) — шкала расширяется, лишнее заштриховано.
  const scaleStart = $derived(Math.min(s.dayStart, ...slots.map((x) => x.start)))
  const scaleEnd = $derived(Math.max(s.dayEnd, ...slots.map(endOf)))
  const px = (min: number) => ((min - scaleStart) / BLOCK_MIN) * PX_PER_BLOCK
  const over = $derived(plan !== null && plan.totalBlocks > s.dayLimit)
  const planned = $derived(new Set(plan?.items.map((x) => x.task.id) ?? []))

  /** Оптимистично: экран меняется сразу; ответ сервера подтверждает (и пересчитывает топ). */
  async function save(items: PlanItem[]) {
    if (!plan?.editable || writing) return
    const d = plan.day
    const wasOver = over
    items = [...items].sort((a, b) => a.start - b.start)
    plan = { ...plan, items, totalBlocks: items.reduce((sum, x) => sum + x.blocks, 0) }
    if (!wasOver && plan.totalBlocks > s.dayLimit) toasts.error(t('game.plan.overLimit', { limit: blocksText(s.dayLimit) }))
    writing = true
    try {
      const fresh = await api.tasks.savePlan(d, items.map((x) => ({ taskId: x.task.id, start: x.start, blocks: x.blocks })))
      if (plan?.day === d) plan = fresh
    } catch (e) {
      toasts.error(errorText(e))
      if (plan?.day === d) {
        plan = null
        load(requestedDay)
      }
    } finally {
      writing = false
    }
  }

  /** Новая раскладка слотов → пункты плана (дела те же, меняются время и объём). */
  function apply(next: Slot[]) {
    if (!plan) return
    const byId = new Map(plan.items.map((x) => [x.task.id, x.task]))
    void save(next.map((sl) => ({ task: byId.get(sl.id)!, start: sl.start, blocks: sl.blocks })))
  }

  async function add(task: Task) {
    picking = false
    if (!plan?.editable || writing || planned.has(task.id)) return
    const d = plan.day
    writing = true
    try {
      acceptAddition(await api.tasks.addPlanTask(d, task.id))
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      writing = false
    }
  }

  function acceptAddition(fresh: Plan) {
    if (plan?.day !== fresh.day) return
    if (!over && fresh.totalBlocks > fresh.settings.dayLimit) {
      toasts.error(t('game.plan.overLimit', { limit: blocksText(fresh.settings.dayLimit) }))
    }
    plan = fresh
    today = fresh.today
  }

  async function showCreate() {
    if (!plan?.editable || writing || loadingColumns) return
    const d = plan.day
    loadingColumns = true
    try {
      columns = await api.tasks.columns.list()
      if (plan?.day !== d) return
      if (columns.length === 0) toasts.error(t('game.noColumns'))
      else creating = true
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      loadingColumns = false
    }
  }

  async function create(input: TaskInput) {
    if (!plan?.editable || writing) return
    const d = plan.day
    writing = true
    try {
      acceptAddition(await api.tasks.createPlanTask(d, input))
      if (plan?.day === d) creating = false
    } finally {
      writing = false
    }
  }

  function setBlocks(id: number, blocks: number) {
    if (blocks >= 1 && blocks <= 40) apply(resize(slots, id, blocks))
  }

  function remove(id: number) {
    if (plan) void save(plan.items.filter((x) => x.task.id !== id))
  }

  /** Сдвиг на полчаса (стрелки): по тем же правилам, что и перетаскивание. */
  function nudge(e: KeyboardEvent, item: PlanItem) {
    if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
    e.preventDefault()
    const start = clampStart(item.start + (e.key === 'ArrowUp' ? -BLOCK_MIN : BLOCK_MIN), item.blocks)
    if (start !== item.start) apply(place(slots, item.task.id, start))
  }

  async function toggleDone(task: Task) {
    if (!plan?.editable || writing) return
    writing = true
    try {
      const next = await api.tasks.update(task.id, { done: !task.done })
      if (next.done) toasts.success(t('game.doneToast'))
      load(requestedDay)
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      writing = false
    }
  }

  function onDragStart(e: DragEvent, item: PlanItem) {
    const top = (e.currentTarget as HTMLElement).getBoundingClientRect().top
    drag = { id: item.task.id, grab: e.clientY - top }
    e.dataTransfer?.setData('text/plain', String(item.task.id))
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
  }

  /** Курсор над шкалой → начало дела (по верху карточки, с учётом места, где её ухватили). */
  function onDragOver(e: DragEvent) {
    if (!drag || !scaleEl) return
    e.preventDefault()
    const item = slots.find((x) => x.id === drag!.id)
    if (!item) return
    const y = e.clientY - scaleEl.getBoundingClientRect().top - drag.grab
    const start = clampStart(snap(scaleStart + (y / PX_PER_BLOCK) * BLOCK_MIN), item.blocks)
    if (preview?.start !== start) preview = { ...item, start }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    const p = preview
    endDrag()
    if (p) apply(place(slots, p.id, p.start))
  }

  function endDrag() {
    drag = null
    preview = null
  }

  function open(task: Task) {
    route.go('game', 'board', String(task.id))
  }
</script>

{#snippet act(icon: IconName, label: string, onclick: () => void, disabled = false)}
  <button class="act" aria-label={label} title={label} {disabled} {onclick}><Icon name={icon} size={14} /></button>
{/snippet}

<div class="page">
  <header>
    {#if day}<DatePager {day} {today} onchange={go} />{/if}
    {#if plan}
      <div class="total" class:over>
        <span>
          <Icon name="clock" size={16} />
          {t('game.plan.total', { used: blocksText(plan.totalBlocks || 0), limit: blocksText(s.dayLimit) })}
          {#if over}<strong>· {t('game.plan.overLimitShort')}</strong>{/if}
        </span>
        <ProgressBar value={plan.totalBlocks / s.dayLimit} color={over ? 'var(--c-red)' : undefined} label={t('game.plan.blocksLabel')} />
      </div>
    {/if}
  </header>

  {#if error && !plan}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(requestedDay)}>{t('common.retry')}</Button></div>
  {:else if !plan}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="grid">
      <section class="scale-card scroll" aria-label={t('game.plan.title')}>
        {#if !plan.editable}<p class="muted note">{t('game.plan.readonly')}</p>{/if}
        <div class="scale" style:height="{px(scaleEnd) + 1}px" bind:this={scaleEl}>
          {#each hourTicks(scaleStart, scaleEnd) as h (h)}
            <div class="tick" class:late={h > s.dayEnd || h < s.dayStart} style:top="{px(h)}px"><span>{formatTime(h)}</span></div>
          {/each}
          {#if scaleStart < s.dayStart}
            <div class="late-zone" style:top="0px" style:height="{px(s.dayStart)}px" title={t('game.plan.afterEnd')}></div>
          {/if}
          {#if scaleEnd > s.dayEnd}
            <div class="late-zone" style:top="{px(s.dayEnd)}px" style:height="{px(scaleEnd) - px(s.dayEnd)}px" title={t('game.plan.afterEnd')}></div>
          {/if}

          <!-- Перетаскивание дублируется стрелками ↑/↓ на выделенном деле. -->
          <div class="items" role="list" ondragover={onDragOver} ondrop={onDrop}>
            {#if preview}
              <div class="ghost" style:top="{px(preview.start)}px" style:height="{preview.blocks * PX_PER_BLOCK - 3}px">
                {formatTime(preview.start)}
              </div>
            {/if}
            {#each plan.items as item (item.task.id)}
              {@const type = item.task.typeId === null ? undefined : types.get(item.task.typeId)}
              {@const end = item.start + item.blocks * BLOCK_MIN}
              <!-- Пункт двигается стрелками ↑/↓ (замена кнопкам и перетаскиванию): ему нужны фокус и keydown. -->
              <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
              <div
                class="item"
                class:done={item.task.done}
                class:overflow={item.start < s.dayStart || end > s.dayEnd}
                class:small={item.blocks === 1}
                class:dragging={drag?.id === item.task.id}
                role="listitem"
                tabindex={plan.editable ? 0 : -1}
                title={plan.editable ? t('game.plan.moveHint') : undefined}
                style:top="{px(item.start)}px"
                style:height="{item.blocks * PX_PER_BLOCK - 3}px"
                style:--type={type?.color ?? 'var(--c-line)'}
                draggable={plan.editable && !writing}
                ondragstart={(e) => onDragStart(e, item)}
                ondragend={endDrag}
                onkeydown={(e) => plan?.editable && nudge(e, item)}
                in:fade={{ duration: 200 }}
              >
                <input type="checkbox" checked={item.task.done} disabled={!plan.editable || writing} aria-label={item.task.done ? t('game.markUndone') : t('game.markDone')}
                  onchange={() => toggleDone(item.task)} />
                <button class="title" title={t('game.plan.openTask')} onclick={() => open(item.task)}>
                  {item.task.title}
                </button>
                <span class="time muted">{formatTime(item.start)}–{formatTime(end)}</span>
                {#if plan.editable}
                  <span class="acts">
                    {@render act('chevronLeft', t('game.plan.lessBlocks'), () => setBlocks(item.task.id, item.blocks - 1), writing || item.blocks <= 1)}
                    <span class="blocks" title={t('game.plan.blocksLabel')}>{blocksText(item.blocks)}</span>
                    {@render act('chevronRight', t('game.plan.moreBlocks'), () => setBlocks(item.task.id, item.blocks + 1), writing || item.blocks >= 40)}
                    {#if day === today && !item.task.done}
                      {@render act('play', t('game.focus.begin'), () => focus.begin(item.task.id), focus.session !== null)}
                    {/if}
                    {@render act('x', t('game.plan.remove'), () => remove(item.task.id), writing)}
                  </span>
                {/if}
              </div>
            {/each}
          </div>
          {#if plan.items.length === 0}
            <p class="empty muted">{plan.editable ? t('game.plan.empty') : t('game.plan.emptyPast')}</p>
          {/if}
        </div>
      </section>

      {#if plan.editable}
        <aside class="side scroll">
          {#if plan.leftovers.length > 0}
            <section class="box leftovers">
              <h2>{t('game.plan.leftovers')}</h2>
              <p class="muted hint">{t('game.plan.leftoversHint')}</p>
              <ul>
                {#each plan.leftovers as task (task.id)}
                  {@render suggestion(task, [])}
                {/each}
              </ul>
            </section>
          {/if}

          <section class="box">
            <h2>{day === today ? t('game.plan.top') : t('game.plan.topDay')}</h2>
            {#if plan.top.length === 0}
              <p class="muted hint">{t('game.plan.topEmpty')}</p>
            {:else}
              <ul>
                {#each plan.top as x (x.task.id)}
                  {@render suggestion(x.task, x.reasons)}
                {/each}
              </ul>
            {/if}
            <div class="add-actions">
              <Button icon="plus" disabled={writing} onclick={() => (picking = true)}>{t('game.plan.pick')}</Button>
              <Button icon="plus" disabled={writing} loading={loadingColumns} onclick={showCreate}>{t('game.plan.newTask')}</Button>
            </div>
          </section>
        </aside>
      {/if}
    </div>
  {/if}
</div>

{#snippet suggestion(task: Task, reasons: string[])}
  {@const path = ancestors(openTasks, task.id)}
  {@const type = task.typeId === null ? undefined : types.get(task.typeId)}
  <li in:fade={{ duration: 200 }} style:--type={type?.color ?? 'var(--c-line)'}>
    <div class="s-body">
      {#if path.length > 0}<span class="path muted">{path.map((p) => p.title).join(' › ')}</span>{/if}
      <button class="s-title" onclick={() => open(task)}>{task.title}</button>
      <span class="s-meta muted">
        {blocksText(task.blocks)} · ★ {task.importance}{task.deadline ? ` · ${task.deadline}` : ''}
      </span>
      {#if reasons.length > 0}
        <span class="reasons">
          {#each reasons as r (r)}<span class="reason {r}">{t(`game.plan.reason.${r as 'quick'}`)}</span>{/each}
        </span>
      {/if}
    </div>
    <Button size="sm" icon="plus" disabled={writing} onclick={() => add(task)}>{t('game.plan.add')}</Button>
  </li>
{/snippet}

<TaskPicker open={picking} {planned} onclose={() => (picking = false)} onpick={add} />
<TaskDialog open={creating} task={null} column={null} {columns} types={[...types.values()]}
  onclose={() => (creating = false)} oncreate={create} />

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    min-height: 0;
    flex: 1;
  }
  header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-3);
  }
  .add-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-2);
  }
  .total {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    min-width: 220px;
    font-size: var(--fs-sm);
    font-weight: 600;
  }
  .total span {
    display: flex;
    align-items: center;
    gap: var(--s-1);
  }
  .total.over {
    color: var(--c-red);
  }
  .state {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
  }
  .grid {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(280px, 2fr);
    gap: var(--s-4);
  }
  .scale-card {
    min-height: 0;
    padding: var(--s-3) var(--s-3) var(--s-4);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
  }
  .note {
    margin: 0 0 var(--s-3);
    font-size: var(--fs-sm);
  }
  .scale {
    position: relative;
    margin-left: 48px;
  }
  .tick {
    position: absolute;
    left: 0;
    right: 0;
    border-top: 1px solid var(--c-line);
  }
  .tick span {
    position: absolute;
    left: -48px;
    top: -9px;
    width: 40px;
    color: var(--c-muted);
    font-size: var(--fs-xs);
    text-align: right;
  }
  .tick.late span {
    color: var(--c-red);
  }
  .late-zone {
    position: absolute;
    left: 0;
    right: 0;
    background: repeating-linear-gradient(-45deg, var(--c-red-soft) 0 6px, transparent 6px 12px);
  }
  .items {
    position: absolute;
    inset: 0;
  }
  .item {
    position: absolute;
    left: 6px;
    right: 0;
    display: flex;
    align-items: flex-start;
    gap: var(--s-2);
    padding: 3px var(--s-2) 3px calc(var(--s-2) + 4px);
    overflow: hidden;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    box-shadow: var(--shadow-sm);
    cursor: grab;
    transition:
      top var(--dur) var(--ease-out),
      height var(--dur) var(--ease-out);
  }
  .item::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 5px;
    background: var(--type);
  }
  .item.small {
    align-items: center;
    padding-top: 0;
    padding-bottom: 0;
  }
  .item.done {
    background: var(--c-surface-2);
    box-shadow: none;
  }
  .item.done .title {
    color: var(--c-muted);
    text-decoration: line-through;
  }
  .item.overflow {
    border-color: var(--c-red);
  }
  .item.dragging {
    opacity: 0.4;
  }
  .item input {
    flex: none;
    width: 16px;
    height: 16px;
    margin: 2px 0 0;
    accent-color: var(--c-green);
    cursor: pointer;
  }
  .item.small input {
    margin-top: 0;
  }
  .title {
    flex: 1;
    min-width: 0;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    font-weight: 700;
    font-size: var(--fs-sm);
    text-align: left;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    cursor: pointer;
  }
  .title:hover {
    text-decoration: underline;
  }
  .time {
    flex: none;
    font-size: var(--fs-xs);
    white-space: nowrap;
  }
  .acts {
    flex: none;
    display: flex;
    align-items: center;
    gap: 1px;
  }
  .blocks {
    min-width: 48px;
    font-size: var(--fs-xs);
    text-align: center;
  }
  .act {
    display: grid;
    place-items: center;
    width: 20px;
    height: 20px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--c-muted);
    cursor: pointer;
  }
  .act:hover:not(:disabled) {
    background: var(--c-surface-2);
    color: var(--c-text);
  }
  .act:disabled {
    opacity: 0.3;
    cursor: default;
  }
  .item:focus-visible {
    outline: var(--bw-strong) solid var(--c-focus);
    outline-offset: 1px;
  }
  .ghost {
    position: absolute;
    left: 6px;
    right: 0;
    z-index: 1;
    padding: 2px var(--s-2);
    border: var(--bw) dashed var(--c-green);
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--c-green-soft) 70%, transparent);
    color: var(--c-green);
    font-size: var(--fs-xs);
    font-weight: 700;
    pointer-events: none; /* иначе dragover шёл бы в призрак, а не в шкалу */
  }
  .empty {
    position: absolute;
    top: var(--s-4);
    left: var(--s-4);
    right: var(--s-4);
    margin: 0;
    text-align: center;
  }
  .side {
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    padding: 2px 6px 6px 2px;
  }
  .box {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding: var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
    box-shadow: var(--shadow-sm);
  }
  .leftovers {
    background: var(--c-amber-soft);
  }
  h2 {
    font-size: var(--fs-md);
  }
  .hint {
    margin: 0;
    font-size: var(--fs-sm);
  }
  .box ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .box li {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-2) var(--s-2) calc(var(--s-2) + 4px);
    position: relative;
    border: 1px solid var(--c-line);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
  }
  .box li::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 4px;
    background: var(--type);
    border-radius: var(--radius-sm) 0 0 var(--radius-sm);
  }
  .s-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .path,
  .s-meta {
    font-size: var(--fs-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .s-title {
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    font-weight: 700;
    text-align: left;
    cursor: pointer;
  }
  .s-title:hover {
    text-decoration: underline;
  }
  .reasons {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-1);
  }
  .reason {
    padding: 0 var(--s-2);
    border: 1px solid var(--c-line);
    border-radius: 999px;
    font-size: var(--fs-xs);
  }
  .reason.overdue {
    border-color: var(--c-red);
    color: var(--c-red);
  }
  .reason.deadline_soon {
    border-color: var(--c-amber);
    background: var(--c-amber-soft);
  }
  .reason.important {
    border-color: var(--c-text);
    font-weight: 700;
  }
  .reason.quick {
    border-color: var(--c-green);
    color: var(--c-green);
  }

  @media (max-width: 960px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
