<script lang="ts">
  import { untrack } from 'svelte'
  import type { StudyCard, StudyGroup } from '../../lib/api/types'
  import { STUDY_KINDS } from '../../lib/study'
  import { visibleRows } from '../../lib/groupTree'
  import { t } from '../../lib/i18n/i18n.svelte'
  import Modal from '../../lib/components/Modal.svelte'
  import Markdown from '../../lib/components/Markdown.svelte'
  import Button from '../../lib/components/Button.svelte'
  let {
    initial,
    groups,
    busy,
    onsave,
    onclose,
  }: {
    initial: StudyCard
    groups: StudyGroup[]
    busy: boolean
    onsave: (card: StudyCard) => void
    onclose: () => void
  } = $props()
  const formId = $props.id()
  let card = $state<StudyCard>(untrack(() => $state.snapshot(initial)))
  const content = $derived(card.kind === 'grammar_rule' ? card.content : null)
</script>

<Modal open title={t(card.kind === 'grammar_rule'
  ? (card.id === 0 ? 'grammar.newRule' : 'grammar.editRule')
  : 'study.editCard')} width={840} {onclose}>
  <form
    id={formId}
    onsubmit={(e) => {
      e.preventDefault()
      onsave(card)
    }}
  >
    <div class="fields">
      {#if card.kind !== 'grammar_rule'}<label
        >{t('study.kind')}<select
          class="input"
          bind:value={card.kind}
          disabled={busy}
          >{#each STUDY_KINDS as k}<option value={k}
              >{t(`study.kinds.${k}`)}</option
            >{/each}</select
        ></label
      >{/if}
      <label
        >{t(content ? 'grammar.topic' : 'study.group')}<select
          class="input"
          bind:value={card.groupId}
          disabled={busy}
          >{#each visibleRows(groups) as row}<option value={row.group.id}
              >{'— '.repeat(row.depth)}{row.group.title}</option
            >{/each}</select
        ></label
      >
    </div>
    {#if content}
      <label>
        {t('grammar.ruleTitle')}
        <input class="input" bind:value={card.front} maxlength="200" lang="en" required disabled={busy} />
      </label>
      <label>
        {t('grammar.formula')}
        <input class="input formula" bind:value={content.formula} maxlength="2000" lang="en" disabled={busy} />
      </label>
      <label>
        {t('grammar.explanation')}
        <textarea class="input" bind:value={card.back} maxlength="20000" rows="6" required disabled={busy}></textarea>
        <span class="muted">{t('grammar.markdownHelp')}</span>
      </label>
      <fieldset>
        <legend>{t('grammar.examples')}</legend>
        {#each content.examples ?? [] as example, index}
          <div class="entry">
            <label>{t('grammar.example')}
              <textarea class="input" bind:value={example.text} maxlength="2000" rows="2" lang="en" required disabled={busy}></textarea>
            </label>
            <label>{t('grammar.translation')}
              <textarea class="input" bind:value={example.translation} maxlength="2000" rows="2" disabled={busy}></textarea>
            </label>
            <Button size="sm" variant="ghost" icon="x" aria-label={t('grammar.removeExample', { index: index + 1 })}
              disabled={busy} onclick={() => content.examples?.splice(index, 1)} />
          </div>
        {/each}
        <Button size="sm" icon="plus" disabled={busy || (content.examples?.length ?? 0) >= 20}
          onclick={() => (content.examples = [...(content.examples ?? []), { text: '', translation: '' }])}>{t('grammar.addExample')}</Button>
      </fieldset>
      <fieldset>
        <legend>{t('grammar.mistakes')}</legend>
        {#each content.mistakes ?? [] as mistake, index}
          <div class="entry">
            <label>{t('grammar.incorrect')}
              <textarea class="input" bind:value={mistake.incorrect} maxlength="2000" rows="2" lang="en" required disabled={busy}></textarea>
            </label>
            <label>{t('grammar.correct')}
              <textarea class="input" bind:value={mistake.correct} maxlength="2000" rows="2" lang="en" required disabled={busy}></textarea>
            </label>
            <Button size="sm" variant="ghost" icon="x" aria-label={t('grammar.removeMistake', { index: index + 1 })}
              disabled={busy} onclick={() => content.mistakes?.splice(index, 1)} />
          </div>
        {/each}
        <Button size="sm" icon="plus" disabled={busy || (content.mistakes?.length ?? 0) >= 20}
          onclick={() => (content.mistakes = [...(content.mistakes ?? []), { incorrect: '', correct: '' }])}>{t('grammar.addMistake')}</Button>
      </fieldset>
    {:else}
      <div class="sides">
      <label
        >{t('study.front')}<textarea
          class="input"
          bind:value={card.front}
          maxlength="20000"
          rows="7"
          required
          disabled={busy}></textarea><Markdown source={card.front} /></label
      >
      <label
        >{t('study.back')}<textarea
          class="input"
          bind:value={card.back}
          maxlength="20000"
          rows="7"
          required
          disabled={busy}></textarea><Markdown source={card.back} /></label
      >
      </div>
      <p class="muted">{t('study.markdownHelp')}</p>
    {/if}
  </form>
  {#snippet footer()}
    <Button variant="ghost" disabled={busy} onclick={onclose}>{t('common.cancel')}</Button>
    <Button type="submit" form={formId} loading={busy} variant="primary">{t('common.save')}</Button>
  {/snippet}
</Modal>

<style>
  form,
  label {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  form {
    gap: var(--s-4);
    max-height: min(65dvh, 44rem);
    overflow: auto;
    padding: 4px;
  }
  .fields,
  .sides {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-4);
  }
  label {
    min-width: 0;
  }
  textarea, .formula {
    font-family: monospace;
  }
  textarea {
    resize: vertical;
  }
  fieldset {
    min-width: 0;
    border: 1px solid var(--c-line);
    border-radius: var(--radius-sm);
    padding: var(--s-3);
    margin: 0;
  }
  legend {
    font-weight: 700;
    padding-inline: var(--s-2);
  }
  .entry {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr)) auto;
    gap: var(--s-2);
    align-items: start;
    margin-bottom: var(--s-3);
  }
  p, .muted {
    font-size: var(--fs-sm);
  }
  @media (max-width: 650px) {
    .fields,
    .sides {
      grid-template-columns: 1fr;
    }
    .entry {
      grid-template-columns: minmax(0, 1fr) auto;
    }
    .entry label {
      grid-column: 1;
    }
    .entry :global(button) {
      grid-column: 2;
      grid-row: 1;
    }
  }
</style>
