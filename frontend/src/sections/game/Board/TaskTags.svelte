<script lang="ts">
  import { t } from '../../../lib/i18n/i18n.svelte'
  import { taskTags } from '../../../lib/taskTags'

  let { note }: { note: string } = $props()
  const tags = $derived(taskTags(note))
</script>

{#if tags.length > 0}
  <ul aria-label={t('game.tags')}>
    {#each tags as tag (tag)}<li>#{tag}</li>{/each}
  </ul>
{/if}

<style>
  ul {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-1);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--fs-xs);
  }
  li {
    max-width: 100%;
    padding: 0 var(--s-2);
    border: 1px solid var(--c-line);
    border-radius: 999px;
    background: var(--c-surface-2);
    color: var(--c-muted);
    overflow-wrap: anywhere;
  }
</style>
