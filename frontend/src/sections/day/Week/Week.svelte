<script lang="ts">
  // Недельный анализ (§17.3): #/day/week[/<понедельник>], без даты — текущая неделя.
  // Слева — блоки «о чём → вывод» (сохраняются целиком), справа — сводка недели.
  import { fade } from 'svelte/transition'
  import { api } from '../../../lib/api/client'
  import type { Week, WeekBlock } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Icon, { type IconName } from '../../../lib/components/Icon.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { addDays, isDay, parseDay, shortDay, today } from '../../../lib/dates'
  import { weekStart } from '../../../lib/habits'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { move } from '../../../lib/journal'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import DayChart from '../DayChart.svelte'
  import HabitStats from '../HabitStats.svelte'

  const now = today()
  const current = weekStart(now)
  const monday = $derived(isDay(route.rest) && route.rest <= now ? weekStart(route.rest) : current)

  // key — стабильный ключ блока для {#each}: тексты меняются, а строка в списке та же.
  type Draft = WeekBlock & { key: number }
  let nextKey = 0
  const toDrafts = (bs: WeekBlock[]): Draft[] => bs.map((b) => ({ ...b, key: nextKey++ }))
  /** То, что реально уйдёт на бэк: без пустых блоков, с обрезанными краями — для «есть изменения». */
  const clean = (bs: WeekBlock[]) =>
    JSON.stringify(bs.map((b) => [b.topic.trim(), b.conclusion.trim()]).filter(([a, b]) => a || b))

  let week = $state<Week | null>(null)
  let blocks = $state<Draft[]>([])
  let savedJSON = $state('[]')
  let error = $state<string | null>(null)
  let saving = $state(false)
  let pendingWeek = $state<string | null>(null)

  const dirty = $derived(clean(blocks) !== savedJSON)
  // Подсказка «пора подвести итоги» — в пт/сб/вс текущей недели, пока анализа нет.
  const weekday = parseDay(now).getDay() // 0 — воскресенье
  const timeHint = $derived(monday === current && (weekday >= 5 || weekday === 0) && blocks.length === 0)

  function apply(w: Week) {
    week = w
    blocks = toDrafts(w.blocks)
    savedJSON = clean(w.blocks)
  }

  let seq = 0
  async function load(m: string) {
    const my = ++seq
    week = null
    error = null
    try {
      const w = await api.journal.week(m)
      if (my === seq) apply(w)
    } catch (e) {
      if (my === seq) error = errorText(e)
    }
  }
  $effect(() => void load(monday))

  function go(m: string) {
    if (dirty) {
      pendingWeek = m
      return
    }
    route.go('day', 'week', m === current ? '' : m)
  }

  async function save() {
    if (!dirty || saving) return
    saving = true
    const m = monday
    try {
      const w = await api.journal.saveWeek(m, blocks.map(({ topic, conclusion }) => ({ topic, conclusion })))
      if (m === monday) apply(w)
      toasts.success(t('journal.week.saved'))
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      saving = false
    }
  }

  function add() {
    blocks.push({ topic: '', conclusion: '', key: nextKey++ })
  }

  $effect(() => {
    if (!dirty) return
    const warn = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  })

  const fmt = (n: number | null) => (n === null ? '—' : n.toLocaleString(i18n.locale, { maximumFractionDigits: 1 }))
  const habitName = (id: number | null) => week?.summary.habits.find((h) => h.habitId === id)?.name
</script>

<svelte:window
  onkeydown={(e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 's') {
      e.preventDefault()
      void save()
    }
  }}
/>

{#snippet act(icon: IconName, label: string, onclick: () => void, disabled = false)}
  <button class="act" aria-label={label} title={label} {disabled} {onclick}><Icon name={icon} size={16} /></button>
{/snippet}

<div class="page">
  <header>
    <button class="arrow" aria-label={t('journal.week.prev')} title={t('journal.week.prev')} onclick={() => go(addDays(monday, -7))}><Icon name="chevronLeft" /></button>
    <h1>{shortDay(monday, i18n.locale)} – {shortDay(addDays(monday, 6), i18n.locale)}</h1>
    <button class="arrow" aria-label={t('journal.week.next')} title={t('journal.week.next')} disabled={monday >= current} onclick={() => go(addDays(monday, 7))}>
      <Icon name="chevronRight" />
    </button>
    {#if monday !== current}
      <Button size="sm" variant="ghost" icon="week" onclick={() => go(current)}>{t('journal.week.current')}</Button>
    {/if}
    <span class="spacer"></span>
    {#if dirty}<span class="muted hint">{t('journal.week.unsaved')}</span>{/if}
    <Button variant="primary" icon="check" disabled={!dirty} loading={saving} onclick={save} title="⌘/Ctrl + S">{t('journal.week.save')}</Button>
  </header>

  {#if error && !week}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(monday)}>{t('common.retry')}</Button></div>
  {:else if !week}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="grid scroll">
      <Card class="analysis">
        <h2>{t('journal.week.analysis')}</h2>
        <p class="muted sub">{t('journal.week.analysisHint')}</p>
        {#if timeHint}<p class="nudge">{t('journal.week.timeHint')}</p>{/if}

        {#if blocks.length === 0}
          <p class="muted empty">{t('journal.week.empty')}</p>
        {/if}
        <ol class="blocks">
          {#each blocks as b, i (b.key)}
            <li in:fade={{ duration: 200 }}>
              <div class="block-head">
                <span class="n">{i + 1}</span>
                <input class="input topic" bind:value={b.topic} maxlength="200" placeholder={t('journal.week.topicPlaceholder')} aria-label={t('journal.week.topic')} />
                <span class="acts">
                  {@render act('chevronLeft', t('journal.moveUp'), () => (blocks = move(blocks, i, -1)), i === 0)}
                  {@render act('chevronRight', t('journal.moveDown'), () => (blocks = move(blocks, i, 1)), i === blocks.length - 1)}
                  {@render act('trash', t('journal.week.removeBlock'), () => blocks.splice(i, 1))}
                </span>
              </div>
              <textarea
                class="input conclusion"
                bind:value={b.conclusion}
                maxlength="20000"
                placeholder={t('journal.week.conclusionPlaceholder')}
                aria-label={t('journal.week.conclusion')}
              ></textarea>
            </li>
          {/each}
        </ol>
        <Button variant="ghost" icon="plus" onclick={add}>{t('journal.week.addBlock')}</Button>
      </Card>

      <div class="side">
        <Card>
          <h2>{t('journal.week.summary')}</h2>
          <div class="tiles">
            <div><span class="muted label">{t('journal.stats.filled')}</span><b>{week.summary.filled}<small class="muted"> / 7</small></b></div>
            <div><span class="muted label">{t('journal.stats.avgRating')}</span><b>{fmt(week.summary.avgRating)}</b></div>
            <div><span class="muted label">{t('journal.stats.avgDopamine')}</span><b>{fmt(week.summary.avgDopamine)}</b></div>
          </div>
          <DayChart days={week.summary.days} height={160} />
        </Card>

        <Card>
          <h2>{t('habits.dayBlock')}</h2>
          {#if week.summary.habits.length === 0}
            <p class="muted empty">{t('journal.week.noHabits')}</p>
          {:else}
            <HabitStats stats={week.summary.habits} />
          {/if}
        </Card>

        {#if week.summary.goals.length > 0}
          <Card>
            <h2>{t('journal.week.goals')}</h2>
            <ul class="goals">
              {#each week.summary.goals as g (g.id)}
                <li>
                  <span>{habitName(g.habitId) ?? ''}{g.title && habitName(g.habitId) ? ' — ' : ''}{g.title}</span>
                  <span class="muted">
                    {shortDay(g.from, i18n.locale)} – {shortDay(g.to, i18n.locale)}{#if g.progress}
                      · {g.progress.done}/{g.progress.total}{/if}
                  </span>
                </li>
              {/each}
            </ul>
          </Card>
        {/if}
      </div>
    </div>
  {/if}
</div>

<ConfirmDialog
  open={pendingWeek !== null}
  title={t('journal.week.unsaved')}
  confirmText={t('common.confirm')}
  danger
  onconfirm={() => {
    const m = pendingWeek!
    pendingWeek = null
    savedJSON = clean(blocks) // сбросить «грязность» — правки отброшены
    route.go('day', 'week', m === current ? '' : m)
  }}
  oncancel={() => (pendingWeek = null)}
/>

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
  .spacer {
    flex: 1;
  }
  .hint {
    font-size: var(--fs-sm);
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
  .grid {
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
    align-items: start;
    gap: var(--s-4);
    padding: 0 var(--s-1) var(--s-4) 0;
  }
  .side {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    min-width: 0;
  }
  h2 {
    margin-bottom: var(--s-3);
    font-size: var(--fs-md);
  }
  .sub {
    margin: calc(-1 * var(--s-2)) 0 var(--s-3);
    font-size: var(--fs-sm);
  }
  .nudge {
    margin-bottom: var(--s-3);
    padding: var(--s-2) var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-amber-soft);
    font-size: var(--fs-sm);
    font-weight: 600;
  }
  .empty {
    font-size: var(--fs-sm);
    margin-bottom: var(--s-3);
  }
  .blocks {
    margin: 0 0 var(--s-3);
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  .blocks li {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding: var(--s-3);
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
  }
  .blocks li:focus-within {
    border-color: var(--c-border);
  }
  .block-head {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  .n {
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    flex: none;
    border-radius: 50%;
    background: var(--c-text);
    color: var(--c-bg);
    font-size: var(--fs-xs);
    font-weight: 700;
  }
  .topic {
    font-weight: 700;
  }
  .conclusion {
    min-height: 80px;
    resize: vertical;
    line-height: 1.6;
    field-sizing: content;
  }
  .acts {
    display: flex;
    gap: 2px;
  }
  .act {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--c-muted);
    cursor: pointer;
  }
  .act:nth-child(1) :global(svg),
  .act:nth-child(2) :global(svg) {
    transform: rotate(90deg);
  }
  .act:hover:not(:disabled) {
    background: var(--c-surface-2);
    color: var(--c-text);
  }
  .act:disabled {
    opacity: 0.3;
    cursor: default;
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--s-2);
    margin-bottom: var(--s-3);
  }
  .tiles div {
    display: flex;
    flex-direction: column;
  }
  .label {
    font-size: var(--fs-xs);
  }
  .tiles b {
    font-size: var(--fs-lg);
  }
  .tiles small {
    font-size: var(--fs-xs);
  }
  .goals {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    font-size: var(--fs-sm);
  }
  .goals li {
    display: flex;
    justify-content: space-between;
    gap: var(--s-2);
  }

  @media (max-width: 960px) {
    .grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
