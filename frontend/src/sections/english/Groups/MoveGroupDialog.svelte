<script lang="ts">
  import { api } from '../../../lib/api/client'
  import type { Group } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Modal from '../../../lib/components/Modal.svelte'
  import { canMove, visibleRows } from '../../../lib/groupTree'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'

  // «Переместить в…» — альтернатива перетаскиванию с клавиатуры (§18.2).
  // group — что переносим (null — закрыт); недоступные цели — disabled, с той же проверкой, что и DnD.
  let { group, groups, onclose, onmoved }: { group: Group | null; groups: Group[]; onclose: () => void; onmoved: () => void } =
    $props()

  let busy = $state(false)
  const rows = $derived(visibleRows(groups))

  async function move(target: number | null) {
    if (!group || busy) return
    busy = true
    try {
      await api.groups.update(group.id, { parentId: target })
      toasts.success(t('groups.moved'))
      onmoved()
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      busy = false
    }
  }
</script>

<Modal open={group !== null} title={t('groups.moveTitle', { name: group?.name ?? '' })} width={420} {onclose}>
  {#if group}
    <p class="muted hint">{t('groups.moveHint')}</p>
    <ul class="targets">
      <li>
        <button class="target" disabled={busy || !canMove(groups, group.id, null)} onclick={() => move(null)}>
          {t('groups.root')}
        </button>
      </li>
      {#each rows as r (r.group.id)}
        <li>
          <button
            class="target"
            class:current={r.group.id === group.parentId}
            style:padding-left="calc(var(--s-3) + {r.depth + 1} * var(--s-4))"
            disabled={busy || !canMove(groups, group.id, r.group.id)}
            onclick={() => move(r.group.id)}
          >
            <span class="dot" style:background={r.group.color ?? 'var(--c-surface)'}></span>
            #{r.group.name}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  {#snippet footer()}
    <Button variant="ghost" onclick={onclose} disabled={busy}>{t('common.cancel')}</Button>
  {/snippet}
</Modal>

<style>
  .hint {
    margin: 0 0 var(--s-3);
    font-size: var(--fs-sm);
  }
  .targets {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 50vh;
    overflow-y: auto;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
  }
  li + li {
    border-top: 1px solid var(--c-line);
  }
  .target {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    width: 100%;
    padding: var(--s-2) var(--s-3);
    border: 0;
    background: var(--c-surface);
    color: inherit;
    font: inherit;
    font-weight: 600;
    text-align: left;
    cursor: pointer;
    transition: background var(--dur-fast) var(--ease-out);
  }
  .target:hover:not(:disabled) {
    background: var(--c-green-soft);
  }
  .target:disabled {
    color: var(--c-muted);
    font-weight: 400;
    cursor: not-allowed;
  }
  .target.current {
    text-decoration: underline;
  }
  .dot {
    flex: none;
    width: 10px;
    height: 10px;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
</style>
