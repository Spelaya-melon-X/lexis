<script lang="ts">
  // Прогресс упражнения (§18.4, C2): по дням — максимальный вес (линия) и тоннаж (столбики), последние полгода.
  // Для упражнений без веса — повторы за день.
  import { api } from '../../../lib/api/client'
  import type { ProgressPoint } from '../../../lib/api/types'
  import Card from '../../../lib/components/Card.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { bars, plot, slotCenter, type Box } from '../../../lib/chart'
  import { parseDay } from '../../../lib/dates'
  import { formatAmount } from '../../../lib/habits'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { weightParts } from '../../../lib/sport'

  let { id }: { id: number } = $props()

  const BOX: Box = { width: 640, height: 180, padX: 12, padY: 14 }

  let name = $state('')
  let points = $state<ProgressPoint[] | null>(null)
  let error = $state<string | null>(null)

  $effect(() => {
    points = null
    api.sport
      .progress(id)
      .then((r) => ((name = r.exercise.name), (points = r.points)))
      .catch((e: unknown) => (error = errorText(e)))
  })

  const weighted = $derived((points ?? []).some((p) => p.maxWeight !== null))
  const graph = $derived.by(() => {
    if (!points || points.length === 0) return null
    const main = points.map((p) => (weighted ? p.maxWeight : p.reps))
    const top = Math.max(...main.map((v) => v ?? 0))
    const line = plot(main.map((v) => (v === null || top === 0 ? null : v / top)), BOX)
    // Точки линии — по центрам слотов столбиков, чтобы вставали над ними.
    line.points = line.points.map((p) => ({ ...p, x: slotCenter(p.index, points!.length, BOX) }))
    line.path = line.points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x} ${p.y}`).join(' ')
    return { line, tonnage: weighted ? bars(points.map((p) => p.tonnage), BOX, 0.4) : [], top }
  })

  const kg = (n: number) => {
    const p = weightParts(n)
    return `${formatAmount(p.value, i18n.locale)} ${t(p.unit === 't' ? 'sport.t' : 'sport.kg')}`
  }
  const day = (d: string) => parseDay(d).toLocaleDateString(i18n.locale, { day: 'numeric', month: 'short' })
  const best = $derived(points && weighted ? Math.max(...points.map((p) => p.maxWeight ?? 0)) : null)
</script>

<Card>
  <h2>{t('sport.progress.title', { name })}</h2>
  {#if error && !points}
    <p class="error">{error}</p>
  {:else if !points}
    <div class="state"><Spinner size={24} /></div>
  {:else if points.length === 0 || !graph}
    <p class="muted">{t('sport.progress.empty')}</p>
  {:else}
    <p class="muted sub">
      {weighted ? t('sport.progress.weighted', { best: kg(best ?? 0) }) : t('sport.progress.bodyweight')} · {t('sport.uses', { count: points.length })}
    </p>
    <svg viewBox="0 0 {BOX.width} {BOX.height}" class="chart" role="img" aria-label={t('sport.progress.title', { name })}>
      {#each graph.tonnage as b (b.index)}
        <rect x={b.x} y={b.y} width={b.w} height={b.h} rx="2"><title>{day(points[b.index]!.day)}: {kg(points[b.index]!.tonnage)}</title></rect>
      {/each}
      <path d={graph.line.path} />
      {#each graph.line.points as p (p.index)}
        {@const pt = points[p.index]!}
        <circle cx={p.x} cy={p.y} r="4"><title>{day(pt.day)}: {weighted ? kg(pt.maxWeight ?? 0) : pt.reps} · {t('sport.sets')} {pt.sets}</title></circle>
      {/each}
    </svg>
    <div class="axis muted"><span>{day(points[0]!.day)}</span><span>{day(points.at(-1)!.day)}</span></div>
    <p class="legend muted">
      <i class="sw line"></i>{weighted ? t('sport.progress.maxWeight') : t('sport.progress.reps')}
      {#if weighted}<i class="sw bar"></i>{t('sport.colTonnage')}{/if}
    </p>
  {/if}
</Card>

<style>
  h2 {
    font-size: var(--fs-lg);
  }
  .sub {
    margin: var(--s-1) 0 var(--s-3);
    font-size: var(--fs-sm);
  }
  .state {
    display: grid;
    place-items: center;
    padding: var(--s-4);
  }
  .chart {
    display: block;
    width: 100%;
    height: auto;
  }
  rect {
    fill: var(--c-green-soft);
    stroke: var(--c-green);
    stroke-width: 1;
  }
  path {
    fill: none;
    stroke: var(--c-text);
    stroke-width: 2.5;
  }
  circle {
    fill: var(--c-surface);
    stroke: var(--c-text);
    stroke-width: 2;
  }
  .axis {
    display: flex;
    justify-content: space-between;
    font-size: var(--fs-xs);
  }
  .legend {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    margin: var(--s-2) 0 0;
    font-size: var(--fs-xs);
  }
  .sw {
    width: 14px;
    height: 10px;
    border-radius: 2px;
  }
  .sw.line {
    height: 3px;
    background: var(--c-text);
  }
  .sw.bar {
    margin-left: var(--s-2);
    background: var(--c-green-soft);
    border: 1px solid var(--c-green);
  }
  .error {
    color: var(--c-red);
  }
</style>
