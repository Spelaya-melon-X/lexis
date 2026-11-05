<script lang="ts">
  import type { DayStats } from '../../../lib/api/types'
  import { parseDay, shortDay } from '../../../lib/dates'
  import { monthLabels, weeks } from '../../../lib/heatmap'
  import { i18n, t } from '../../../lib/i18n/i18n.svelte'

  let { days, selected, onselect }: { days: DayStats[]; selected: string | null; onselect: (day: string) => void } = $props()

  const CELL = 12
  const GAP = 3
  const STEP = CELL + GAP
  const LEFT = 30 // подписи дней недели
  const TOP = 18 // подписи месяцев

  const cols = $derived(weeks(days))
  const months = $derived(monthLabels(cols))
  const width = $derived(LEFT + cols.length * STEP)
  const height = TOP + 7 * STEP

  // Пн/Ср/Пт по локали: 2 ноября 2026 — понедельник.
  const weekdayLabel = (row: number) => new Date(2026, 10, 2 + row).toLocaleDateString(i18n.locale, { weekday: 'short' })
  const monthName = (day: string) => parseDay(day).toLocaleDateString(i18n.locale, { month: 'short' })

  function onkeydown(e: KeyboardEvent, day: string) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      onselect(day)
    }
  }
</script>

<div class="wrap">
  <svg viewBox="0 0 {width} {height}" style:max-width="{width}px" role="group" aria-label={t('stats.calendar')}>
    {#each months as m (m.col)}
      <text class="label" x={LEFT + m.col * STEP} y={11}>{monthName(m.day)}</text>
    {/each}
    {#each [0, 2, 4] as row (row)}
      <text class="label" x={0} y={TOP + row * STEP + CELL - 2}>{weekdayLabel(row)}</text>
    {/each}

    {#each cols as col, ci (ci)}
      {#each col as cell, ri (ri)}
        {#if cell}
          <rect
            class="cell l{cell.level}"
            class:selected={cell.day.day === selected}
            x={LEFT + ci * STEP}
            y={TOP + ri * STEP}
            width={CELL}
            height={CELL}
            rx="2"
            role="button"
            tabindex="0"
            aria-label="{shortDay(cell.day.day, i18n.locale)}: {t('stats.dayWords', { count: cell.day.words })}"
            onclick={() => onselect(cell.day.day)}
            onkeydown={(e) => onkeydown(e, cell.day.day)}
          >
            <title>{shortDay(cell.day.day, i18n.locale)}: {t('stats.dayWords', { count: cell.day.words })}</title>
          </rect>
        {/if}
      {/each}
    {/each}
  </svg>

  <div class="legend muted">
    <span>{t('stats.less')}</span>
    {#each [0, 1, 2, 3, 4] as l (l)}<span class="swatch l{l}"></span>{/each}
    <span>{t('stats.more')}</span>
  </div>
</div>

<style>
  .wrap {
    overflow-x: auto;
  }
  svg {
    display: block;
    width: 100%;
    height: auto;
  }
  .label {
    font-size: 10px;
    fill: var(--c-muted);
  }
  .cell {
    stroke: var(--c-border);
    stroke-width: 1;
    cursor: pointer;
    transition: stroke-width var(--dur-fast) var(--ease-out);
  }
  .cell:hover,
  .cell:focus-visible {
    stroke-width: 2;
    outline: none;
  }
  .cell.selected {
    stroke-width: 2.5;
  }
  /* 0 — пустой день, 1..4 — светлее → темнее зелёный */
  .l0 {
    fill: var(--c-surface-2);
    stroke: var(--c-line);
  }
  .l1 {
    fill: #bbf7d0;
  }
  .l2 {
    fill: #4ade80;
  }
  .l3 {
    fill: #16a34a;
  }
  .l4 {
    fill: #14532d;
  }
  .legend {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 4px;
    margin-top: var(--s-2);
    font-size: var(--fs-xs);
  }
  .swatch {
    width: 12px;
    height: 12px;
    border: 1px solid var(--c-border);
    border-radius: 2px;
  }
  .swatch.l0 {
    background: var(--c-surface-2);
    border-color: var(--c-line);
  }
  .swatch.l1 {
    background: #bbf7d0;
  }
  .swatch.l2 {
    background: #4ade80;
  }
  .swatch.l3 {
    background: #16a34a;
  }
  .swatch.l4 {
    background: #14532d;
  }
</style>
