<script lang="ts">
  import { fade } from 'svelte/transition'
  import { session } from '../../lib/state/session.svelte'
  import Results from './Results.svelte'
  import Session from './Session.svelte'
  import Setup from './Setup.svelte'
</script>

<!-- Фаза — в модуле session: уход на другой таб посреди тренировки её не теряет. -->
{#key session.phase}
  <div class="phase" in:fade={{ duration: 200 }}>
    {#if session.phase === 'session' && session.active}
      <Session active={session.active} />
    {:else if session.phase === 'results' && session.results}
      <Results result={session.results} groupId={session.active?.groupId ?? null} />
    {:else}
      <Setup />
    {/if}
  </div>
{/key}

<style>
  .phase {
    height: 100%;
    min-height: 0;
  }
</style>
