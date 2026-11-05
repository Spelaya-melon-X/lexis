<script lang="ts">
  import { api } from '../../../lib/api/client'
  import type { Group } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Modal from '../../../lib/components/Modal.svelte'
  import { canCreateIn, visibleRows } from '../../../lib/groupTree'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'

  // Пустая группа (папка) — раньше группы появлялись только из тегов (§18.2).
  // parent — где создать по умолчанию (открытая сейчас группа или корень).
  interface Props {
    open: boolean
    groups: Group[]
    parent: number | null
    onclose: () => void
    oncreated: (g: Group) => void
  }
  let { open, groups, parent, onclose, oncreated }: Props = $props()

  let name = $state('')
  let parentId = $state<number | null>(null)
  let saving = $state(false)
  let error = $state<string | null>(null)

  const rows = $derived(visibleRows(groups))

  $effect(() => {
    if (!open) return
    name = ''
    parentId = parent !== null && canCreateIn(groups, parent) ? parent : null
    error = null
  })

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (saving) return
    saving = true
    error = null
    try {
      oncreated(await api.groups.create({ name, parentId }))
    } catch (err) {
      error = errorText(err)
    } finally {
      saving = false
    }
  }
</script>

<Modal {open} title={t('groups.newGroupTitle')} {onclose}>
  <form id="group-new" class="form" onsubmit={save}>
    <label>
      <span>{t('groups.name')}</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input class="input" bind:value={name} required autofocus />
      <small class="muted">{t('groups.nameHint')}</small>
    </label>
    <label>
      <span>{t('groups.parent')}</span>
      <select class="input" bind:value={parentId}>
        <option value={null}>{t('groups.root')}</option>
        {#each rows as r (r.group.id)}
          <!-- Отступ в <option> только пробелами: стили внутри select браузеры почти не применяют. -->
          <option value={r.group.id} disabled={!canCreateIn(groups, r.group.id)}>
            {'  '.repeat(r.depth + 1)}#{r.group.name}
          </option>
        {/each}
      </select>
    </label>
    {#if error}<p class="error">{error}</p>{/if}
  </form>
  {#snippet footer()}
    <Button variant="ghost" onclick={onclose} disabled={saving}>{t('common.cancel')}</Button>
    <Button variant="primary" type="submit" form="group-new" icon="plus" loading={saving}>{t('groups.create')}</Button>
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
  .error {
    color: var(--c-red);
    font-weight: 600;
  }
</style>
