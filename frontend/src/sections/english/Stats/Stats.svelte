<script lang="ts">
  import { onMount } from 'svelte'
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { Achievement, Calendar, Overview, Range, Trends } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import LevelBars from '../../../lib/components/LevelBars.svelte'
  import Segmented from '../../../lib/components/Segmented.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { formatDuration } from '../../../lib/dates'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import Achievements from './Achievements.svelte'
  import DayPanel from './DayPanel.svelte'
  import Heatmap from './Heatmap.svelte'
  import TrendsChart from './TrendsChart.svelte'

  let today = $state<Overview | null>(null) // плитки — всегда про сегодня
  let calendar = $state<Calendar | null>(null)
  let achievements = $state<Achievement[]>([])
  let error = $state<string | null>(null)

  let selected = $state<string | null>(null) // день из календаря; null — сегодня
  let dayView = $state<Overview | null>(null)
  let range = $state<Range>('week')
  let trends = $state<Trends | null>(null)

  const fetchDay = latest(api.stats.overview)
  const fetchTrends = latest(api.stats.trends)

  async function load() {
    try {
      const [o, c, a] = await Promise.all([api.stats.overview(), api.stats.calendar(), api.achievements.list()])
      today = o
      calendar = c
      achievements = a
      error = null
    } catch (e) {
      error = errorText(e)
    }
  }
  onMount(load)

  // Выбранный день — отдельный запрос; быстрые клики по календарю не путают ответы (latest).
  $effect(() => {
    const day = selected
    if (!day) {
      fetchDay.cancel()
      dayView = null
      return
    }
    void fetchDay(day).then((r) => !r.stale && (dayView = r.value)).catch((e) => (error = errorText(e)))
  })

  $effect(() => {
    void fetchTrends(range).then((r) => !r.stale && (trends = r.value)).catch((e) => (error = errorText(e)))
  })

  const shownDay = $derived(selected ? dayView : today)
</script>

<div class="stats scroll">
  {#if error && !today}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={load}>{t('common.retry')}</Button></div>
  {:else if !today || !calendar}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="tiles">
      <Card padding="sm" class="tile streak">
        <span class="label"><Icon name="flame" size={16} /> {t('stats.streak')}</span>
        <span class="value">{t('stats.streakDays', { count: today.streak.current })}</span>
        <span class="sub muted">{t('stats.best', { count: today.streak.best })}</span>
      </Card>
      <Card padding="sm" class="tile">
        <span class="label muted">{t('stats.wordsToday')}</span>
        <span class="value">{today.day.words}</span>
        <span class="sub muted">{t('stats.today')}</span>
      </Card>
      <Card padding="sm" class="tile">
        <span class="label muted">{t('stats.timeToday')}</span>
        <span class="value">{formatDuration(today.day.seconds)}</span>
        <span class="sub muted">{t('stats.groupsToday')}: {today.groupsTrained}</span>
      </Card>
      <Card padding="sm" class="tile">
        <span class="label muted">{t('stats.learned')}</span>
        <span class="value">{today.totals.learned}<small class="muted"> / {today.totals.words}</small></span>
        <span class="sub muted">{t('stats.learnedHint')}</span>
      </Card>
    </div>

    <Card>
      <div class="card-head">
        <h3>{t('stats.calendar')}</h3>
        <span class="muted hint">{t('stats.calendarHint')}</span>
      </div>
      <Heatmap days={calendar.days} {selected} onselect={(d) => (selected = d === selected ? null : d)} />
    </Card>

    <div class="row">
      <Card>
        {#if shownDay}<DayPanel overview={shownDay} />{:else}<Spinner />{/if}
      </Card>
      <Card>
        <div class="card-head">
          <h3>{t('stats.trends')}</h3>
          <Segmented
            label={t('stats.trends')}
            value={range}
            options={[{ value: 'week', label: t('stats.week') }, { value: 'month', label: t('stats.month') }]}
            onchange={(r) => (range = r)}
          />
        </div>
        {#if trends}<TrendsChart days={trends.days} />{:else}<Spinner />{/if}
      </Card>
    </div>

    <div class="row">
      <Card>
        <h3>{t('stats.levels')}</h3>
        <LevelBars levels={today.totals.levels} />
      </Card>
      <Card>
        <h3>{t('stats.achievements')}</h3>
        <Achievements items={achievements} />
      </Card>
    </div>
  {/if}
</div>

<style>
  .stats {
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    padding: 2px 6px 6px 2px;
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: var(--s-3);
  }
  :global(.tile) {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  :global(.tile.streak) {
    background: var(--c-amber-soft);
  }
  .label {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: var(--fs-xs);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .value {
    font-size: var(--fs-xl);
    font-weight: 800;
    font-variant-numeric: tabular-nums;
  }
  .value small {
    font-size: var(--fs-md);
    font-weight: 600;
  }
  .sub {
    font-size: var(--fs-xs);
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-3);
  }
  h3 {
    font-size: var(--fs-md);
    margin-bottom: var(--s-3);
  }
  .card-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--s-2);
    margin-bottom: var(--s-3);
  }
  .card-head h3 {
    margin: 0;
  }
  .hint {
    font-size: var(--fs-xs);
  }
  .state {
    height: 100%;
    display: grid;
    place-items: center;
    align-content: center;
    gap: var(--s-3);
  }
  @media (max-width: 860px) {
    .tiles {
      grid-template-columns: repeat(2, 1fr);
    }
    .row {
      grid-template-columns: 1fr;
    }
  }
</style>
