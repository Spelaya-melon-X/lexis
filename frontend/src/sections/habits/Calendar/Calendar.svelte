<script lang="ts">
  // Календарь привычек (§17.4): строки — привычки, колонки — дни; цели — полосы над строкой.
  // #/habits/calendar[/<понедельник YYYY-MM-DD> | /<YYYY-MM>]: день — неделя, месяц — месяц.
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { Checkin, Goal, Habit } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import Segmented from '../../../lib/components/Segmented.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { addDays, addMonths, isDay, isMonth, monthOf, monthTitle, parseDay, shortDay, today } from '../../../lib/dates'
  import { cellState, daysRange, formatAmount, goalSpans, weekStart } from '../../../lib/habits'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { route } from '../../../lib/state/route.svelte'

  type Mode = 'week' | 'month'

  const now = today()
  const mode = $derived<Mode>(isMonth(route.rest) ? 'month' : 'week')
  /** Начало окна: понедельник недели или 1-е число месяца. */
  const anchor = $derived(
    mode === 'month' ? `${route.rest}-01` : weekStart(isDay(route.rest) && route.rest <= now ? route.rest : now),
  )
  const days = $derived(
    mode === 'month' ? daysRange(anchor, addDays(addMonths(monthOf(anchor), 1) + '-01', -1)) : daysRange(anchor, addDays(anchor, 6)),
  )
  const isCurrent = $derived(days.includes(now))

  let data = $state<{ habits: Habit[]; checkins: Map<string, Checkin>; goals: Goal[] } | null>(null)
  let error = $state<string | null>(null)

  const fetchAll = latest((from: string, to: string) =>
    Promise.all([api.habits.list(true), api.habits.checkins(from, to), api.habits.goals(from, to)]),
  )

  function load(from: string, to: string) {
    error = null
    void fetchAll(from, to)
      .then((r) => {
        if (r.stale) return
        const [hs, cs, gs] = r.value
        const checkins = new Map(cs.map((c) => [`${c.habitId}/${c.day}`, c]))
        // Архивная — только если в окне у неё есть отметки (история не пропадает из вида).
        const habits = hs.filter((h) => !h.archived || cs.some((c) => c.habitId === h.id))
        data = { habits, checkins, goals: gs }
      })
      .catch((e) => (error = errorText(e)))
  }
  $effect(() => load(days[0]!, days.at(-1)!))

  const generalGoals = $derived(goalSpans((data?.goals ?? []).filter((g) => g.habitId === null), days))
  const goalsOf = (id: number) => goalSpans((data?.goals ?? []).filter((g) => g.habitId === id), days)

  function go(m: Mode, a: string) {
    const isNow = m === 'month' ? a === monthOf(now) : a === weekStart(now)
    // Текущая неделя — без хвоста в адресе; месяц — всегда с ним (так режим виден из адреса).
    route.go('habits', 'calendar', m === 'week' && isNow ? '' : a)
  }
  function step(dir: -1 | 1) {
    if (mode === 'month') go('month', addMonths(monthOf(anchor), dir))
    else go('week', addDays(anchor, 7 * dir))
  }
  function setMode(m: Mode) {
    if (m === mode) return
    if (m === 'month') go('month', monthOf(isCurrent ? now : anchor))
    else go('week', weekStart(isCurrent ? now : anchor))
  }

  const title = $derived(
    mode === 'month' ? monthTitle(monthOf(anchor), i18n.locale) : `${shortDay(days[0]!, i18n.locale)} – ${shortDay(days[6]!, i18n.locale)}`,
  )
  const weekday = (d: string) => parseDay(d).toLocaleDateString(i18n.locale, { weekday: 'short' })

  function cellTitle(h: Habit, day: string, c: Checkin | undefined): string {
    const parts = [`${h.name} · ${shortDay(day, i18n.locale)}`]
    if (c) {
      parts.push(t(h.kind === 'negative' ? 'habits.slip' : 'habits.done'))
      if (c.amount !== null) parts.push(`${formatAmount(c.amount, i18n.locale)} ${h.unit}`.trim())
      if (c.rating !== null) parts.push(`${t('habits.rating')}: ${c.rating}`)
      if (c.trigger) parts.push(`${t('habits.trigger')}: ${c.trigger}`)
    }
    return parts.join('\n')
  }
  /** Подпись полосы: текст цели, иначе прогресс («4 из 9 дн.» / «без срыва 4 из 9 дн.»). */
  function goalLabel(g: Goal, h?: Habit): string {
    if (g.title || !g.progress) return g.title
    const key = h?.kind === 'negative' ? 'habits.goalProgressNegative' : 'habits.goalProgressPositive'
    return t(key, { done: g.progress.done, total: g.progress.total })
  }
</script>

<div class="page">
  <header>
    <button class="arrow" aria-label={t('habits.prevPeriod')} title={t('habits.prevPeriod')} onclick={() => step(-1)}><Icon name="chevronLeft" /></button>
    <h1>{title}</h1>
    <button class="arrow" aria-label={t('habits.nextPeriod')} title={t('habits.nextPeriod')} disabled={days.at(-1)! >= now} onclick={() => step(1)}>
      <Icon name="chevronRight" />
    </button>
    {#if !isCurrent}
      <Button size="sm" variant="ghost" icon="calendar" onclick={() => go(mode, mode === 'month' ? monthOf(now) : weekStart(now))}>{t('habits.thisPeriod')}</Button>
    {/if}
    <span class="spacer"></span>
    <Segmented
      label={t('nav.habits.calendar')}
      value={mode}
      options={[
        { value: 'week', label: t('habits.week') },
        { value: 'month', label: t('habits.month') },
      ]}
      onchange={setMode}
    />
  </header>

  {#if error && !data}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(days[0]!, days.at(-1)!)}>{t('common.retry')}</Button></div>
  {:else if !data}
    <div class="state"><Spinner size={28} /></div>
  {:else if data.habits.length === 0}
    <div class="state">
      <p>{t('habits.empty')}</p>
      <Button variant="primary" icon="plus" onclick={() => route.go('habits', 'manage')}>{t('habits.goManage')}</Button>
    </div>
  {:else}
    <div class="scroll body">
      <div class="grid {mode}" style:--cols={days.length}>
        <!-- шапка: дни -->
        <span class="corner"></span>
        {#each days as d (d)}
          <a class="dayhead" class:today={d === now} class:future={d > now} href={d > now ? undefined : `#/habits/check/${d}`}>
            {#if mode === 'week'}<small>{weekday(d)}</small>{/if}
            <span>{Number(d.slice(8))}</span>
          </a>
        {/each}

        <!-- общие цели -->
        {#each generalGoals as s (s.goal.id)}
          <span class="label muted small">{t('habits.generalGoals')}</span>
          <span class="lane" style:grid-column="{s.start + 2} / {s.end + 3}" title="{s.goal.title} · {s.goal.from} – {s.goal.to}">
            <span class="bar general">{s.goal.title}</span>
          </span>
        {/each}

        {#each data.habits as h (h.id)}
          {#each goalsOf(h.id) as s (s.goal.id)}
            <span class="label"></span>
            <span class="lane" style:grid-column="{s.start + 2} / {s.end + 3}" title="{goalLabel(s.goal, h)} · {s.goal.from} – {s.goal.to}">
              <span class="bar" style:--goal={h.color ?? 'var(--c-text)'}>{mode === 'week' ? goalLabel(s.goal, h) : ''}</span>
            </span>
          {/each}

          <span class="label name" class:archived={h.archived}>
            <span class="dot" style:background={h.color ?? 'transparent'}></span>
            {h.name}
          </span>
          {#each days as d (d)}
            {@const c = data.checkins.get(`${h.id}/${d}`)}
            {@const st = cellState(h.kind, h.startDay, d, c !== undefined, now)}
            <span class="cell {st}" class:today={d === now} title={cellTitle(h, d, c)}>
              {#if mode === 'week' && c}
                {#if c.amount !== null}<b>{formatAmount(c.amount, i18n.locale)}</b>{#if h.unit}<small>{h.unit}</small>{/if}{/if}
                {#if c.rating !== null}<small>★{c.rating}</small>{/if}
                {#if c.trigger}<small class="trig">{c.trigger}</small>{/if}
              {/if}
            </span>
          {/each}
        {/each}
      </div>

      <div class="legend muted">
        <span><i class="cell done"></i>{t('habits.legendDone')}</span>
        <span><i class="cell slip"></i>{t('habits.legendSlip')}</span>
        <span><i class="cell clean"></i>{t('habits.legendClean')}</span>
        <span><i class="bar sample"></i>{t('habits.legendGoal')}</span>
      </div>
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
    min-width: 12ch;
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
    text-align: center;
  }
  h1::first-letter {
    text-transform: uppercase;
  }
  .spacer {
    flex: 1;
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

  .grid {
    display: grid;
    grid-template-columns: minmax(120px, 180px) repeat(var(--cols), minmax(0, 1fr));
    gap: 3px;
    align-items: stretch;
  }
  .corner {
    grid-column: 1;
  }
  .dayhead {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 2px 0;
    border-radius: var(--radius-sm);
    color: inherit;
    font-size: var(--fs-sm);
    font-weight: 700;
    text-decoration: none;
  }
  .dayhead small {
    font-size: var(--fs-xs);
    font-weight: 600;
    color: var(--c-muted);
    text-transform: capitalize;
  }
  a.dayhead:hover {
    background: var(--c-surface-2);
  }
  .dayhead.today {
    background: var(--c-text);
    color: var(--c-bg);
  }
  .dayhead.today small {
    color: inherit;
  }
  .dayhead.future {
    opacity: 0.4;
  }
  .month .dayhead {
    font-size: var(--fs-xs);
  }

  .label {
    grid-column: 1;
    display: flex;
    align-items: center;
    gap: var(--s-2);
    min-width: 0;
    font-size: var(--fs-sm);
  }
  .small {
    font-size: var(--fs-xs);
  }
  .name {
    font-weight: 700;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .name.archived {
    opacity: 0.55;
  }
  .dot {
    width: 12px;
    height: 12px;
    flex: none;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }

  /* Полоса цели: напоминание, а не план — тонкая, полупрозрачная, с подписью на неделе */
  .lane {
    display: flex;
    align-items: flex-end;
  }
  .bar {
    flex: 1;
    min-height: 8px;
    padding: 0 var(--s-2);
    border-radius: 999px;
    background: color-mix(in srgb, var(--goal, var(--c-text)) 25%, transparent);
    border: 1px solid color-mix(in srgb, var(--goal, var(--c-text)) 60%, transparent);
    font-size: var(--fs-xs);
    font-weight: 600;
    line-height: 16px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .bar.general {
    --goal: var(--c-focus);
  }

  .cell {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 1px;
    min-height: 28px;
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    font-size: var(--fs-sm);
    line-height: 1.1;
    overflow: hidden;
  }
  .week .cell {
    min-height: 56px;
    padding: 2px;
  }
  .cell.today {
    outline: var(--bw) solid var(--c-focus);
    outline-offset: 1px;
  }
  .cell.done {
    background: var(--c-green);
    border-color: var(--c-border);
    color: var(--c-bg);
  }
  .cell.slip {
    background: var(--c-red);
    border-color: var(--c-border);
    color: var(--c-bg);
  }
  .cell.clean {
    background: var(--c-green-soft);
  }
  .cell.before,
  .cell.future {
    background: none;
    border-style: dashed;
  }
  .cell small {
    font-size: var(--fs-xs);
  }
  .trig {
    max-width: 100%;
    padding: 0 2px;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-4);
    font-size: var(--fs-sm);
  }
  .legend span {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  .legend .cell {
    display: inline-block;
    width: 16px;
    min-height: 16px;
    height: 16px;
  }
  .legend .sample {
    display: inline-block;
    flex: none;
    width: 28px;
  }
</style>
