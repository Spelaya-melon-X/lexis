<script lang="ts">
  import { untrack } from 'svelte'
  import type { AmbitionInput, AmbitionKind, Ambitions } from '../../lib/api/types'
  import { descendants } from '../../lib/ambitions'
  import Button from '../../lib/components/Button.svelte'
  import Modal from '../../lib/components/Modal.svelte'
  import { t } from '../../lib/i18n/i18n.svelte'

  let { kind, initial, data, id = null, saving, onsave, onclose }: {
    kind: AmbitionKind; initial: AmbitionInput; data: Ambitions; id?: number | null
    saving: boolean; onsave: (v: AmbitionInput) => void; onclose: () => void
  } = $props()
  const formId = $props.id()
  let title = $state(untrack(() => initial.title))
  let note = $state(untrack(() => initial.note))
  let deadline = $state(untrack(() => initial.deadline ?? ''))
  let position = $state(untrack(() => initial.position))
  let status = $state(untrack(() => initial.status ?? 'want'))
  let categoryId = $state(untrack(() => initial.categoryId ?? null))
  let projectId = $state(untrack(() => initial.projectId ?? null))
  let parentId = $state(untrack(() => initial.parentId ?? null))
  let requires = $state(untrack(() => initial.requires ?? []))
  const forbidden = $derived(id === null ? new Set<number>() : descendants(data.steps, id))
  const steps = $derived(data.steps.filter(s => s.projectId === projectId))

  function save(e: SubmitEvent) {
    e.preventDefault()
    const v: AmbitionInput = { title, note, deadline: deadline || null, position }
    if (kind === 'dreams') Object.assign(v, { status, categoryId, projectId })
    if (kind === 'steps') Object.assign(v, { parentId, projectId, requires, done: initial.done ?? false })
    onsave(v)
  }
</script>

<Modal open title={id === null ? t(`ambitions.new.${kind}`) : t(`ambitions.edit.${kind}`)} onclose={() => !saving && onclose()} width={560}>
  <form id={formId} onsubmit={save} class="form">
    <label>
      <span>{t('ambitions.title')}</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input class="input" bind:value={title} required maxlength={kind === 'categories' ? 60 : 200} disabled={saving} autofocus
        placeholder={kind === 'dreams' ? t('ambitions.dreamPlaceholder') : undefined} />
    </label>
    {#if kind === 'dreams'}
      <div class="row">
        <label><span>{t('ambitions.category')}</span>
          <select class="input" bind:value={categoryId} disabled={saving}>
            <option value={null}>{t('ambitions.uncategorized')}</option>
            {#each data.categories as c}<option value={c.id}>{c.title}</option>{/each}
          </select>
        </label>
        {#if id !== null}
          <label><span>{t('ambitions.statusLabel')}</span>
            <select class="input" bind:value={status} disabled={saving}>
              {#each ['want', 'active', 'fulfilled'] as const as s}<option value={s}>{t(`ambitions.status.${s}`)}</option>{/each}
            </select>
          </label>
        {/if}
      </div>
    {/if}
    {#if kind !== 'categories'}
      <label><span>{t('ambitions.note')}</span><textarea class="input" bind:value={note} maxlength={20000} rows="3" disabled={saving}></textarea></label>
      <div class="row">
        <label><span>{t('ambitions.deadline')}</span><input class="input" type="date" bind:value={deadline} disabled={saving} /></label>
        {#if kind === 'steps'}
          <label><span>{t('ambitions.parent')}</span>
            <select class="input" bind:value={parentId} disabled={saving}>
              <option value={null}>{t('ambitions.root')}</option>
              {#each steps.filter(s => !forbidden.has(s.id)) as s}<option value={s.id}>{s.title}</option>{/each}
            </select>
          </label>
        {/if}
      </div>
    {/if}
    <details class="advanced">
      <summary>{t('ambitions.moreOptions')}</summary>
      <div class="extra">
        <label><span>{t('ambitions.position')}</span><input class="input" type="number" bind:value={position} min="0" max="2000" required disabled={saving} /></label>
        {#if kind === 'dreams'}
          <label><span>{t('ambitions.project')}</span>
            <select class="input" bind:value={projectId} disabled={saving}>
              <option value={null}>{t('ambitions.unlinked')}</option>
              {#each data.projects as p}<option value={p.id}>{p.title}</option>{/each}
            </select>
          </label>
        {/if}
      </div>
    </details>
    {#if kind === 'steps'}
      <fieldset disabled={saving}>
        <legend>{t('ambitions.requires')}</legend><p class="muted">{t('ambitions.arrow')}</p>
        {#each steps.filter(s => s.id !== id) as s}
          <label class="check"><input type="checkbox" bind:group={requires} value={s.id} /><span>{s.title}</span></label>
        {/each}
        {#if steps.filter(s => s.id !== id).length === 0}<p class="muted">{t('ambitions.noDependencies')}</p>{/if}
      </fieldset>
    {/if}
  </form>
  {#snippet footer()}
    <Button variant="ghost" onclick={onclose} disabled={saving}>{t('common.cancel')}</Button>
    <Button type="submit" form={formId} variant="primary" loading={saving} disabled={!title.trim()}>{id === null ? t('ambitions.create') : t('common.save')}</Button>
  {/snippet}
</Modal>

<style>
  .form,.extra{display:flex;flex-direction:column;gap:var(--s-4)}
  label{display:flex;flex-direction:column;gap:var(--s-1);min-width:0}
  label>span,legend{font-weight:600;font-size:var(--fs-sm)}
  .row{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:var(--s-3)}
  textarea{resize:vertical}.advanced summary{cursor:pointer;color:var(--c-muted);font-size:var(--fs-sm)}
  .extra{padding-top:var(--s-3)}
  fieldset{margin:0;max-height:240px;overflow:auto;border:var(--bw) solid var(--c-line);border-radius:var(--radius-sm);padding:var(--s-3)}
  fieldset p{font-size:var(--fs-xs);margin-bottom:var(--s-3)}
  .check{flex-direction:row;align-items:center;gap:var(--s-2);padding:var(--s-1) 0;overflow-wrap:anywhere}
  .check input{accent-color:var(--c-green)}.check span{font-weight:400}
</style>
