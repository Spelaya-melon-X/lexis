<script lang="ts">
  // «Тренировки» (§18.4, §18.5.7): сверху по центру — неделя (сегодня подсвечено, в днях — тренировки);
  // ниже — форма: новая тренировка на выбранный день (клик по дню) или правка выбранной (клик по тренировке).
  // Под формой — итоги недели. Неделя — в адресе: #/sport/workouts/<понедельник>.
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { Exercise, Workout } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { addDays, isDay, parseDay, today as todayOf } from '../../../lib/dates'
  import { daysRange, weekStart } from '../../../lib/habits'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import WeekStats from './WeekStats.svelte'
  import WorkoutForm from './WorkoutForm.svelte'

  const today = todayOf()
  const monday = $derived(weekStart(isDay(route.rest) ? route.rest : today))
  const week = $derived(daysRange(monday, addDays(monday, 6)))

  let workouts = $state<Workout[] | null>(null)
  let directory = $state<Exercise[]>([])
  let titles = $state<string[]>([])
  let error = $state<string | null>(null)
  let editing = $state<Workout | null>(null) // null — в форме новая тренировка
  let pickedDay = $state<string | null>(null) // день новой тренировки, выбранный в неделе

  // Новая тренировка — на выбранный день; не выбран — сегодня (если это эта неделя), иначе понедельник.
  const formDay = $derived(pickedDay && week.includes(pickedDay) ? pickedDay : week.includes(today) ? today : monday)

  const fetchWeek = latest((from: string, to: string) => api.sport.workouts(from, to))

  function load(m: string) {
    error = null
    void fetchWeek(m, addDays(m, 6))
      .then((r) => !r.stale && (workouts = r.value.workouts))
      .catch((e: unknown) => (error = errorText(e)))
  }
  $effect(() => load(monday))

  function loadDirectory() {
    api.sport.exercises().then((x) => (directory = x)).catch(() => {})
    api.sport.titles().then((x) => (titles = x)).catch(() => {})
  }
  onMount(loadDirectory)

  function go(m: string) {
    editing = null
    pickedDay = null
    route.go('sport', 'workouts', m === weekStart(today) ? '' : m)
  }

  function pickDay(d: string) {
    if (d > today) return
    editing = null
    pickedDay = d
  }

  function onsaved(w: Workout, created: boolean) {
    toasts.success(created ? t('sport.added') : t('sport.saved'))
    editing = null // форма снова готова к новой тренировке
    pickedDay = w.day
    loadDirectory() // новые упражнения и названия — в подсказки, «прошлый раз» — свежий
    if (weekStart(w.day) !== monday) route.go('sport', 'workouts', weekStart(w.day) === weekStart(today) ? '' : weekStart(w.day))
    else load(monday)
  }

  const byDay = $derived.by(() => {
    const m = new Map<string, Workout[]>()
    for (const w of workouts ?? []) m.set(w.day, [...(m.get(w.day) ?? []), w])
    return m
  })
  const weekday = (d: string) => parseDay(d).toLocaleDateString(i18n.locale, { weekday: 'short' })
  const dayNum = (d: string) => parseDay(d).getDate()
  const fmt = (d: string) => parseDay(d).toLocaleDateString(i18n.locale, { day: 'numeric', month: 'short' })
</script>

<div class="page scroll">
  <header>
    <button class="arrow" aria-label={t('sport.prevWeek')} title={t('sport.prevWeek')} onclick={() => go(addDays(monday, -7))}><Icon name="chevronLeft" /></button>
    <h1>{fmt(monday)} — {fmt(addDays(monday, 6))}</h1>
    <button class="arrow" aria-label={t('sport.nextWeek')} title={t('sport.nextWeek')} disabled={addDays(monday, 7) > today}
      onclick={() => go(addDays(monday, 7))}><Icon name="chevronRight" /></button>
    {#if monday !== weekStart(today)}
      <Button size="sm" variant="ghost" icon="sun" onclick={() => go(weekStart(today))}>{t('sport.thisWeek')}</Button>
    {/if}
  </header>

  {#if error && !workouts}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(monday)}>{t('common.retry')}</Button></div>
  {:else if !workouts}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="week">
      {#each week as d (d)}
        <!-- Клик по дню — выбрать его для новой тренировки; клик по тренировке — открыть её. -->
        <div
          class="day"
          class:today={d === today}
          class:future={d > today}
          class:picked={!editing && d === formDay}
          role="button"
          tabindex={d > today ? -1 : 0}
          aria-label={t('sport.pickDay', { day: `${weekday(d)} ${dayNum(d)}` })}
          onclick={() => pickDay(d)}
          onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), pickDay(d))}
        >
          <div class="day-head">
            <span class="wd">{weekday(d)}</span>
            <span class="num">{dayNum(d)}</span>
          </div>
          {#each byDay.get(d) ?? [] as w (w.id)}
            <button class="chip" class:active={editing?.id === w.id} onclick={(e) => (e.stopPropagation(), (editing = w))} in:fade={{ duration: 200 }}>
              {w.title}
              {#if w.kcal}<small>{w.kcal} {t('sport.kcalShort')}</small>{/if}
            </button>
          {/each}
        </div>
      {/each}
    </div>

    {#key editing?.id ?? `new-${formDay}`}
      <div in:fade={{ duration: 200 }}>
        <WorkoutForm workout={editing} day={formDay} {today} {directory} {titles} {onsaved}
          ondeleted={() => ((editing = null), toasts.success(t('sport.deleted')), load(monday))} oncancel={() => (editing = null)} />
      </div>
    {/key}

    <WeekStats {workouts} days={week} />
  {/if}
</div>

<style>
  .page {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    padding: 2px 6px var(--s-4) 2px;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-2);
  }
  h1 {
    min-width: 200px;
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
    text-align: center;
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
    opacity: 0.3;
    cursor: default;
  }
  .state {
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
    padding: var(--s-6);
  }
  .week {
    display: grid;
    grid-template-columns: repeat(7, minmax(0, 1fr));
    gap: var(--s-2);
  }
  .day {
    min-height: 110px;
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    padding: var(--s-2);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    cursor: pointer;
    transition: box-shadow var(--dur-fast) var(--ease-out);
  }
  .day:hover {
    box-shadow: var(--shadow-sm);
  }
  .day.today {
    background: var(--c-green-soft);
  }
  .day.picked {
    box-shadow: var(--shadow);
    border-width: var(--bw-strong);
  }
  .day.future {
    border-style: dashed;
    border-color: var(--c-line);
    cursor: default;
    box-shadow: none;
  }
  .day-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }
  .wd {
    color: var(--c-muted);
    font-size: var(--fs-xs);
    text-transform: uppercase;
    font-weight: 700;
  }
  .num {
    font-weight: 800;
  }
  .chip {
    display: flex;
    flex-direction: column;
    padding: 2px var(--s-2);
    border: 1px solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    font: inherit;
    font-size: var(--fs-xs);
    font-weight: 700;
    text-align: left;
    overflow-wrap: anywhere;
    cursor: pointer;
  }
  .chip small {
    color: var(--c-muted);
    font-weight: 400;
  }
  .chip.active {
    background: var(--c-text);
    color: var(--c-bg);
  }
  .chip.active small {
    color: inherit;
  }
  @media (max-width: 700px) {
    .week {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
