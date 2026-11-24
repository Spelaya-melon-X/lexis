<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../../../lib/api/client'
  import type { Group, TrainingDirection } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { pathTo, visibleRows } from '../../../lib/groupTree'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { route } from '../../../lib/state/route.svelte'
  import { session } from '../../../lib/state/session.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import DirectionSelect from './DirectionSelect.svelte'

  let groups = $state<Group[] | null>(null)
  let loadError = $state<string | null>(null)
  // Группа: из адреса (#training/12 — кнопка «Тренировать» в группах), иначе из прошлой сессии.
  let groupId = $state<number | null>(
    /^\d+$/.test(route.rest) ? Number(route.rest) : (session.active?.groupId ?? null),
  )
  let free = $state(false)
  let strategy = $state<TrainingDirection>(session.active?.strategy ?? 'en_ru')
  let starting = $state(false)
  let nothingDue = $state(false)

  const selected = $derived(groups?.find((g) => g.id === groupId) ?? null)
  // Порядок дерева (родитель, потом его подгруппы); у подгруппы перед именем — путь предков.
  // Тренировка группы берёт слова всех её подгрупп (§18.2), due — тоже по поддереву.
  const rows = $derived(groups ? visibleRows(groups) : [])

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
    const group = selected
    const direction = strategy
    if (!group || starting) return
    starting = true
    nothingDue = false
    try {
      const res = await api.training.start(group.id, direction, asFree)
      if (res.sessionId === null) {
        nothingDue = true // сервер не создал сессию: на сегодня карточек нет (§7)
        return
      }
      session.begin({ sessionId: res.sessionId, groupId: group.id, groupName: group.name, strategy: direction, free: asFree }, res.cards)
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
      <Button variant="primary" icon="cards" onclick={() => route.go('english', 'add')}>{t('groups.goAdd')}</Button>
    </div>
  {:else}
    <section>
      <h2>{t('training.group')}</h2>
      <div class="groups">
        {#each rows as { group: g, depth } (g.id)}
          <button class="group" class:active={g.id === groupId} aria-pressed={g.id === groupId}
            onclick={() => { groupId = g.id; nothingDue = false }}>
            <span class="dot" style:background={g.color ?? 'var(--c-surface)'}></span>
            {#if depth > 0}
              <span class="path muted">{pathTo(groups, g.id).slice(0, -1).map((p) => p.name).join(' › ')} ›</span>
            {/if}
            <span class="name">#{g.name}</span>
            <span class="due" class:zero={g.due === 0}>{g.due}</span>
          </button>
        {/each}
      </div>
    </section>

    <DirectionSelect bind:value={strategy} disabled={starting} />

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
  .group {
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    cursor: pointer;
    transition:
      transform var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out);
  }
  .group:hover {
    transform: translate(-1px, -1px);
    box-shadow: var(--shadow-sm);
  }
  .group.active {
    background: var(--c-green-soft);
    box-shadow: var(--shadow);
  }
  .path {
    font-weight: 400;
    font-size: var(--fs-sm);
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
</style>
