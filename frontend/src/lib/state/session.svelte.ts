// Активная тренировка. В модуле, а не в компоненте: таб пересоздаётся при переключении,
// и без этого уход на «Статистику» посреди сессии терял бы очередь. На сервере сессия
// живёт в БД; после перезагрузки страницы клиентская очередь теряется — тогда просто новая сессия.
import type { Answer, FinishResult, Strategy, TrainingCard } from '../api/types'
import { answer, createQueue, type QueueState } from '../training'

export interface Active {
  sessionId: number
  groupId: number
  groupName: string
  strategy: Strategy
  free: boolean
  q: QueueState
  startedAt: number // Date.now() — только для таймера на экране; длительность считает сервер
}

type Phase = 'setup' | 'session' | 'results'

let phase = $state<Phase>('setup')
let active = $state<Active | null>(null)
let results = $state<FinishResult | null>(null)

export const session = {
  get phase(): Phase {
    return phase
  },
  get active(): Active | null {
    return active
  },
  get results(): FinishResult | null {
    return results
  },

  begin(a: Omit<Active, 'q' | 'startedAt'>, cards: TrainingCard[]) {
    active = { ...a, q: createQueue(cards), startedAt: Date.now() }
    results = null
    phase = 'session'
  },
  apply(a: Answer, requeueAt?: number) {
    if (active) active.q = answer(active.q, a, requeueAt)
  },
  showResults(r: FinishResult) {
    results = r
    phase = 'results'
  },
  /** Назад к выбору; группа из прошлой сессии остаётся выбранной. */
  reset() {
    phase = 'setup'
    results = null
  },
}
