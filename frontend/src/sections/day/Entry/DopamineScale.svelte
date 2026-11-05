<script lang="ts">
  // Дофаминовая шкала (§17.2): 7 квадратиков; клик — вписать, что дало дофамин,
  // заполненный квадратик «загорается». Уровень = число заполненных.
  import { tick } from 'svelte'
  import { slide } from 'svelte/transition'
  import Button from '../../../lib/components/Button.svelte'
  import { t } from '../../../lib/i18n/i18n.svelte'
  import { MAX_DOPAMINE_LEN } from '../../../lib/journal'

  interface Props {
    slots: (string | null)[]
    onsave: (slot: number, text: string) => Promise<void>
  }
  let { slots, onsave }: Props = $props()

  let selected = $state<number | null>(null)
  let draft = $state('')
  let saving = $state(false)
  let input = $state<HTMLInputElement>()

  async function open(i: number) {
    if (selected === i) {
      selected = null
      return
    }
    selected = i
    draft = slots[i] ?? ''
    await tick()
    input?.focus()
  }

  async function save(text: string) {
    if (selected === null) return
    saving = true
    try {
      await onsave(selected, text.trim())
      selected = null
    } finally {
      saving = false
    }
  }
</script>

<div class="scale" role="group" aria-label={t('journal.dopamine')}>
  {#each slots as text, i (i)}
    <button
      class="sq"
      class:lit={text !== null}
      class:selected={selected === i}
      aria-label={text ?? t('journal.dopamineSlot', { n: i + 1 })}
      aria-pressed={selected === i}
      title={text ?? undefined}
      onclick={() => open(i)}
    ></button>
  {/each}
</div>

{#if selected !== null}
  {@const slot = selected}
  <form
    class="editor"
    transition:slide={{ duration: 200 }}
    onsubmit={(e) => {
      e.preventDefault()
      void save(draft)
    }}
  >
    <input
      bind:this={input}
      bind:value={draft}
      class="input"
      maxlength={MAX_DOPAMINE_LEN}
      placeholder={t('journal.dopaminePlaceholder')}
      onkeydown={(e) => e.key === 'Escape' && (selected = null)}
    />
    <Button type="submit" variant="primary" size="sm" icon="check" loading={saving} disabled={!draft.trim()}>{t('common.save')}</Button>
    {#if slots[slot] !== null}
      <Button size="sm" variant="ghost" icon="x" disabled={saving} onclick={() => save('')}>{t('journal.dopamineClear')}</Button>
    {/if}
  </form>
{:else if slots.some((s) => s !== null)}
  <ol class="list">
    {#each slots as text, i (i)}
      {#if text !== null}
        <li><button class="item" onclick={() => open(i)}>{text}</button></li>
      {/if}
    {/each}
  </ol>
{:else}
  <p class="muted hint">{t('journal.dopamineHint')}</p>
{/if}

<style>
  .scale {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: var(--s-2);
    padding: var(--s-2);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface-2);
  }
  .sq {
    aspect-ratio: 1;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    cursor: pointer;
    transition:
      background var(--dur) var(--ease-out),
      transform var(--dur-fast) var(--ease-out),
      box-shadow var(--dur) var(--ease-out);
  }
  .sq:hover {
    transform: translate(-1px, -1px);
    box-shadow: var(--shadow-sm);
  }
  /* «Загорается»: заливка + свечение + короткий spring-«пульс» при включении */
  .lit {
    background: var(--c-green);
    box-shadow:
      var(--shadow-sm),
      0 0 12px color-mix(in srgb, var(--c-green) 55%, transparent);
    animation: light-up var(--dur-slow) var(--ease-spring);
  }
  .selected {
    outline: var(--bw) solid var(--c-focus);
    outline-offset: 2px;
  }
  @keyframes light-up {
    from {
      transform: scale(0.85);
    }
    to {
      transform: scale(1);
    }
  }

  .editor {
    display: flex;
    gap: var(--s-2);
    align-items: center;
    margin-top: var(--s-3);
  }
  .list {
    margin: var(--s-3) 0 0;
    padding-left: var(--s-5);
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .item {
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
    cursor: pointer;
  }
  .item:hover {
    text-decoration: underline;
  }
  .hint {
    margin-top: var(--s-3);
    font-size: var(--fs-sm);
  }
</style>
