<script lang="ts">
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { api } from '../../../lib/api/client'
  import type { Group } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import LevelBars from '../../../lib/components/LevelBars.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { canMove, dropParent, pathTo, visibleRows } from '../../../lib/groupTree'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import GroupDetail from './GroupDetail.svelte'
  import MoveGroupDialog from './MoveGroupDialog.svelte'
  import NewGroupDialog from './NewGroupDialog.svelte'

  const COLLAPSED_KEY = 'lexis.groupsCollapsed'

  let groups = $state<Group[] | null>(null)
  let error = $state<string | null>(null)
  let collapsed = $state(new Set<number>(savedCollapsed()))
  let moving = $state<Group | null>(null)
  let creating = $state(false)

  // Перетаскивание (§18.2): что тащим и куда упадёт. into — подсветка строки, иначе линия «рядом».
  let dragId = $state<number | null>(null)
  let drop = $state<{ row: number | 'root'; parent: number | null; into: boolean } | null>(null)

  // Выбранная группа — в адресе (#/english/groups/12): работают «назад» и обновление страницы.
  const selectedId = $derived(/^\d+$/.test(route.rest) ? Number(route.rest) : null)
  const rows = $derived(groups ? visibleRows(groups, collapsed) : [])
  const dragged = $derived(groups?.find((g) => g.id === dragId) ?? null)

  function savedCollapsed(): number[] {
    try {
      const v: unknown = JSON.parse(localStorage.getItem(COLLAPSED_KEY) ?? '[]')
      return Array.isArray(v) ? v.filter((x): x is number => typeof x === 'number') : []
    } catch {
      return []
    }
  }

  function setCollapsed(next: Set<number>) {
    collapsed = next
    try {
      localStorage.setItem(COLLAPSED_KEY, JSON.stringify([...next]))
    } catch {
      // не страшно: просто не запомнится
    }
  }

  function toggle(id: number) {
    const next = new Set(collapsed)
    if (!next.delete(id)) next.add(id)
    setCollapsed(next)
  }

  // Открыли группу внутри свёрнутой (ссылкой, из тренировки) — разворачиваем предков, чтобы её было видно.
  $effect(() => {
    if (!groups || selectedId === null) return
    const hidden = pathTo(groups, selectedId).slice(0, -1).filter((g) => collapsed.has(g.id))
    if (hidden.length > 0) setCollapsed(new Set([...collapsed].filter((id) => !hidden.some((g) => g.id === id))))
  })

  async function load() {
    try {
      groups = await api.groups.list()
      error = null
    } catch (e) {
      error = errorText(e)
    }
  }
  onMount(load)

  function onDragStart(e: DragEvent, g: Group) {
    dragId = g.id
    e.dataTransfer?.setData('text/plain', String(g.id))
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
  }

  function onDragOver(e: DragEvent, g: Group) {
    if (!groups || dragId === null) return
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    const target = dropParent(g, e.clientY - rect.top, rect.height)
    if (!canMove(groups, dragId, target.parent)) {
      drop = null
      return // без preventDefault браузер покажет «нельзя»
    }
    e.preventDefault()
    drop = { row: g.id, ...target }
  }

  function onDragOverRoot(e: DragEvent) {
    if (!groups || dragId === null || !canMove(groups, dragId, null)) return
    e.preventDefault()
    drop = { row: 'root', parent: null, into: true }
  }

  function onDragEnd() {
    dragId = null
    drop = null
  }

  async function onDrop(e: DragEvent) {
    e.preventDefault()
    const id = dragId
    const target = drop
    onDragEnd()
    if (id === null || !target) return
    try {
      await api.groups.update(id, { parentId: target.parent })
      if (target.parent !== null && collapsed.has(target.parent)) toggle(target.parent) // показать, куда упала
      toasts.success(t('groups.moved'))
      await load()
    } catch (err) {
      toasts.error(errorText(err))
    }
  }

  async function onMoved() {
    moving = null
    await load()
  }

  async function onCreated(g: Group) {
    creating = false
    toasts.success(t('groups.created'))
    await load()
    route.go('english', 'groups', String(g.id))
  }
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
        <Button variant="primary" icon="cards" onclick={() => route.go('english', 'add')}>{t('groups.goAdd')}</Button>
        <Button icon="plus" onclick={() => (creating = true)}>{t('groups.newGroup')}</Button>
      </div>
    {:else}
      <div class="toolbar">
        <Button size="sm" icon="plus" onclick={() => (creating = true)}>{t('groups.newGroup')}</Button>
      </div>
      {#if dragged && dragged.parentId !== null}
        <div
          class="root-zone"
          class:over={drop?.row === 'root'}
          role="region"
          aria-label={t('groups.dropRoot')}
          ondragover={onDragOverRoot}
          ondragleave={() => drop?.row === 'root' && (drop = null)}
          ondrop={onDrop}
          transition:fade={{ duration: 150 }}
        >
          {t('groups.dropRoot')}
        </div>
      {/if}
      <ul>
        {#each rows as { group: g, depth, hasChildren } (g.id)}
          <li
            in:fade={{ duration: 200 }}
            style:--depth={depth}
            class:dragging={g.id === dragId}
            class:drop-into={drop?.row === g.id && drop.into}
            class:drop-beside={drop?.row === g.id && !drop.into}
            draggable="true"
            ondragstart={(e) => onDragStart(e, g)}
            ondragover={(e) => onDragOver(e, g)}
            ondragleave={() => drop?.row === g.id && (drop = null)}
            ondrop={onDrop}
            ondragend={onDragEnd}
          >
            {#if hasChildren}
              <button class="toggle" aria-expanded={!collapsed.has(g.id)}
                aria-label={collapsed.has(g.id) ? t('groups.expand') : t('groups.collapse')}
                title={collapsed.has(g.id) ? t('groups.expand') : t('groups.collapse')}
                onclick={() => toggle(g.id)}>
                <Icon name={collapsed.has(g.id) ? 'chevronRight' : 'chevronDown'} size={16} />
              </button>
            {:else}
              <span class="toggle-space"></span>
            {/if}
            <a class="item" class:active={g.id === selectedId} href="#/english/groups/{g.id}" aria-current={g.id === selectedId ? 'page' : undefined}>
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
                {#if hasChildren && g.ownWordCount !== g.wordCount}<span>{t('groups.own', { count: g.ownWordCount })}</span>{/if}
                {#if g.due > 0}<span class="due">{t('groups.due', { count: g.due })}</span>{/if}
              </div>
              <LevelBars levels={g.levels} compact />
            </a>
            <button class="more" aria-label="{t('groups.move')} #{g.name}" title={t('groups.move')} onclick={() => (moving = g)}>
              <Icon name="more" size={18} />
            </button>
          </li>
        {/each}
      </ul>
      <p class="hint muted">{t('groups.treeHint')}</p>
    {/if}
  </aside>

  <div class="detail">
    {#if selectedId !== null}
      {#key selectedId}
        <div class="detail-inner" in:fade={{ duration: 200 }}>
          <GroupDetail id={selectedId} groups={groups ?? []} onchanged={load} onmove={(g) => (moving = g)} />
        </div>
      {/key}
    {:else if groups && groups.length > 0}
      <div class="state muted"><Icon name="chevronLeft" size={28} /><p>{t('groups.select')}</p></div>
    {/if}
  </div>
</section>

<MoveGroupDialog group={moving} groups={groups ?? []} onclose={() => (moving = null)} onmoved={onMoved} />
<NewGroupDialog open={creating} groups={groups ?? []} parent={selectedId} onclose={() => (creating = false)} oncreated={onCreated} />

<style>
  .groups {
    display: grid;
    grid-template-columns: 340px minmax(0, 1fr);
    gap: var(--s-5);
    height: 100%;
    min-height: 0;
  }
  .list {
    height: 100%;
    padding: 2px 6px 6px 2px;
  }
  .toolbar {
    display: flex;
    justify-content: flex-end;
    margin-bottom: var(--s-2);
  }
  .root-zone {
    margin-bottom: var(--s-2);
    padding: var(--s-3);
    border: var(--bw) dashed var(--c-border);
    border-radius: var(--radius-sm);
    color: var(--c-muted);
    font-size: var(--fs-sm);
    text-align: center;
    transition: background var(--dur-fast) var(--ease-out);
  }
  .root-zone.over {
    background: var(--c-green-soft);
    color: var(--c-text);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  li {
    position: relative;
    display: grid;
    grid-template-columns: 20px minmax(0, 1fr) auto;
    align-items: start;
    gap: var(--s-1);
    padding-left: calc(var(--depth) * var(--s-4));
  }
  li.dragging {
    opacity: 0.5;
  }
  /* «рядом»: линия у левого края уровня, куда упадёт группа */
  li.drop-beside::after {
    content: '';
    position: absolute;
    left: calc(var(--depth) * var(--s-4) + 20px);
    right: 0;
    bottom: calc(-1 * var(--s-1) - 1px);
    height: var(--bw);
    background: var(--c-green);
  }
  li.drop-into .item {
    background: var(--c-green-soft);
    box-shadow: var(--shadow-sm);
  }
  .toggle,
  .more {
    display: grid;
    place-items: center;
    margin-top: var(--s-3);
    padding: 0;
    border: 0;
    background: none;
    color: var(--c-muted);
    cursor: pointer;
    border-radius: var(--radius-sm);
  }
  .toggle:hover,
  .more:hover {
    color: var(--c-text);
    background: var(--c-surface-2);
  }
  .more {
    width: 28px;
    height: 28px;
    margin-top: var(--s-2);
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
      box-shadow var(--dur-fast) var(--ease-out),
      background var(--dur-fast) var(--ease-out);
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
    flex-wrap: wrap;
    gap: 0 var(--s-3);
    font-size: var(--fs-sm);
  }
  .due {
    color: var(--c-text);
    font-weight: 600;
  }
  .hint {
    margin: var(--s-4) 0 0;
    font-size: var(--fs-xs);
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
