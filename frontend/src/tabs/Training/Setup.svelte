<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../../lib/api/client'
  import type { Group, Strategy } from '../../lib/api/types'
  import Button from '../../lib/components/Button.svelte'
  import Card from '../../lib/components/Card.svelte'
  import Icon from '../../lib/components/Icon.svelte'
  import Spinner from '../../lib/components/Spinner.svelte'
  import { errorText, t } from '../../lib/i18n/i18n.svelte'
  import { route } from '../../lib/state/route.svelte'
  import { session } from '../../lib/state/session.svelte'
  import { toasts } from '../../lib/state/toasts.svelte'

  const STRATEGIES: Strategy[] = ['en_ru', 'ru_en', 'context']
  const STRATEGY_KEY = 'lexis.strategy'

  let groups = $state<Group[] | null>(null)
  let loadError = $state<string | null>(null)
  // Группа: из адреса (#training/12 — кнопка «Тренировать» в группах), иначе из прошлой сессии.
  let groupId = $state<number | null>(
    /^\d+$/.test(route.rest) ? Number(route.rest) : (session.active?.groupId ?? null),
  )
  let strategy = $state<Strategy>(savedStrategy())
  let free = $state(false)
  let starting = $state(false)
  let nothingDue = $state(false)

  const selected = $derived(groups?.find((g) => g.id === groupId) ?? null)

  function savedStrategy(): Strategy {
    try {
      const v = localStorage.getItem(STRATEGY_KEY)
      if (v === 'en_ru' || v === 'ru_en' || v === 'context') return v
    } catch {
      // нет localStorage — по умолчанию
    }
    return 'en_ru'
  }

  async function load() {
    try {
      groups = await api.groups.list()
      loadError = null
    } catch (e) {
      loadError = errorText(e)
    }
  }
  onMount(load)

  async function start(asFree = free) {
    if (!selected || starting) return
    starting = true
    nothingDue = false
    try {
      localStorage.setItem(STRATEGY_KEY, strategy)
    } catch {
      // не страшно
    }
    try {
      const res = await api.training.start(selected.id, strategy, asFree)
      if (res.sessionId === null) {
        nothingDue = true // сервер не создал сессию: на сегодня карточек нет (§7)
        return
      }
      session.begin({ sessionId: res.sessionId, groupId: selected.id, groupName: selected.name, strategy, free: asFree }, res.cards)
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      starting = false
    }
  }
</script>

<div class="setup scroll">
  {#if loadError && !groups}
    <div class="state"><p>{loadError}</p><Button icon="refresh" onclick={load}>{t('common.retry')}</Button></div>
  {:else if !groups}
    <div class="state"><Spinner size={28} /></div>
  {:else if groups.length === 0}
    <div class="state">
      <Icon name="folder" size={32} />
      <p class="muted">{t('training.noGroups')}</p>
      <Button variant="primary" icon="cards" onclick={() => route.go('add')}>{t('groups.goAdd')}</Button>
    </div>
  {:else}
    <section>
      <h2>{t('training.group')}</h2>
      <div class="groups">
        {#each groups as g (g.id)}
          <button class="group" class:active={g.id === groupId} aria-pressed={g.id === groupId}
            onclick={() => { groupId = g.id; nothingDue = false }}>
            <span class="dot" style:background={g.color ?? 'var(--c-surface)'}></span>
            <span class="name">#{g.name}</span>
            <span class="due" class:zero={g.due === 0}>{g.due}</span>
          </button>
        {/each}
      </div>
    </section>

    <section>
      <h2>{t('training.strategy')}</h2>
      <div class="strategies">
        {#each STRATEGIES as s (s)}
          <button class="strategy" class:active={s === strategy} aria-pressed={s === strategy} onclick={() => (strategy = s)}>
            <span class="s-title">{t(`training.strategies.${s}.title`)}</span>
            <span class="s-desc muted">{t(`training.strategies.${s}.desc`)}</span>
          </button>
        {/each}
      </div>
    </section>

    <label class="free">
      <input type="checkbox" bind:checked={free} />
      <span><strong>{t('training.free')}</strong> <span class="muted">— {t('training.freeHint')}</span></span>
    </label>

    {#if nothingDue}
      <Card class="nothing">
        <Icon name="check" size={28} />
        <div>
          <p><strong>{t('training.nothingDue')}</strong></p>
          <p class="muted">{t('training.nothingDueHint')}</p>
        </div>
        <Button icon="play" loading={starting} onclick={() => start(true)}>{t('training.startFree')}</Button>
      </Card>
    {/if}

    <div class="actions">
      <Button variant="primary" size="lg" icon="play" loading={starting} disabled={!selected || selected.wordCount === 0} onclick={() => start()}>
        {selected ? t('training.start') : t('training.chooseGroup')}
      </Button>
    </div>
  {/if}
</div>

<style>
  .setup {
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--s-5);
    max-width: 760px;
    width: 100%;
    margin: 0 auto;
    padding: 2px 6px 6px 2px;
  }
  h2 {
    font-size: var(--fs-md);
    margin-bottom: var(--s-3);
  }
  .groups {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-2);
  }
  .group,
  .strategy {
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    cursor: pointer;
    transition:
      transform var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out);
  }
  .group:hover,
  .strategy:hover {
    transform: translate(-1px, -1px);
    box-shadow: var(--shadow-sm);
  }
  .group.active,
  .strategy.active {
    background: var(--c-green-soft);
    box-shadow: var(--shadow);
  }
  .group {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-3);
    font-weight: 600;
  }
  .dot {
    width: 10px;
    height: 10px;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
  .due {
    min-width: 22px;
    padding: 0 6px;
    border-radius: 999px;
    background: var(--c-text);
    color: var(--c-bg);
    font-size: var(--fs-xs);
    font-weight: 700;
  }
  .due.zero {
    background: var(--c-line);
    color: var(--c-muted);
  }
  .strategies {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--s-3);
  }
  .strategy {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    padding: var(--s-3) var(--s-4);
    text-align: left;
  }
  .s-title {
    font-size: var(--fs-lg);
    font-weight: 800;
  }
  .s-desc {
    font-size: var(--fs-sm);
  }
  .free {
    display: flex;
    align-items: baseline;
    gap: var(--s-2);
    cursor: pointer;
  }
  .free input {
    width: 18px;
    height: 18px;
    accent-color: var(--c-green);
  }
  :global(.nothing) {
    display: flex;
    align-items: center;
    gap: var(--s-4);
  }
  :global(.nothing) > div {
    flex: 1;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
  }
  .state {
    height: 100%;
    display: grid;
    place-items: center;
    align-content: center;
    gap: var(--s-3);
    text-align: center;
  }
  @media (max-width: 860px) {
    .strategies {
      grid-template-columns: 1fr;
    }
  }
</style>
