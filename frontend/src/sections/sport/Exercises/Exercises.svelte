<script lang="ts">
  // «Упражнения» (§18.4, §18.5.7): справочник — переименовать, удалить неиспользованное.
  // Пополняется сам из тренировок. Клик по названию — прогресс упражнения (#/sport/exercises/<id>).
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { api } from '../../../lib/api/client'
  import type { Exercise } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { shortDay } from '../../../lib/dates'
  import { formatAmount } from '../../../lib/habits'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import Progress from './Progress.svelte'

  let list = $state<Exercise[] | null>(null)
  let error = $state<string | null>(null)
  let query = $state('')
  let renaming = $state<{ id: number; name: string } | null>(null)
  let deleting = $state<Exercise | null>(null)
  let busy = $state(false)

  const openId = $derived(/^\d+$/.test(route.rest) ? Number(route.rest) : null)
  const shown = $derived((list ?? []).filter((e) => e.name.toLowerCase().includes(query.trim().toLowerCase())))

  async function load() {
    try {
      list = await api.sport.exercises()
      error = null
    } catch (e) {
      error = errorText(e)
    }
  }
  onMount(load)

  async function rename(id: number, name: string) {
    try {
      await api.sport.renameExercise(id, name)
      renaming = null
      await load()
    } catch (e) {
      toasts.error(errorText(e))
    }
  }

  async function remove() {
    const e = deleting
    if (!e) return
    busy = true
    try {
      await api.sport.removeExercise(e.id)
      deleting = null
      await load()
    } catch (err) {
      toasts.error(errorText(err))
    } finally {
      busy = false
    }
  }

  const lastText = (l: NonNullable<Exercise['last']>) =>
    `${l.sets} × ${l.reps}${l.weight !== null ? ` × ${formatAmount(l.weight, i18n.locale)} ${t('sport.kg')}` : ''}`
</script>

<div class="page scroll">
  <header>
    <h1>{t('nav.sport.exercises')}</h1>
    <input class="input search" bind:value={query} placeholder={t('sport.search')} aria-label={t('sport.search')} />
  </header>
  <p class="muted hint">{t('sport.exercisesHint')}</p>

  {#if openId !== null}
    <div class="progress">
      <a class="back" href="#/sport/exercises">← {t('sport.progress.back')}</a>
      {#key openId}<Progress id={openId} />{/key}
    </div>
  {/if}

  {#if error && !list}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={load}>{t('common.retry')}</Button></div>
  {:else if !list}
    <div class="state"><Spinner size={28} /></div>
  {:else if list.length === 0}
    <p class="muted">{t('sport.noExercises')}</p>
  {:else}
    <ul>
      {#each shown as e (e.id)}
        <li in:fade={{ duration: 200 }}>
          <div class="main">
            {#if renaming?.id === e.id}
              <form class="rename" onsubmit={(ev) => (ev.preventDefault(), renaming && rename(e.id, renaming.name))}>
                <!-- svelte-ignore a11y_autofocus -->
                <input class="input" bind:value={renaming.name} maxlength="60" autofocus aria-label={t('sport.rename')} />
                <Button type="submit" size="sm" variant="primary" icon="check" aria-label={t('common.save')} />
                <Button size="sm" variant="ghost" icon="x" aria-label={t('common.cancel')} onclick={() => (renaming = null)} />
              </form>
            {:else}
              <span class="name-row">
                <a class="name" class:active={e.id === openId} href="#/sport/exercises/{e.id}" title={t('sport.progress.open')}>{e.name}</a>
                <button class="act small" aria-label={t('sport.rename')} title={t('sport.rename')} onclick={() => (renaming = { id: e.id, name: e.name })}>
                  <Icon name="edit" size={14} />
                </button>
              </span>
            {/if}
            <span class="muted meta">
              {t('sport.uses', { count: e.uses })}{e.lastDay ? ` · ${shortDay(e.lastDay, i18n.locale)}` : ''}
              {#if e.last}· {t('sport.lastTime', { text: lastText(e.last) })}{/if}
            </span>
          </div>
          <button class="act" aria-label={t('common.delete')} title={e.uses > 0 ? t('sport.inUse') : t('common.delete')} disabled={e.uses > 0}
            onclick={() => (deleting = e)}><Icon name="trash" size={16} /></button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<ConfirmDialog open={deleting !== null} title={t('sport.deleteExercise', { name: deleting?.name ?? '' })} confirmText={t('common.delete')} danger
  loading={busy} onconfirm={remove} oncancel={() => (deleting = null)}>
  {t('sport.deleteExerciseText')}
</ConfirmDialog>

<style>
  .page {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
    max-width: 820px;
    padding: 2px 6px var(--s-4) 2px;
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
  .search {
    width: 220px;
  }
  .hint {
    margin: 0;
  }
  .state {
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
    padding: var(--s-6);
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
    align-items: center;
    gap: var(--s-3);
    padding: var(--s-2) var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
    box-shadow: var(--shadow-sm);
  }
  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .name-row {
    display: flex;
    align-items: center;
    gap: var(--s-1);
  }
  .name {
    color: inherit;
    font-weight: 700;
    text-decoration: none;
  }
  .name:hover,
  .name.active {
    text-decoration: underline;
  }
  .act.small {
    width: 24px;
    height: 24px;
  }
  .progress {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .back {
    color: var(--c-muted);
    font-size: var(--fs-sm);
    font-weight: 600;
    text-decoration: none;
  }
  .meta {
    font-size: var(--fs-xs);
  }
  .rename {
    display: flex;
    gap: var(--s-1);
  }
  .rename .input {
    flex: 1;
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
  .act:hover:not(:disabled) {
    background: var(--c-surface-2);
    color: var(--c-text);
  }
  .act:disabled {
    opacity: 0.3;
    cursor: default;
  }
</style>
