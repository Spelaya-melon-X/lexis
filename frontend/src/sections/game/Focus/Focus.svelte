<script lang="ts">
  // «Фокус» (§18.3, B4): настройка (таймер + чек-лист ритуала) → работа ⇄ перерыв → итог.
  // Состояние сессии — в БД; здесь только отображение и действия. Таймеры — от серверных отметок (state/focus).
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { api } from '../../../lib/api/client'
  import type { FocusAction, FocusOutcome, FocusSession, Plan } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import ProgressBar from '../../../lib/components/ProgressBar.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { today } from '../../../lib/dates'
  import { currentBreakSec, formatClock, ritualDone } from '../../../lib/focus'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { focus } from '../../../lib/state/focus.svelte'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import { blocksText } from '../format'

  let ritual = $state<string[]>([])
  let setupMin = $state(10)
  let checked = $state<number[]>([])
  let summary = $state<FocusSession | null>(null) // итог только что завершённой сессии
  let plan = $state<Plan | null>(null) // для быстрого старта, когда сессии нет
  let busy = $state(false)
  let trigger = $state('')
  let suggestions = $state<string[]>([])
  let confirm = $state<FocusOutcome | null>(null)

  const s = $derived(focus.session)
  const tl = $derived(focus.timeline)
  const targetSec = $derived(s ? s.plannedBlocks * 30 * 60 : 0)
  const ritualKey = $derived(s ? `lexis.ritual.${s.id}` : '')

  onMount(() => {
    focus.refresh()
    api.focus.ritual().then((r) => (ritual = r)).catch(() => {})
    api.focus.settings().then((st) => (setupMin = st.setupMin)).catch(() => {})
    api.focus.triggers().then((r) => (suggestions = r)).catch(() => {})
    api.tasks.plan(today()).then((p) => (plan = p)).catch(() => {})
  })

  // Отметки ритуала — на фронте, по id сессии (B9): перезагрузка их не сбрасывает.
  $effect(() => {
    if (!ritualKey) return
    try {
      const v: unknown = JSON.parse(localStorage.getItem(ritualKey) ?? '[]')
      checked = Array.isArray(v) ? v.filter((x): x is number => typeof x === 'number') : []
    } catch {
      checked = []
    }
  })

  function toggleRitual(i: number) {
    checked = checked.includes(i) ? checked.filter((x) => x !== i) : [...checked, i]
    try {
      localStorage.setItem(ritualKey, JSON.stringify(checked))
    } catch {
      // не страшно
    }
  }

  async function run(fn: () => Promise<FocusSession>) {
    if (busy) return
    busy = true
    try {
      const next = await fn()
      focus.apply(next)
      if (next.finishedAt) {
        summary = next
        try {
          localStorage.removeItem(`lexis.ritual.${next.id}`)
        } catch {
          // не страшно
        }
        if (next.points > 0) toasts.success(t('game.focus.pointsEarned', { count: next.points }))
        if (next.outcome === 'done') toasts.success(t('game.doneToast'))
      }
    } catch (e) {
      toasts.error(errorText(e))
      focus.refresh()
    } finally {
      busy = false
    }
  }

  const act = (a: FocusAction) => s && run(() => api.focus.act(s.id, a))

  function finish(o: FocusOutcome) {
    confirm = null
    if (s) void run(() => api.focus.finish(s.id, o))
  }

  async function distract(gaveIn: boolean) {
    const text = trigger.trim()
    if (!s || !text) return
    await run(() => api.focus.distract(s.id, text, gaveIn))
    trigger = ''
    api.focus.triggers().then((r) => (suggestions = r)).catch(() => {})
  }

  const openPlanItems = $derived(plan?.items.filter((x) => !x.task.done) ?? [])
</script>

<div class="page scroll">
  {#if s && tl}
    <section class="session {tl.phase}" in:fade={{ duration: 200 }}>
      <p class="task muted">{t(`game.focus.phase.${tl.phase as 'work'}`)}</p>
      <h1>{s.taskTitle}</h1>

      {#if tl.phase === 'setup'}
        {@const left = setupMin * 60 - tl.setupSec}
        <div class="clock" class:over={left < 0}>{left >= 0 ? formatClock(left) : `+${formatClock(-left)}`}</div>
        <p class="muted hint">{left >= 0 ? t('game.focus.setupHint') : t('game.focus.setupOver')}</p>
        {#if ritual.length > 0}
          <ul class="ritual">
            {#each ritual as item, i (i)}
              <li>
                <label class:checked={checked.includes(i)}>
                  <input type="checkbox" checked={checked.includes(i)} onchange={() => toggleRitual(i)} />
                  {item}
                </label>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="muted hint">{t('game.focus.noRitual')} <a href="#/game/settings">{t('game.focus.setupRitual')}</a></p>
        {/if}
        <div class="actions">
          <Button variant="primary" size="lg" icon="play" loading={busy} disabled={!ritualDone(ritual.length, checked)} onclick={() => act('work')}>
            {t('game.focus.startWork')}
          </Button>
          <Button variant="ghost" onclick={() => (confirm = 'abandoned')}>{t('game.focus.abandon')}</Button>
        </div>
      {:else}
        {#if tl.phase === 'work'}
          <div class="clock">{formatClock(tl.workSec)}</div>
          <ProgressBar value={tl.workSec / targetSec} label={t('game.focus.plan', { time: blocksText(s.plannedBlocks) })} />
          <p class="muted hint">
            {#if tl.workSec >= targetSec}
              <strong class="extra">{t('game.focus.extra', { time: formatClock(tl.workSec - targetSec) })}</strong>
            {:else}
              {t('game.focus.plan', { time: blocksText(s.plannedBlocks) })} · {t('game.focus.left', { time: formatClock(targetSec - tl.workSec) })}
            {/if}
          </p>
        {:else}
          {@const thisBreak = currentBreakSec(s, focus.now)}
          <div class="clock" class:over={tl.breakAvailableSec === 0 && tl.breakOverrunSec > 0}>
            {tl.breakAvailableSec > 0 ? formatClock(tl.breakAvailableSec) : `+${formatClock(tl.breakOverrunSec)}`}
          </div>
          <p class="muted hint">
            {tl.breakAvailableSec > 0 ? t('game.focus.breakLeft') : t('game.focus.breakOver')} · {t('game.focus.thisBreak', { time: formatClock(thisBreak) })}
          </p>
        {/if}

        <p class="bank">
          <Icon name="pause" size={16} />
          {t('game.focus.bank', { available: formatClock(tl.breakAvailableSec), earned: formatClock(tl.breakEarnedSec) })}
        </p>

        <div class="actions">
          {#if tl.phase === 'work'}
            <Button icon="pause" loading={busy} onclick={() => act('break')}>{t('game.focus.break')}</Button>
          {:else}
            <Button variant="primary" icon="play" loading={busy} onclick={() => act('resume')}>{t('game.focus.resume')}</Button>
          {/if}
          <Button variant="primary" icon="check" loading={busy} onclick={() => (confirm = 'done')}>{t('game.focus.done')}</Button>
          <Button variant="ghost" onclick={() => (confirm = 'stopped')}>{t('game.focus.stop')}</Button>
        </div>

        <form class="distract" onsubmit={(e) => e.preventDefault()}>
          <h2>{t('game.focus.distracted')}</h2>
          <input class="input" list="focus-triggers" bind:value={trigger} maxlength="200" placeholder={t('game.focus.triggerPlaceholder')}
            aria-label={t('game.focus.distracted')} />
          <datalist id="focus-triggers">
            {#each suggestions as x (x)}<option value={x}></option>{/each}
          </datalist>
          <div class="pair">
            <Button size="sm" variant="danger" disabled={!trigger.trim() || busy} onclick={() => distract(true)}>{t('game.focus.gaveIn')}</Button>
            <Button size="sm" disabled={!trigger.trim() || busy} onclick={() => distract(false)}>{t('game.focus.resisted')}</Button>
          </div>
          {#if s.distractions.length > 0}
            <ul class="distractions">
              {#each [...s.distractions].reverse() as d (d.id)}
                <li class:gave={d.gaveIn} in:fade={{ duration: 200 }}>
                  <span>{d.trigger}</span>
                  <span class="muted">{d.gaveIn ? t('game.focus.gaveIn') : t('game.focus.resisted')}</span>
                </li>
              {/each}
            </ul>
          {/if}
        </form>
      {/if}
    </section>
  {:else if summary}
    {@const st = summary.timeline}
    <section class="summary" in:fade={{ duration: 200 }}>
      <p class="muted">{t(`game.focus.outcome.${summary.outcome ?? 'stopped'}`)}</p>
      <h1>{summary.taskTitle}</h1>
      <div class="tiles">
        <div><span class="muted">{t('game.focus.work')}</span><strong>{formatClock(st.workSec)}</strong></div>
        <div><span class="muted">{t('game.focus.setup')}</span><strong>{formatClock(st.setupSec)}</strong></div>
        <div><span class="muted">{t('game.focus.breaks')}</span><strong>{formatClock(st.breakSec)}</strong></div>
        <div><span class="muted">{t('game.focus.distractions')}</span><strong>{summary.distractions.length}</strong></div>
        <div class="pts"><span class="muted">{t('game.focus.points')}</span><strong>+{summary.points}</strong></div>
      </div>
      <div class="actions">
        <Button variant="primary" icon="clock" onclick={() => route.go('game', 'plan')}>{t('game.focus.toPlan')}</Button>
        <Button variant="ghost" onclick={() => (summary = null)}>{t('common.close')}</Button>
      </div>
    </section>
  {:else}
    <section class="idle" in:fade={{ duration: 200 }}>
      <Icon name="target" size={36} />
      <h1>{t('game.focus.idle')}</h1>
      <p class="muted">{t('game.focus.idleHint')}</p>
      {#if !plan}
        <Spinner size={24} />
      {:else if openPlanItems.length > 0}
        <ul class="quick">
          {#each openPlanItems as item (item.task.id)}
            <li>
              <span>{item.task.title} <span class="muted">· {blocksText(item.blocks)}</span></span>
              <Button size="sm" variant="primary" icon="play" onclick={() => focus.begin(item.task.id)}>{t('game.focus.begin')}</Button>
            </li>
          {/each}
        </ul>
      {:else}
        <Button icon="clock" onclick={() => route.go('game', 'plan')}>{t('game.focus.toPlan')}</Button>
      {/if}
    </section>
  {/if}
</div>

<ConfirmDialog
  open={confirm !== null}
  title={confirm ? t(`game.focus.confirm.${confirm}`) : ''}
  danger={confirm === 'abandoned'}
  loading={busy}
  onconfirm={() => confirm && finish(confirm)}
  oncancel={() => (confirm = null)}
>
  {confirm ? t(`game.focus.confirmText.${confirm}`) : ''}
</ConfirmDialog>

<style>
  .page {
    flex: 1;
    min-height: 0;
    display: flex;
    justify-content: center;
    padding: var(--s-4) var(--s-2);
  }
  section {
    width: min(640px, 100%);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--s-3);
    text-align: center;
  }
  h1 {
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
  }
  h2 {
    font-size: var(--fs-md);
  }
  .task {
    margin: 0;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    font-size: var(--fs-xs);
  }
  .clock {
    font-size: 72px;
    font-weight: 800;
    font-variant-numeric: tabular-nums;
    line-height: 1;
    letter-spacing: -0.03em;
  }
  .clock.over {
    color: var(--c-red);
  }
  .session :global(.track) {
    width: 100%;
  }
  .hint {
    margin: 0;
  }
  .extra {
    color: var(--c-green);
  }
  .bank {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    margin: 0;
    padding: var(--s-1) var(--s-3);
    border: 1px solid var(--c-line);
    border-radius: 999px;
    font-size: var(--fs-sm);
  }
  .ritual {
    width: 100%;
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    text-align: left;
  }
  .ritual label {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    padding: var(--s-2) var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    font-weight: 600;
    cursor: pointer;
    transition: background var(--dur-fast) var(--ease-out);
  }
  .ritual label.checked {
    background: var(--c-green-soft);
  }
  .ritual input {
    width: 20px;
    height: 20px;
    accent-color: var(--c-green);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: var(--s-2);
  }
  .distract {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    margin-top: var(--s-4);
    padding: var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
    text-align: left;
  }
  .pair {
    display: flex;
    gap: var(--s-2);
  }
  .distractions {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    font-size: var(--fs-sm);
  }
  .distractions li {
    display: flex;
    justify-content: space-between;
    gap: var(--s-2);
    padding: var(--s-1) var(--s-2);
    border-left: var(--bw-strong) solid var(--c-green);
  }
  .distractions li.gave {
    border-left-color: var(--c-red);
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
    gap: var(--s-2);
    width: 100%;
  }
  .tiles div {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    padding: var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
  }
  .tiles span {
    font-size: var(--fs-xs);
  }
  .tiles strong {
    font-size: var(--fs-lg);
    font-variant-numeric: tabular-nums;
  }
  .tiles .pts {
    background: var(--c-green-soft);
  }
  .quick {
    width: 100%;
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .quick li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    text-align: left;
    font-weight: 600;
  }
</style>
