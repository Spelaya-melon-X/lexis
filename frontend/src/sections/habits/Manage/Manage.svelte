<script lang="ts">
  // «Мои привычки» (§17.4): создать / изменить / в архив / удалить / порядок.
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { api } from '../../../lib/api/client'
  import type { Habit } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Icon, { type IconName } from '../../../lib/components/Icon.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import HabitDialog from '../HabitDialog.svelte'

  let all = $state<Habit[] | null>(null) // все, включая архив: порядок шлём полным списком
  let error = $state<string | null>(null)
  let showArchived = $state(false)
  let dialogOpen = $state(false)
  let editing = $state<Habit | null>(null)
  let deleting = $state<Habit | null>(null)
  let busy = $state(false)

  const shown = $derived((all ?? []).filter((h) => showArchived || !h.archived))
  const archivedCount = $derived((all ?? []).filter((h) => h.archived).length)

  async function load() {
    try {
      all = await api.habits.list(true)
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
  function openEdit(h: Habit) {
    editing = h
    dialogOpen = true
  }

  function saved(h: Habit) {
    dialogOpen = false
    if (!all) return
    all = all.some((x) => x.id === h.id) ? all.map((x) => (x.id === h.id ? h : x)) : [...all, h]
  }

  async function setArchived(h: Habit, archived: boolean) {
    try {
      saved(await api.habits.update(h.id, { archived }))
    } catch (e) {
      toasts.error(errorText(e))
    }
  }

  async function remove() {
    if (!deleting || !all) return
    busy = true
    try {
      await api.habits.remove(deleting.id)
      all = all.filter((x) => x.id !== deleting!.id)
      deleting = null
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
    }
  }

  /**
   * Сдвиг среди видимых: меняем местами с соседом по видимому списку, а на бэк шлём
   * полный порядок (архивные остаются на своих местах между ними).
   */
  async function shift(h: Habit, dir: -1 | 1) {
    if (!all) return
    const neighbour = shown[shown.indexOf(h) + dir]
    if (!neighbour) return
    const prev = all
    const i = all.indexOf(h)
    const j = all.indexOf(neighbour)
    const next = [...all]
    ;[next[i], next[j]] = [next[j]!, next[i]!]
    all = next // сразу — интерфейс не ждёт сеть
    try {
      await api.habits.reorder(next.map((x) => x.id))
    } catch (e) {
      all = prev
      toasts.error(errorText(e))
    }
  }
</script>

{#snippet act(icon: IconName, label: string, onclick: () => void, disabled = false)}
  <button class="act" aria-label={label} title={label} {disabled} {onclick}><Icon name={icon} size={16} /></button>
{/snippet}

<div class="page">
  <header>
    <h1>{t('nav.habits.manage')}</h1>
    {#if archivedCount > 0}
      <label class="toggle"><input type="checkbox" bind:checked={showArchived} /> {t('habits.showArchived')} ({archivedCount})</label>
    {/if}
    <Button variant="primary" icon="plus" onclick={openNew}>{t('habits.new')}</Button>
  </header>

  {#if error && !all}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={load}>{t('common.retry')}</Button></div>
  {:else if !all}
    <div class="state"><Spinner size={28} /></div>
  {:else if shown.length === 0}
    <div class="state">
      <p class="muted">{t('habits.empty')}</p>
    </div>
  {:else}
    <ul class="list scroll">
      {#each shown as h, i (h.id)}
        <li class:archived={h.archived} in:fade={{ duration: 200 }}>
          <span class="dot" style:background={h.color ?? 'transparent'}></span>
          <button class="name" onclick={() => openEdit(h)}>{h.name}</button>
          <span class="badge {h.kind}">{t(h.kind === 'positive' ? 'habits.positive' : 'habits.negative')}</span>
          <span class="tracks muted">
            {#if h.trackTrigger}<span>{t('habits.trackTrigger')}</span>{/if}
            {#if h.trackRating}<span>{t('habits.rating')}</span>{/if}
            {#if h.trackAmount}<span>{t('habits.amount')}{h.unit ? `, ${h.unit}` : ''}</span>{/if}
            {#if h.archived}<span>{t('habits.archived')}</span>{/if}
          </span>
          <span class="acts">
            {@render act('chevronLeft', t('habits.moveUp'), () => shift(h, -1), i === 0)}
            {@render act('chevronRight', t('habits.moveDown'), () => shift(h, 1), i === shown.length - 1)}
            {@render act('edit', t('journal.edit'), () => openEdit(h))}
            {#if h.archived}
              {@render act('refresh', t('habits.unarchive'), () => setArchived(h, false))}
            {:else}
              {@render act('archive', t('habits.archive'), () => setArchived(h, true))}
            {/if}
            {@render act('trash', t('common.delete'), () => (deleting = h))}
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<HabitDialog open={dialogOpen} habit={editing} onclose={() => (dialogOpen = false)} onsaved={saved} />

<ConfirmDialog
  open={deleting !== null}
  title={t('habits.deleteTitle', { name: deleting?.name ?? '' })}
  confirmText={t('common.delete')}
  danger
  loading={busy}
  onconfirm={remove}
  oncancel={() => (deleting = null)}
>
  {t('habits.deleteText')}
</ConfirmDialog>

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
    gap: var(--s-4);
  }
  h1 {
    flex: 1;
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-sm);
    color: var(--c-muted);
    cursor: pointer;
  }
  .state {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
  }
  .list {
    margin: 0;
    padding: 0 var(--s-1) var(--s-4) 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    padding: var(--s-2) var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
    box-shadow: var(--shadow-sm);
  }
  li.archived {
    opacity: 0.6;
    box-shadow: none;
  }
  .dot {
    width: 14px;
    height: 14px;
    flex: none;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
  .name {
    padding: 0;
    border: 0;
    background: none;
    font-weight: 700;
    text-align: left;
    cursor: pointer;
  }
  .name:hover {
    text-decoration: underline;
  }
  .badge {
    padding: 0 var(--s-2);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    font-size: var(--fs-xs);
    font-weight: 700;
  }
  .badge.positive {
    background: var(--c-green-soft);
  }
  .badge.negative {
    background: var(--c-red-soft);
  }
  .tracks {
    flex: 1;
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-2);
    font-size: var(--fs-sm);
  }
  .tracks span + span::before {
    content: '·';
    margin-right: var(--s-2);
  }
  .acts {
    display: flex;
    gap: 2px;
  }
  .act {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
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
</style>
