// Карта навигации (§17.1): что показывать для каждого раздела/подраздела.
// Сами id и порядок — в lib/state/route.ts (SECTIONS); Record<Page<S>, …> заставляет
// описать здесь каждый подраздел — забытый ловит svelte-check.
import type { Component } from 'svelte'
import type { IconName } from '../lib/components/Icon.svelte'
import type { TKey } from '../lib/i18n/i18n.svelte'
import { SECTIONS, type Page, type Section } from '../lib/state/route'
import AddWords from './english/AddWords/AddWords.svelte'
import Groups from './english/Groups/Groups.svelte'
import Settings from './english/Settings/Settings.svelte'
import Stats from './english/Stats/Stats.svelte'
import Training from './english/Training/Training.svelte'
import DayCalendar from './day/Calendar/Calendar.svelte'
import DayEntry from './day/Entry/Entry.svelte'
import DayStats from './day/Stats/Stats.svelte'
import DayWeek from './day/Week/Week.svelte'
import HabitsCalendar from './habits/Calendar/Calendar.svelte'
import HabitsCheck from './habits/Check/Check.svelte'
import HabitsGoals from './habits/Goals/Goals.svelte'
import HabitsManage from './habits/Manage/Manage.svelte'
import HabitsNotes from './habits/Notes/Notes.svelte'
import SettingsData from './settings/Data.svelte'
import SettingsGeneral from './settings/General.svelte'

export interface PageDef {
  icon: IconName
  label: TKey
  view: Component
}

interface SectionDef<S extends Section> {
  icon: IconName
  label: TKey
  header?: false // false — раздел не показывается в списке шапки
  pages: Record<Page<S>, PageDef>
}

const NAV: { [S in Section]: SectionDef<S> } = {
  english: {
    icon: 'book',
    label: 'nav.section.english',
    pages: {
      add: { icon: 'cards', label: 'nav.english.add', view: AddWords },
      groups: { icon: 'folder', label: 'nav.english.groups', view: Groups },
      training: { icon: 'play', label: 'nav.english.training', view: Training },
      stats: { icon: 'chart', label: 'nav.english.stats', view: Stats },
      settings: { icon: 'settings', label: 'nav.english.settings', view: Settings },
    },
  },
  day: {
    icon: 'sun',
    label: 'nav.section.day',
    pages: {
      entry: { icon: 'sun', label: 'nav.day.entry', view: DayEntry },
      calendar: { icon: 'calendar', label: 'nav.day.calendar', view: DayCalendar },
      week: { icon: 'week', label: 'nav.day.week', view: DayWeek },
      stats: { icon: 'chart', label: 'nav.day.stats', view: DayStats },
    },
  },
  habits: {
    icon: 'repeat',
    label: 'nav.section.habits',
    pages: {
      check: { icon: 'check', label: 'nav.habits.check', view: HabitsCheck },
      calendar: { icon: 'calendar', label: 'nav.habits.calendar', view: HabitsCalendar },
      goals: { icon: 'target', label: 'nav.habits.goals', view: HabitsGoals },
      notes: { icon: 'note', label: 'nav.habits.notes', view: HabitsNotes },
      manage: { icon: 'list', label: 'nav.habits.manage', view: HabitsManage },
    },
  },
  settings: {
    icon: 'settings',
    label: 'nav.section.settings',
    header: false, // ⚙ справа в шапке, а не в списке разделов
    pages: {
      general: { icon: 'settings', label: 'nav.settings.general', view: SettingsGeneral },
      data: { icon: 'archive', label: 'nav.settings.data', view: SettingsData },
    },
  },
}

/** Разделы для шапки (без ⚙ — он отдельной иконкой справа). */
export const sections = (Object.keys(SECTIONS) as Section[])
  .filter((id) => NAV[id].header !== false)
  .map((id) => ({ id, icon: NAV[id].icon, label: NAV[id].label }))

/** Подразделы раздела в порядке SECTIONS. */
export function pagesOf(section: Section): (PageDef & { id: Page })[] {
  // Индексация Record по объединению типов TS не сужает — один каст здесь вместо кастов в шаблонах.
  const pages = NAV[section].pages as Record<string, PageDef>
  return SECTIONS[section].map((id) => ({ id, ...pages[id]! }))
}
