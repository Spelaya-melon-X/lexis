<script lang="ts">
  // «Цели» (§17.4): цели на период — напоминание, а не план. Прогресс — у цели с привычкой.
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { api } from '../../../lib/api/client'
  import type { Goal, Habit } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import ProgressBar from '../../../lib/components/ProgressBar.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { shortDay, today } from '../../../lib/dates'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import GoalDialog from './GoalDialog.svelte'

  let goals = $state<Goal[] | null>(null)
  let habits = $state<Habit[]>([])
  let error = $state<string | null>(null)
  let dialogOpen = $state(false)
  let editing = $state<Goal | null>(null)
  let deleting = $state<Goal | null>(null)
  let busy = $state(false)

  const now = today()
  const byId = $derived(new Map(habits.map((h) => [h.id, h])))
  // Текущие и будущие — по началу (ближайшие сверху); прошедшие — свежие сверху (бэк отдаёт так).
  const current = $derived((goals ?? []).filter((g) => g.to >= now).sort((a, b) => a.from.localeCompare(b.from)))
  const past = $derived((goals ?? []).filter((g) => g.to < now))

  async function load() {
    try {
      const [gs, hs] = await Promise.all([api.habits.goals(), api.habits.list(true)])
      goals = gs
      habits = hs
      error = null
    } catch (e) {
      error = errorText(e)
    }
  }
  onMount(load)

  function openNew() {
    editing = null
    dialogOpen = true
  }

  function saved(g: Goal) {
    dialogOpen = false
    if (!goals) return
    goals = goals.some((x) => x.id === g.id) ? goals.map((x) => (x.id === g.id ? g : x)) : [g, ...goals]
  }

  async function remove() {
    if (!deleting || !goals) return
    busy = true
    try {
      await api.habits.removeGoal(deleting.id)
      goals = goals.filter((x) => x.id !== deleting!.id)
      deleting = null
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
    }
  }

  function progressText(g: Goal): string {
    if (!g.progress) return ''
    const negative = g.habitId !== null && byId.get(g.habitId)?.kind === 'negative'
    return t(negative ? 'habits.goalProgressNegative' : 'habits.goalProgressPositive', { done: g.progress.done, total: g.progress.total })
  }
</script>

{#snippet item(g: Goal)}
  {@const h = g.habitId !== null ? byId.get(g.habitId) : undefined}
  <li in:fade={{ duration: 200 }} class:upcoming={g.from > now}>
    <div class="top">
      {#if h}
        <span class="dot" style:background={h.color ?? 'transparent'}></span>
        <b>{h.name}</b>
      {/if}
      {#if g.title}<span class:muted={h}>{h ? '— ' : ''}{g.title}</span>{/if}
      <span class="dates muted">{shortDay(g.from, i18n.locale)} – {shortDay(g.to, i18n.locale)}</span>
      <span class="acts">
        <button class="act" aria-label={t('journal.edit')} title={t('journal.edit')} onclick={() => ((editing = g), (dialogOpen = true))}>
          <Icon name="edit" size={16} />
        </button>
        <button class="act" aria-label={t('common.delete')} title={t('common.delete')} onclick={() => (deleting = g)}>
          <Icon name="trash" size={16} />
        </button>
      </span>
    </div>
    {#if g.progress}
      <div class="progress">
        <ProgressBar value={g.progress.done / g.progress.total} color={h?.color ?? undefined} label={progressText(g)} />
        <span class="muted small">{progressText(g)}</span>
      </div>
    {/if}
  </li>
{/snippet}

<div class="page">
  <header>
    <div>
      <h1>{t('habits.goals')}</h1>
      <p class="muted small">{t('habits.goalsHint')}</p>
    </div>
    <Button variant="primary" icon="plus" onclick={openNew}>{t('habits.goalNew')}</Button>
  </header>

  {#if error && !goals}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={load}>{t('common.retry')}</Button></div>
  {:else if !goals}
    <div class="state"><Spinner size={28} /></div>
  {:else if goals.length === 0}
    <div class="state"><p class="muted">{t('habits.goalsEmpty')}</p></div>
  {:else}
    <div class="scroll body">
      {#if current.length}
        <h2>{t('habits.goalsCurrent')}</h2>
        <ul>{#each current as g (g.id)}{@render item(g)}{/each}</ul>
      {/if}
      {#if past.length}
        <h2>{t('habits.goalsPast')}</h2>
        <ul class="past">{#each past as g (g.id)}{@render item(g)}{/each}</ul>
      {/if}
    </div>
  {/if}
</div>

<GoalDialog open={dialogOpen} goal={editing} {habits} onclose={() => (dialogOpen = false)} onsaved={saved} />

<ConfirmDialog
  open={deleting !== null}
  title={t('habits.goalDeleteTitle')}
  confirmText={t('common.delete')}
  danger
  loading={busy}
  onconfirm={remove}
  oncancel={() => (deleting = null)}
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
    justify-content: space-between;
    gap: var(--s-4);
  }
  h1 {
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
  }
  h2 {
    font-size: var(--fs-md);
  }
  .small {
    font-size: var(--fs-sm);
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
    gap: var(--s-3);
    padding: 0 var(--s-1) var(--s-4) 0;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  li {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding: var(--s-3) var(--s-4);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
    box-shadow: var(--shadow-sm);
  }
  li.upcoming {
    border-style: dashed;
  }
  .past li {
    opacity: 0.65;
    box-shadow: none;
  }
  .top {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    flex-wrap: wrap;
  }
  .dot {
    width: 12px;
    height: 12px;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
  .dates {
    margin-left: auto;
    font-size: var(--fs-sm);
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
  .act:hover {
    background: var(--c-surface-2);
    color: var(--c-text);
  }
  .progress {
    display: grid;
    grid-template-columns: 1fr auto;
    align-items: center;
    gap: var(--s-3);
  }
</style>
