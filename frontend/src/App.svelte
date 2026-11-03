<script lang="ts">
  import { onMount, type Component } from 'svelte'
  import { fade } from 'svelte/transition'
  import { api } from './lib/api/client'
  import type { Locale } from './lib/api/types'
  import Icon, { type IconName } from './lib/components/Icon.svelte'
  import Toaster from './lib/components/Toaster.svelte'
  import { errorText, i18n, t } from './lib/i18n/i18n.svelte'
  import { route } from './lib/state/route.svelte'
  import type { Tab } from './lib/state/route'
  import { toasts } from './lib/state/toasts.svelte'
  import AddWords from './tabs/AddWords/AddWords.svelte'
  import Groups from './tabs/Groups/Groups.svelte'
  import Settings from './tabs/Settings/Settings.svelte'
  import Stats from './tabs/Stats/Stats.svelte'
  import Training from './tabs/Training/Training.svelte'

  const tabs: { id: Tab; icon: IconName; view: Component }[] = [
    { id: 'add', icon: 'cards', view: AddWords },
    { id: 'groups', icon: 'folder', view: Groups },
    { id: 'training', icon: 'play', view: Training },
    { id: 'stats', icon: 'chart', view: Stats },
    { id: 'settings', icon: 'settings', view: Settings },
  ]
  const current = $derived(tabs.find((x) => x.id === route.tab) ?? tabs[0]!)

  // Язык: источник правды — /api/settings; localStorage (внутри i18n) — для первого кадра.
  onMount(async () => {
    try {
      i18n.locale = (await api.settings.get()).language
    } catch (e) {
      toasts.error(errorText(e))
    }
  })

  async function switchLocale(l: Locale) {
    if (l === i18n.locale) return
    const prev = i18n.locale
    i18n.locale = l // сразу — интерфейс не ждёт сеть
    try {
      await api.settings.update({ language: l })
    } catch (e) {
      i18n.locale = prev
      toasts.error(errorText(e))
    }
  }
</script>

<div class="shell">
  <header>
    <a class="logo" href="#add">
      <img src="/favicon.svg" alt="" width="28" height="28" />
      <span>{t('app.title')}</span>
    </a>

    <nav aria-label="tabs">
      {#each tabs as tab (tab.id)}
        <a href="#{tab.id}" class="tab" class:active={tab.id === route.tab} aria-current={tab.id === route.tab ? 'page' : undefined}>
          <Icon name={tab.icon} size={18} />
          <span>{t(`tabs.${tab.id}`)}</span>
        </a>
      {/each}
    </nav>

    <div class="lang" role="group" aria-label={t('lang.switch')}>
      {#each ['ru', 'en'] as const as l (l)}
        <button class:active={i18n.locale === l} aria-pressed={i18n.locale === l} onclick={() => switchLocale(l)}>
          {t(`lang.${l}`)}
        </button>
      {/each}
    </div>
  </header>

  <main>
    <!-- §12: переключение табов — плавное появление -->
    {#key current.id}
      <div class="view" in:fade={{ duration: 200 }}>
        <current.view />
      </div>
    {/key}
  </main>
</div>

<Toaster />

<style>
  .shell {
    display: grid;
    grid-template-rows: var(--header-h) 1fr;
    height: 100%;
  }

  header {
    display: flex;
    align-items: center;
    gap: var(--s-5);
    padding: 0 var(--s-5);
    border-bottom: var(--bw-strong) solid var(--c-border);
    background: var(--c-surface);
  }

  .logo {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-lg);
    font-weight: 800;
    letter-spacing: -0.02em;
    color: inherit;
    text-decoration: none;
  }

  nav {
    display: flex;
    gap: var(--s-1);
    margin: 0 auto;
  }

  .tab {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-3);
    border: var(--bw) solid transparent;
    border-radius: var(--radius-sm);
    color: var(--c-muted);
    font-weight: 600;
    text-decoration: none;
    transition:
      color var(--dur-fast) var(--ease-out),
      background var(--dur-fast) var(--ease-out);
  }
  .tab:hover {
    color: var(--c-text);
    background: var(--c-surface-2);
  }
  .tab.active {
    color: var(--c-text);
    border-color: var(--c-border);
    background: var(--c-green-soft);
    box-shadow: var(--shadow-sm);
  }

  .lang {
    display: flex;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
  .lang button {
    padding: var(--s-1) var(--s-3);
    border: 0;
    background: var(--c-surface);
    font-weight: 700;
    font-size: var(--fs-sm);
    cursor: pointer;
  }
  .lang button + button {
    border-left: var(--bw) solid var(--c-border);
  }
  .lang button.active {
    background: var(--c-text);
    color: var(--c-bg);
  }

  main {
    min-height: 0; /* чтобы скролл жил внутри раздела, а не у страницы (§12) */
    overflow: hidden;
  }
  .view {
    height: 100%;
    max-width: var(--content-w);
    margin: 0 auto;
    padding: var(--s-5);
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  /* Узкое окно: подписи табов прячем, остаются иконки */
  @media (max-width: 860px) {
    .tab span {
      display: none;
    }
  }
</style>
