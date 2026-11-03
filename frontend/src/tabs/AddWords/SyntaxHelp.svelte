<script lang="ts">
  import Button from '../../lib/components/Button.svelte'
  import { t } from '../../lib/i18n/i18n.svelte'

  let { oninsert }: { oninsert: () => void } = $props()
</script>

<details class="help">
  <summary>{t('add.help.title')}: <code>{t('add.help.line')}</code></summary>
  <ul>
    <li>{t('add.help.optional')}</li>
    <li>{t('add.help.examples')}</li>
    <li>{t('add.help.forms')}</li>
    <li>{t('add.help.tags')}</li>
    <li>{t('add.help.comments')}</li>
  </ul>
  <Button size="sm" icon="plus" onclick={oninsert}>{t('add.help.insertExample')}</Button>
</details>

<style>
  .help {
    font-size: var(--fs-sm);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    padding: var(--s-2) var(--s-3);
    background: var(--c-surface-2);
  }
  summary {
    cursor: pointer;
    font-weight: 600;
  }
  code {
    font-weight: 500;
    background: var(--c-surface);
    padding: 0 4px;
    border-radius: var(--radius-sm);
  }
  ul {
    margin: var(--s-2) 0 var(--s-3);
    padding-left: var(--s-4);
    color: var(--c-muted);
  }
</style>
