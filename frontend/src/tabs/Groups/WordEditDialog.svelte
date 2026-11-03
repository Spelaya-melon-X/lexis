<script lang="ts">
  import { api } from '../../lib/api/client'
  import type { Word } from '../../lib/api/types'
  import Button from '../../lib/components/Button.svelte'
  import Modal from '../../lib/components/Modal.svelte'
  import { splitLines, splitTags } from '../../lib/form'
  import { errorText, t } from '../../lib/i18n/i18n.svelte'

  // word — что редактируем (null — диалог закрыт). Поля — копия значений слова на момент открытия.
  let { word, onclose, onsaved }: { word: Word | null; onclose: () => void; onsaved: (w: Word) => void } = $props()

  let term = $state('')
  let translation = $state('')
  let examples = $state('')
  let groups = $state('')
  let saving = $state(false)
  let error = $state<string | null>(null)

  // При открытии на другом слове — заново заполнить форму.
  $effect(() => {
    if (!word) return
    term = word.term
    translation = word.translation
    examples = (word.examples ?? []).map((e) => e.text).join('\n')
    groups = (word.groups ?? []).map((g) => g.name).join(' ')
    error = null
  })

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (!word || saving) return
    saving = true
    error = null
    try {
      // Отправляем все поля: examples/groups заменяют целиком (§11), бэк нормализует их по правилам §4.
      const saved = await api.words.update(word.id, {
        term,
        translation,
        examples: splitLines(examples),
        groups: splitTags(groups),
      })
      onsaved(saved)
    } catch (err) {
      error = errorText(err) // показываем в форме: правка не закрывается, можно исправить
    } finally {
      saving = false
    }
  }
</script>

<Modal open={word !== null} title={t('words.editTitle')} width={560} {onclose}>
  <form id="word-edit" class="form" onsubmit={save}>
    <label>
      <span>{t('words.term')}</span>
      <input class="input" bind:value={term} required />
      <small class="muted">{t('words.termHint')}</small>
    </label>
    <label>
      <span>{t('words.translation')}</span>
      <input class="input" bind:value={translation} required />
    </label>
    <label>
      <span>{t('words.examples')}</span>
      <textarea class="input" rows="4" bind:value={examples}></textarea>
    </label>
    <label>
      <span>{t('words.groups')}</span>
      <input class="input" bind:value={groups} />
    </label>
    {#if error}<p class="error">{error}</p>{/if}
  </form>
  {#snippet footer()}
    <Button variant="ghost" onclick={onclose} disabled={saving}>{t('common.cancel')}</Button>
    <Button variant="primary" type="submit" form="word-edit" loading={saving}>{t('common.save')}</Button>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  label {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    font-weight: 600;
    font-size: var(--fs-sm);
  }
  small {
    font-weight: 400;
  }
  textarea {
    resize: vertical;
    font-family: ui-monospace, 'SF Mono', Menlo, monospace;
  }
  .error {
    color: var(--c-red);
    font-weight: 600;
  }
</style>
