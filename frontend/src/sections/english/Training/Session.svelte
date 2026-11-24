<script lang="ts">
  import { fade, fly } from 'svelte/transition'
  import { api } from '../../../lib/api/client'
  import type { Answer, Level } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import LevelBadge from '../../../lib/components/LevelBadge.svelte'
  import ProgressBar from '../../../lib/components/ProgressBar.svelte'
  import { formatDuration } from '../../../lib/dates'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { session, type Active } from '../../../lib/state/session.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import { current, finished, progress } from '../../../lib/training'
  import WordCard from './WordCard.svelte'

  let { active }: { active: Active } = $props()

  let revealed = $state(false)
  let typed = $state('')
  let busy = $state(false)
  let confirmFinish = $state(false)
  let change = $state<{ from: Level; to: Level } | null>(null) // последняя смена уровня — короткая подсказка
  let requeued = $state<number | null>(null)
  let answered = $state(0) // счётчик ответов — ключ, чтобы подсказка под карточкой анимировалась заново
  let now = $state(Date.now())
  let input = $state<HTMLInputElement>()

  const card = $derived(current(active.q))

  // Таймер на экране — раз в секунду; cleanup эффекта останавливает его при уходе с экрана.
  $effect(() => {
    const id = setInterval(() => (now = Date.now()), 1000)
    return () => clearInterval(id)
  })

  $effect(() => {
    if (card && !revealed && !confirmFinish) input?.focus({ preventScroll: true })
  })

  function reveal() {
    if (!card || revealed) return
    revealed = true
  }

  async function answer(a: Answer) {
    if (!card || !revealed || busy) return
    busy = true
    try {
      const res = await api.training.answer(active.sessionId, card.wordId, a)
      change = res.levelBefore !== res.levelAfter ? { from: res.levelBefore, to: res.levelAfter } : null
      requeued = a === 'again' ? (res.requeueAt ?? null) : null
      session.apply(a, res.requeueAt)
      answered++
      revealed = false
      typed = ''
      if (finished(active.q)) await finish()
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
    }
  }

  async function finish() {
    busy = true
    try {
      session.showResults(await api.training.finish(active.sessionId))
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
      confirmFinish = false
    }
  }

  function onkeydown(e: KeyboardEvent) {
    if (!card || e.defaultPrevented || e.repeat || confirmFinish || e.isComposing || e.metaKey || e.ctrlKey || e.altKey) return
    const target = e.target instanceof HTMLElement ? e.target : null
    const inInput = target === input
    const interactive = target?.closest('input, textarea, select, button, a, [contenteditable]')
    if (!revealed) {
      if ((e.key === 'Enter' && (inInput || !interactive)) || (e.key === ' ' && !interactive)) {
        e.preventDefault()
        reveal()
      }
      return
    }
    const keys: Record<string, Answer> = { '1': 'good', '2': 'hard', '3': 'again' }
    const a = keys[e.key]
    if (a && !target?.closest('input, textarea, select, [contenteditable]')) {
      e.preventDefault()
      void answer(a)
    }
  }
</script>

<svelte:window {onkeydown} />

<div class="session scroll">
  <header class="bar">
    <div class="info">
      <strong>#{active.groupName}</strong>
      <span class="muted"> · {t(`training.directions.${active.strategy}`)}</span>
      {#if active.free}<span class="muted"> · {t('training.free')}</span>{/if}
    </div>
    <span class="muted counter">{t('training.counter', { done: active.q.done, total: active.q.total })} · {formatDuration((now - active.startedAt) / 1000)}</span>
    <Button size="sm" variant="ghost" icon="x" disabled={busy} onclick={() => (confirmFinish = true)}>{t('training.finish')}</Button>
  </header>
  <ProgressBar value={progress(active.q)} label={t('training.counter', { done: active.q.done, total: active.q.total })} />

  {#if card}
    <div class="stage">
      {#key answered}
        <WordCard {card} {revealed} strategy={active.strategy} />
      {/key}
      <input
        class="input answer-input"
        bind:this={input}
        bind:value={typed}
        aria-label={t('training.yourAnswer')}
        placeholder={t('training.yourAnswer')}
        readonly={revealed}
        autocomplete="off"
        spellcheck="false"
      />

      <div class="controls">
        {#if !revealed}
          <Button variant="primary" size="lg" onclick={reveal}>{t('training.show')}</Button>
          <span class="muted hint">{t('training.showHint')}</span>
        {:else}
          <div class="answers">
            <button class="ans good" disabled={busy} onclick={() => answer('good')}><kbd>1</kbd>{t('training.good')}</button>
            <button class="ans hard" disabled={busy} onclick={() => answer('hard')}><kbd>2</kbd>{t('training.hard')}</button>
            <button class="ans again" disabled={busy} onclick={() => answer('again')}><kbd>3</kbd>{t('training.again')}</button>
          </div>
          <span class="muted hint">{t('training.keysHint')}</span>
        {/if}
      </div>

      <div class="feedback">
        {#key answered}
          {#if change}
            <span class="change" in:fly={{ y: 8, duration: 220 }}>
              <LevelBadge level={change.from} /> → <LevelBadge level={change.to} />
            </span>
          {:else if requeued}
            <span class="muted" in:fade={{ duration: 200 }}>{t('training.requeued', { count: requeued })}</span>
          {/if}
        {/key}
      </div>
    </div>
  {/if}
</div>

<ConfirmDialog open={confirmFinish} title={t('training.finishTitle')} confirmText={t('training.finish')} loading={busy}
  onconfirm={finish} oncancel={() => (confirmFinish = false)}>
  {t('training.finishBody')}
</ConfirmDialog>

<style>
  .session {
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
    max-width: 720px;
    width: 100%;
    margin: 0 auto;
    padding: 2px 6px 6px 2px;
  }
  .bar {
    display: flex;
    align-items: center;
    gap: var(--s-3);
  }
  .info {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .counter {
    font-variant-numeric: tabular-nums;
    font-size: var(--fs-sm);
  }
  .stage {
    flex: 1 0 auto;
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: var(--s-4);
    padding-block: var(--s-3);
  }
  .answer-input {
    max-width: 360px;
    text-align: center;
    align-self: center;
  }
  .answer-input[readonly]:placeholder-shown {
    visibility: hidden;
  }
  .controls {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--s-2);
    min-height: 92px;
  }
  .hint {
    font-size: var(--fs-xs);
  }
  .answers {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--s-3);
    width: 100%;
  }
  .ans {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-2);
    padding: var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    font-size: var(--fs-lg);
    font-weight: 700;
    cursor: pointer;
    box-shadow: var(--shadow-sm);
    transition:
      transform var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out);
  }
  .ans:hover:not(:disabled) {
    transform: translate(-1px, -1px);
    box-shadow: 3px 3px 0 var(--c-border);
  }
  .ans:active:not(:disabled) {
    transform: translate(2px, 2px);
    box-shadow: none;
  }
  .ans:disabled {
    opacity: 0.6;
    cursor: wait;
  }
  .good {
    background: var(--c-green-soft);
  }
  .hard {
    background: var(--c-amber-soft);
  }
  .again {
    background: var(--c-red-soft);
  }
  kbd {
    font: inherit;
    font-size: var(--fs-xs);
    padding: 0 6px;
    border: 1px solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
  }
  .feedback {
    min-height: 28px;
    display: flex;
    justify-content: center;
    font-size: var(--fs-sm);
  }
  .change {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
  }
  @media (max-width: 560px) {
    .bar, .answers { gap: var(--s-2); }
    .counter { font-size: var(--fs-xs); }
    .ans {
      min-height: 44px;
      padding: var(--s-2);
      font-size: var(--fs-sm);
      overflow-wrap: anywhere;
    }
    kbd { display: none; }
  }
</style>
