<script lang="ts">
  // Статистика спорта (§18.4, C2): за 7 / 30 дней — тренировки, сожжено и съедено ккал по дням (рядом, без «баланса»),
  // время, тоннаж, средние; таблица по дням и упражнения (клик — прогресс упражнения).
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { SportStats } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import Segmented from '../../../lib/components/Segmented.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { bars, type Box } from '../../../lib/chart'
  import { addDays, parseDay, shortDay, today } from '../../../lib/dates'
  import { formatAmount } from '../../../lib/habits'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { weightParts } from '../../../lib/sport'

  type Span = '7' | '30'
  const SPAN_KEY = 'lexis.sportStatsSpan'
  const BOX: Box = { width: 640, height: 160, padX: 8, padY: 12 }

  function storedSpan(): Span {
    try {
      return localStorage.getItem(SPAN_KEY) === '30' ? '30' : '7'
    } catch {
      return '7'
    }
  }

  const now = today()
  let span = $state<Span>(storedSpan())
  let data = $state<SportStats | null>(null)
  let error = $state<string | null>(null)
  const fetchStats = latest(api.sport.stats)

  function load(s: Span) {
    error = null
    void fetchStats(addDays(now, -(Number(s) - 1)), now)
      .then((r) => !r.stale && (data = r.value))
      .catch((e: unknown) => (error = errorText(e)))
  }
  $effect(() => load(span))

  function setSpan(s: Span) {
    span = s
    try {
      localStorage.setItem(SPAN_KEY, s)
    } catch {
      // не запомнили — не страшно
    }
  }

  // Два ряда столбиков на общей шкале: съедено (слева в слоте) и сожжено (справа).
  const chart = $derived.by(() => {
    if (!data) return null
    const eaten = data.days.map((d) => d.eatenKcal ?? 0)
    const burned = data.days.map((d) => d.burnedKcal)
    const max = Math.max(...eaten, ...burned, 0)
    if (max === 0) return null
    const half = (b: ReturnType<typeof bars>[number], dir: -1 | 1) => ({ ...b, x: b.x + (dir * b.w) / 2 })
    return {
      eaten: bars(eaten, BOX, 0.55, max).map((b) => half(b, -1)),
      burned: bars(burned, BOX, 0.55, max).map((b) => half(b, 1)),
    }
  })

  const num = (n: number | null) => (n === null ? '—' : Math.round(n).toLocaleString(i18n.locale))
  const kg = (n: number | null) => {
    if (n === null || n === 0) return '—'
    const p = weightParts(n)
    return `${formatAmount(p.value, i18n.locale)} ${t(p.unit === 't' ? 'sport.t' : 'sport.kg')}`
  }
  const day = (d: string) => parseDay(d).toLocaleDateString(i18n.locale, { weekday: 'short', day: 'numeric', month: 'short' })
</script>

<div class="page">
  <header>
    <h1>{t('nav.sport.stats')}</h1>
    <Segmented
      label={t('nav.sport.stats')}
      value={span}
      options={[
        { value: '7', label: t('game.stats.week') },
        { value: '30', label: t('game.stats.month') },
      ]}
      onchange={setSpan}
    />
  </header>

  {#if error && !data}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(span)}>{t('common.retry')}</Button></div>
  {:else if !data}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    {@const tot = data.totals}
    <div class="scroll body">
      <div class="tiles">
        <Card padding="sm" class="tile"><span class="muted label">{t('sport.statWorkouts')}</span><span class="value">{tot.workouts}</span>
          <small class="muted">{t('sport.stats.workoutDays', { n: tot.workoutDays, total: data.days.length })}</small></Card>
        <Card padding="sm" class="tile"><span class="muted label">{t('sport.statKcal')}</span><span class="value">{num(tot.burnedKcal)}</span>
          <small class="muted">{t('sport.perWorkout', { v: num(tot.avgBurned) })}</small></Card>
        <Card padding="sm" class="tile"><span class="muted label">{t('sport.stats.eaten')}</span><span class="value">{num(tot.avgEatenPerDay)}</span>
          <small class="muted">{t('sport.stats.eatenHint', { n: tot.foodDays })}</small></Card>
        <Card padding="sm" class="tile"><span class="muted label">{t('sport.statTime')}</span><span class="value">{num(tot.durationMin)} {t('sport.min')}</span>
          <small class="muted">{t('sport.perWorkout', { v: num(tot.avgDuration) })}</small></Card>
        <Card padding="sm" class="tile"><span class="muted label">{t('sport.statTonnage')}</span><span class="value">{kg(tot.tonnage)}</span>
          <small class="muted">{t('sport.perWorkout', { v: kg(tot.avgTonnage) })}</small></Card>
      </div>

      <Card>
        <h2>{t('sport.stats.kcalChart')}</h2>
        {#if !chart}
          <p class="muted">{t('sport.stats.noKcal')}</p>
        {:else}
          <svg viewBox="0 0 {BOX.width} {BOX.height}" class="chart" role="img" aria-label={t('sport.stats.kcalChart')}>
            {#each chart.eaten as b (`e${b.index}`)}
              <rect class="eaten" x={b.x} y={b.y} width={b.w} height={b.h} rx="2"><title>{day(data.days[b.index]!.day)}: {t('sport.stats.eaten')} {num(data.days[b.index]!.eatenKcal)}</title></rect>
            {/each}
            {#each chart.burned as b (`b${b.index}`)}
              <rect class="burned" x={b.x} y={b.y} width={b.w} height={b.h} rx="2"><title>{day(data.days[b.index]!.day)}: {t('sport.statKcal')} {num(data.days[b.index]!.burnedKcal)}</title></rect>
            {/each}
          </svg>
          <div class="axis muted"><span>{shortDay(data.from, i18n.locale)}</span><span>{shortDay(data.to, i18n.locale)}</span></div>
          <p class="legend muted"><i class="sw eaten"></i>{t('sport.stats.eaten')} <i class="sw burned"></i>{t('sport.statKcal')}</p>
        {/if}
      </Card>

      <div class="tables">
        <Card padding="none">
          <table>
            <thead><tr><th>{t('sport.colDay')}</th><th>{t('sport.statWorkouts')}</th><th>{t('sport.stats.burnedShort')}</th><th>{t('sport.stats.eatenShort')}</th><th>{t('sport.colTonnage')}</th></tr></thead>
            <tbody>
              {#each [...data.days].reverse() as r (r.day)}
                <tr class:empty={r.workouts === 0 && r.eatenKcal === null}>
                  <td>{day(r.day)}</td><td>{r.workouts || '—'}</td><td>{r.burnedKcal ? num(r.burnedKcal) : '—'}</td><td>{num(r.eatenKcal)}</td><td>{kg(r.tonnage)}</td>
                </tr>
              {/each}
            </tbody>
            <tfoot>
              <tr><td>{t('sport.total')}</td><td>{tot.workouts}</td><td>{num(tot.burnedKcal)}</td><td>{num(tot.eatenKcal)}</td><td>{kg(tot.tonnage)}</td></tr>
              <tr class="muted"><td>{t('sport.stats.perDay')}</td><td></td><td>{num(tot.avgBurnedPerDay)}</td><td>{num(tot.avgEatenPerDay)}</td><td></td></tr>
            </tfoot>
          </table>
        </Card>

        <Card padding="none">
          {#if data.exercises.length === 0}
            <p class="muted pad">{t('sport.stats.noExercises')}</p>
          {:else}
            <table>
              <thead><tr><th>{t('sport.exercise')}</th><th>{t('sport.colTimes')}</th><th>{t('sport.sets')}</th><th>{t('sport.colReps')}</th><th>{t('sport.colMax')}</th><th>{t('sport.colTonnage')}</th></tr></thead>
              <tbody>
                {#each data.exercises as e (e.id)}
                  <tr>
                    <td><a href="#/sport/exercises/{e.id}" title={t('sport.progress.open')}>{e.name}</a></td>
                    <td>{e.workouts}</td><td>{e.sets}</td><td>{e.reps}</td><td>{e.maxWeight === null ? '—' : kg(e.maxWeight)}</td><td>{kg(e.tonnage)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </Card>
      </div>
      <p class="muted hint">{t('sport.stats.hint')}</p>
    </div>
  {/if}
</div>

<style>
  .page {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }
  header {
    display: flex;
    align-items: center;
    gap: var(--s-4);
  }
  h1 {
    flex: 1;
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
  }
  h2 {
    font-size: var(--fs-md);
    margin-bottom: var(--s-3);
  }
  .state {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
  }
  .body {
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    padding: 2px 6px var(--s-4) 2px;
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--s-3);
  }
  .tiles :global(.tile) {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .label {
    font-size: var(--fs-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .value {
    font-size: var(--fs-xl);
    font-weight: 800;
    font-variant-numeric: tabular-nums;
  }
  small {
    font-size: var(--fs-xs);
  }
  .chart {
    display: block;
    width: 100%;
    height: auto;
  }
  rect.eaten {
    fill: var(--c-amber);
  }
  rect.burned {
    fill: var(--c-green);
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
    width: 12px;
    height: 10px;
    border-radius: 2px;
  }
  .sw.eaten {
    background: var(--c-amber);
  }
  .sw.burned {
    background: var(--c-green);
    margin-left: var(--s-2);
  }
  .tables {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    gap: var(--s-4);
    align-items: start;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--fs-sm);
    font-variant-numeric: tabular-nums;
  }
  th,
  td {
    padding: var(--s-1) var(--s-3);
    border-bottom: 1px solid var(--c-line);
    text-align: right;
  }
  th:first-child,
  td:first-child {
    text-align: left;
  }
  th {
    color: var(--c-muted);
    font-size: var(--fs-xs);
    font-weight: 700;
    text-transform: uppercase;
  }
  tr.empty td {
    color: var(--c-muted);
  }
  tfoot td {
    font-weight: 700;
    border-bottom: 0;
  }
  tfoot tr.muted td {
    font-weight: 400;
  }
  td a {
    color: inherit;
    font-weight: 600;
  }
  .pad {
    padding: var(--s-3);
  }
  .hint {
    margin: 0;
    font-size: var(--fs-xs);
  }
</style>
