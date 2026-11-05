<script lang="ts">
  import { onMount } from 'svelte'
  import { fade, fly } from 'svelte/transition'
  import { api } from './lib/api/client'
  import type { Locale } from './lib/api/types'
  import Icon from './lib/components/Icon.svelte'
  import Toaster from './lib/components/Toaster.svelte'
  import { errorText, i18n, t } from './lib/i18n/i18n.svelte'
  import { reminders } from './lib/state/reminders.svelte'
  import { route } from './lib/state/route.svelte'
  import { toasts } from './lib/state/toasts.svelte'
  import { pagesOf, sections } from './sections/nav'

  const pages = $derived(pagesOf(route.section))
  const current = $derived(pages.find((p) => p.id === route.page) ?? pages[0]!)

  // Язык: источник правды — /api/settings; localStorage (внутри i18n) — для первого кадра.
  onMount(async () => {
    reminders.start()
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
    <a class="logo" href="#/english/add">
      <img src="/favicon.svg" alt="" width="28" height="28" />
      <span>{t('app.title')}</span>
    </a>

    <nav class="sections" aria-label={t('nav.sections')}>
      {#each sections as s (s.id)}
        {@const active = s.id === route.section}
        <a href={route.sectionHref(s.id)} class="section" class:active aria-current={active ? 'page' : undefined}>
          <Icon name={s.icon} size={18} />
          <span>{t(s.label)}</span>
          <!-- напоминания (§17.11): точка — день не оценён вечером, число — неотмеченные привычки -->
          {#if s.id === 'day' && reminders.dayDue}
            <i class="dot" title={t('reminders.day')} aria-label={t('reminders.day')}></i>
          {:else if s.id === 'habits' && reminders.habitsDue > 0}
            <i class="count" title={t('reminders.habits', { count: reminders.habitsDue })} aria-label={t('reminders.habits', { count: reminders.habitsDue })}
              >{reminders.habitsDue}</i
            >
          {/if}
        </a>
      {/each}
    </nav>

    <a
      href={route.sectionHref('settings')}
      class="gear"
      class:active={route.section === 'settings'}
      aria-current={route.section === 'settings' ? 'page' : undefined}
      aria-label={t('globalSettings.open')}
      title={t('globalSettings.open')}
    >
      <Icon name="settings" size={20} />
    </a>

    <div class="lang" role="group" aria-label={t('lang.switch')}>
      {#each ['ru', 'en'] as const as l (l)}
        <button class:active={i18n.locale === l} aria-pressed={i18n.locale === l} onclick={() => switchLocale(l)}>
          {t(`lang.${l}`)}
        </button>
      {/each}
    </div>
  </header>

  <div class="body">
    <aside>
      {#key route.section}
        <nav class="pages" aria-label={t('nav.pages')} in:fly={{ x: -8, duration: 200 }}>
          {#each pages as p (p.id)}
            {@const active = p.id === route.page}
            <a href="#/{route.section}/{p.id}" class="page" class:active aria-current={active ? 'page' : undefined} title={t(p.label)}>
              <Icon name={p.icon} size={18} />
              <span>{t(p.label)}</span>
            </a>
          {/each}
        </nav>
      {/key}
    </aside>

    <main>
      <!-- §12: переключение разделов — плавное появление -->
      {#key `${route.section}/${current.id}`}
        <div class="view" in:fade={{ duration: 200 }}>
          <current.view />
        </div>
      {/key}
    </main>
  </div>
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

  .sections {
    display: flex;
    gap: var(--s-1);
    margin: 0 auto;
  }

  /* Раздел в шапке и подраздел в сайдбаре выглядят одинаково (бывшие табы) */
  .section,
  .page {
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
  .section:hover,
  .page:hover {
    color: var(--c-text);
    background: var(--c-surface-2);
  }
  .section.active,
  .page.active {
    color: var(--c-text);
    border-color: var(--c-border);
    background: var(--c-green-soft);
    box-shadow: var(--shadow-sm);
  }

  .section {
    position: relative;
  }
  .dot,
  .count {
    position: absolute;
    top: -6px;
    right: -6px;
    display: grid;
    place-items: center;
    border: var(--bw) solid var(--c-border);
    border-radius: 999px;
    background: var(--c-amber);
    font-style: normal;
    animation: pop var(--dur-slow) var(--ease-spring);
  }
  .dot {
    width: 12px;
    height: 12px;
  }
  .count {
    min-width: 20px;
    height: 20px;
    padding: 0 4px;
    color: var(--c-text);
    font-size: var(--fs-xs);
    font-weight: 800;
    line-height: 1;
  }
  @keyframes pop {
    from {
      transform: scale(0);
    }
    to {
      transform: scale(1);
    }
  }

  .gear {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border: var(--bw) solid transparent;
    border-radius: var(--radius-sm);
    color: var(--c-muted);
    transition:
      color var(--dur-fast) var(--ease-out),
      background var(--dur-fast) var(--ease-out);
  }
  .gear:hover {
    color: var(--c-text);
    background: var(--c-surface-2);
  }
  .gear.active {
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

  .body {
    display: grid;
    grid-template-columns: var(--sidebar-w) 1fr;
    min-height: 0;
  }

  aside {
    border-right: var(--bw) solid var(--c-border);
    background: var(--c-surface);
    overflow-y: auto;
  }
  .pages {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    padding: var(--s-4) var(--s-3);
  }

  main {
    min-width: 0;
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

  /* Узкое окно: подписи прячем, остаются иконки; сайдбар — колонка иконок */
  @media (max-width: 860px) {
    .section span,
    .page span {
      display: none;
    }
    .body {
      grid-template-columns: var(--sidebar-w-narrow) 1fr;
    }
    .pages {
      align-items: center;
      padding: var(--s-3) var(--s-1);
    }
  }
</style>
