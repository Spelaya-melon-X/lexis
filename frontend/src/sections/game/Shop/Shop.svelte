<script lang="ts">
  // «Награды» (§18.3, B4): баланс очков, магазин своих наград, журнал начислений и трат.
  // Покупка не уводит баланс в минус (проверяет бэк); по галочке — запись в дофамин дня (B15).
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { api } from '../../../lib/api/client'
  import type { LedgerEntry, Reward } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Icon, { type IconName } from '../../../lib/components/Icon.svelte'
  import Modal from '../../../lib/components/Modal.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'

  const PAGE = 20

  let rewards = $state<Reward[] | null>(null) // все, включая архив: порядок шлём полным списком
  let balance = $state(0)
  let ledger = $state<LedgerEntry[]>([])
  let next = $state<number | null>(null)
  let error = $state<string | null>(null)
  let showArchived = $state(false)

  let buying = $state<Reward | null>(null)
  let toDopamine = $state(false)
  let editing = $state<Reward | null>(null) // null + dialogOpen — новая
  let dialogOpen = $state(false)
  let title = $state('')
  let price = $state(5)
  let deleting = $state<Reward | null>(null)
  let busy = $state(false)

  const shown = $derived((rewards ?? []).filter((r) => showArchived || !r.archived))
  const archivedCount = $derived((rewards ?? []).filter((r) => r.archived).length)

  async function load() {
    try {
      const [rs, pts] = await Promise.all([api.focus.rewards(true), api.focus.points(undefined, PAGE)])
      rewards = rs
      balance = pts.balance
      ledger = pts.entries
      next = pts.next
      error = null
    } catch (e) {
      error = errorText(e)
    }
  }
  onMount(load)

  async function more() {
    if (next === null) return
    try {
      const pts = await api.focus.points(next, PAGE)
      ledger = [...ledger, ...pts.entries]
      next = pts.next
    } catch (e) {
      toasts.error(errorText(e))
    }
  }

  async function buy() {
    const r = buying
    if (!r) return
    busy = true
    try {
      const p = await api.focus.buy(r.id, toDopamine)
      toasts.success(t('game.shop.bought', { title: r.title }))
      if (p.dopamine === 'added') toasts.success(t('game.shop.dopamineAdded'))
      if (p.dopamine === 'full') toasts.error(t('game.shop.dopamineFull'))
      if (p.dopamine === 'failed') toasts.error(t('game.shop.dopamineFailed'))
      buying = null
      await load()
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
    }
  }

  function openDialog(r: Reward | null) {
    editing = r
    title = r?.title ?? ''
    price = r?.price ?? 5
    dialogOpen = true
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    try {
      if (editing) await api.focus.updateReward(editing.id, { title, price })
      else await api.focus.createReward(title, price)
      dialogOpen = false
      await load()
    } catch (err) {
      toasts.error(errorText(err))
    } finally {
      busy = false
    }
  }

  async function setArchived(r: Reward, archived: boolean) {
    try {
      await api.focus.updateReward(r.id, { archived })
      await load()
    } catch (e) {
      toasts.error(errorText(e))
    }
  }

  async function remove() {
    const r = deleting
    if (!r) return
    busy = true
    try {
      await api.focus.removeReward(r.id)
      deleting = null
      await load()
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
    }
  }

  /** Сдвиг среди видимых; на бэк — полный порядок (архивные остаются на местах), как в «Моих привычках». */
  async function shift(r: Reward, dir: -1 | 1) {
    if (!rewards) return
    const neighbour = shown[shown.indexOf(r) + dir]
    if (!neighbour) return
    const ids = rewards.map((x) => x.id)
    const i = ids.indexOf(r.id)
    const j = ids.indexOf(neighbour.id)
    ;[ids[i], ids[j]] = [ids[j]!, ids[i]!]
    try {
      await api.focus.reorderRewards(ids)
      await load()
    } catch (e) {
      toasts.error(errorText(e))
    }
  }

  const when = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
</script>

{#snippet act(icon: IconName, label: string, onclick: () => void, disabled = false)}
  <button class="act" aria-label={label} title={label} {disabled} {onclick}><Icon name={icon} size={16} /></button>
{/snippet}

<div class="page scroll">
  <header>
    <h1>{t('nav.game.shop')}</h1>
    <div class="balance" aria-label={t('game.shop.balance')}>
      <Icon name="trophy" size={20} />
      <strong>{balance}</strong>
      <span class="muted">{t('game.shop.points', { count: balance })}</span>
    </div>
  </header>
  <p class="muted hint">{t('game.shop.hint')}</p>

  {#if error && !rewards}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={load}>{t('common.retry')}</Button></div>
  {:else if !rewards}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="toolbar">
      {#if archivedCount > 0}
        <label class="toggle"><input type="checkbox" bind:checked={showArchived} /> {t('game.shop.showArchived')} ({archivedCount})</label>
      {/if}
      <Button variant="primary" icon="plus" onclick={() => openDialog(null)}>{t('game.shop.new')}</Button>
    </div>

    {#if shown.length === 0}
      <p class="muted empty">{t('game.shop.empty')}</p>
    {:else}
      <ul class="rewards">
        {#each shown as r, i (r.id)}
          <li class:archived={r.archived} in:fade={{ duration: 200 }}>
            <button class="title" onclick={() => openDialog(r)}>{r.title}</button>
            <span class="price">{r.price} <Icon name="trophy" size={14} /></span>
            {#if !r.archived}
              <Button size="sm" variant="primary" disabled={balance < r.price} onclick={() => ((buying = r), (toDopamine = false))}>
                {t('game.shop.buy')}
              </Button>
            {/if}
            <span class="acts">
              {@render act('chevronLeft', t('game.moveUp'), () => shift(r, -1), i === 0)}
              {@render act('chevronRight', t('game.moveDown'), () => shift(r, 1), i === shown.length - 1)}
              {#if r.archived}
                {@render act('refresh', t('game.shop.unarchive'), () => setArchived(r, false))}
              {:else}
                {@render act('archive', t('game.shop.archive'), () => setArchived(r, true))}
              {/if}
              {@render act('trash', t('common.delete'), () => (deleting = r))}
            </span>
          </li>
        {/each}
      </ul>
    {/if}

    <section class="ledger">
      <h2>{t('game.shop.ledger')}</h2>
      {#if ledger.length === 0}
        <p class="muted">{t('game.shop.ledgerEmpty')}</p>
      {:else}
        <ul>
          {#each ledger as x (x.id)}
            <li in:fade={{ duration: 200 }}>
              <span class="delta" class:plus={x.delta > 0}>{x.delta > 0 ? `+${x.delta}` : x.delta}</span>
              <span class="what">{x.kind === 'focus' ? t('game.shop.earned', { title: x.title }) : t('game.shop.spent', { title: x.title })}</span>
              <span class="muted when">{when(x.at)}</span>
            </li>
          {/each}
        </ul>
        {#if next !== null}<Button size="sm" variant="ghost" onclick={more}>{t('game.shop.more')}</Button>{/if}
      {/if}
    </section>
  {/if}
</div>

<ConfirmDialog
  open={buying !== null}
  title={t('game.shop.buyTitle', { title: buying?.title ?? '', price: buying?.price ?? 0 })}
  confirmText={t('game.shop.buy')}
  loading={busy}
  onconfirm={buy}
  oncancel={() => (buying = null)}
>
  <p>{t('game.shop.after', { balance: balance - (buying?.price ?? 0) })}</p>
  <label class="check"><input type="checkbox" bind:checked={toDopamine} /> {t('game.shop.toDopamine')}</label>
</ConfirmDialog>

<Modal open={dialogOpen} title={editing ? t('game.shop.edit') : t('game.shop.new')} onclose={() => (dialogOpen = false)} width={420}>
  <form id="reward-form" class="form" onsubmit={save}>
    <label>
      <span>{t('game.shop.title')}</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input class="input" bind:value={title} maxlength="100" placeholder={t('game.shop.titlePlaceholder')} required autofocus />
    </label>
    <label>
      <span>{t('game.shop.price')}</span>
      <input class="input" type="number" min="1" max="100000" bind:value={price} required />
      <small class="muted">{t('game.shop.priceHint')}</small>
    </label>
  </form>
  {#snippet footer()}
    <Button variant="ghost" onclick={() => (dialogOpen = false)} disabled={busy}>{t('common.cancel')}</Button>
    <Button type="submit" form="reward-form" variant="primary" loading={busy}>{editing ? t('common.save') : t('game.create')}</Button>
  {/snippet}
</Modal>

<ConfirmDialog
  open={deleting !== null}
  title={t('game.shop.deleteTitle', { title: deleting?.title ?? '' })}
  confirmText={t('common.delete')}
  danger
  loading={busy}
  onconfirm={remove}
  oncancel={() => (deleting = null)}
>
  {t('game.shop.deleteText')}
</ConfirmDialog>

<style>
  .page {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
    max-width: 760px;
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
  h2 {
    font-size: var(--fs-lg);
  }
  .balance {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-4);
    border: var(--bw) solid var(--c-border);
    border-radius: 999px;
    background: var(--c-amber-soft);
    box-shadow: var(--shadow-sm);
  }
  .balance strong {
    font-size: var(--fs-xl);
    font-variant-numeric: tabular-nums;
  }
  .hint,
  .empty {
    margin: 0;
  }
  .state {
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
    padding: var(--s-6);
  }
  .toolbar {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--s-4);
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-sm);
    color: var(--c-muted);
    cursor: pointer;
  }
  .rewards,
  .ledger ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .rewards li {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    padding: var(--s-2) var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
    box-shadow: var(--shadow-sm);
  }
  .rewards li.archived {
    opacity: 0.6;
    box-shadow: none;
  }
  .title {
    flex: 1;
    min-width: 0;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    font-weight: 700;
    text-align: left;
    cursor: pointer;
  }
  .title:hover {
    text-decoration: underline;
  }
  .price {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
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
  .ledger {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    margin-top: var(--s-4);
  }
  .ledger li {
    display: flex;
    align-items: baseline;
    gap: var(--s-3);
    padding: var(--s-1) 0;
    border-bottom: 1px solid var(--c-line);
    font-size: var(--fs-sm);
  }
  .delta {
    min-width: 40px;
    font-weight: 800;
    color: var(--c-red);
    font-variant-numeric: tabular-nums;
  }
  .delta.plus {
    color: var(--c-green);
  }
  .what {
    flex: 1;
    min-width: 0;
  }
  .when {
    white-space: nowrap;
  }
  .check {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    color: var(--c-text);
    cursor: pointer;
  }
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  .form label {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  .form label > span {
    font-weight: 600;
    font-size: var(--fs-sm);
  }
  small {
    font-size: var(--fs-xs);
  }
</style>
