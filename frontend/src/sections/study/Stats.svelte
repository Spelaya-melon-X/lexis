<script lang="ts">
  import { api } from '../../lib/api/client'
  import { studyLevel, type StudyLevel } from '../../lib/study'
  import Practice from './Practice.svelte'
  import Markdown from '../../lib/components/Markdown.svelte'
  import type {
    StudyKnowledge,
    StudyPracticeCard,
    StudyStats,
  } from '../../lib/api/types'
  import { latest } from '../../lib/async'
  import { bars, slotCenter } from '../../lib/chart'
  import { weekday, intensity } from '../../lib/heatmap'
  import { shortDay, longDay } from '../../lib/dates'
  import { t, errorText, i18n } from '../../lib/i18n/i18n.svelte'
  import Button from '../../lib/components/Button.svelte'
  import Card from '../../lib/components/Card.svelte'
  import Spinner from '../../lib/components/Spinner.svelte'
  import Segmented from '../../lib/components/Segmented.svelte'

  let range = $state('30'),
    data = $state<StudyStats | null>(null),
    loading = $state(true),
    failure = $state(''),
    selected = $state<string | null>(null)
  let knowledge = $state<StudyKnowledge[]>([]),
    session = $state<StudyPracticeCard[] | null>(null),
    starting = $state(false)
  const fetchStats = latest(async (days: number) => {
    const [stats, knowledge] = await Promise.all([
      api.study.stats(days),
      api.study.knowledge(),
    ])
    return { stats, knowledge }
  })
  const levels: StudyLevel[] = ['new', 'weak', 'learning', 'strong']
  const weakest = $derived(
    knowledge
      .filter((c) => c.total > 0)
      .slice()
      .sort((a, b) => a.score - b.score || a.cardId - b.cardId)
      .slice(0, 10),
  )
  async function nextBatch() {
    return (await api.study.practice({ groupId: null, cardIds: [], limit: 10 }))
      .cards
  }
  async function trainWeak() {
    starting = true
    try {
      session = await nextBatch()
    } catch (e) {
      failure = errorText(e)
    } finally {
      starting = false
    }
  }
  async function load(days: number) {
    loading = true
    failure = ''
    data = null
    selected = null
    try {
      const result = await fetchStats(days)
      if (!result.stale) {
        data = result.value.stats
        knowledge = result.value.knowledge
        loading = false
      }
    } catch (e) {
      failure = errorText(e)
      loading = false
    }
  }
  $effect(() => {
    void load(Number(range))
    return () => fetchStats.cancel()
  })
  const days = $derived(data?.days ?? [])
  const peak = $derived(Math.max(1, ...days.map((d) => d.total)))
  const box = $derived({
    width: Math.max(640, days.length * 10 + 64),
    height: 220,
    padX: 32,
    padY: 20,
  })
  const columns = $derived(
    bars(
      days.map((d) => d.total),
      box,
      0.3,
      peak,
    ),
  )
  const chosen = $derived(days.find((d) => d.day === selected))
  const labels = $derived(
    [
      ...new Set([0, Math.floor((days.length - 1) / 2), days.length - 1]),
    ].filter((i) => i >= 0),
  )
  const percent = $derived(
    data?.total ? Math.round((100 * data.known) / data.total) : null,
  )
</script>

{#if session}
  <Practice
    cards={session}
    reverse={false}
    onrestart={nextBatch}
    onclose={() => {
      session = null
      void load(Number(range))
    }}
  />
{:else}
  <div class="page">
    <div class="head">
      <div>
        <h1>{t('studyStats.title')}</h1>
        <p class="muted">{t('studyStats.subtitle')}</p>
      </div>
      <Segmented
        value={range}
        label={t('studyStats.period')}
        options={['7', '30', '90', '365'].map((value) => ({
          value,
          label: t('studyStats.days', { count: value }),
        }))}
        onchange={(value) => (range = value)}
      />
    </div>
    {#if loading}<Spinner />{:else if failure}<p role="alert">{failure}</p>
      <Button onclick={() => load(Number(range))}>{t('common.retry')}</Button
      >{:else if data}
      <div class="metrics">
        <Card
          ><span class="muted">{t('studyStats.reviewed')}</span><strong
            >{data.total}</strong
          ><small>{t('studyStats.attempts')}</small></Card
        >
        <Card
          ><span class="muted">{t('studyStats.remembered')}</span><strong
            >{percent === null ? '—' : `${percent}%`}</strong
          ><small
            >{t('studyStats.knownCount', {
              known: data.known,
              total: data.total,
            })}</small
          ></Card
        >
        <Card
          ><span class="muted">{t('studyStats.activeDays')}</span><strong
            >{data.activeDays}</strong
          ><small>{t('studyStats.inPeriod')}</small></Card
        >
        <Card
          ><span class="muted">{t('studyStats.streak')}</span><strong
            >{data.bestStreak}</strong
          ><small>{t('studyStats.streakHelp')}</small></Card
        >
      </div>
      <Card>
        <h2>{t('studyStats.knowledge')}</h2>
        <p class="muted">{t('studyStats.knowledgeHelp')}</p>
        <div class="levels">
          {#each levels as level}
            {@const count = knowledge.filter(
              (c) => studyLevel(c) === level,
            ).length}
            <div>
              <b>{t(`studyStats.levels.${level}`)}</b><strong>{count}</strong>
              <div class="level-track">
                <span
                  style:width="{knowledge.length
                    ? (100 * count) / knowledge.length
                    : 0}%"
                ></span>
              </div>
            </div>
          {/each}
        </div>
      </Card>
      <Card>
        <div class="head">
          <h2>{t('studyStats.weakest')}</h2>
          <Button
            disabled={starting || knowledge.length === 0}
            onclick={trainWeak}>{t('studyStats.practiceWeak')}</Button
          >
        </div>
        <p class="muted">{t('studyStats.scoreHelp')}</p>
        {#if !weakest.length}<p>{t('studyStats.noRanked')}</p>{/if}
        <div class="weakest">
          {#each weakest as c (c.cardId)}<article>
              <div class="head">
                <span>{c.groupTitle} · {t(`study.kinds.${c.kind}`)}</span><b
                  >{t('study.score', { score: c.score })}</b
                >
              </div>
              <Markdown source={c.front} /><small
                >{t('studyStats.cardAnswers', {
                  known: c.known,
                  total: c.total,
                  wrong: c.total - c.known,
                })}</small
              >
            </article>{/each}
        </div>
      </Card>
      {#if data.total === 0}<Card
          ><h2>{t('studyStats.empty')}</h2>
          <p class="muted">{t('studyStats.emptyHelp')}</p>
          <a href="#/study/cards">{t('study.backToCards')}</a></Card
        >{/if}
      <Card
        ><h2>{t('studyStats.chart')}</h2>
        <div class="chart">
          <svg
            viewBox="0 0 {box.width} {box.height + 30}"
            style:width="{box.width}px"
            role="img"
            aria-label={t('studyStats.chart')}
          >
            <line
              x1={box.padX}
              x2={box.width - box.padX}
              y1={box.height - box.padY}
              y2={box.height - box.padY}
            />
            <text x="0" y={box.padY + 5}>{peak}</text><text
              x="8"
              y={box.height - box.padY}>0</text
            >
            {#each columns as b}
              {@const day = days[b.index]!}
              {@const knownHeight = (b.h * day.known) / day.total}
              <g
                ><title
                  >{longDay(day.day, i18n.locale)}: {t(
                    'studyStats.daySummary',
                    {
                      total: day.total,
                      known: day.known,
                      again: day.total - day.known,
                    },
                  )}</title
                ><rect
                  class="again"
                  x={b.x}
                  y={b.y}
                  width={b.w}
                  height={b.h}
                  rx="2"
                /><rect
                  class="known"
                  x={b.x}
                  y={box.height - box.padY - knownHeight}
                  width={b.w}
                  height={knownHeight}
                /></g
              >
            {/each}
            {#each labels as i}{#if days[i]}<text
                  x={slotCenter(i, days.length, box)}
                  y={box.height + 18}
                  text-anchor="middle"
                  >{shortDay(days[i]!.day, i18n.locale)}</text
                >{/if}{/each}
          </svg>
        </div>
        <div class="legend">
          <span><i class="known"></i>{t('studyStats.known')}</span><span
            ><i class="again"></i>{t('studyStats.again')}</span
          >
        </div></Card
      >
      <Card
        ><h2>{t('studyStats.calendar')}</h2>
        <p class="muted">{t('studyStats.calendarHelp')}</p>
        <div class="calendar-wrap">
          <div
            class="calendar"
            role="group"
            aria-label={t('studyStats.calendar')}
          >
            {#each Array(days[0] ? weekday(days[0].day) : 0) as _}<span
              ></span>{/each}
            {#each days as day}<button
                style:--level={intensity(day.total, peak)}
                class:active={selected === day.day}
                aria-pressed={selected === day.day}
                aria-label="{longDay(day.day, i18n.locale)}: {t(
                  'studyStats.daySummary',
                  {
                    total: day.total,
                    known: day.known,
                    again: day.total - day.known,
                  },
                )}"
                title="{longDay(day.day, i18n.locale)}: {day.total}"
                onclick={() => (selected = day.day)}
              ></button>{/each}
          </div>
        </div>
        {#if chosen}<p aria-live="polite">
            <b>{longDay(chosen.day, i18n.locale)}</b> · {t(
              'studyStats.daySummary',
              {
                total: chosen.total,
                known: chosen.known,
                again: chosen.total - chosen.known,
              },
            )}
          </p>{/if}
        <details>
          <summary>{t('studyStats.byDay')}</summary>
          <div class="table-wrap">
            <table>
              <thead
                ><tr
                  ><th>{t('studyStats.date')}</th><th
                    >{t('studyStats.reviewed')}</th
                  ><th>{t('studyStats.known')}</th><th
                    >{t('studyStats.again')}</th
                  ></tr
                ></thead
              ><tbody
                >{#each days
                  .filter((d) => d.total > 0)
                  .slice()
                  .reverse() as day}<tr
                    ><th>{shortDay(day.day, i18n.locale)}</th><td
                      >{day.total}</td
                    ><td>{day.known}</td><td>{day.total - day.known}</td></tr
                  >{/each}</tbody
              >
            </table>
          </div>
        </details>
      </Card>
      <p class="muted">{t('studyStats.countHelp')}</p>
    {/if}
  </div>
{/if}

<style>
  .levels {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--s-4);
    margin-top: var(--s-4);
  }
  .levels > div {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .level-track {
    height: 12px;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    overflow: hidden;
    background: var(--c-surface-2);
  }
  .level-track span {
    display: block;
    height: 100%;
    background: var(--c-green);
  }
  .weakest {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(280px, 100%), 1fr));
    gap: var(--s-3);
    margin-top: var(--s-4);
  }
  .weakest article {
    min-width: 0;
    padding: var(--s-3);
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
  }

  .page {
    display: flex;
    flex-direction: column;
    gap: var(--s-5);
    overflow-y: auto;
    min-width: 0;
    padding-bottom: var(--s-7);
  }
  .page > :global(*) {
    flex-shrink: 0;
  }
  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--s-3);
  }
  h1 {
    font-size: var(--fs-xl);
  }
  h2 {
    font-size: var(--fs-lg);
    margin-bottom: var(--s-3);
  }
  p {
    margin-top: var(--s-2);
  }
  .metrics {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--s-4);
  }
  .metrics :global(.card) {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  strong {
    font-size: var(--fs-2xl);
  }
  small {
    color: var(--c-muted);
  }
  .chart,
  .calendar-wrap,
  .table-wrap {
    overflow-x: auto;
    max-width: 100%;
  }
  .chart svg {
    display: block;
    min-width: 100%;
    height: auto;
  }
  .chart line {
    stroke: var(--c-line);
  }
  .chart text {
    font-size: 12px;
    fill: var(--c-muted);
  }
  .known {
    fill: var(--c-green);
    background: var(--c-green);
  }
  .again {
    fill: var(--c-amber);
    background: var(--c-amber);
  }
  .legend {
    display: flex;
    gap: var(--s-4);
    justify-content: flex-end;
    font-size: var(--fs-sm);
    margin-top: var(--s-2);
  }
  .legend span {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  .legend i {
    width: 12px;
    height: 12px;
    border: 1px solid var(--c-border);
    border-radius: 2px;
  }
  .calendar {
    display: grid;
    grid-template-rows: repeat(7, 16px);
    grid-auto-flow: column;
    grid-auto-columns: 16px;
    gap: 4px;
    width: max-content;
    padding: var(--s-2);
    margin: var(--s-3) 0;
  }
  .calendar button {
    background: color-mix(
      in srgb,
      var(--c-green) calc(var(--level) * 25%),
      var(--c-surface-2)
    );
    border: 1px solid var(--c-line);
    border-radius: var(--radius-sm);
    cursor: pointer;
  }
  .calendar button.active {
    outline: 2px solid var(--c-text);
    outline-offset: 1px;
  }
  summary {
    cursor: pointer;
    margin-top: var(--s-4);
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--fs-sm);
    margin-top: var(--s-3);
  }
  th,
  td {
    text-align: left;
    padding: var(--s-2);
    border-bottom: var(--bw) solid var(--c-line);
  }
  .table-wrap {
    max-height: 320px;
  }
  a {
    color: inherit;
    text-decoration: underline;
  }
  @media (max-width: 850px) {
    .metrics {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  @media (max-width: 420px) {
    .metrics {
      gap: var(--s-2);
    }
  }
</style>
