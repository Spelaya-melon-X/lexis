<script lang="ts">
  // Итоги недели (§18.5.7): сколько потратил, сколько поднято, средние на тренировку,
  // таблица по дням (ккал, минуты, тоннаж) и какие упражнения делал.
  import type { Workout } from '../../../lib/api/types'
  import Card from '../../../lib/components/Card.svelte'
  import { parseDay } from '../../../lib/dates'
  import { formatAmount } from '../../../lib/habits'
  import { i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { weekSummary, weightParts } from '../../../lib/sport'

  let { workouts, days }: { workouts: Workout[]; days: string[] } = $props()

  const s = $derived(weekSummary(workouts, days))
  const num = (n: number | null, digits = 0) => (n === null ? '—' : n.toLocaleString(i18n.locale, { maximumFractionDigits: digits }))
  const kg = (n: number | null) => {
    if (n === null) return '—'
    const p = weightParts(n)
    return `${formatAmount(p.value, i18n.locale)} ${t(p.unit === 't' ? 'sport.t' : 'sport.kg')}`
  }
  const day = (d: string) => parseDay(d).toLocaleDateString(i18n.locale, { weekday: 'short', day: 'numeric' })
</script>

<Card>
  <h2>{t('sport.weekStats')}</h2>
  {#if s.workouts === 0}
    <p class="muted">{t('sport.weekEmpty')}</p>
  {:else}
    <div class="tiles">
      <div><span class="muted">{t('sport.statWorkouts')}</span><strong>{s.workouts}</strong></div>
      <div><span class="muted">{t('sport.statKcal')}</span><strong>{num(s.kcal)}</strong><small class="muted">{t('sport.perWorkout', { v: num(s.avgKcal) })}</small></div>
      <div><span class="muted">{t('sport.statTime')}</span><strong>{num(s.durationMin)} {t('sport.min')}</strong><small class="muted">{t('sport.perWorkout', { v: num(s.avgDuration) })}</small></div>
      <div><span class="muted">{t('sport.statTonnage')}</span><strong>{kg(s.tonnage)}</strong><small class="muted">{t('sport.perWorkout', { v: kg(s.avgTonnage) })}</small></div>
    </div>

    <div class="tables">
      <table>
        <thead><tr><th>{t('sport.colDay')}</th><th>{t('sport.statWorkouts')}</th><th>{t('sport.kcalShort')}</th><th>{t('sport.min')}</th><th>{t('sport.colTonnage')}</th></tr></thead>
        <tbody>
          {#each s.days as r (r.day)}
            <tr class:empty={r.workouts === 0}>
              <td>{day(r.day)}</td><td>{r.workouts || '—'}</td><td>{r.kcal || '—'}</td><td>{r.durationMin || '—'}</td><td>{r.tonnage ? kg(r.tonnage) : '—'}</td>
            </tr>
          {/each}
        </tbody>
        <tfoot>
          <tr><td>{t('sport.total')}</td><td>{s.workouts}</td><td>{num(s.kcal)}</td><td>{num(s.durationMin)}</td><td>{kg(s.tonnage)}</td></tr>
          <tr class="muted"><td>{t('sport.average')}</td><td></td><td>{num(s.avgKcal)}</td><td>{num(s.avgDuration)}</td><td>{kg(s.avgTonnage)}</td></tr>
        </tfoot>
      </table>

      {#if s.exercises.length > 0}
        <table>
          <thead><tr><th>{t('sport.exercise')}</th><th>{t('sport.colTimes')}</th><th>{t('sport.sets')}</th><th>{t('sport.colReps')}</th><th>{t('sport.colMax')}</th><th>{t('sport.colTonnage')}</th></tr></thead>
          <tbody>
            {#each s.exercises as e (e.name)}
              <tr><td>{e.name}</td><td>{e.times}</td><td>{e.sets}</td><td>{e.reps}</td><td>{e.maxWeight === null ? '—' : kg(e.maxWeight)}</td><td>{e.tonnage ? kg(e.tonnage) : '—'}</td></tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
    <p class="muted hint">{t('sport.averageHint')}</p>
  {/if}
</Card>

<style>
  h2 {
    font-size: var(--fs-lg);
    margin-bottom: var(--s-3);
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: var(--s-3);
    margin-bottom: var(--s-4);
  }
  .tiles div {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
  }
  .tiles span {
    font-size: var(--fs-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .tiles strong {
    font-size: var(--fs-xl);
    font-variant-numeric: tabular-nums;
  }
  small {
    font-size: var(--fs-xs);
  }
  .tables {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
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
    padding: var(--s-1) var(--s-2);
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
  .hint {
    margin: var(--s-3) 0 0;
    font-size: var(--fs-xs);
  }
</style>
