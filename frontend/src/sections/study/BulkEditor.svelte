<script lang="ts">
  import { untrack } from 'svelte'
  import { api } from '../../lib/api/client'
  import type { StudyData, StudyGroup, StudyParse } from '../../lib/api/types'
  import { latest } from '../../lib/async'
  import { visibleRows } from '../../lib/groupTree'
  import { t, errorText, lineErrorText } from '../../lib/i18n/i18n.svelte'
  import Modal from '../../lib/components/Modal.svelte'
  import Markdown from '../../lib/components/Markdown.svelte'
  import Button from '../../lib/components/Button.svelte'
  import { toasts } from '../../lib/state/toasts.svelte'
  let {
    groupId,
    groups,
    grammar,
    onsave,
    onclose,
  }: {
    groupId: number
    groups: StudyGroup[]
    grammar: boolean
    onsave: (data: StudyData, targetGroup: number) => void
    onclose: () => void
  } = $props()
  let targetGroup = $state(untrack(() => groupId))
  let text = $state(''),
    result = $state<StudyParse | null>(null),
    parsing = $state(false),
    busy = $state(false),
    failure = $state('')
  const parse = latest(api.study.parse)
  $effect(() => {
    const source = text
    const mode = grammar
    parse.cancel()
    result = null
    failure = ''
    if (!source.trim()) {
      parsing = false
      return
    }
    parsing = true
    const timer = setTimeout(async () => {
      try {
        const v = await parse(source, mode)
        if (!v.stale) {
          result = v.value
          parsing = false
        }
      } catch (e) {
        failure = errorText(e)
        parsing = false
      }
    }, 300)
    return () => {
      clearTimeout(timer)
      parse.cancel()
    }
  })
  async function save() {
    if (busy || parsing || !result?.cards.length || result.errors.length) return
    const target = targetGroup
    busy = true
    try {
      onsave(await api.study.bulk(target, text), target)
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
    }
  }
</script>

<Modal
  open
  title={t('study.bulk')}
  width={1000}
  onclose={() => {
    if (!busy) onclose()
  }}
>
  {#if grammar}
    <label class="topic">
      {t('grammar.topic')}
      <select class="input" bind:value={targetGroup} disabled={busy}>
        {#each visibleRows(groups) as row (row.group.id)}
          <option value={row.group.id}>{'— '.repeat(row.depth)}{row.group.title}</option>
        {/each}
      </select>
    </label>
    <p>{t('grammar.bulkSyntax')}</p>
    <details>
      <summary>{t('grammar.bulkSample')}</summary>
      <pre>{t('grammar.bulkExample')}</pre>
    </details>
  {:else}
    <p>{t('study.syntax')}</p>
    <pre>{t('study.example')}</pre>
    <p class="muted">{t('study.escapeHelp')}</p>
  {/if}
  <div class="layout">
    <label
      >{t('study.source')}<textarea
        class="input"
        bind:value={text}
        disabled={busy}
        rows="16"
        onkeydown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault()
            save()
          }
        }}></textarea></label
    >
    <div class="preview" aria-live="polite">
      <h3>{t('study.preview')}</h3>
      {#if parsing}<p>{t('study.parsing')}</p>{/if}{#if failure}<p role="alert">
          {failure}
        </p>{/if}
      {#each result?.errors ?? [] as err}<p class="error">
          {t('study.recordError', { record: err.record, line: err.line })}: {lineErrorText(
            err.code,
          )}
        </p>{/each}
      {#each result?.cards ?? [] as card (card.record)}
        <article>
          <small>{t('study.record', { record: card.record })}</small>
          {#if card.kind === 'grammar_rule'}
            <h4 lang="en">{card.front}</h4>
            {#if card.content.formula}<code lang="en">{card.content.formula}</code>{/if}
            <Markdown source={card.back} />
            {#each card.content.examples ?? [] as example}
              <p><strong lang="en">{example.text}</strong>{#if example.translation}<br />{example.translation}{/if}</p>
            {/each}
            {#each card.content.mistakes ?? [] as mistake}
              <p lang="en"><del>{mistake.incorrect}</del> → {mistake.correct}</p>
            {/each}
          {:else}
            <small> · {t(`study.kinds.${card.kind}`)}</small>
            <Markdown source={card.front} />
            <hr />
            <Markdown source={card.back} />
          {/if}
        </article>
      {/each}
    </div>
  </div>
  <div class="actions">
    <Button
      variant="primary"
      loading={busy}
      disabled={parsing ||
        !result?.cards.length ||
        !!result.errors.length ||
        !!failure}
      onclick={save}
      >{t('study.createCards', { count: result?.cards.length ?? 0 })}</Button
    >
  </div>
</Modal>

<style>
  pre {
    background: var(--c-surface-2);
    padding: var(--s-3);
    white-space: pre-wrap;
    margin: var(--s-3) 0;
    font-size: var(--fs-sm);
  }
  .topic {
    max-width: 350px;
    margin-bottom: var(--s-3);
  }
  details {
    margin-top: var(--s-2);
  }
  summary {
    cursor: pointer;
  }
  .layout {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-4);
    margin-top: var(--s-4);
    max-height: 55vh;
    overflow: auto;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    min-width: 0;
  }
  textarea {
    font-family: monospace;
    resize: vertical;
  }
  .preview {
    min-width: 0;
  }
  .preview article {
    border: var(--bw) solid var(--c-line);
    padding: var(--s-3);
    margin: var(--s-3) 0;
    border-radius: var(--radius-sm);
    overflow-wrap: anywhere;
  }
  .preview h4 {
    margin: var(--s-1) 0;
  }
  .error {
    color: var(--c-red);
  }
  .actions {
    margin-top: var(--s-4);
  }
  @media (max-width: 650px) {
    .layout {
      grid-template-columns: 1fr;
    }
  }
</style>
