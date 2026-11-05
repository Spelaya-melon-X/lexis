<script lang="ts">
  import type { DayStats } from '../../../lib/api/types'
  import { bars, plot, slotCenter, type Box } from '../../../lib/chart'
  import { shortDay } from '../../../lib/dates'
  import { i18n, t } from '../../../lib/i18n/i18n.svelte'

  // Столбики — слов за день (левая шкала), линия — доля «Знаю» (правая шкала, 0–100%).
  let { days }: { days: DayStats[] } = $props()

  const box: Box = { width: 640, height: 200, padX: 36, padY: 14 }
  const n = $derived(days.length)
  const maxWords = $derived(Math.max(...days.map((d) => d.words), 0))
  const columns = $derived(bars(days.map((d) => d.words), box))
  // Линия по центрам столбиков: поля plot сдвигаем на полслота.
  const line = $derived(
    n === 0 ? { points: [], path: '' } : plot(days.map((d) => d.goodShare), { ...box, padX: box.padX + (box.width - 2 * box.padX) / n / 2 }),
  )
  const xLabels = $derived(n === 0 ? [] : [...new Set([0, Math.floor((n - 1) / 2), n - 1])])
  const bottom = box.height - box.padY
</script>

<figure>
  <svg viewBox="0 0 {box.width} {box.height + 22}" role="img" aria-label={t('stats.trends')}>
    <line class="axis-line" x1={box.padX} x2={box.width - box.padX} y1={bottom} y2={bottom} />
    <text class="axis" x={box.padX - 6} y={box.padY + 4} text-anchor="end">{maxWords}</text>
    <text class="axis" x={box.padX - 6} y={bottom} text-anchor="end">0</text>
    <text class="axis share" x={box.width - box.padX + 6} y={box.padY + 4}>100%</text>
    <text class="axis share" x={box.width - box.padX + 6} y={bottom}>0%</text>

    {#each columns as b (b.index)}
      <rect class="bar" x={b.x} y={b.y} width={b.w} height={b.h} rx="2">
        <title>{shortDay(days[b.index]!.day, i18n.locale)}: {t('stats.dayWords', { count: days[b.index]!.words })}</title>
      </rect>
    {/each}

    {#if line.points.length > 1}<path class="line" d={line.path} />{/if}
    {#each line.points as p (p.index)}
      <circle class="dot" cx={p.x} cy={p.y} r="4"><title>{shortDay(days[p.index]!.day, i18n.locale)}: {Math.round(p.value * 100)}%</title></circle>
    {/each}

    {#each xLabels as i (i)}
      <text class="axis" x={slotCenter(i, n, box)} y={box.height + 16} text-anchor="middle">{shortDay(days[i]!.day, i18n.locale)}</text>
    {/each}
  </svg>
  <figcaption class="legend muted">
    <span><i class="key bar-key"></i>{t('stats.trendsWords')}</span>
    <span><i class="key line-key"></i>{t('stats.trendsShare')}</span>
  </figcaption>
</figure>

<style>
  figure {
    margin: 0;
  }
  svg {
    display: block;
    width: 100%;
    height: auto;
  }
  .axis {
    font-size: 12px;
    fill: var(--c-muted);
  }
  .axis-line {
    stroke: var(--c-border);
    stroke-width: 1.5;
  }
  .bar {
    fill: var(--c-level-3);
    stroke: var(--c-border);
    stroke-width: 1.5;
  }
  .line {
    fill: none;
    stroke: var(--c-green);
    stroke-width: 3;
    stroke-linejoin: round;
  }
  .dot {
    fill: var(--c-surface);
    stroke: var(--c-green);
    stroke-width: 2.5;
  }
  .legend {
    display: flex;
    gap: var(--s-4);
    font-size: var(--fs-xs);
  }
  .key {
    display: inline-block;
    width: 12px;
    height: 12px;
    margin-right: 4px;
    vertical-align: -2px;
    border: 1px solid var(--c-border);
  }
  .bar-key {
    background: var(--c-level-3);
  }
  .line-key {
    height: 3px;
    border: 0;
    background: var(--c-green);
    vertical-align: 3px;
  }
</style>
