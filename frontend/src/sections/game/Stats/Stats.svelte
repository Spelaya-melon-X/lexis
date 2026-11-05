<script lang="ts">
  // Статистика «Игры» (§18.3, B5): за сегодня / 7 / 30 дней — время в фокусе, выполненные дела, сессии,
  // отвлечения и топ триггеров; рекорды за всё время и ачивки игры (game_*).
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { Achievement, FocusStats } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import Segmented from '../../../lib/components/Segmented.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { bars, type Box } from '../../../lib/chart'
  import { addDays, longDay, shortDay, today } from '../../../lib/dates'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import Achievements from '../../english/Stats/Achievements.svelte'
  import { blocksTextSec } from '../format'

  type Span = 'day' | 'week' | 'month'
  const SPAN_KEY = 'lexis.gameStatsSpan'
  const DAYS: Record<Span, number> = { day: 1, week: 7, month: 30 }
  const BOX: Box = { width: 640, height: 160, padX: 8, padY: 12 }

  function storedSpan(): Span {
    try {
      const v = localStorage.getItem(SPAN_KEY)
      return v === 'day' || v === 'month' ? v : 'week'
    } catch {
      return 'week'
    }
  }

  const now = today()
  let span = $state<Span>(storedSpan())
  let data = $state<FocusStats | null>(null)
  let achievements = $state<Achievement[]>([])
  let error = $state<string | null>(null)

  const fetchStats = latest(api.focus.stats)

  function load(s: Span) {
    error = null
    void fetchStats(addDays(now, -(DAYS[s] - 1)), now)
      .then((r) => !r.stale && (data = r.value))
      .catch((e: unknown) => (error = errorText(e)))
    api.achievements
      .list()
      .then((a) => (achievements = a.filter((x) => x.code.startsWith('game_'))))
      .catch(() => {}) // ачивки — вспомогательный блок
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

  const chart = $derived(data && span !== 'day' ? bars(data.days.map((d) => d.workSec), BOX, 0.3) : [])
  const date = (d: string) => shortDay(d, i18n.locale)
</script>

<div class="page">
  <header>
    <h1>{t('nav.game.stats')}</h1>
    <Segmented
      label={t('nav.game.stats')}
      value={span}
      options={[
        { value: 'day', label: t('game.stats.today') },
        { value: 'week', label: t('game.stats.week') },
        { value: 'month', label: t('game.stats.month') },
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
        <Card padding="sm" class="tile"><span class="muted label">{t('game.stats.inFocus')}</span><span class="value">{blocksTextSec(tot.workSec)}</span></Card>
        <Card padding="sm" class="tile"><span class="muted label">{t('game.stats.tasksDone')}</span><span class="value">{tot.tasksDone}</span></Card>
        <Card padding="sm" class="tile"><span class="muted label">{t('game.stats.sessions')}</span><span class="value">{tot.sessions}</span></Card>
        <Card padding="sm" class="tile">
          <span class="muted label">{t('game.stats.distractions')}</span>
          <span class="value">{tot.distractions}</span>
          {#if tot.distractions > 0}
            <small class="muted">{t('game.stats.gaveResisted', { gave: tot.gaveIn, resisted: tot.distractions - tot.gaveIn })}</small>
          {/if}
        </Card>
        <Card padding="sm" class="tile">
          <span class="muted label">{t('game.stats.overhead')}</span>
          <span class="value small">{blocksTextSec(tot.setupSec)} / {blocksTextSec(tot.breakSec)}</span>
          <small class="muted">{t('game.stats.overheadHint')}</small>
        </Card>
      </div>

      {#if span !== 'day'}
        <Card>
          <h2>{t('game.stats.chart')}</h2>
          {#if tot.workSec === 0}
            <p class="muted">{t('game.stats.noWork')}</p>
          {:else}
            <svg viewBox="0 0 {BOX.width} {BOX.height}" class="chart" role="img" aria-label={t('game.stats.chart')}>
              {#each chart as b (b.index)}
                {@const d = data.days[b.index]!}
                <rect x={b.x} y={b.y} width={b.w} height={b.h} rx="2"><title>{date(d.day)}: {blocksTextSec(d.workSec)}</title></rect>
              {/each}
            </svg>
            <div class="axis muted"><span>{date(data.from)}</span><span>{date(data.to)}</span></div>
          {/if}
        </Card>
      {/if}

      <div class="cols">
        <Card>
          <h2>{t('game.stats.doneList')}</h2>
          {#if data.tasksDone.length === 0}
            <p class="muted">{t('game.stats.noneDone')}</p>
          {:else}
            <ul class="list">
              {#each [...data.tasksDone].reverse() as x (x.id)}
                <li><span>✓ {x.title}</span>{#if span !== 'day'}<span class="muted">{date(x.day)}</span>{/if}</li>
              {/each}
            </ul>
          {/if}
        </Card>

        <Card>
          <h2>{t('game.stats.triggers')}</h2>
          {#if data.triggers.length === 0}
            <p class="muted">{t('game.stats.noTriggers')}</p>
          {:else}
            <ul class="list">
              {#each data.triggers as x (x.trigger)}
                <li>
                  <span>{x.trigger}</span>
                  <span class="share" title={t('game.stats.gaveResisted', { gave: x.gaveIn, resisted: x.total - x.gaveIn })}>
                    <span class="gave" style:width="{(x.gaveIn / x.total) * 100}%"></span>
                  </span>
                  <span class="muted num">{x.gaveIn}/{x.total}</span>
                </li>
              {/each}
            </ul>
            <p class="muted legend">{t('game.stats.triggersHint')}</p>
          {/if}
        </Card>
      </div>

      {#if data.sessions.length > 0}
        <Card>
          <h2>{t('game.stats.sessionsList')}</h2>
          <ul class="list">
            {#each [...data.sessions].reverse() as x (x.id)}
              <li>
                <span>{x.taskTitle}</span>
                <span class="muted">
                  {t(`game.focus.outcome.${x.outcome}`)} · {blocksTextSec(x.workSec)}{x.distractions ? ` · ${t('game.stats.distrShort', { n: x.distractions })}` : ''}{span !== 'day' ? ` · ${date(x.day)}` : ''}
                </span>
              </li>
            {/each}
          </ul>
        </Card>
      {/if}

      <Card>
        <h2>{t('game.stats.records')}</h2>
        <div class="records">
          <div><span class="muted">{t('game.stats.totalFocus')}</span><strong>{blocksTextSec(data.records.totalWorkSec)}</strong></div>
          <div><span class="muted">{t('game.stats.totalTasks')}</span><strong>{data.records.totalTasks}</strong></div>
          <div>
            <span class="muted">{t('game.stats.bestDay')}</span>
            <strong>{data.records.bestDay ? blocksTextSec(data.records.bestDay.workSec) : '—'}</strong>
            {#if data.records.bestDay}<small class="muted">{longDay(data.records.bestDay.day, i18n.locale)}</small>{/if}
          </div>
          <div>
            <span class="muted">{t('game.stats.bestWeek')}</span>
            <strong>{data.records.bestWeek ? blocksTextSec(data.records.bestWeek.workSec) : '—'}</strong>
            {#if data.records.bestWeek}<small class="muted">{t('game.stats.weekOf', { date: date(data.records.bestWeek.day) })}</small>{/if}
          </div>
        </div>
      </Card>

      {#if achievements.length > 0}
        <Card>
          <h2>{t('stats.achievements')}</h2>
          <Achievements items={achievements} />
        </Card>
      {/if}
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
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s-3) var(--s-4);
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
  .value.small {
    font-size: var(--fs-lg);
  }
  small {
    font-size: var(--fs-xs);
  }
  .chart {
    display: block;
    width: 100%;
    height: auto;
  }
  .chart rect {
    fill: var(--c-green);
  }
  .axis {
    display: flex;
    justify-content: space-between;
    font-size: var(--fs-xs);
  }
  .cols {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    gap: var(--s-4);
  }
  .list {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  .list li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-3);
    padding: var(--s-1) 0;
    border-bottom: 1px solid var(--c-line);
    font-size: var(--fs-sm);
  }
  .list li > span:first-child {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .share {
    flex: none;
    width: 80px;
    height: 8px;
    border-radius: 999px;
    background: var(--c-green-soft);
    overflow: hidden;
  }
  .gave {
    display: block;
    height: 100%;
    background: var(--c-red);
  }
  .num {
    min-width: 36px;
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .legend {
    margin: var(--s-2) 0 0;
    font-size: var(--fs-xs);
  }
  .records {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--s-3);
  }
  .records div {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .records span {
    font-size: var(--fs-xs);
  }
  .records strong {
    font-size: var(--fs-lg);
  }
</style>
