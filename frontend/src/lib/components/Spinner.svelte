<script lang="ts">
  let { size = 18 }: { size?: number } = $props()
</script>

<span class="spinner" style:--size="{size}px" aria-hidden="true"></span>

<style>
  .spinner {
    display: inline-block;
    width: var(--size);
    height: var(--size);
    border: var(--bw) solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 700ms linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
