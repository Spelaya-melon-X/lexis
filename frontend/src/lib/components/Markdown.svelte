<script lang="ts">
  import { renderMarkdown } from '../markdown'
  import 'katex/dist/katex.min.css'
  let { source }: { source: string } = $props()
  const html = $derived(renderMarkdown(source))
</script>

<div class="markdown">{@html html}</div>

<style>
  .markdown {
    min-width: 0;
    max-width: 100%;
    flex-shrink: 0;
    overflow-wrap: anywhere;
    line-height: 1.6;
    overflow-x: auto;
  }
  .markdown :global(p),
  .markdown :global(ul),
  .markdown :global(ol),
  .markdown :global(pre),
  .markdown :global(blockquote),
  .markdown :global(table) {
    margin: var(--s-3) 0;
  }
  .markdown :global(ul),
  .markdown :global(ol) {
    padding-left: var(--s-5);
  }
  .markdown :global(h1),
  .markdown :global(h2),
  .markdown :global(h3) {
    margin: var(--s-4) 0 var(--s-2);
    line-height: 1.3;
  }
  .markdown :global(h1) {
    font-size: var(--fs-xl);
  }
  .markdown :global(h2) {
    font-size: var(--fs-lg);
  }
  .markdown :global(pre) {
    padding: var(--s-3);
    background: var(--c-surface-2);
    overflow-x: auto;
    border-radius: var(--radius-sm);
  }
  .markdown :global(code) {
    white-space: pre-wrap;
  }
  .markdown :global(pre code) {
    white-space: pre;
  }
  .markdown :global(blockquote) {
    border-left: var(--bw-strong) solid var(--c-border);
    padding-left: var(--s-3);
    color: var(--c-muted);
  }
  .markdown :global(table) {
    border-collapse: collapse;
    display: block;
    overflow-x: auto;
  }
  .markdown :global(th),
  .markdown :global(td) {
    border: var(--bw) solid var(--c-line);
    padding: var(--s-2);
  }
  .markdown :global(img) {
    max-width: 100%;
    height: auto;
  }
  .markdown :global(.katex-display) {
    overflow-x: auto;
    overflow-y: hidden;
    padding: var(--s-2) 0;
    max-width: 100%;
  }
  .markdown :global(a) {
    color: inherit;
    text-decoration: underline;
  }
  .markdown :global(.katex) {
    font-size: 1.1em;
  }
</style>
