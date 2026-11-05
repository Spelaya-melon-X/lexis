<script lang="ts">
  // Календарь месяца (§17.2): #/day/calendar[/YYYY-MM]. Клетка — цвет оценки и полоска
  // дофамина; клик — открыть день.
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { DaySummary } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { addMonths, isMonth, monthGrid, monthOf, monthTitle, today } from '../../../lib/dates'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { DOPAMINE_SLOTS, isFilled, ratingTone, summarize } from '../../../lib/journal'
  import { route } from '../../../lib/state/route.svelte'

  const now = today()
  const month = $derived(isMonth(route.rest) ? route.rest : monthOf(now))
  const grid = $derived(monthGrid(month))
  const weekdays = $derived(t('journal.weekdays').split(','))

  let days = $state<Map<string, DaySummary> | null>(null)
  let error = $state<string | null>(null)
  const fetchDays = latest(api.journal.days)

  function load(m: string) {
    error = null
    const all = monthGrid(m).flat().filter((d) => d !== null)
    void fetchDays(all[0]!, all.at(-1)!)
      .then((r) => !r.stale && (days = new Map(r.value.days.map((d) => [d.day, d]))))
      .catch((e) => (error = errorText(e)))
  }
  $effect(() => load(month))

  const summary = $derived(days ? summarize([...days.values()], now) : null)
  const fmt = (n: number | null) => (n === null ? '—' : n.toLocaleString(i18n.locale, { maximumFractionDigits: 1 }))

  function go(m: string) {
    route.go('day', 'calendar', m === monthOf(now) ? '' : m)
  }
</script>

<div class="page">
  <header>
    <button class="arrow" aria-label={t('journal.prevMonth')} title={t('journal.prevMonth')} onclick={() => go(addMonths(month, -1))}>
      <Icon name="chevronLeft" />
    </button>
    <h1>{monthTitle(month, i18n.locale)}</h1>
    <button
      class="arrow"
      aria-label={t('journal.nextMonth')}
      title={t('journal.nextMonth')}
      disabled={month >= monthOf(now)}
      onclick={() => go(addMonths(month, 1))}
    >
      <Icon name="chevronRight" />
    </button>
    {#if month !== monthOf(now)}
      <Button size="sm" variant="ghost" icon="calendar" onclick={() => go(monthOf(now))}>{t('journal.thisMonth')}</Button>
    {/if}
  </header>

  {#if error && !days}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(month)}>{t('common.retry')}</Button></div>
  {:else if !days || !summary}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="scroll body">
      <div class="tiles">
        <Card padding="sm" class="tile">
          <span class="muted label">{t('journal.filledDays')}</span>
          <span class="value">{summary.filled}</span>
        </Card>
        <Card padding="sm" class="tile">
          <span class="muted label">{t('journal.avgRating')}</span>
          <span class="value">{fmt(summary.avgRating)}<small class="muted"> / 10</small></span>
        </Card>
        <Card padding="sm" class="tile">
          <span class="muted label">{t('journal.avgDopamine')}</span>
          <span class="value">{fmt(summary.avgDopamine)}<small class="muted"> / 7</small></span>
        </Card>
      </div>

      <div class="cal" role="grid" aria-label={monthTitle(month, i18n.locale)}>
        <div class="row head" role="row">
          {#each weekdays as w (w)}<span role="columnheader">{w}</span>{/each}
        </div>
        {#each grid as week, wi (wi)}
          <div class="row" role="row">
            {#each week as d, di (di)}
              {#if d === null}
                <span class="cell out" role="gridcell"></span>
              {:else}
                {@const s = days.get(d)}
                {@const num = Number(d.slice(8))}
                {#if d > now}
                  <span class="cell future" role="gridcell"><span class="num">{num}</span></span>
                {:else}
                  <a
                    class="cell tone-{ratingTone(s?.rating ?? null)}"
                    class:today={d === now}
                    class:filled={s && isFilled(s)}
                    role="gridcell"
                    href="#/day/entry/{d}"
                  >
                    <span class="num">{num}</span>
                    {#if s?.rating != null}<span class="rate">{s.rating}</span>{/if}
                    {#if s && s.dopamineLevel > 0}
                      <span class="pips" title="{t('journal.dopamine')}: {s.dopamineLevel}/{DOPAMINE_SLOTS}">
                        {#each { length: DOPAMINE_SLOTS } as _, i (i)}<i class:on={i < s.dopamineLevel}></i>{/each}
                      </span>
                    {/if}
                  </a>
                {/if}
              {/if}
            {/each}
          </div>
        {/each}
      </div>
      <p class="muted hint">{t('journal.calendarHint')}</p>
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
    gap: var(--s-2);
  }
  h1 {
    min-width: 11ch;
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
    text-align: center;
  }
  h1::first-letter {
    text-transform: uppercase;
  }
  .arrow {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    cursor: pointer;
  }
  .arrow:disabled {
    opacity: 0.35;
    cursor: default;
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
    grid-template-columns: repeat(3, 1fr);
    gap: var(--s-3);
  }
  .tiles :global(.tile) {
    display: flex;
    flex-direction: column;
  }
  .label {
    font-size: var(--fs-sm);
  }
  .value {
    font-size: var(--fs-xl);
    font-weight: 800;
  }
  .value small {
    font-size: var(--fs-sm);
    font-weight: 600;
  }

  .cal {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  .row {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: var(--s-1);
  }
  .head span {
    text-align: center;
    font-size: var(--fs-xs);
    font-weight: 700;
    color: var(--c-muted);
    text-transform: uppercase;
  }
  .cell {
    position: relative;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    min-height: 72px;
    padding: var(--s-1) var(--s-2);
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    color: inherit;
    text-decoration: none;
    transition:
      transform var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out);
  }
  a.cell:hover {
    transform: translate(-1px, -1px);
    border-color: var(--c-border);
    box-shadow: var(--shadow-sm);
  }
  .filled {
    border-color: var(--c-border);
  }
  .today {
    outline: var(--bw) solid var(--c-focus);
    outline-offset: 1px;
  }
  .out {
    border-color: transparent;
    background: none;
  }
  .future {
    opacity: 0.4;
  }
  .tone-red {
    background: var(--c-red-soft);
  }
  .tone-amber {
    background: var(--c-amber-soft);
  }
  .tone-green {
    background: var(--c-green-soft);
  }
  .num {
    font-weight: 700;
    font-size: var(--fs-sm);
  }
  .rate {
    position: absolute;
    top: var(--s-1);
    right: var(--s-2);
    font-weight: 800;
  }
  .pips {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 2px;
  }
  .pips i {
    height: 6px;
    border-radius: 1px;
    background: color-mix(in srgb, var(--c-text) 12%, transparent);
  }
  .pips i.on {
    background: var(--c-green);
  }
  .hint {
    font-size: var(--fs-sm);
  }

  @media (max-width: 860px) {
    .cell {
      min-height: 56px;
    }
    .rate {
      position: static;
    }
  }
</style>
