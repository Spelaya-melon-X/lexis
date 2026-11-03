<script lang="ts">
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { api } from '../../lib/api/client'
  import type { Group } from '../../lib/api/types'
  import Button from '../../lib/components/Button.svelte'
  import Icon from '../../lib/components/Icon.svelte'
  import LevelBars from '../../lib/components/LevelBars.svelte'
  import Spinner from '../../lib/components/Spinner.svelte'
  import { errorText, t } from '../../lib/i18n/i18n.svelte'
  import { route } from '../../lib/state/route.svelte'
  import GroupDetail from './GroupDetail.svelte'

  let groups = $state<Group[] | null>(null)
  let error = $state<string | null>(null)

  // Выбранная группа — в адресе (#groups/12): работают «назад» и обновление страницы.
  const selectedId = $derived(/^\d+$/.test(route.rest) ? Number(route.rest) : null)

  async function load() {
    try {
      groups = await api.groups.list()
      error = null
    } catch (e) {
      error = errorText(e)
    }
  }
  onMount(load)
</script>

<section class="groups" class:has-detail={selectedId !== null}>
  <aside class="list scroll">
    {#if error && !groups}
      <div class="state">
        <p>{error}</p>
        <Button icon="refresh" onclick={load}>{t('common.retry')}</Button>
      </div>
    {:else if !groups}
      <div class="state"><Spinner size={28} /></div>
    {:else if groups.length === 0}
      <div class="state">
        <Icon name="folder" size={32} />
        <p class="muted">{t('groups.empty')}</p>
        <Button variant="primary" icon="cards" onclick={() => route.go('add')}>{t('groups.goAdd')}</Button>
      </div>
    {:else}
      <ul>
        {#each groups as g (g.id)}
          <li in:fade={{ duration: 200 }}>
            <a class="item" class:active={g.id === selectedId} href="#groups/{g.id}" aria-current={g.id === selectedId ? 'page' : undefined}>
              <div class="row">
                <span class="dot" style:background={g.color ?? 'var(--c-surface)'}></span>
                <span class="name">#{g.name}</span>
                <span class="status" class:done={g.repeatedToday}>
                  {#if g.repeatedToday}<Icon name="check" size={14} />{/if}
                  {g.repeatedToday ? t('groups.repeated') : t('groups.notRepeated')}
                </span>
              </div>
              <div class="meta muted">
                <span>{t('common.words', { count: g.wordCount })}</span>
                {#if g.due > 0}<span class="due">{t('groups.due', { count: g.due })}</span>{/if}
              </div>
              <LevelBars levels={g.levels} compact />
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  </aside>

  <div class="detail">
    {#if selectedId !== null}
      {#key selectedId}
        <div class="detail-inner" in:fade={{ duration: 200 }}>
          <GroupDetail id={selectedId} onchanged={load} />
        </div>
      {/key}
    {:else if groups && groups.length > 0}
      <div class="state muted"><Icon name="chevronLeft" size={28} /><p>{t('groups.select')}</p></div>
    {/if}
  </div>
</section>

<style>
  .groups {
    display: grid;
    grid-template-columns: 320px minmax(0, 1fr);
    gap: var(--s-5);
    height: 100%;
    min-height: 0;
  }
  .list {
    height: 100%;
    padding: 2px 6px 6px 2px;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .item {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding: var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    color: inherit;
    text-decoration: none;
    transition:
      transform var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out);
  }
  .item:hover {
    transform: translate(-1px, -1px);
    box-shadow: var(--shadow-sm);
  }
  .item.active {
    box-shadow: var(--shadow);
    background: var(--c-green-soft);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  .dot {
    flex: none;
    width: 12px;
    height: 12px;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
  .name {
    flex: 1;
    min-width: 0;
    font-weight: 700;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .status {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 0 var(--s-2);
    border: var(--bw) solid var(--c-red);
    border-radius: 999px;
    color: var(--c-red);
    font-size: var(--fs-xs);
    font-weight: 700;
    white-space: nowrap;
  }
  .status.done {
    border-color: var(--c-green);
    color: var(--c-green);
  }
  .meta {
    display: flex;
    gap: var(--s-3);
    font-size: var(--fs-sm);
  }
  .due {
    color: var(--c-text);
    font-weight: 600;
  }
  .detail {
    min-height: 0;
    height: 100%;
  }
  .detail-inner {
    height: 100%;
  }
  .state {
    height: 100%;
    display: grid;
    place-items: center;
    align-content: center;
    gap: var(--s-3);
    text-align: center;
  }

  /* Узко: либо список, либо детали (назад — ссылкой «Все группы» в деталях) */
  @media (max-width: 860px) {
    .groups {
      grid-template-columns: 1fr;
    }
    .groups.has-detail .list,
    .groups:not(.has-detail) .detail {
      display: none;
    }
  }
</style>
