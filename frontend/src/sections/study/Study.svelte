<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { api } from '../../lib/api/client'
  import type {
    StudyData,
    StudyGroup,
    StudyCard,
    StudyInputs,
    GrammarRule,
    StudyPracticeCard,
    StudyKnowledge,
  } from '../../lib/api/types'
  import { studyCards, studyLibrary } from '../../lib/study'
  import {
    visibleRows,
    canMove,
    canCreateIn,
    pathTo,
  } from '../../lib/groupTree'
  import { t, errorText } from '../../lib/i18n/i18n.svelte'
  import { toasts } from '../../lib/state/toasts.svelte'
  import Button from '../../lib/components/Button.svelte'
  import Icon from '../../lib/components/Icon.svelte'
  import Spinner from '../../lib/components/Spinner.svelte'
  import Markdown from '../../lib/components/Markdown.svelte'
  import Modal from '../../lib/components/Modal.svelte'
  import ConfirmDialog from '../../lib/components/ConfirmDialog.svelte'
  import CardEditor from './CardEditor.svelte'
  import BulkEditor from './BulkEditor.svelte'
  import Practice from './Practice.svelte'
  import GrammarReader from './GrammarReader.svelte'

  let { grammar = false }: { grammar?: boolean } = $props()
  const viewId = $props.id()
  let data = $state<StudyData>({ groups: [], cards: [] }),
    loading = $state(true),
    busy = $state(false),
    failure = $state('')
  let group = $state<number | null>(null),
    selected = $state(new Set<number>()),
    reverse = $state(false)
  let editing = $state<StudyCard | null>(null),
    bulk = $state(false),
    session = $state<StudyPracticeCard[] | null>(null),
    reading = $state<{ index: number; trigger: string } | null>(null)
  let groupEditor = $state<StudyGroup | null>(null),
    deleting = $state<{ kind: 'groups' | 'cards'; id: number } | null>(null)
  let limit = $state(10),
    knowledge = $state<StudyKnowledge[]>([])
  const scoreById = $derived(new Map(knowledge.map((k) => [k.cardId, k.score])))
  const library = $derived(studyLibrary(data, grammar))
  const rows = $derived(visibleRows(library.groups))
  const destinationGroup = $derived(group ?? (grammar ? rows[0]?.group.id : undefined))
  const cards = $derived(studyCards(library, group))
  const rules = $derived(cards.filter((c): c is GrammarRule => c.kind === 'grammar_rule'))
  const practiceCards = $derived(cards.filter((c) => c.kind !== 'grammar_rule'))
  const current = $derived(library.groups.find((g) => g.id === group))
  const available = $derived(selected.size || cards.length)
  const batchSize = $derived(Math.min(limit || 1, available))
  const targets = $derived(
    groupEditor
      ? library.groups.filter((g) =>
          groupEditor!.id === 0
            ? canCreateIn(library.groups, g.id)
            : g.id !== groupEditor!.id &&
              (g.id === groupEditor!.parentId ||
                canMove(library.groups, groupEditor!.id, g.id)),
        )
      : [],
  )
  async function load() {
    loading = true
    failure = ''
    try {
      ;[data, knowledge] = await Promise.all([
        api.study.get(),
        grammar ? Promise.resolve([]) : api.study.knowledge(),
      ])
    } catch (e) {
      failure = errorText(e)
    } finally {
      loading = false
    }
  }
  onMount(load)
  async function run(action: () => Promise<StudyData>) {
    if (busy) return false
    busy = true
    try {
      data = await action()
      const visible = new Set(studyCards(studyLibrary(data, grammar), group).map((c) => c.id))
      selected = new Set([...selected].filter((id) => visible.has(id)))
      return true
    } catch (e) {
      toasts.error(errorText(e))
      return false
    } finally {
      busy = false
    }
  }
  function choose(id: number | null) {
    group = id
    selected = new Set()
  }
  async function closeReader() {
    const trigger = reading?.trigger
    reading = null
    await tick()
    if (trigger) document.getElementById(trigger)?.focus()
  }
  function toggle(id: number) {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    selected = next
  }
  function newCard() {
    const groupId = destinationGroup
    if (groupId === undefined) return
    const fields = { id: 0, groupId, front: '', back: '' }
    editing = grammar
      ? { ...fields, kind: 'grammar_rule', content: { formula: '' } }
      : { ...fields, kind: 'definition' }
  }
  async function saveCard(c: StudyCard) {
    const { id, groupId, front, back } = c
    const input: StudyInputs['cards'] = c.kind === 'grammar_rule'
      ? { groupId, kind: c.kind, front, back, content: c.content }
      : { groupId, kind: c.kind, front, back }
    if (
      await run(() =>
        id === 0
          ? api.study.create('cards', input)
          : api.study.update('cards', id, input),
      )
    )
      editing = null
  }
  async function saveGroup() {
    const g = groupEditor
    if (!g) return
    const { id, title, parentId } = g
    if (
      await run(() =>
        id === 0
          ? api.study.create('groups', { title, parentId, ...(grammar ? { grammar } : {}) })
          : api.study.update('groups', id, { title, parentId }),
      )
    )
      groupEditor = null
  }
  async function remove() {
    const d = deleting
    if (!d) return
    if (await run(() => api.study.remove(d.kind, d.id))) {
      if (d.kind === 'groups' && group === d.id) choose(null)
      selected = new Set(
        [...selected].filter((id) => data.cards.some((c) => c.id === id)),
      )
      deleting = null
    }
  }
  async function requestBatch() {
    const result = await api.study.practice({
      groupId: group,
      cardIds: [...selected],
      limit,
    })
    return result.cards
  }
  async function start() {
    if (busy || !Number.isInteger(limit) || limit < 1 || limit > 10000) return
    busy = true
    try {
      const next = await requestBatch()
      if (next.length) session = next
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
    }
  }
  async function closePractice() {
    session = null
    selected = new Set()
    try {
      knowledge = await api.study.knowledge()
    } catch (e) {
      toasts.error(errorText(e))
    }
  }
</script>

{#if session}<Practice
    cards={session}
    {reverse}
    onclose={closePractice}
    onrestart={requestBatch}
  />{:else if reading !== null && rules.length}
  <GrammarReader {rules} initialIndex={reading.index} onclose={closeReader} />
{:else}
  <div class="page">
    <div class="head">
      <div>
        <h1>{t(grammar ? 'grammar.title' : 'study.title')}</h1>
        <p class="muted">{t(grammar ? 'grammar.subtitle' : 'study.subtitle')}</p>
      </div>
      <Button
        icon="plus"
        disabled={loading || busy}
        onclick={() =>
          (groupEditor = {
            id: 0,
            title: '',
            parentId:
              current && canCreateIn(library.groups, current.id)
                ? current.id
                : null,
          })}>{t(grammar ? 'grammar.newTopic' : 'study.newGroup')}</Button
      >
    </div>
    {#if loading}<Spinner />{:else if failure}<p role="alert">{failure}</p>
      <Button onclick={load}>{t('common.retry')}</Button>{:else}
      <div class="layout">
        <aside>
          <nav aria-label={t(grammar ? 'grammar.topics' : 'study.groups')}>
            <button class:active={group === null} aria-pressed={group === null} onclick={() => choose(null)}
              >{t(grammar ? 'grammar.all' : 'study.all')} <small>{library.cards.length}</small></button
            >
            {#each rows as row}<button
                class:active={group === row.group.id}
                aria-pressed={group === row.group.id}
                style:padding-left="calc(var(--s-3) + {row.depth} * var(--s-4))"
                onclick={() => choose(row.group.id)}
                >{row.group.title}<small
                  >{studyCards(library, row.group.id).length}</small
                ></button
              >{/each}
          </nav>
        </aside>
        <section class="materials" aria-labelledby={viewId + '-materials'}>
          <div class="head">
            <h2 id={viewId + '-materials'}>
              {current
                ? pathTo(library.groups, current.id)
                    .map((g) => g.title)
                    .join(' / ')
                : t(grammar ? 'grammar.all' : 'study.all')}
            </h2>
            {#if current}<div class="actions">
                <Button
                  size="sm"
                  disabled={busy}
                  onclick={() => (groupEditor = { ...current! })}
                  >{t(grammar ? 'grammar.editTopic' : 'study.editGroup')}</Button
                ><Button
                  size="sm"
                  disabled={busy}
                  onclick={() =>
                    (deleting = { kind: 'groups', id: current!.id })}
                  >{t('common.delete')}</Button
                >
              </div>{/if}
          </div>
          {#if grammar}
            {#if library.groups.length}
              <div class="actions">
                <Button icon="plus" disabled={busy} onclick={newCard}>{t('grammar.newRule')}</Button>
                <Button disabled={busy} onclick={() => (bulk = true)}>{t('study.bulk')}</Button>
                {#if rules.length}
                  <Button variant="primary" icon="book" id={viewId + '-read'}
                    onclick={(event) => (reading = { index: 0, trigger: event.currentTarget.id })}>{t('grammar.read')}</Button>
                {/if}
              </div>
            {/if}
          {:else if group !== null}<div class="actions">
              <Button icon="plus" disabled={busy} onclick={newCard}
                >{t('study.newCard')}</Button
              ><Button disabled={busy} onclick={() => (bulk = true)}
                >{t('study.bulk')}</Button
              >
            </div>{:else}<p class="muted">{t('study.chooseGroup')}</p>{/if}
          {#if !cards.length}<div class="empty">
              <h3>{t(grammar ? 'grammar.empty' : 'study.empty')}</h3>
              <p>
                {grammar
                  ? t(library.groups.length ? 'grammar.emptyRules' : 'grammar.emptyTopics')
                  : t(group === null ? 'study.emptyGroups' : 'study.emptyCards')}
              </p>
            </div>
          {:else if grammar}
            <div class="rules">
              {#each rules as rule, index (rule.id)}
                <article class="rule">
                  <button class="open-rule" id={viewId + '-rule-' + rule.id}
                    onclick={(event) => (reading = { index, trigger: event.currentTarget.id })}>
                    <span>
                      <strong lang="en">{rule.front}</strong>
                      {#if rule.content.formula}<code>{rule.content.formula}</code>{/if}
                    </span>
                    <Icon name="chevronRight" size={18} />
                  </button>
                  <div class="actions">
                    <Button size="sm" disabled={busy} onclick={() => (editing = rule)}>{t('common.edit')}</Button>
                    <Button size="sm" disabled={busy} onclick={() => (deleting = { kind: 'cards', id: rule.id })}>{t('common.delete')}</Button>
                  </div>
                </article>
              {/each}
            </div>
          {:else}
            <div class="training">
              <label
                >{t('study.direction')}<select
                  class="input"
                  bind:value={reverse}
                  ><option value={false}>{t('study.forward')}</option><option
                    value={true}>{t('study.reverse')}</option
                  ></select
                ></label
              >
              <label
                >{t('study.batchSize')}<input
                  class="input"
                  type="number"
                  min="1"
                  max="10000"
                  step="1"
                  bind:value={limit}
                  disabled={busy}
                /></label
              >
              <Button
                variant="primary"
                disabled={busy ||
                  !Number.isInteger(limit) ||
                  limit < 1 ||
                  limit > 10000}
                onclick={start}
                >{selected.size
                  ? t('study.trainSelected', { count: batchSize })
                  : t('study.trainBatch', { count: batchSize })}</Button
              >
              <Button
                size="sm"
                onclick={() =>
                  (selected =
                    selected.size === cards.length
                      ? new Set()
                      : new Set(cards.map((c) => c.id)))}
                >{selected.size === cards.length
                  ? t('study.deselect')
                  : t('study.selectAll')}</Button
              >
            </div>
            <p class="muted">{t('study.priorityHelp')}</p>
            <div class="cards">
              {#each practiceCards as c (c.id)}<article>
                  <small
                    >{t('study.score', {
                      score: scoreById.get(c.id) ?? 0,
                    })}</small
                  >
                  <div class="cardhead">
                    <label class="check"
                      ><input
                        type="checkbox"
                        checked={selected.has(c.id)}
                        onchange={() => toggle(c.id)}
                      />{t(`study.kinds.${c.kind}`)}</label
                    ><span class="group-name"
                      >{library.groups.find((g) => g.id === c.groupId)
                        ?.title}</span
                    >
                  </div>
                  <Markdown source={c.front} />
                  <details>
                    <summary>{t('study.showBack')}</summary><Markdown
                      source={c.back}
                    />
                  </details>
                  <div class="actions">
                    <Button
                      size="sm"
                      disabled={busy}
                      onclick={() => (editing = { ...c })}
                      >{t('common.edit')}</Button
                    ><Button
                      size="sm"
                      disabled={busy}
                      onclick={() => (deleting = { kind: 'cards', id: c.id })}
                      >{t('common.delete')}</Button
                    >
                  </div>
                </article>{/each}
            </div>{/if}
        </section>
      </div>{/if}
  </div>{/if}
{#if editing}<CardEditor
    initial={editing}
    groups={library.groups}
    {busy}
    onsave={saveCard}
    onclose={() => {
      if (!busy) editing = null
    }}
  />{/if}
{#if bulk && destinationGroup !== undefined}<BulkEditor
    groupId={destinationGroup}
    {grammar}
    groups={library.groups}
    onsave={(v, targetGroup) => {
      data = v
      if (grammar) choose(targetGroup)
      bulk = false
    }}
    onclose={() => (bulk = false)}
  />{/if}
{#if groupEditor}<Modal
    open
    title={t(grammar
      ? (groupEditor.id === 0 ? 'grammar.newTopic' : 'grammar.editTopic')
      : (groupEditor.id === 0 ? 'study.newGroup' : 'study.editGroup'))}
    onclose={() => {
      if (!busy) groupEditor = null
    }}
  >
    <form
      class="form"
      onsubmit={(e) => {
        e.preventDefault()
        saveGroup()
      }}
    >
      <label
        >{t(grammar ? 'grammar.topicTitle' : 'study.groupTitle')}<input
          class="input"
          bind:value={groupEditor.title}
          maxlength="100"
          required
          disabled={busy}
        /></label
      >
      <label
        >{t(grammar ? 'grammar.parent' : 'study.parent')}<select
          class="input"
          bind:value={groupEditor.parentId}
          disabled={busy}
          ><option value={null}>{t('study.root')}</option
          >{#each targets as g}<option value={g.id}
              >{pathTo(library.groups, g.id)
                .map((p) => p.title)
                .join(' / ')}</option
            >{/each}</select
        ></label
      >
      <p class="muted">{t(grammar ? 'grammar.topicHelp' : 'study.groupHelp')}</p>
      <Button type="submit" loading={busy} variant="primary"
        >{t('common.save')}</Button
      >
    </form></Modal
  >{/if}
<ConfirmDialog
  open={deleting !== null}
  title={t('common.delete')}
  danger
  loading={busy}
  oncancel={() => (deleting = null)}
  onconfirm={remove}
  ><p>
    {t(
      deleting?.kind === 'groups'
        ? (grammar ? 'grammar.deleteTopicHelp' : 'study.deleteGroupHelp')
        : (grammar ? 'grammar.deleteRuleHelp' : 'study.deleteCardHelp'),
    )}
  </p></ConfirmDialog
>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: var(--s-5);
    overflow-y: auto;
    padding-bottom: var(--s-7);
  }
  .head,
  .actions,
  .training,
  .cardhead {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-3);
    flex-wrap: wrap;
  }
  .head h1 {
    font-size: var(--fs-xl);
  }
  .head h2 {
    font-size: var(--fs-lg);
    overflow-wrap: anywhere;
  }
  .head p {
    margin-top: var(--s-2);
  }
  .layout {
    display: grid;
    grid-template-columns: 210px minmax(0, 1fr);
    gap: var(--s-5);
  }
  .materials {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    min-width: 0;
  }
  nav {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  nav button {
    display: flex;
    justify-content: space-between;
    gap: var(--s-2);
    padding: var(--s-3);
    text-align: left;
    border: var(--bw) solid transparent;
    background: transparent;
    border-radius: var(--radius-sm);
    cursor: pointer;
    overflow-wrap: anywhere;
  }
  nav button.active {
    background: var(--c-green-soft);
    border-color: var(--c-border);
  }
  small {
    color: var(--c-muted);
  }
  .actions {
    justify-content: flex-start;
  }
  .training {
    justify-content: flex-start;
    background: var(--c-surface-2);
    padding: var(--s-3);
    border-radius: var(--radius-sm);
  }
  .training label,
  .form label {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(310px, 100%), 1fr));
    align-items: start;
    gap: var(--s-4);
  }
  .rules {
    display: grid;
    gap: var(--s-3);
  }
  .rule {
    padding: 0;
    gap: 0;
  }
  .open-rule {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-4);
    padding: var(--s-4);
    border: 0;
    border-radius: var(--radius-sm) var(--radius-sm) 0 0;
    background: transparent;
    text-align: left;
    cursor: pointer;
    overflow-wrap: anywhere;
  }
  .open-rule:hover, .open-rule:active {
    background: var(--c-surface-2);
  }
  .open-rule span {
    display: grid;
    min-width: 0;
    gap: var(--s-1);
  }
  .open-rule strong {
    font-size: var(--fs-lg);
  }
  .open-rule code {
    color: var(--c-muted);
    font-size: var(--fs-sm);
    white-space: pre-wrap;
  }
  .rule .actions {
    border-top: 1px solid var(--c-line);
    padding: var(--s-2) var(--s-4);
  }
  article {
    min-width: 0;
    border: var(--bw) solid var(--c-border);
    padding: var(--s-4);
    border-radius: var(--radius);
    background: var(--c-surface);
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  .check {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    background: var(--c-green-soft);
    padding: var(--s-1) var(--s-2);
    border-radius: var(--radius-sm);
    font-size: var(--fs-sm);
  }
  .group-name {
    font-size: var(--fs-xs);
    color: var(--c-muted);
    overflow-wrap: anywhere;
  }
  summary {
    cursor: pointer;
    color: var(--c-muted);
  }
  .empty {
    padding: var(--s-7);
    text-align: center;
    border: var(--bw) dashed var(--c-line);
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }
  @media (max-width: 700px) {
    .layout {
      grid-template-columns: 1fr;
    }
    nav {
      max-height: 200px;
      overflow-y: auto;
    }
    .empty {
      padding: var(--s-4);
    }
  }
</style>
