<script lang="ts">
  // «Наблюдения и выводы» (§17.4): лента, новые сверху; дата — автоматически; текст — простой (§17.6).
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { api } from '../../../lib/api/client'
  import type { Habit, Note, NoteFilter, NoteInput, NoteKind } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import Segmented from '../../../lib/components/Segmented.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import NoteEditor from './NoteEditor.svelte'

  let habits = $state<Habit[]>([])
  let notes = $state<Note[] | null>(null)
  let next = $state<number | null>(null)
  let error = $state<string | null>(null)
  let loadingMore = $state(false)
  let editingId = $state<number | null>(null)
  let deleting = $state<Note | null>(null)
  let busy = $state(false)

  // Фильтры: '' — все; 'general' — только общие.
  let habitFilter = $state<string>('')
  let kindFilter = $state<NoteKind | ''>('')

  const byId = $derived(new Map(habits.map((h) => [h.id, h])))

  function filter(): NoteFilter {
    return {
      habit: habitFilter === '' ? undefined : habitFilter === 'general' ? 'general' : Number(habitFilter),
      kind: kindFilter || undefined,
    }
  }

  let seq = 0
  async function load() {
    const my = ++seq
    error = null
    try {
      const r = await api.habits.notes(filter())
      if (my !== seq) return
      notes = r.notes
      next = r.next
    } catch (e) {
      if (my === seq) error = errorText(e)
    }
  }

  onMount(async () => {
    try {
      habits = await api.habits.list(true)
    } catch (e) {
      toasts.error(errorText(e))
    }
  })
  // Смена фильтра — лента с начала.
  $effect(() => {
    void habitFilter
    void kindFilter
    notes = null
    void load()
  })

  async function more() {
    if (next === null || loadingMore) return
    loadingMore = true
    try {
      const r = await api.habits.notes({ ...filter(), before: next })
      notes = [...(notes ?? []), ...r.notes]
      next = r.next
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      loadingMore = false
    }
  }

  function matches(n: Note): boolean {
    const f = filter()
    if (f.kind && n.kind !== f.kind) return false
    if (f.habit === 'general') return n.habitId === null
    if (typeof f.habit === 'number') return n.habitId === f.habit
    return true
  }

  async function create(input: NoteInput): Promise<boolean> {
    try {
      const n = await api.habits.createNote(input)
      if (notes && matches(n)) notes = [n, ...notes]
      return true
    } catch (e) {
      toasts.error(errorText(e))
      return false
    }
  }

  async function update(id: number, input: NoteInput): Promise<boolean> {
    try {
      const n = await api.habits.updateNote(id, input)
      // После правки запись может перестать подходить под фильтр — тогда убираем из ленты.
      if (notes) notes = matches(n) ? notes.map((x) => (x.id === id ? n : x)) : notes.filter((x) => x.id !== id)
      editingId = null
      return true
    } catch (e) {
      toasts.error(errorText(e))
      return false
    }
  }

  async function remove() {
    if (!deleting || !notes) return
    busy = true
    try {
      await api.habits.removeNote(deleting.id)
      notes = notes.filter((x) => x.id !== deleting!.id)
      deleting = null
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
    }
  }

  const when = (iso: string) =>
    new Date(iso).toLocaleString(i18n.locale, { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })
</script>

<div class="page">
  <h1>{t('habits.notes')}</h1>

  <div class="scroll body">
    <Card>
      <NoteEditor {habits} submitText={t('habits.noteAdd')} onsubmit={create} />
    </Card>

    <div class="filters">
      <Segmented
        label={t('habits.filterKind')}
        value={kindFilter}
        options={[
          { value: '', label: t('habits.notesAll') },
          { value: 'observation', label: t('habits.noteObservation') },
          { value: 'insight', label: t('habits.noteInsight') },
        ]}
        onchange={(v) => (kindFilter = v)}
      />
      <select class="input habit" bind:value={habitFilter} aria-label={t('habits.filterHabit')}>
        <option value="">{t('habits.notesAll')}</option>
        <option value="general">{t('habits.noteGeneral')}</option>
        {#each habits as h (h.id)}<option value={String(h.id)}>{h.name}</option>{/each}
      </select>
    </div>

    {#if error && !notes}
      <div class="state"><p>{error}</p><Button icon="refresh" onclick={load}>{t('common.retry')}</Button></div>
    {:else if !notes}
      <div class="state"><Spinner size={28} /></div>
    {:else if notes.length === 0}
      <div class="state"><p class="muted">{t('habits.notesEmpty')}</p></div>
    {:else}
      <ul class="feed">
        {#each notes as n (n.id)}
          {@const h = n.habitId !== null ? byId.get(n.habitId) : undefined}
          <li class={n.kind} in:fade={{ duration: 200 }}>
            {#if editingId === n.id}
              <NoteEditor
                {habits}
                initial={{ kind: n.kind, habitId: n.habitId, body: n.body }}
                submitText={t('common.save')}
                oncancel={() => (editingId = null)}
                onsubmit={(input) => update(n.id, input)}
              />
            {:else}
              <div class="meta">
                <span class="badge">{t(n.kind === 'insight' ? 'habits.noteInsight' : 'habits.noteObservation')}</span>
                {#if h}
                  <span class="habit-tag"><span class="dot" style:background={h.color ?? 'transparent'}></span>{h.name}</span>
                {:else}
                  <span class="muted">{t('habits.noteGeneral')}</span>
                {/if}
                <span class="muted when">
                  {when(n.createdAt)}{#if n.updatedAt !== n.createdAt} · {t('habits.noteEdited')}{/if}
                </span>
                <button class="act" aria-label={t('journal.edit')} title={t('journal.edit')} onclick={() => (editingId = n.id)}><Icon name="edit" size={16} /></button>
                <button class="act" aria-label={t('common.delete')} title={t('common.delete')} onclick={() => (deleting = n)}><Icon name="trash" size={16} /></button>
              </div>
              <p class="text">{n.body}</p>
            {/if}
          </li>
        {/each}
      </ul>
      {#if next !== null}
        <div class="more"><Button variant="ghost" loading={loadingMore} onclick={more}>{t('habits.notesMore')}</Button></div>
      {/if}
    {/if}
  </div>
</div>

<ConfirmDialog
  open={deleting !== null}
  title={t('habits.noteDeleteTitle')}
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
  h1 {
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
  }
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    padding: 0 var(--s-1) var(--s-4) 0;
  }
  .filters {
    display: flex;
    align-items: center;
    gap: var(--s-3);
  }
  .habit {
    width: auto;
    min-width: 180px;
  }
  .state {
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
    padding: var(--s-6) 0;
  }
  .feed {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  li {
    padding: var(--s-3) var(--s-4);
    border: var(--bw) solid var(--c-border);
    border-left-width: 6px;
    border-radius: var(--radius);
    background: var(--c-surface);
  }
  /* вывод — главное, что остаётся от наблюдений: выделен зелёным */
  li.insight {
    border-left-color: var(--c-green);
    box-shadow: var(--shadow-sm);
  }
  .meta {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-sm);
  }
  .badge {
    padding: 0 var(--s-2);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    font-size: var(--fs-xs);
    font-weight: 700;
  }
  .insight .badge {
    background: var(--c-green-soft);
  }
  .habit-tag {
    display: flex;
    align-items: center;
    gap: var(--s-1);
    font-weight: 600;
  }
  .dot {
    width: 10px;
    height: 10px;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
  .when {
    margin-left: auto;
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
  .text {
    margin-top: var(--s-2);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    line-height: 1.6;
  }
  .more {
    display: flex;
    justify-content: center;
  }
</style>
