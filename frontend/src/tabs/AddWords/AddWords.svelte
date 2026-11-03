<script lang="ts">
  import { fade } from 'svelte/transition'
  import { latest } from '../../lib/async'
  import { api } from '../../lib/api/client'
  import type { ParseResult } from '../../lib/api/types'
  import Button from '../../lib/components/Button.svelte'
  import Icon from '../../lib/components/Icon.svelte'
  import Spinner from '../../lib/components/Spinner.svelte'
  import { errorText, lineErrorText, t } from '../../lib/i18n/i18n.svelte'
  import { keepLines, lineRange } from '../../lib/lines'
  import { draft } from '../../lib/state/draft.svelte'
  import { toasts } from '../../lib/state/toasts.svelte'
  import PreviewCard from './PreviewCard.svelte'
  import SyntaxHelp from './SyntaxHelp.svelte'

  const DEBOUNCE_MS = 300
  const parse = latest(api.words.parse)

  let textarea: HTMLTextAreaElement | undefined = $state()
  let result = $state<ParseResult | null>(null)
  let parsing = $state(false)
  let parseError = $state<string | null>(null)
  let saving = $state(false)

  const cardCount = $derived(result?.cards.length ?? 0)
  const errorCount = $derived(result?.errors.length ?? 0)

  // Живое превью: после паузы в наборе — /parse (ничего не пишет). Cleanup эффекта
  // сбрасывает таймер при каждом нажатии, а latest() отбрасывает опоздавшие ответы.
  $effect(() => {
    const text = draft.text
    if (!text.trim()) {
      parse.cancel()
      result = null
      parsing = false
      parseError = null
      return
    }
    parsing = true
    const timer = setTimeout(async () => {
      try {
        const r = await parse(text)
        if (r.stale) return
        result = r.value
        parseError = null
      } catch (e) {
        parseError = errorText(e)
      }
      parsing = false
    }, DEBOUNCE_MS)
    return () => clearTimeout(timer)
  })

  async function create() {
    if (cardCount === 0 || saving) return
    saving = true
    const text = draft.text
    try {
      // bulk парсит текст заново на сервере — сохраняется ровно то, что в поле, даже если превью не успело обновиться
      const res = await api.words.bulk(text)
      toasts.success(t('add.created', { created: res.created, merged: res.merged }))
      if (res.errors.length > 0) {
        draft.text = keepLines(text, res.errors.map((e) => e.line))
        toasts.info(t('add.keptErrors'))
      } else {
        draft.text = ''
      }
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      saving = false
    }
  }

  function onkeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault()
      void create()
    }
  }

  // Клик по ошибке — выделить её строку в поле и прокрутить к ней.
  function focusLine(line: number) {
    const range = lineRange(draft.text, line)
    if (!textarea || !range) return
    textarea.focus()
    textarea.setSelectionRange(range[0], range[1])
    const lineHeight = parseFloat(getComputedStyle(textarea).lineHeight) || 24
    textarea.scrollTop = Math.max(0, (line - 1) * lineHeight - textarea.clientHeight / 3)
  }

  function insertExample() {
    const ex = t('add.exampleText')
    draft.text = draft.text.trim() ? `${draft.text.replace(/\n*$/, '')}\n${ex}` : ex
    textarea?.focus()
  }
</script>

<section class="add">
  <div class="editor">
    <label for="words-input" class="label">{t('add.inputLabel')}</label>
    <textarea
      id="words-input"
      class="input"
      bind:this={textarea}
      bind:value={draft.text}
      {onkeydown}
      placeholder={t('add.placeholder')}
      spellcheck="false"
      autocomplete="off"
    ></textarea>
    <SyntaxHelp oninsert={insertExample} />
  </div>

  <div class="preview">
    <div class="preview-head">
      <h2>{t('add.preview')}</h2>
      <span class="summary muted">
        {#if parsing}
          <Spinner size={14} /> {t('add.parsing')}
        {:else if result}
          {t('add.cardsCount', { count: cardCount })}{#if errorCount} · <span class="err-count">{t('add.errorsCount', { count: errorCount })}</span>{/if}
        {/if}
      </span>
    </div>

    <div class="list scroll">
      {#if parseError}
        <p class="parse-error"><Icon name="alert" size={18} /> {t('add.parseFailed')}: {parseError}</p>
      {:else if !result}
        <p class="empty muted">{t('add.previewEmpty')}</p>
      {:else}
        {#if errorCount > 0}
          <div class="errors" transition:fade={{ duration: 150 }}>
            <h3>{t('add.errorsTitle')}</h3>
            <ul>
              {#each result.errors as err (err.line)}
                <li>
                  <button class="error" onclick={() => focusLine(err.line)}>
                    <span class="where">{t('add.line', { line: err.line })}</span>
                    <span class="why">{lineErrorText(err.code)}</span>
                    <code>{err.text}</code>
                  </button>
                </li>
              {/each}
            </ul>
          </div>
        {/if}
        <!-- §12: добавление слова — fade-in -->
        {#each result.cards as card (`${card.line}:${card.term}`)}
          <div in:fade={{ duration: 200 }}><PreviewCard {card} /></div>
        {/each}
      {/if}
    </div>
  </div>

  <footer class="actions">
    <span class="muted hint">{t('add.shortcut')}</span>
    <Button variant="primary" size="lg" icon="check" loading={saving} disabled={cardCount === 0} onclick={create}>
      {#if saving}{t('add.creating')}{:else if cardCount > 0}{t('add.create', { count: cardCount })}{:else}{t('add.createNone')}{/if}
    </Button>
  </footer>
</section>

<style>
  .add {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    grid-template-rows: minmax(0, 1fr) auto;
    gap: var(--s-4) var(--s-5);
    height: 100%;
    min-height: 0;
  }

  .editor,
  .preview {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
    min-height: 0;
  }

  .label,
  h2 {
    font-size: var(--fs-md);
    font-weight: 700;
  }

  textarea {
    flex: 1;
    min-height: 0;
    resize: none;
    font-family: ui-monospace, 'SF Mono', Menlo, monospace;
    font-size: var(--fs-sm);
    line-height: 1.7;
    box-shadow: var(--shadow);
  }
  textarea:focus {
    box-shadow: var(--shadow);
  }

  .preview-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--s-3);
  }
  .summary {
    display: inline-flex;
    align-items: center;
    gap: var(--s-1);
    font-size: var(--fs-sm);
  }
  .err-count {
    color: var(--c-red);
    font-weight: 600;
  }

  .list {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
    padding: 2px 6px 6px 2px; /* место для жёстких теней карточек */
  }

  .empty {
    margin: auto;
    text-align: center;
    max-width: 280px;
  }

  .parse-error {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    color: var(--c-red);
  }

  .errors {
    border: var(--bw) solid var(--c-red);
    border-radius: var(--radius-sm);
    background: var(--c-red-soft);
    padding: var(--s-3);
  }
  .errors h3 {
    font-size: var(--fs-sm);
    color: var(--c-red);
    margin-bottom: var(--s-2);
  }
  .errors ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  .error {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 0 var(--s-2);
    width: 100%;
    padding: var(--s-1) var(--s-2);
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    text-align: left;
    font-size: var(--fs-sm);
    cursor: pointer;
  }
  .error:hover {
    background: rgb(255 255 255 / 0.6);
  }
  .where {
    font-weight: 700;
    color: var(--c-red);
    white-space: nowrap;
  }
  .error code {
    grid-column: 2;
    color: var(--c-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .actions {
    grid-column: 1 / -1;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--s-4);
    padding-top: var(--s-3);
    border-top: var(--bw) solid var(--c-line);
  }
  .hint {
    font-size: var(--fs-sm);
  }

  @media (max-width: 860px) {
    .add {
      grid-template-columns: 1fr;
      grid-template-rows: minmax(0, 1fr) minmax(0, 1fr) auto;
    }
  }
</style>
