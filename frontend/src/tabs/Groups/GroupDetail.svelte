<script lang="ts">
  import { latest } from '../../lib/async'
  import { api } from '../../lib/api/client'
  import type { GroupDetail, Range, Word } from '../../lib/api/types'
  import Button from '../../lib/components/Button.svelte'
  import Card from '../../lib/components/Card.svelte'
  import ConfirmDialog from '../../lib/components/ConfirmDialog.svelte'
  import LevelBars from '../../lib/components/LevelBars.svelte'
  import Segmented from '../../lib/components/Segmented.svelte'
  import Spinner from '../../lib/components/Spinner.svelte'
  import { errorText, t } from '../../lib/i18n/i18n.svelte'
  import { route } from '../../lib/state/route.svelte'
  import { toasts } from '../../lib/state/toasts.svelte'
  import ColorPicker from './ColorPicker.svelte'
  import ProductivityChart from './ProductivityChart.svelte'
  import WordEditDialog from './WordEditDialog.svelte'
  import WordRow from './WordRow.svelte'

  // onchanged — после любой правки: список слева пересчитывает due/уровни/имя.
  let { id, onchanged }: { id: number; onchanged: () => void } = $props()

  let detail = $state<GroupDetail | null>(null)
  let loadError = $state<string | null>(null)
  let range = $state<Range>('month')

  let renaming = $state(false)
  let nameDraft = $state('')
  let confirmDeleteGroup = $state(false)
  let deletingGroup = $state(false)

  let editing = $state<Word | null>(null)
  let deletingWord = $state<Word | null>(null)
  let wordBusy = $state(false)

  const fetchDetail = latest(api.groups.get)

  async function load() {
    try {
      const r = await fetchDetail(id, range)
      if (r.stale) return
      detail = r.value
      loadError = null
    } catch (e) {
      loadError = errorText(e)
    }
  }

  // Перезагрузка при смене периода графика (id внутри компонента не меняется — {#key} в родителе).
  $effect(() => {
    void range
    void load()
  })

  async function afterChange(message: string) {
    toasts.success(message)
    await load()
    onchanged()
  }

  async function rename(e: SubmitEvent) {
    e.preventDefault()
    try {
      await api.groups.update(id, { name: nameDraft })
      renaming = false
      await afterChange(t('groups.saved'))
    } catch (err) {
      toasts.error(errorText(err))
    }
  }

  async function setColor(color: string | null) {
    try {
      await api.groups.update(id, { color })
      await load()
      onchanged()
    } catch (err) {
      toasts.error(errorText(err))
    }
  }

  async function deleteGroup() {
    deletingGroup = true
    try {
      await api.groups.remove(id)
      toasts.success(t('groups.deleted'))
      onchanged()
      route.go('groups')
    } catch (err) {
      toasts.error(errorText(err))
    } finally {
      deletingGroup = false
      confirmDeleteGroup = false
    }
  }

  async function deleteWord() {
    if (!deletingWord) return
    wordBusy = true
    try {
      await api.words.remove(deletingWord.id)
      deletingWord = null
      await afterChange(t('words.deleted'))
    } catch (err) {
      toasts.error(errorText(err))
    } finally {
      wordBusy = false
    }
  }

  async function unarchive(w: Word) {
    try {
      await api.words.unarchive(w.id)
      await afterChange(t('words.unarchived'))
    } catch (err) {
      toasts.error(errorText(err))
    }
  }

  async function onsaved() {
    editing = null
    await afterChange(t('words.saved'))
  }
</script>

{#if loadError && !detail}
  <div class="state">
    <p>{loadError}</p>
    <Button icon="refresh" onclick={load}>{t('common.retry')}</Button>
  </div>
{:else if !detail}
  <div class="state"><Spinner size={28} /></div>
{:else}
  {@const g = detail.group}
  {@const a = detail.analytics}
  <div class="detail scroll">
    <header class="head">
      <a class="back" href="#groups">← {t('groups.back')}</a>
      {#if renaming}
        <form class="rename" onsubmit={rename}>
          <!-- svelte-ignore a11y_autofocus -->
          <input class="input" bind:value={nameDraft} autofocus aria-label={t('groups.rename')} />
          <Button type="submit" variant="primary" size="sm" icon="check">{t('common.save')}</Button>
          <Button size="sm" variant="ghost" onclick={() => (renaming = false)}>{t('common.cancel')}</Button>
        </form>
      {:else}
        <div class="title">
          <span class="dot" style:background={g.color ?? 'var(--c-surface)'}></span>
          <h2>#{g.name}</h2>
          <Button size="sm" variant="ghost" icon="edit" aria-label={t('groups.rename')} title={t('groups.rename')}
            onclick={() => { nameDraft = g.name; renaming = true }} />
        </div>
      {/if}
      <div class="head-actions">
        <ColorPicker value={g.color} onchange={setColor} />
        <Button variant="primary" icon="play" disabled={g.wordCount === 0} onclick={() => route.go('training', String(g.id))}>
          {t('groups.train')}
        </Button>
        <Button variant="ghost" icon="trash" aria-label={t('groups.delete')} title={t('groups.delete')} onclick={() => (confirmDeleteGroup = true)} />
      </div>
    </header>

    <div class="tiles">
      <Card padding="sm"><span class="tile-label muted">{t('groups.dueToday')}</span><span class="tile-value">{g.due}</span></Card>
      <Card padding="sm"><span class="tile-label muted">{t('groups.mastered')}</span><span class="tile-value">{a.masteredPercent}%</span></Card>
      <Card padding="sm"><span class="tile-label muted">{t('groups.sessionsWeek')}</span><span class="tile-value">{t('groups.times', { count: a.sessionsThisWeek })}</span></Card>
      <Card padding="sm"><span class="tile-label muted">{t('groups.sessionsMonth')}</span><span class="tile-value">{t('groups.times', { count: a.sessionsThisMonth })}</span></Card>
    </div>

    <div class="charts">
      <Card>
        <div class="card-head">
          <h3>{t('groups.productivity')}</h3>
          <Segmented
            label={t('groups.productivityHint')}
            value={range}
            options={[{ value: 'week', label: t('groups.week') }, { value: 'month', label: t('groups.month') }]}
            onchange={(r) => (range = r)}
          />
        </div>
        <ProductivityChart days={a.productivity} />
      </Card>
      <Card>
        <h3>{t('groups.levels')}</h3>
        <LevelBars levels={g.levels} />
      </Card>
    </div>

    <Card padding="none">
      <h3 class="words-title">{t('groups.wordsTitle')} <span class="muted">· {detail.words.length}</span></h3>
      {#if detail.words.length === 0}
        <p class="muted empty">{t('groups.wordsEmpty')}</p>
      {:else}
        <ul class="words">
          {#each detail.words as w (w.id)}
            <WordRow word={w} onedit={(x) => (editing = x)} ondelete={(x) => (deletingWord = x)} onunarchive={unarchive} />
          {/each}
        </ul>
      {/if}
    </Card>
  </div>

  <ConfirmDialog open={confirmDeleteGroup} title={t('groups.deleteTitle', { name: g.name })} danger
    confirmText={t('common.delete')} loading={deletingGroup} onconfirm={deleteGroup} oncancel={() => (confirmDeleteGroup = false)}>
    {t('groups.deleteBody')}
  </ConfirmDialog>
{/if}

<ConfirmDialog open={deletingWord !== null} title={t('words.deleteTitle', { term: deletingWord?.term ?? '' })} danger
  confirmText={t('common.delete')} loading={wordBusy} onconfirm={deleteWord} oncancel={() => (deletingWord = null)}>
  {t('words.deleteBody')}
</ConfirmDialog>

<WordEditDialog word={editing} onclose={() => (editing = null)} {onsaved} />

<style>
  .state {
    height: 100%;
    display: grid;
    place-items: center;
    align-content: center;
    gap: var(--s-3);
  }
  .detail {
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    padding: 2px 6px 6px 2px;
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-3);
  }
  .back {
    display: none;
    width: 100%;
    color: var(--c-muted);
    text-decoration: none;
    font-weight: 600;
  }
  .title {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    min-width: 0;
  }
  .title h2 {
    font-size: var(--fs-xl);
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dot {
    flex: none;
    width: 16px;
    height: 16px;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
  .rename {
    display: flex;
    gap: var(--s-2);
    align-items: center;
  }
  .head-actions {
    display: flex;
    align-items: center;
    gap: var(--s-3);
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: var(--s-3);
  }
  .tiles :global(.card) {
    display: flex;
    flex-direction: column;
  }
  .tile-label {
    font-size: var(--fs-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .tile-value {
    font-size: var(--fs-xl);
    font-weight: 800;
  }
  .charts {
    display: grid;
    grid-template-columns: 2fr 1fr;
    gap: var(--s-3);
  }
  h3 {
    font-size: var(--fs-md);
    margin-bottom: var(--s-3);
  }
  .card-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-2);
  }
  .card-head h3 {
    margin: 0;
  }
  .words-title {
    padding: var(--s-3) var(--s-4) 0;
  }
  .words {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .empty {
    padding: 0 var(--s-4) var(--s-4);
  }

  @media (max-width: 860px) {
    .back {
      display: block;
    }
    .tiles {
      grid-template-columns: repeat(2, 1fr);
    }
    .charts {
      grid-template-columns: 1fr;
    }
  }
</style>
