<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import type { GrammarRule } from '../../lib/api/types'
  import Button from '../../lib/components/Button.svelte'
  import Markdown from '../../lib/components/Markdown.svelte'
  import { t } from '../../lib/i18n/i18n.svelte'

  let { rules, initialIndex, onclose }: {
    rules: GrammarRule[]; initialIndex: number; onclose: () => void
  } = $props()
  let index = $state(untrack(() => initialIndex))
  const rule = $derived(rules[index]!)
  let viewport: HTMLDivElement
  let heading = $state<HTMLHeadingElement>()

  onMount(() => heading?.focus({ preventScroll: true }))

  function move(delta: number) {
    const next = index + delta
    if (next < 0 || next >= rules.length) return
    index = next
    viewport.scrollTop = 0
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey || e.isComposing) return
    if (e.target instanceof HTMLElement && e.target.closest('a, input, textarea, select, [contenteditable]')) return
    if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      e.preventDefault()
      move(e.key === 'ArrowLeft' ? -1 : 1)
    } else if (e.key === 'Escape') {
      onclose()
    }
  }
</script>

<svelte:window {onkeydown} />

<div class="reader">
  <header>
    <Button variant="ghost" icon="chevronLeft" onclick={onclose}>{t('grammar.back')}</Button>
    <p class="position" role="status">{t('grammar.position', { index: index + 1, total: rules.length })}</p>
  </header>

  <div class="viewport scroll" bind:this={viewport}>
    {#key rule.id}
      <article>
        <h1 lang="en" tabindex="-1" bind:this={heading}>{rule.front}</h1>
        {#if rule.content.formula.trim()}
          <div class="formula">
            <h2>{t('grammar.formula')}</h2>
            <code lang="en">{rule.content.formula}</code>
          </div>
        {/if}
        <Markdown source={rule.back} />

        {#if rule.content.examples?.length}
          <section>
            <h2>{t('grammar.examples')}</h2>
            <ul>
              {#each rule.content.examples as example}
                <li>
                  <p class="example" lang="en">{example.text}</p>
                  {#if example.translation.trim()}<p class="translation">{example.translation}</p>{/if}
                </li>
              {/each}
            </ul>
          </section>
        {/if}

        {#if rule.content.mistakes?.length}
          <section>
            <h2>{t('grammar.mistakes')}</h2>
            <ul>
              {#each rule.content.mistakes as mistake}
                <li class="mistake">
                  <div class="incorrect"><span>{t('grammar.incorrect')}</span><code lang="en">{mistake.incorrect}</code></div>
                  <div class="correct"><span>{t('grammar.correct')}</span><code lang="en">{mistake.correct}</code></div>
                </li>
              {/each}
            </ul>
          </section>
        {/if}
      </article>
    {/key}
  </div>

  <nav aria-label={t('grammar.pagination')}>
    <Button icon="chevronLeft" aria-disabled={index === 0} onclick={() => move(-1)}>{t('grammar.previous')}</Button>
    <Button variant="primary" icon="chevronRight" aria-disabled={index === rules.length - 1} onclick={() => move(1)}>{t('grammar.next')}</Button>
  </nav>
</div>

<style>
  .reader {
    width: 100%;
    max-width: 760px;
    margin-inline: auto;
    min-height: 0;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  header, nav {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--s-2);
  }
  nav {
    padding: 0 4px 4px 2px;
  }
  .position {
    font-size: var(--fs-sm);
    color: var(--c-muted);
    font-variant-numeric: tabular-nums;
  }
  .viewport {
    flex: 1;
    padding: 2px 6px 6px 2px;
  }
  article {
    display: grid;
    gap: var(--s-5);
    padding: clamp(1rem, 5vw, 3rem);
    border: var(--bw-strong) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
    box-shadow: var(--shadow);
    overflow-wrap: anywhere;
    animation: appear var(--dur-fast) var(--ease-out);
  }
  @keyframes appear {
    from { opacity: .45; }
  }
  h1 {
    font-size: clamp(1.8rem, 5vw, 3rem);
    line-height: 1.12;
    letter-spacing: -.035em;
    text-wrap: balance;
  }
  h2 {
    font-size: var(--fs-md);
    line-height: 1.4;
  }
  .formula {
    display: grid;
    gap: var(--s-2);
    padding: var(--s-3) var(--s-4);
    border-left: var(--bw-strong) solid var(--c-green);
    border-radius: var(--radius-sm);
    background: var(--c-green-soft);
  }
  code, .example, .translation {
    white-space: pre-wrap;
  }
  .formula code {
    font-size: var(--fs-md);
  }
  section {
    display: grid;
    gap: var(--s-3);
    padding-top: var(--s-4);
    border-top: 1px solid var(--c-line);
  }
  ul {
    display: grid;
    gap: var(--s-3);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .example {
    font-weight: 650;
  }
  .translation {
    font-style: italic;
    color: var(--c-muted);
  }
  .mistake {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(200px, 100%), 1fr));
    gap: var(--s-2);
  }
  .mistake div {
    display: grid;
    align-content: start;
    gap: var(--s-1);
    padding: var(--s-3);
    border-radius: var(--radius-sm);
  }
  .mistake span {
    font-size: var(--fs-xs);
    font-weight: 700;
  }
  .incorrect { background: var(--c-red-soft); }
  .correct { background: var(--c-green-soft); }
  nav :global(.btn) { min-height: 44px; }
  nav :global(button:last-child svg) { order: 1; }
  nav :global(button[aria-disabled='true']) {
    opacity: .45;
    cursor: default;
    transform: none;
    box-shadow: none;
  }
</style>
