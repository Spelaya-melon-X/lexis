<script lang="ts">
  // «Что классного сделал»: добавить / изменить / удалить / переставить.
  // Каждое изменение отдаёт наружу новый список целиком — PATCH заменяет его на бэке.
  import { fade } from 'svelte/transition'
  import Icon, { type IconName } from '../../../lib/components/Icon.svelte'
  import { t } from '../../../lib/i18n/i18n.svelte'
  import { MAX_WIN_LEN, move } from '../../../lib/journal'

  interface Props {
    wins: string[]
    onchange: (wins: string[]) => void
  }
  let { wins, onchange }: Props = $props()

  let draft = $state('')
  let editing = $state<number | null>(null)
  let editDraft = $state('')

  function add() {
    const v = draft.trim()
    if (!v) return
    onchange([...wins, v])
    draft = ''
  }

  function commitEdit() {
    if (editing === null) return
    const i = editing
    const v = editDraft.trim()
    editing = null
    if (v === wins[i]) return
    // Пустой текст при правке = удалить пункт.
    onchange(v ? wins.map((w, j) => (j === i ? v : w)) : wins.filter((_, j) => j !== i))
  }

  /** Автофокус поля правки: action вместо bind:this — полей может быть одно на строку. */
  function focus(el: HTMLInputElement) {
    el.focus()
    el.select()
  }
</script>

{#snippet act(icon: IconName, label: string, onclick: () => void, disabled = false)}
  <button class="act" aria-label={label} title={label} {disabled} {onclick}><Icon name={icon} size={16} /></button>
{/snippet}

{#if wins.length === 0}
  <p class="muted empty">{t('journal.winsEmpty')}</p>
{:else}
  <ol class="wins">
    {#each wins as w, i (`${i}:${w}`)}
      <li in:fade={{ duration: 200 }}>
        <span class="n">{i + 1}.</span>
        {#if editing === i}
          <input
            class="input"
            bind:value={editDraft}
            maxlength={MAX_WIN_LEN}
            use:focus
            onblur={commitEdit}
            onkeydown={(e) => {
              if (e.key === 'Enter') commitEdit()
              if (e.key === 'Escape') editing = null
            }}
          />
        {:else}
          <button
            class="text"
            title={t('journal.edit')}
            onclick={() => {
              editing = i
              editDraft = w
            }}>{w}</button
          >
          <span class="acts">
            {@render act('chevronLeft', t('journal.moveUp'), () => onchange(move(wins, i, -1)), i === 0)}
            {@render act('chevronRight', t('journal.moveDown'), () => onchange(move(wins, i, 1)), i === wins.length - 1)}
            {@render act('trash', t('journal.remove'), () => onchange(wins.filter((_, j) => j !== i)))}
          </span>
        {/if}
      </li>
    {/each}
  </ol>
{/if}

<form
  class="add"
  onsubmit={(e) => {
    e.preventDefault()
    add()
  }}
>
  <Icon name="plus" size={18} />
  <input class="input" bind:value={draft} maxlength={MAX_WIN_LEN} placeholder={t('journal.winPlaceholder')} />
</form>

<style>
  .wins {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    min-height: 36px;
    border-bottom: 1px solid var(--c-line);
  }
  .n {
    width: 1.6em;
    flex: none;
    color: var(--c-muted);
    font-variant-numeric: tabular-nums;
  }
  .text {
    flex: 1;
    min-width: 0;
    padding: var(--s-1) 0;
    border: 0;
    background: none;
    text-align: left;
    cursor: text;
    overflow-wrap: anywhere;
  }
  .acts {
    display: flex;
    gap: 2px;
    opacity: 0;
    transition: opacity var(--dur-fast) var(--ease-out);
  }
  li:hover .acts,
  li:focus-within .acts {
    opacity: 1;
  }
  .act {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--c-muted);
    cursor: pointer;
  }
  /* стрелки «влево/вправо» повёрнуты — получаются «вверх/вниз» без новых иконок */
  .act:nth-child(1) :global(svg),
  .act:nth-child(2) :global(svg) {
    transform: rotate(90deg);
  }
  .act:hover:not(:disabled) {
    background: var(--c-surface-2);
    color: var(--c-text);
  }
  .act:disabled {
    opacity: 0.3;
    cursor: default;
  }
  .empty {
    font-size: var(--fs-sm);
  }
  .add {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    margin-top: var(--s-3);
    color: var(--c-muted);
  }
</style>
