import type {
  Achievement,
  AnswerResult,
  BulkResult,
  Calendar,
  FinishResult,
  Group,
  GroupDetail,
  GroupPatch,
  ImportResult,
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
  data: {
    import: (file: File) => {
      const fd = new FormData()
      fd.append('file', file)
      return request<ImportResult>('POST', '/data/import', fd)
    },
    /** Экспорт — обычная ссылка на скачивание (браузер сам сохранит файл по Content-Disposition). */
    exportUrl: (format: 'json' | 'csv') => `/api/data/export${qs({ format })}`,
    reset: () => post<void>('/data/reset', { confirm: true }),
  },
}
