<script lang="ts">
  // График раздела «День» (§17.5): столбики — дофамин (0..7, левая шкала), линия — оценка дня
  // (1..10, правая шкала). Шкалы фиксированные: «2 из 7» не должно выглядеть полным столбиком.
  import type { DaySummary } from '../../lib/api/types'
  import { bars, plot, slotCenter, type Box } from '../../lib/chart'
  import { parseDay, shortDay } from '../../lib/dates'
  import { i18n, t } from '../../lib/i18n/i18n.svelte'
  import { DOPAMINE_SLOTS } from '../../lib/journal'

  let { days, height = 200 }: { days: DaySummary[]; height?: number } = $props()

  const box = $derived<Box>({ width: 640, height, padX: 36, padY: 14 })
  const n = $derived(days.length)
  const columns = $derived(bars(days.map((d) => d.dopamineLevel), box, 0.3, DOPAMINE_SLOTS))
  // Линия по центрам столбиков: поля plot сдвигаем на полслота. Оценка 1..10 → 0..1.
  const line = $derived(
    n === 0
      ? { points: [], path: '' }
      : plot(
          days.map((d) => (d.rating === null ? null : (d.rating - 1) / 9)),
          { ...box, padX: box.padX + (box.width - 2 * box.padX) / n / 2 },
        ),
  )
  // Неделя — подпись у каждого дня (день недели), месяц — три опорные даты.
  const xLabels = $derived(n <= 7 ? days.map((_, i) => i) : [...new Set([0, Math.floor((n - 1) / 2), n - 1])])
  const label = (i: number) =>
    n <= 7 ? parseDay(days[i]!.day).toLocaleDateString(i18n.locale, { weekday: 'short' }) : shortDay(days[i]!.day, i18n.locale)
  const bottom = $derived(box.height - box.padY)
</script>

<figure>
  <svg viewBox="0 0 {box.width} {box.height + 22}" role="img" aria-label={t('journal.stats.chart')}>
    <line class="axis-line" x1={box.padX} x2={box.width - box.padX} y1={bottom} y2={bottom} />
    <text class="axis dop" x={box.padX - 6} y={box.padY + 4} text-anchor="end">{DOPAMINE_SLOTS}</text>
    <text class="axis dop" x={box.padX - 6} y={bottom} text-anchor="end">0</text>
    <text class="axis rate" x={box.width - box.padX + 6} y={box.padY + 4}>10</text>
    <text class="axis rate" x={box.width - box.padX + 6} y={bottom}>1</text>

    {#each columns as b (b.index)}
      <rect class="bar" x={b.x} y={b.y} width={b.w} height={b.h} rx="2">
        <title>{shortDay(days[b.index]!.day, i18n.locale)}: {t('journal.stats.dopamine')} {days[b.index]!.dopamineLevel}/{DOPAMINE_SLOTS}</title>
      </rect>
    {/each}

    {#if line.points.length > 1}<path class="line" d={line.path} />{/if}
    {#each line.points as p (p.index)}
      <circle class="dot" cx={p.x} cy={p.y} r="4">
        <title>{shortDay(days[p.index]!.day, i18n.locale)}: {t('journal.stats.rating')} {days[p.index]!.rating}</title>
      </circle>
    {/each}

    {#each xLabels as i (i)}
      <text class="axis" x={slotCenter(i, n, box)} y={box.height + 16} text-anchor="middle">{label(i)}</text>
    {/each}
  </svg>
  <figcaption class="legend muted">
    <span><i class="key bar-key"></i>{t('journal.stats.dopamine')}</span>
    <span><i class="key line-key"></i>{t('journal.stats.rating')}</span>
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
  .axis.dop {
    fill: var(--c-green);
  }
  .axis.rate {
    fill: var(--c-focus);
  }
  .axis-line {
    stroke: var(--c-border);
    stroke-width: 1.5;
  }
  .bar {
    fill: var(--c-green);
    stroke: var(--c-border);
    stroke-width: 1.5;
  }
  .line {
    fill: none;
    stroke: var(--c-focus);
    stroke-width: 3;
    stroke-linejoin: round;
  }
  .dot {
    fill: var(--c-surface);
    stroke: var(--c-focus);
    stroke-width: 3;
  }
  .legend {
    display: flex;
    gap: var(--s-4);
    margin-top: var(--s-2);
    font-size: var(--fs-sm);
  }
  .legend span {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  .key {
    display: inline-block;
    width: 14px;
    height: 10px;
    border: 1.5px solid var(--c-border);
  }
  .bar-key {
    background: var(--c-green);
  }
  .line-key {
    height: 0;
    border: 0;
    border-top: 3px solid var(--c-focus);
  }
</style>
