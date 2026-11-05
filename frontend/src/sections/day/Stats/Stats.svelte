<script lang="ts">
  // Статистика раздела «День» (§17.5): оценка дня и дофамин по дням, выполнение привычек.
  // Период — последние 7 или 30 дней, включая сегодня.
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { DaySummary, HabitStat } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import Segmented from '../../../lib/components/Segmented.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { addDays, longDay, today } from '../../../lib/dates'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { bestDay, summarize } from '../../../lib/journal'
  import DayChart from '../DayChart.svelte'
  import HabitStats from '../HabitStats.svelte'

  type Span = 7 | 30
  const SPAN_KEY = 'lexis.dayStatsSpan'

  function storedSpan(): Span {
    try {
      return localStorage.getItem(SPAN_KEY) === '30' ? 30 : 7
    } catch {
      return 7
    }
  }

  const now = today()
  let span = $state<Span>(storedSpan())
  let data = $state<{ days: DaySummary[]; habits: HabitStat[] } | null>(null)
  let error = $state<string | null>(null)

  const fetchAll = latest((from: string, to: string) => Promise.all([api.journal.days(from, to), api.habits.stats(from, to)]))

  function load(s: Span) {
    error = null
    void fetchAll(addDays(now, -(s - 1)), now)
      .then((r) => !r.stale && (data = { days: r.value[0].days, habits: r.value[1] }))
      .catch((e) => (error = errorText(e)))
  }
  $effect(() => load(span))

  function setSpan(s: Span) {
    span = s
    try {
      localStorage.setItem(SPAN_KEY, String(s))
    } catch {
      // не запомнили — не страшно
    }
  }

  const summary = $derived(data ? summarize(data.days, now) : null)
  const best = $derived(data ? bestDay(data.days) : null)
  const fmt = (n: number | null) => (n === null ? '—' : n.toLocaleString(i18n.locale, { maximumFractionDigits: 1 }))
</script>

<div class="page">
  <header>
    <h1>{t('journal.stats.title')}</h1>
    <Segmented
      label={t('journal.stats.title')}
      value={String(span) as '7' | '30'}
      options={[
        { value: '7', label: t('journal.stats.last7') },
        { value: '30', label: t('journal.stats.last30') },
      ]}
      onchange={(v) => setSpan(v === '30' ? 30 : 7)}
    />
  </header>

  {#if error && !data}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(span)}>{t('common.retry')}</Button></div>
  {:else if !data || !summary}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="scroll body">
      <div class="tiles">
        <Card padding="sm" class="tile">
          <span class="muted label">{t('journal.stats.filled')}</span>
          <span class="value">{summary.filled}<small class="muted"> / {span}</small></span>
        </Card>
        <Card padding="sm" class="tile">
          <span class="muted label">{t('journal.stats.avgRating')}</span>
          <span class="value">{fmt(summary.avgRating)}<small class="muted"> / 10</small></span>
        </Card>
        <Card padding="sm" class="tile">
          <span class="muted label">{t('journal.stats.avgDopamine')}</span>
          <span class="value">{fmt(summary.avgDopamine)}<small class="muted"> / 7</small></span>
        </Card>
        <Card padding="sm" class="tile">
          <span class="muted label">{t('journal.stats.bestDay')}</span>
          {#if best}
            <a class="value best" href="#/day/entry/{best.day}">{best.rating}<small class="muted"> · {longDay(best.day, i18n.locale)}</small></a>
          {:else}
            <span class="value">—</span>
          {/if}
        </Card>
      </div>

      <Card>
        <h2>{t('journal.stats.chart')}</h2>
        <DayChart days={data.days} />
      </Card>

      <Card>
        <h2>{t('journal.stats.habits')}</h2>
        {#if data.habits.length === 0}
          <p class="muted">{t('journal.stats.noHabits')}</p>
        {:else}
          <HabitStats stats={data.habits} />
        {/if}
      </Card>
    </div>
  {/if}
</div>

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
    align-items: center;
    justify-content: space-between;
    gap: var(--s-4);
  }
  h1 {
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
  }
  h2 {
    margin-bottom: var(--s-3);
    font-size: var(--fs-md);
  }
  .state {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
  }
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    padding: 0 var(--s-1) var(--s-4) 0;
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--s-3);
  }
  .tiles :global(.tile) {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .label {
    font-size: var(--fs-sm);
  }
  .value {
    font-size: var(--fs-xl);
    font-weight: 800;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .value small {
    font-size: var(--fs-sm);
    font-weight: 600;
  }
  .best {
    color: inherit;
    text-decoration: none;
  }
  .best:hover small {
    text-decoration: underline;
  }

  @media (max-width: 860px) {
    .tiles {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
