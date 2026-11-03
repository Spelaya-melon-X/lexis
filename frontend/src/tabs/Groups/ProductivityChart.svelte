<script lang="ts">
  import type { DayPoint } from '../../lib/api/types'
  import { plot, xOf, type Box } from '../../lib/chart'
  import { shortDay } from '../../lib/dates'
  import { i18n, t } from '../../lib/i18n/i18n.svelte'

  let { days }: { days: DayPoint[] } = $props()

  const box: Box = { width: 640, height: 200, padX: 40, padY: 16 }
  const chart = $derived(plot(days.map((d) => d.goodShare), box))
  const gridY = [0, 0.5, 1]
  const y = (v: number) => box.padY + (1 - v) * (box.height - 2 * box.padY)
  // Подписи оси X: первый, средний и последний день — больше не влезает на неделе/месяце без наложений.
  const xLabels = $derived(
    days.length === 0 ? [] : [...new Set([0, Math.floor((days.length - 1) / 2), days.length - 1])],
  )
  const pct = (v: number) => `${Math.round(v * 100)}%`
</script>

<figure>
  <svg viewBox="0 0 {box.width} {box.height + 22}" role="img" aria-label={t('groups.productivityHint')}>
    {#each gridY as g (g)}
      <line class="grid" x1={box.padX} x2={box.width - box.padX} y1={y(g)} y2={y(g)} />
      <text class="axis" x={box.padX - 8} y={y(g) + 4} text-anchor="end">{pct(g)}</text>
    {/each}
    {#each xLabels as i (i)}
      <text class="axis" x={xOf(i, days.length, box)} y={box.height + 16} text-anchor="middle">{shortDay(days[i]!.day, i18n.locale)}</text>
    {/each}

    {#if chart.points.length > 1}
      <path class="line" d={chart.path} />
    {/if}
    {#each chart.points as p (p.index)}
      <circle class="dot" cx={p.x} cy={p.y} r="5">
        <title>{shortDay(days[p.index]!.day, i18n.locale)}: {pct(p.value)}</title>
      </circle>
    {/each}
  </svg>
  {#if chart.points.length === 0}
    <figcaption class="muted">{t('groups.noSessions')}</figcaption>
  {/if}
</figure>

<style>
  figure {
    position: relative;
    margin: 0;
  }
  svg {
    display: block;
    width: 100%;
    height: auto;
  }
  .grid {
    stroke: var(--c-line);
    stroke-width: 1;
    stroke-dasharray: 4 4;
  }
  .axis {
    font-size: 12px;
    fill: var(--c-muted);
  }
  .line {
    fill: none;
    stroke: var(--c-green);
    stroke-width: 3;
    stroke-linejoin: round;
    stroke-linecap: round;
  }
  .dot {
    fill: var(--c-surface);
    stroke: var(--c-border);
    stroke-width: 2;
    transition: r var(--dur-fast) var(--ease-out);
  }
  .dot:hover {
    r: 7;
    fill: var(--c-green);
  }
  figcaption {
    position: absolute;
    inset: 0 0 22px 40px;
    display: grid;
    place-items: center;
    font-size: var(--fs-sm);
  }
</style>
