<script lang="ts">
  // «+ Любое дело» (§18.3, B3): поиск по всем открытым делам-листьям, которых ещё нет в плане.
  import { api } from '../../../lib/api/client'
  import type { Task } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Modal from '../../../lib/components/Modal.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { ancestors } from '../../../lib/tasks'
  import { blocksText } from '../format'

  interface Props {
    open: boolean
    planned: Set<number> // уже в плане — не предлагаем
    onclose: () => void
    onpick: (task: Task) => void
  }
  let { open, planned, onclose, onpick }: Props = $props()

  let all = $state<Task[] | null>(null)
  let error = $state<string | null>(null)
  let query = $state('')

  $effect(() => {
    if (!open) return
    query = ''
    all = null
    api.tasks
      .list('false')
      .then((ts) => (all = ts))
      .catch((e: unknown) => (error = errorText(e)))
  })

  const found = $derived.by(() => {
    const q = query.trim().toLowerCase()
    return (all ?? []).filter((x) => x.leaf && !x.done && !planned.has(x.id) && (q === '' || x.title.toLowerCase().includes(q)))
  })
</script>

<Modal {open} title={t('game.plan.pickTitle')} width={520} {onclose}>
  <!-- svelte-ignore a11y_autofocus -->
  <input class="input search" bind:value={query} placeholder={t('game.plan.pickSearch')} aria-label={t('game.plan.pickSearch')} autofocus />
  {#if error && !all}
    <p class="error">{error}</p>
  {:else if !all}
    <div class="state"><Spinner size={24} /></div>
  {:else if found.length === 0}
    <p class="muted state">{t('game.plan.pickEmpty')}</p>
  {:else}
    <ul class="scroll">
      {#each found as task (task.id)}
        {@const path = ancestors(all, task.id)}
        <li>
          <button onclick={() => onpick(task)}>
            {#if path.length > 0}<span class="path muted">{path.map((p) => p.title).join(' › ')}</span>{/if}
            <span class="title">{task.title}</span>
            <span class="meta muted">{blocksText(task.blocks)} · ★ {task.importance}{task.deadline ? ` · ${task.deadline}` : ''}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  {#snippet footer()}
    <Button variant="ghost" onclick={onclose}>{t('common.close')}</Button>
  {/snippet}
</Modal>

<style>
  .search {
    width: 100%;
    margin-bottom: var(--s-3);
  }
  .state {
    display: grid;
    place-items: center;
    padding: var(--s-5);
  }
  ul {
    max-height: 50vh;
    margin: 0;
    padding: 0;
    list-style: none;
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
  }
  li + li {
    border-top: 1px solid var(--c-line);
  }
  button {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    padding: var(--s-2) var(--s-3);
    border: 0;
    background: var(--c-surface);
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition: background var(--dur-fast) var(--ease-out);
  }
  button:hover {
    background: var(--c-green-soft);
  }
  .path,
  .meta {
    font-size: var(--fs-xs);
  }
  .title {
    font-weight: 700;
  }
  .error {
    color: var(--c-red);
  }
</style>
