<script lang="ts">
  import type { Word } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import LevelBadge from '../../../lib/components/LevelBadge.svelte'
  import { t } from '../../../lib/i18n/i18n.svelte'
  import { displayTerm } from '../../../lib/text'

  interface Props {
    word: Word
    onedit: (w: Word) => void
    ondelete: (w: Word) => void
    onunarchive: (w: Word) => void
  }
  let { word, onedit, ondelete, onunarchive }: Props = $props()
</script>

<li class="row">
  <div class="text">
    <span class="term">{displayTerm(word.term, word.forms)}</span>
    <span class="translation muted">{word.translation}</span>
  </div>
  <LevelBadge level={word.level} />
  <div class="actions">
    {#if word.level === 5}
      <Button size="sm" variant="ghost" icon="archive" title={t('words.unarchive')} aria-label={t('words.unarchive')} onclick={() => onunarchive(word)} />
    {/if}
    <Button size="sm" variant="ghost" icon="edit" title={t('words.edit')} aria-label={t('words.edit')} onclick={() => onedit(word)} />
    <Button size="sm" variant="ghost" icon="trash" title={t('words.delete')} aria-label={t('words.delete')} onclick={() => ondelete(word)} />
  </div>
</li>

<style>
  .row {
    display: grid;
    grid-template-columns: 1fr auto auto;
    align-items: center;
    gap: var(--s-3);
    padding: var(--s-2) var(--s-3);
    border-bottom: 1px solid var(--c-line);
  }
  .row:hover {
    background: var(--c-surface-2);
  }
  .text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .term {
    font-weight: 700;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .translation {
    font-size: var(--fs-sm);
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .actions {
    display: flex;
    gap: 2px;
  }
</style>
