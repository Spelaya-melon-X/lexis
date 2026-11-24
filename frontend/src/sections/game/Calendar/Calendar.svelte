<script lang="ts">
  import { api } from '../../../lib/api/client'
  import type { CalendarItem, Task, TaskCalendar } from '../../../lib/api/types'
  import { latest } from '../../../lib/async'
  import Button from '../../../lib/components/Button.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { addDays, addMonths, isMonth, longDay, monthGrid, monthOf, monthTitle } from '../../../lib/dates'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { formatTime } from '../../../lib/plan'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import TaskPicker from '../Plan/TaskPicker.svelte'

  const requestedMonth = $derived(isMonth(route.rest) ? route.rest : '')
  let calendar = $state<TaskCalendar | null>(null)
  let error = $state<string | null>(null)
  let pickingDay = $state<string | null>(null)
  let addingDay = $state<string | null>(null)

  const month = $derived(requestedMonth || monthOf(calendar?.from ?? ''))
  const weekdays = $derived(t('journal.weekdays').split(','))
  const byDay = $derived.by(() => {
    const days = new Map<string, CalendarItem[]>()
    for (const item of calendar?.items ?? []) {
      const items = days.get(item.day)
      if (items) items.push(item)
      else days.set(item.day, [item])
    }
    return days
  })
  const planned = $derived(new Set(pickingDay ? byDay.get(pickingDay)?.map((item) => item.taskId) : []))
  const fetchCalendar = latest(api.tasks.calendar)

  function load(m: string) {
    calendar = null
    error = null
    const from = m ? `${m}-01` : undefined
    const to = m ? addDays(`${addMonths(m, 1)}-01`, -1) : undefined
    void fetchCalendar(from, to)
      .then((r) => { if (!r.stale) calendar = r.value })
      .catch((e: unknown) => (error = errorText(e)))
  }

  $effect(() => {
    pickingDay = null
    load(requestedMonth)
    return fetchCalendar.cancel
  })

  function go(m: string) {
    route.go('game', 'calendar', m)
  }

  async function add(task: Task) {
    const day = pickingDay
    if (!day || addingDay) return
    pickingDay = null
    addingDay = day
    try {
      const plan = await api.tasks.addPlanTask(day, task.id)
      if (calendar && day >= calendar.from && day <= calendar.to) {
        calendar = {
          ...calendar,
          today: plan.today,
          items: [
            ...calendar.items.filter((item) => item.day !== day),
            ...plan.items.map((item) => ({
              day, taskId: item.task.id, start: item.start, title: item.task.title, done: item.task.done,
            })),
          ],
        }
      }
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      addingDay = null
    }
  }
</script>

<div class="page">
  <header>
    <Button size="sm" variant="ghost" icon="chevronLeft" aria-label={t('journal.prevMonth')}
      disabled={!month || addingDay !== null} onclick={() => go(addMonths(month, -1))} />
    <h1>{month ? monthTitle(month, i18n.locale) : t('nav.game.calendar')}</h1>
    <Button size="sm" variant="ghost" icon="chevronRight" aria-label={t('journal.nextMonth')}
      disabled={!month || addingDay !== null} onclick={() => go(addMonths(month, 1))} />
    <Button size="sm" variant="ghost" icon="sun" disabled={addingDay !== null}
      onclick={() => requestedMonth ? go('') : load('')}>{t('date.today')}</Button>
  </header>

  {#if error}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(requestedMonth)}>{t('common.retry')}</Button></div>
  {:else if !calendar}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="scroll body">
      <div class="cal">
        {#each weekdays as w (w)}<span class="weekday">{w}</span>{/each}
        {#each monthGrid(month).flat() as day, i (day ?? i)}
          {#if day === null}
            <span aria-hidden="true"></span>
          {:else}
            {@const date = longDay(day, i18n.locale)}
            <div class="cell" class:today={day === calendar.today}>
              <a href="#/game/plan/{day}" title={date} aria-current={day === calendar.today ? 'date' : undefined}>
                <time datetime={day}>{Number(day.slice(8))}</time>
                <ul>
                  {#each byDay.get(day) ?? [] as item (item.taskId)}
                    <li class:done={item.done}><span class="muted">{formatTime(item.start)}</span> {item.title}</li>
                  {/each}
                </ul>
              </a>
              {#if day >= calendar.today}
                <button class="add" type="button" aria-label={t('game.calendar.add', { date })}
                  title={t('game.calendar.add', { date })} disabled={addingDay !== null} onclick={() => (pickingDay = day)}>
                  {#if addingDay === day}<Spinner size={14} />{:else}<Icon name="plus" size={16} />{/if}
                </button>
              {/if}
            </div>
          {/if}
        {/each}
      </div>
    </div>
  {/if}
</div>

<TaskPicker open={pickingDay !== null} day={pickingDay ?? undefined} {planned} onclose={() => (pickingDay = null)} onpick={add} />

<style>
  .page {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: var(--s-4);
    min-height: 0;
  }
  header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s-2);
  }
  h1 {
    min-width: 11ch;
    font-size: var(--fs-xl);
    text-align: center;
  }
  h1::first-letter {
    text-transform: uppercase;
  }
  .state {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
  }
  .body {
    padding: var(--s-1) var(--s-1) var(--s-4);
  }
  .cal {
    display: grid;
    grid-template-columns: repeat(7, minmax(0, 1fr));
    gap: var(--s-1);
    min-width: 630px;
  }
  .weekday {
    text-align: center;
    font-size: var(--fs-xs);
    font-weight: 700;
    color: var(--c-muted);
    text-transform: uppercase;
  }
  .cell {
    position: relative;
    display: flex;
    min-height: 78px;
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
  }
  .cell:hover {
    border-color: var(--c-border);
  }
  .today {
    outline: var(--bw) solid var(--c-focus);
  }
  a {
    flex: 1;
    min-width: 0;
    padding: var(--s-1) var(--s-2);
    color: inherit;
    text-decoration: none;
  }
  a:hover time {
    text-decoration: underline;
  }
  time {
    font-weight: 700;
  }
  ul {
    display: grid;
    gap: var(--s-1);
    margin: var(--s-1) 0 0;
    padding: 0;
    list-style: none;
  }
  li {
    padding: 2px var(--s-1);
    border-radius: var(--radius-sm);
    background: var(--c-surface-2);
    font-size: var(--fs-xs);
    overflow-wrap: anywhere;
  }
  .done {
    text-decoration: line-through;
    color: var(--c-muted);
  }
  .add {
    position: absolute;
    top: 1px;
    right: 1px;
    display: grid;
    place-items: center;
    width: 27px;
    height: 27px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    cursor: pointer;
  }
  .add:hover:not(:disabled) {
    background: var(--c-green-soft);
  }
  .add:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
