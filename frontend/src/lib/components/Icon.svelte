<script lang="ts" module>
  // Линейные иконки 24×24 (§12: inline SVG, минималистичные). Только stroke, без заливки —
  // цвет берётся из currentColor, толщина единая.
  const paths = {
    cards: 'M7 3h12v14H7z M4 7v14h12',
    folder: 'M3 6h6l2 2h10v11H3z',
    play: 'M7 4l13 8-13 8z',
    chart: 'M4 20V10 M10 20V4 M16 20v-7 M22 20H2',
    settings: 'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z M12 2v3 M12 19v3 M4.9 4.9l2.1 2.1 M17 17l2.1 2.1 M2 12h3 M19 12h3 M4.9 19.1L7 17 M17 7l2.1-2.1',
    plus: 'M12 5v14 M5 12h14',
    check: 'M5 12l5 5 9-10',
    x: 'M6 6l12 12 M18 6L6 18',
    trash: 'M4 7h16 M9 7V4h6v3 M6 7l1 13h10l1-13',
    edit: 'M4 20h4L19 9l-4-4L4 16z M13 7l4 4',
    archive: 'M3 4h18v4H3z M5 8v12h14V8 M10 12h4',
    upload: 'M12 16V4 M7 9l5-5 5 5 M4 20h16',
    download: 'M12 4v12 M7 11l5 5 5-5 M4 20h16',
    refresh: 'M20 11a8 8 0 1 0-2.3 5.7 M20 4v7h-7',
    flame: 'M12 22c4 0 7-3 7-7 0-5-5-7-5-12-3 2-4 5-4 7-2-1-3-3-3-4-2 3-2 5-2 9 0 4 3 7 7 7z',
    trophy: 'M8 4h8v5a4 4 0 0 1-8 0z M8 6H4v2a4 4 0 0 0 4 4 M16 6h4v2a4 4 0 0 1-4 4 M12 13v4 M8 21h8 M10 17h4v4h-4z',
    chevronLeft: 'M15 5l-7 7 7 7',
    chevronRight: 'M9 5l7 7-7 7',
    alert: 'M12 3l10 18H2z M12 10v5 M12 18v.01',
    clock: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z M12 7v5l3 3',
  } as const

  export type IconName = keyof typeof paths
</script>

<script lang="ts">
  interface Props {
    name: IconName
    size?: number
    label?: string // нет label — иконка декоративная, скрыта от скринридеров
  }
  let { name, size = 20, label }: Props = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2"
  stroke-linecap="round"
  stroke-linejoin="round"
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}
>
  <path d={paths[name]} />
</svg>

<style>
  svg {
    flex: none;
    display: block;
  }
</style>
