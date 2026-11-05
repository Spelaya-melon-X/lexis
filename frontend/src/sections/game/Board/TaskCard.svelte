<script lang="ts">
  // Карточка дела на доске: тип, название, объём, важность, дедлайн. Перетаскивается;
  // без мыши — кнопки ↑↓←→ (видны при наведении и фокусе внутри карточки).
  import type { Task, TaskType } from '../../../lib/api/types'
  import Icon, { type IconName } from '../../../lib/components/Icon.svelte'
  import { shortDay } from '../../../lib/dates'
  import { i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { deadlineState } from '../../../lib/tasks'
  import { blocksText, blocksTextSec } from '../format'

  interface Props {
    task: Task
    path: Task[] // предки шага от корня: «Курсовая › Глава 1»
    type: TaskType | undefined
    today: string
    dragging: boolean
    canUp: boolean
    canDown: boolean
    canLeft: boolean
    canRight: boolean
    onopen: () => void
    ongraph: () => void
    ontoggle: () => void
    onshift: (dir: -1 | 1) => void
    onside: (dir: -1 | 1) => void
    ondragstart: (e: DragEvent) => void
    ondragend: () => void
  }
  let { task, path, type, today, dragging, canUp, canDown, canLeft, canRight, onopen, ongraph, ontoggle, onshift, onside, ondragstart, ondragend }: Props =
    $props()

  const due = $derived(deadlineState(task.deadline, today))
  const dueText = $derived.by(() => {
    if (!task.deadline) return ''
    const date = shortDay(task.deadline, i18n.locale)
    if (due === 'overdue') return t('game.overdue', { date })
    return due === 'today' ? t('game.dueToday') : t('game.dueDate', { date })
  })
</script>

{#snippet act(icon: IconName, label: string, onclick: () => void, disabled: boolean)}
  <button class="act" aria-label={label} title={label} {disabled} {onclick}><Icon name={icon} size={14} /></button>
{/snippet}

<article
  class="card"
  class:done={task.done}
  class:dragging
  style:--type={type?.color ?? 'var(--c-line)'}
  data-id={task.id}
  draggable="true"
  {ondragstart}
  {ondragend}
>
  {#if path.length > 0}
    <button class="path" title={t('game.graph')} onclick={ongraph}>
      <Icon name="graph" size={12} />
      {path.map((p) => p.title).join(' › ')}
    </button>
  {/if}
  <div class="top">
    <input type="checkbox" checked={task.done} aria-label={task.done ? t('game.markUndone') : t('game.markDone')} onchange={ontoggle} />
    <button class="title" onclick={onopen}>{task.title}</button>
  </div>
  <div class="meta">
    {#if type}<span class="type">{type.name}</span>{/if}
    <span class="pill" title={t('game.blocks')}><Icon name="clock" size={12} /> {blocksText(task.blocks)}</span>
    {#if task.workSec > 0}
      <span class="pill focus" title={t('game.stats.inFocus')}><Icon name="target" size={12} /> {blocksTextSec(task.workSec)}</span>
    {/if}
    <span class="pill imp" class:high={task.importance >= 8} title={t('game.importanceShort', { n: task.importance })}>★ {task.importance}</span>
    {#if task.deadline}<span class="pill due {due}">{dueText}</span>{/if}
  </div>
  <div class="acts">
    {@render act('chevronLeft', t('game.moveLeft'), () => onside(-1), !canLeft)}
    <!-- «вверх» — тот же шеврон, повёрнутый стилем: отдельной иконки не держим -->
    <span class="up">{@render act('chevronDown', t('game.moveUp'), () => onshift(-1), !canUp)}</span>
    {@render act('chevronDown', t('game.moveDown'), () => onshift(1), !canDown)}
    {@render act('chevronRight', t('game.moveRight'), () => onside(1), !canRight)}
  </div>
</article>

<style>
  .card {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-3) var(--s-2) calc(var(--s-3) + 4px);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    box-shadow: var(--shadow-sm);
    cursor: grab;
    transition:
      transform var(--dur-fast) var(--ease-out),
      opacity var(--dur-fast) var(--ease-out);
  }
  /* цвет типа — полоса слева */
  .card::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 5px;
    background: var(--type);
    border-radius: 2px 0 0 2px;
  }
  .card:hover {
    transform: translate(-1px, -1px);
  }
  .card.dragging {
    opacity: 0.4;
  }
  .card.done {
    box-shadow: none;
    background: var(--c-surface-2);
  }
  .card.done .title {
    color: var(--c-muted);
    text-decoration: line-through;
  }
  .path {
    display: flex;
    align-items: center;
    gap: var(--s-1);
    margin-right: 96px; /* не залезать под кнопки ←↑↓→ */
    padding: 0;
    border: 0;
    background: none;
    color: var(--c-muted);
    font: inherit;
    font-size: var(--fs-xs);
    text-align: left;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    cursor: pointer;
  }
  .path:hover {
    color: var(--c-text);
    text-decoration: underline;
  }
  .top {
    display: flex;
    align-items: flex-start;
    gap: var(--s-2);
  }
  .top input {
    flex: none;
    width: 18px;
    height: 18px;
    margin-top: 1px;
    accent-color: var(--c-green);
    cursor: pointer;
  }
  .title {
    flex: 1;
    min-width: 0;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    font-weight: 700;
    text-align: left;
    overflow-wrap: anywhere;
    cursor: pointer;
  }
  .title:hover {
    text-decoration: underline;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-1);
    font-size: var(--fs-xs);
  }
  .type {
    padding: 0 var(--s-2);
    border-radius: 999px;
    background: var(--type);
    color: #fff;
    font-weight: 700;
  }
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 0 var(--s-2);
    border: 1px solid var(--c-line);
    border-radius: 999px;
    color: var(--c-muted);
  }
  .pill.focus {
    border-color: var(--c-green);
    color: var(--c-green);
  }
  .imp.high {
    border-color: var(--c-amber);
    color: var(--c-amber);
    font-weight: 700;
  }
  .due.soon,
  .due.today {
    border-color: var(--c-amber);
    background: var(--c-amber-soft);
    color: var(--c-text);
  }
  .due.overdue {
    border-color: var(--c-red);
    background: var(--c-red-soft);
    color: var(--c-red);
    font-weight: 700;
  }
  .acts {
    position: absolute;
    top: 2px;
    right: 2px;
    display: flex;
    gap: 1px;
    padding: 1px;
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    opacity: 0;
    transition: opacity var(--dur-fast) var(--ease-out);
  }
  .card:hover .acts,
  .card:focus-within .acts {
    opacity: 1;
  }
  .act {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
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
  .up :global(svg) {
    transform: rotate(180deg);
  }
</style>
