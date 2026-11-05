<script lang="ts">
  // «Построить промежуточные шаги» (§18.3, B2): список шагов (название + объём).
  // Важность, дедлайн и тип шаги наследуют от дела — поменять можно потом у каждого шага.
  import { tick } from 'svelte'
  import { api } from '../../../lib/api/client'
  import type { Task } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import Modal from '../../../lib/components/Modal.svelte'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { blocksText } from '../format'

  interface Props {
    task: Task | null // что разбиваем (null — закрыт)
    onclose: () => void
    onsplit: (tree: Task[]) => void
  }
  let { task, onclose, onsplit }: Props = $props()

  let steps = $state<{ title: string; blocks: number }[]>([])
  let saving = $state(false)
  let error = $state<string | null>(null)
  let list: HTMLElement | undefined = $state()

  $effect(() => {
    if (!task) return
    steps = [{ title: '', blocks: 1 }, { title: '', blocks: 1 }]
    error = null
  })

  const total = $derived(steps.reduce((sum, s) => sum + (s.title.trim() ? s.blocks : 0), 0))

  async function focusStep(i: number) {
    await tick()
    list?.querySelectorAll<HTMLInputElement>('input[type=text]')[i]?.focus()
  }

  function addAfter(i: number) {
    steps.splice(i + 1, 0, { title: '', blocks: 1 })
    void focusStep(i + 1)
  }

  function onKey(e: KeyboardEvent, i: number) {
    if (e.key === 'Enter' && !e.isComposing) {
      e.preventDefault()
      if (steps[i]!.title.trim()) addAfter(i)
    }
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (!task || saving) return
    const filled = steps.filter((s) => s.title.trim())
    saving = true
    error = null
    try {
      onsplit(await api.tasks.split(task.id, filled))
    } catch (err) {
      error = errorText(err)
    } finally {
      saving = false
    }
  }
</script>

<Modal open={task !== null} title={t('game.splitTitle', { title: task?.title ?? '' })} width={560} {onclose}>
  <form id="split-form" class="form" onsubmit={save}>
    <p class="muted hint">{t('game.splitHint')}</p>
    <ol bind:this={list}>
      {#each steps as step, i (i)}
        <li>
          <input
            class="input title"
            type="text"
            bind:value={step.title}
            maxlength="200"
            placeholder={t('game.stepPlaceholder', { n: i + 1 })}
            onkeydown={(e) => onKey(e, i)}
          />
          <input class="input blocks" type="number" min="1" max="40" bind:value={step.blocks} aria-label={t('game.blocks')} title={blocksText(step.blocks)} />
          <button type="button" class="remove" aria-label={t('game.removeStep')} title={t('game.removeStep')} disabled={steps.length === 1}
            onclick={() => steps.splice(i, 1)}>
            <Icon name="x" size={16} />
          </button>
        </li>
      {/each}
    </ol>
    <div class="foot">
      <Button size="sm" variant="ghost" icon="plus" onclick={() => addAfter(steps.length - 1)}>{t('game.addStep')}</Button>
      {#if total > 0}<span class="muted">Σ {blocksText(total)}</span>{/if}
    </div>
    {#if error}<p class="error">{error}</p>{/if}
  </form>

  {#snippet footer()}
    <Button variant="ghost" onclick={onclose} disabled={saving}>{t('common.cancel')}</Button>
    <Button type="submit" form="split-form" variant="primary" icon="steps" loading={saving} disabled={total === 0}>{t('game.split')}</Button>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  .hint {
    margin: 0;
    font-size: var(--fs-sm);
  }
  ol {
    margin: 0;
    padding-left: var(--s-5);
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  .title {
    flex: 1;
    min-width: 0;
  }
  .blocks {
    width: 72px;
  }
  .remove {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--c-muted);
    cursor: pointer;
  }
  .remove:hover:not(:disabled) {
    background: var(--c-surface-2);
    color: var(--c-text);
  }
  .remove:disabled {
    opacity: 0.3;
    cursor: default;
  }
  .foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: var(--fs-sm);
  }
  .error {
    color: var(--c-red);
    font-size: var(--fs-sm);
  }
</style>
