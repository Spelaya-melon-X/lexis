<script lang="ts">
  // Рассказ о дне: простой текст (§17.6), автосохранение через 1 с после паузы в наборе,
  // при уходе из поля и при смене дня (компонент пересоздаётся через {#key day} —
  // onDestroy дописывает несохранённое в «свой» день).
  import { onDestroy, untrack } from 'svelte'
  import { api } from '../../../lib/api/client'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'

  const DEBOUNCE_MS = 1000

  interface Props {
    day: string
    initial: string
  }
  let { day, initial }: Props = $props()

  // Начальное значение берём один раз: дальше источник правды — поле, а не ответы сервера
  // (иначе ответ на старый запрос «откатил» бы то, что напечатано после него).
  let text = $state(untrack(() => initial))
  let saved = untrack(() => initial)
  let status = $state<'idle' | 'saving' | 'saved' | 'error'>('idle')
  let timer: ReturnType<typeof setTimeout> | undefined

  function schedule() {
    clearTimeout(timer)
    timer = setTimeout(flush, DEBOUNCE_MS)
  }

  async function flush() {
    clearTimeout(timer)
    const v = text
    if (v === saved) return
    status = 'saving'
    try {
      await api.journal.update(day, { story: v })
      saved = v
      status = text === v ? 'saved' : 'idle' // пока сохраняли, могли допечатать — тогда ждём следующий flush
    } catch (e) {
      status = 'error'
      toasts.error(errorText(e))
    }
  }

  onDestroy(() => void flush())
</script>

<div class="story">
  <textarea
    class="input"
    bind:value={text}
    placeholder={t('journal.storyPlaceholder')}
    aria-label={t('journal.story')}
    oninput={schedule}
    onblur={flush}
  ></textarea>
  <span class="status muted" class:error={status === 'error'} aria-live="polite">
    {#if status === 'saving'}{t('journal.saving')}{:else if status === 'saved'}✓ {t('journal.saved')}{:else if status === 'error'}{t('journal.saveError')}{/if}
  </span>
</div>

<style>
  .story {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  textarea {
    min-height: 180px;
    resize: vertical;
    line-height: 1.6;
    field-sizing: content; /* растёт по тексту там, где поддерживается; иначе — ручной resize */
  }
  .status {
    align-self: flex-end;
    min-height: 1.5em;
    font-size: var(--fs-xs);
  }
  .error {
    color: var(--c-red);
  }
</style>
