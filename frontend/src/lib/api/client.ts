import type {
  Achievement,
  AnswerResult,
  BulkResult,
  Calendar,
  Checkin,
  CheckinInput,
  Day,
  DayPatch,
  FinishResult,
  Group,
  GroupDetail,
  GroupPatch,
  Goal,
  GoalInput,
  Habit,
  HabitStat,
  HabitFields,
  HabitPatch,
  ImportResult,
  JournalDays,
  Note,
  NoteFilter,
  NoteInput,
  Overview,
  ParseResult,
  Range,
  Settings,
  StartResult,
  Strategy,
  Answer,
  Trends,
  Word,
  WordFilter,
  WordPatch,
  Week,
  WeekBlock,
} from './types'

/**
 * Ошибка API. code — snake_case из {"error":{"code","message"}} бэкенда (SPEC §14),
 * по нему UI берёт текст из i18n (errors.<code>). Особые коды клиента:
 * "network" — сервер недоступен, "bad_response" — ответ не в нашем формате.
 */
export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

/** Достаёт ApiError из тела ответа с ошибкой. Чистая функция — тестируется отдельно. */
export function errorFromResponse(status: number, body: string): ApiError {
  try {
    const parsed: unknown = JSON.parse(body)
    if (isErrorBody(parsed)) {
      return new ApiError(status, parsed.error.code, parsed.error.message)
    }
  } catch {
    // не JSON — например, 502 от прокси Vite, пока Go-сервер не запущен
  }
  return new ApiError(status, status >= 500 ? 'internal' : 'bad_response', body.slice(0, 200) || `HTTP ${status}`)
}

function isErrorBody(v: unknown): v is { error: { code: string; message: string } } {
  if (typeof v !== 'object' || v === null || !('error' in v)) return false
  const e = (v as { error: unknown }).error
  return typeof e === 'object' && e !== null && typeof (e as { code?: unknown }).code === 'string'
}

/** Query-строка без пустых значений: qs({group: 3, level: undefined}) → "?group=3". */
export function qs(params: Record<string, string | number | undefined | null>): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

async function request<T>(method: string, path: string, body?: unknown | FormData): Promise<T> {
  const init: RequestInit = { method, headers: { Accept: 'application/json' } }
  if (body instanceof FormData) {
    init.body = body // Content-Type с boundary браузер выставит сам
  } else if (body !== undefined) {
    init.body = JSON.stringify(body)
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
  }

  let res: Response
  try {
    res = await fetch(`/api${path}`, init)
  } catch (e) {
    throw new ApiError(0, 'network', e instanceof Error ? e.message : String(e))
  }
  if (!res.ok) {
    throw errorFromResponse(res.status, await res.text())
  }
  if (res.status === 204) {
    return undefined as T
  }
  try {
    return (await res.json()) as T
  } catch {
    throw new ApiError(res.status, 'bad_response', 'response is not JSON')
  }
}

const get = <T>(path: string) => request<T>('GET', path)
const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body)

/** Все эндпоинты бэкенда (SPEC §11) — одна точка, чтобы URL не расползались по компонентам. */
export const api = {
  words: {
    parse: (text: string) => post<ParseResult>('/words/parse', { text }),
    bulk: (text: string) => post<BulkResult>('/words/bulk', { text }),
    list: (f: WordFilter = {}) => get<{ words: Word[] }>(`/words${qs({ ...f })}`).then((r) => r.words),
    update: (id: number, p: WordPatch) => request<Word>('PATCH', `/words/${id}`, p),
    remove: (id: number) => request<void>('DELETE', `/words/${id}`),
    unarchive: (id: number) => post<Word>(`/words/${id}/unarchive`),
  },
  groups: {
    list: () => get<{ groups: Group[] }>('/groups').then((r) => r.groups),
    get: (id: number, range: Range = 'month') => get<GroupDetail>(`/groups/${id}${qs({ range })}`),
    update: (id: number, p: GroupPatch) => request<Group>('PATCH', `/groups/${id}`, p),
    remove: (id: number) => request<void>('DELETE', `/groups/${id}`),
  },
  training: {
    start: (groupId: number, strategy: Strategy, free = false) =>
      post<StartResult>('/training/start', { groupId, strategy, free }),
    answer: (sessionId: number, wordId: number, answer: Answer) =>
      post<AnswerResult>(`/training/${sessionId}/answer`, { wordId, answer }),
    finish: (sessionId: number) => post<FinishResult>(`/training/${sessionId}/finish`),
  },
  stats: {
    overview: (day?: string) => get<Overview>(`/stats/overview${qs({ day })}`),
    calendar: (from?: string, to?: string) => get<Calendar>(`/stats/calendar${qs({ from, to })}`),
    trends: (range: Range = 'week') => get<Trends>(`/stats/trends${qs({ range })}`),
  },
  achievements: {
    list: () => get<{ achievements: Achievement[] }>('/achievements').then((r) => r.achievements),
  },
  settings: {
    get: () => get<Settings>('/settings'),
    update: (p: Partial<Settings>) => request<Settings>('PUT', '/settings', p),
  },
  journal: {
    day: (day: string) => get<Day>(`/journal/days/${day}`),
    update: (day: string, p: DayPatch) => request<Day>('PATCH', `/journal/days/${day}`, p),
    /** Пустой text гасит квадратик. */
    dopamine: (day: string, slot: number, text: string) => request<Day>('PUT', `/journal/days/${day}/dopamine/${slot}`, { text }),
    days: (from?: string, to?: string) => get<JournalDays>(`/journal/days${qs({ from, to })}`),
    week: (monday: string) => get<Week>(`/journal/weeks/${monday}`),
    /** Блоки заменяются целиком; [] — удалить анализ недели. */
    saveWeek: (monday: string, blocks: WeekBlock[]) => request<Week>('PUT', `/journal/weeks/${monday}`, { blocks }),
  },
  habits: {
    list: (archived = false) => get<{ habits: Habit[] }>(`/habits${qs({ archived: archived ? 'true' : undefined })}`).then((r) => r.habits),
    create: (f: HabitFields) => post<Habit>('/habits', f),
    update: (id: number, p: HabitPatch) => request<Habit>('PATCH', `/habits/${id}`, p),
    remove: (id: number) => request<void>('DELETE', `/habits/${id}`),
    /** Полный список id всех привычек (и архивных) в новом порядке. */
    reorder: (ids: number[]) => request<void>('PUT', '/habits/order', { ids }),
    stats: (from: string, to: string) =>
      get<{ from: string; to: string; habits: HabitStat[] }>(`/habits/stats${qs({ from, to })}`).then((r) => r.habits),
    checkins: (from: string, to: string) =>
      get<{ checkins: Checkin[] }>(`/habits/checkins${qs({ from, to })}`).then((r) => r.checkins),
    saveDay: (day: string, checkins: CheckinInput[]) =>
      request<{ checkins: Checkin[] }>('PUT', `/habits/checkins/${day}`, { checkins }).then((r) => r.checkins),
    /** Без окна — все цели. */
    goals: (from?: string, to?: string) => get<{ goals: Goal[] }>(`/habits/goals${qs({ from, to })}`).then((r) => r.goals),
    createGoal: (g: GoalInput) => post<Goal>('/habits/goals', g),
    updateGoal: (id: number, g: GoalInput) => request<Goal>('PUT', `/habits/goals/${id}`, g),
    removeGoal: (id: number) => request<void>('DELETE', `/habits/goals/${id}`),
    notes: (f: NoteFilter = {}) => get<{ notes: Note[]; next: number | null }>(`/habits/notes${qs({ ...f })}`),
    createNote: (n: NoteInput) => post<Note>('/habits/notes', n),
    updateNote: (id: number, n: NoteInput) => request<Note>('PUT', `/habits/notes/${id}`, n),
    removeNote: (id: number) => request<void>('DELETE', `/habits/notes/${id}`),
  },
  data: {
    import: (file: File) => {
      const fd = new FormData()
      fd.append('file', file)
      return request<ImportResult>('POST', '/data/import', fd)
    },
    /** Экспорт — обычная ссылка на скачивание (браузер сам сохранит файл по Content-Disposition). */
    exportUrl: (format: 'json' | 'csv') => `/api/data/export${qs({ format })}`,
    /** english — только английский; all — ещё дневник и привычки. Настройки не трогает. */
    reset: (scope: 'english' | 'all') => post<void>('/data/reset', { confirm: true, scope }),
  },
}
