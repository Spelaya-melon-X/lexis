<script lang="ts">
  import { untrack } from 'svelte'
  import { api } from '../../lib/api/client'
  import {
    createQueue,
    answer as applyAnswer,
    progress,
  } from '../../lib/training'
  import type { StudyReviewInput, StudyPracticeCard } from '../../lib/api/types'
  import { t, errorText } from '../../lib/i18n/i18n.svelte'
  import Button from '../../lib/components/Button.svelte'
  import Markdown from '../../lib/components/Markdown.svelte'
  import ProgressBar from '../../lib/components/ProgressBar.svelte'
  let {
    cards,
    reverse,
    onclose,
    onrestart,
  }: {
    cards: StudyPracticeCard[]
    reverse: boolean
    onclose: () => void
    onrestart: () => Promise<StudyPracticeCard[]>
  } = $props()
  let revealed = $state(false),
    attempts = $state(0),
    mistakes = $state(0)
  let queue = $state(untrack(() => createQueue(cards.map((c) => ({ ...c })))))
  let saving = $state(false),
    failure = $state(''),
    pending = $state<StudyReviewInput | null>(null)
  let container = $state<HTMLDivElement>()
  const card = $derived(queue.queue[0])
  async function answer(good: boolean) {
    if (!revealed || !card || saving) return
    saving = true
    failure = ''
    try {
      pending ??= {
        id: crypto.randomUUID(),
        cardId: card.id,
        known: good,
        reverse,
      }
      await api.study.review(pending)
      card.score += pending.known ? 1 : -1
      attempts++
      if (!pending.known) mistakes++
      queue = applyAnswer(queue, pending.known ? 'good' : 'again', 3)
      revealed = false
      pending = null
      container?.scrollTo({ top: 0 })
    } catch (e) {
      failure = errorText(e)
    } finally {
      saving = false
    }
  }
  async function restart() {
    if (saving || pending) return
    saving = true
    failure = ''
    try {
      const next = await onrestart()
      queue = createQueue(next)
      attempts = 0
      mistakes = 0
      revealed = false
      container?.scrollTo({ top: 0 })
    } catch (e) {
      failure = errorText(e)
    } finally {
      saving = false
    }
  }
  function key(e: KeyboardEvent) {
    if (saving || pending || e.metaKey || e.ctrlKey || e.altKey || e.repeat)
      return
    const el = e.target as HTMLElement
    if (el.closest('button,input,textarea,select,a')) return
    if (card && (e.key === ' ' || e.key === 'Enter')) {
      e.preventDefault()
      revealed = true
    } else if (revealed && e.key === '1') {
      e.preventDefault()
      answer(true)
    } else if (revealed && e.key === '2') {
      e.preventDefault()
      answer(false)
    }
  }
</script>

<svelte:window onkeydown={key} />
<div class="practice" bind:this={container}>
  <div class="head">
    <h1>{t('study.practice')}</h1>
    <Button disabled={saving} onclick={onclose}>{t('common.close')}</Button>
  </div>
  <ProgressBar value={progress(queue)} label={t('study.progress')} />
  <p aria-live="polite">{queue.done} / {queue.total}</p>
  {#if card}
    <div class="card">
      <small>{t('study.score', { score: card.score })}</small>
      <span class="badge"
        >{reverse && !revealed
          ? t('study.recallConcept')
          : t(`study.kinds.${card.kind}`)}</span
      ><Markdown source={reverse ? card.back : card.front} />
      {#if revealed}<hr />
        <Markdown source={reverse ? card.front : card.back} />{/if}
    </div>
    <div class="actions">
      {#if revealed}<Button
          variant="primary"
          disabled={saving || pending !== null}
          onclick={() => answer(true)}>{t('study.known')}</Button
        ><Button
          disabled={saving || pending !== null}
          onclick={() => answer(false)}>{t('study.again')}</Button
        >{:else}<Button variant="primary" onclick={() => (revealed = true)}
          >{t('study.reveal')}</Button
        >{/if}
    </div>
    <p class="muted">{t('study.shortcuts')}</p>
  {:else}<h2>{t('study.finished')}</h2>
    <p>
      {t('study.batchResult', {
        done: queue.done,
        total: queue.total,
        attempts,
        mistakes,
      })}
    </p>
    <div class="actions">
      <Button icon="repeat" disabled={saving} onclick={restart}
        >{t('study.restart')}</Button
      ><Button onclick={onclose}>{t('study.backToCards')}</Button><a
        href="#/study/stats">{t('nav.study.stats')}</a
      >
    </div>{/if}
  {#if saving}<p role="status">{t('study.savingAnswer')}</p>{/if}
  {#if failure}<div role="alert">
      <p>{failure}</p>
      <Button
        disabled={saving}
        onclick={() => (card ? answer(pending?.known ?? false) : restart())}
        >{t('common.retry')}</Button
      >
    </div>{/if}
</div>

<style>
  .practice {
    min-width: 0;
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    padding-bottom: var(--s-7);
  }
  .practice > :global(*) {
    /* Scroll the session instead of shrinking its content below its natural height. */
    flex-shrink: 0;
  }
  .head,
  .actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-3);
    flex-wrap: wrap;
  }
  .actions {
    justify-content: center;
  }
  .card {
    min-width: 0;
    max-width: 100%;
    overflow-wrap: anywhere;
    background: var(--c-surface);
    border: var(--bw-strong) solid var(--c-border);
    box-shadow: var(--shadow);
    border-radius: var(--radius);
    padding: var(--s-5);
    min-height: 240px;
  }
  .badge {
    display: inline-block;
    background: var(--c-green-soft);
    border-radius: var(--radius-sm);
    padding: var(--s-1) var(--s-2);
    font-size: var(--fs-sm);
  }
  hr {
    border: 0;
    border-top: var(--bw) solid var(--c-line);
    margin: var(--s-5) 0;
  }
</style>
