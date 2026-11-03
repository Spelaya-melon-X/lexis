<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLAttributes } from 'svelte/elements'

  interface Props extends HTMLAttributes<HTMLDivElement> {
    shadow?: boolean
    padding?: 'none' | 'sm' | 'md' | 'lg'
    children?: Snippet
  }
  let { shadow = true, padding = 'md', children, class: cls = '', ...rest }: Props = $props()
</script>

<div class="card pad-{padding} {cls}" class:shadow {...rest}>
  {@render children?.()}
</div>

<style>
  .card {
    background: var(--c-surface);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
  }
  .shadow {
    box-shadow: var(--shadow);
  }
  .pad-sm {
    padding: var(--s-3);
  }
  .pad-md {
    padding: var(--s-4);
  }
  .pad-lg {
    padding: var(--s-5);
  }
</style>
