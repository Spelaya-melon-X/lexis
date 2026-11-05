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
  GroupChildren,
  GroupCreate,
  GroupDetail,
  FocusAction,
  FocusOutcome,
  FocusSession,
  FocusStats,
  Exercise,
  FoodDay,
  GameSettings,
  Meal,
  ProgressPoint,
  SportStats,
  LedgerEntry,
  Purchase,
  Reward,
  GroupPatch,
  Plan,
  Task,
  TaskColumn,
  TaskFilter,
  TaskInput,
  TaskPatch,
  TaskStep,
  TaskType,
  Workout,
  WorkoutInput,
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
    create: (p: GroupCreate) => post<Group>('/groups', p),
    update: (id: number, p: GroupPatch) => request<Group>('PATCH', `/groups/${id}`, p),
    remove: (id: number, children: GroupChildren = 'lift') => request<void>('DELETE', `/groups/${id}${qs({ children })}`),
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
  tasks: {
    list: (done: TaskFilter = 'false') => get<{ tasks: Task[] }>(`/tasks${qs({ done })}`).then((r) => r.tasks),
    create: (t: TaskInput) => post<Task>('/tasks', t),
    update: (id: number, p: TaskPatch) => request<Task>('PATCH', `/tasks/${id}`, p),
    /** beforeId — перед каким делом встать (null — в конец колонки). */
    move: (id: number, columnId: number, beforeId: number | null) => post<void>(`/tasks/${id}/move`, { columnId, beforeId }),
    remove: (id: number) => request<void>('DELETE', `/tasks/${id}`),
    /** Разбить на шаги → всё дерево дела. */
    split: (id: number, steps: TaskStep[]) => post<{ tasks: Task[] }>(`/tasks/${id}/split`, { steps }).then((r) => r.tasks),
    /** Дерево, в которое входит дело (от корня, обход в глубину). */
    tree: (id: number) => get<{ tasks: Task[] }>(`/tasks/${id}/tree`).then((r) => r.tasks),
    plan: (day: string) => get<Plan>(`/tasks/plan/${day}`),
    /** Весь план дня (PUT заменяет); пункты не пересекаются по времени. */
    savePlan: (day: string, items: { taskId: number; start: number; blocks: number }[]) => request<Plan>('PUT', `/tasks/plan/${day}`, { items }),
    settings: () => get<GameSettings>('/tasks/settings'),
    updateSettings: (p: Partial<GameSettings>) => request<GameSettings>('PUT', '/tasks/settings', p),
    types: {
      list: () => get<{ types: TaskType[] }>('/tasks/types').then((r) => r.types),
      create: (name: string, color: string | null) => post<TaskType>('/tasks/types', { name, color }),
      update: (id: number, p: { name?: string; color?: string | null }) => request<TaskType>('PATCH', `/tasks/types/${id}`, p),
      remove: (id: number) => request<void>('DELETE', `/tasks/types/${id}`),
      reorder: (ids: number[]) => request<void>('PUT', '/tasks/types/order', { ids }),
    },
    columns: {
      list: () => get<{ columns: TaskColumn[] }>('/tasks/columns').then((r) => r.columns),
      create: (name: string) => post<TaskColumn>('/tasks/columns', { name }),
      rename: (id: number, name: string) => request<TaskColumn>('PATCH', `/tasks/columns/${id}`, { name }),
      /** to — куда перенести дела колонки (обязательно, если они есть). */
      remove: (id: number, to?: number) => request<void>('DELETE', `/tasks/columns/${id}${qs({ to })}`),
      reorder: (ids: number[]) => request<void>('PUT', '/tasks/columns/order', { ids }),
    },
  },
  focus: {
    active: () => get<{ session: FocusSession | null; now: string }>('/focus/active'),
    start: (taskId: number) => post<FocusSession>('/focus/sessions', { taskId }),
    get: (id: number) => get<FocusSession>(`/focus/sessions/${id}`),
    act: (id: number, action: FocusAction) => post<FocusSession>(`/focus/sessions/${id}/${action}`),
    distract: (id: number, trigger: string, gaveIn: boolean) => post<FocusSession>(`/focus/sessions/${id}/distractions`, { trigger, gaveIn }),
    finish: (id: number, outcome: FocusOutcome) => post<FocusSession>(`/focus/sessions/${id}/finish`, { outcome }),
    stats: (from: string, to: string) => get<FocusStats>(`/focus/stats${qs({ from, to })}`),
    triggers: (q = '') => get<{ triggers: string[] }>(`/focus/triggers${qs({ q: q || undefined })}`).then((r) => r.triggers),
    ritual: () => get<{ items: string[] }>('/focus/ritual').then((r) => r.items),
    saveRitual: (items: string[]) => request<{ items: string[] }>('PUT', '/focus/ritual', { items }).then((r) => r.items),
    settings: () => get<{ setupMin: number }>('/focus/settings'),
    updateSettings: (setupMin: number) => request<{ setupMin: number }>('PUT', '/focus/settings', { setupMin }),
    rewards: (archived = false) => get<{ rewards: Reward[] }>(`/focus/rewards${qs({ archived: archived ? 'true' : undefined })}`).then((r) => r.rewards),
    createReward: (title: string, price: number) => post<Reward>('/focus/rewards', { title, price }),
    updateReward: (id: number, p: { title?: string; price?: number; archived?: boolean }) => request<Reward>('PATCH', `/focus/rewards/${id}`, p),
    removeReward: (id: number) => request<void>('DELETE', `/focus/rewards/${id}`),
    reorderRewards: (ids: number[]) => request<void>('PUT', '/focus/rewards/order', { ids }),
    buy: (id: number, dopamine: boolean) => post<Purchase>(`/focus/rewards/${id}/buy`, { dopamine }),
    points: (before?: number, limit?: number) =>
      get<{ balance: number; entries: LedgerEntry[]; next: number | null }>(`/focus/points${qs({ before, limit })}`),
  },
  sport: {
    workouts: (from?: string, to?: string) =>
      get<{ from: string; to: string; workouts: Workout[] }>(`/sport/workouts${qs({ from, to })}`),
    get: (id: number) => get<Workout>(`/sport/workouts/${id}`),
    create: (w: WorkoutInput) => post<Workout>('/sport/workouts', w),
    update: (id: number, w: WorkoutInput) => request<Workout>('PUT', `/sport/workouts/${id}`, w),
    remove: (id: number) => request<void>('DELETE', `/sport/workouts/${id}`),
    titles: (q = '') => get<{ titles: string[] }>(`/sport/titles${qs({ q: q || undefined })}`).then((r) => r.titles),
    exercises: (q = '') => get<{ exercises: Exercise[] }>(`/sport/exercises${qs({ q: q || undefined })}`).then((r) => r.exercises),
    renameExercise: (id: number, name: string) => request<Exercise>('PATCH', `/sport/exercises/${id}`, { name }),
    removeExercise: (id: number) => request<void>('DELETE', `/sport/exercises/${id}`),
    progress: (id: number, from?: string, to?: string) =>
      get<{ exercise: { id: number; name: string }; points: ProgressPoint[] }>(`/sport/exercises/${id}/progress${qs({ from, to })}`),
    food: (day: string) => get<FoodDay>(`/sport/meals/${day}`),
    saveFood: (day: string, meals: Meal[]) => request<FoodDay>('PUT', `/sport/meals/${day}`, { meals }),
    mealSuggestions: (q = '') => get<{ meals: Meal[] }>(`/sport/meals/suggest${qs({ q: q || undefined })}`).then((r) => r.meals),
    stats: (from: string, to: string) => get<SportStats>(`/sport/stats${qs({ from, to })}`),
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
