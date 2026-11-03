// Всплывающие сообщения. Модуль с $state вместо store: любой компонент вызывает
// toasts.error(...), <Toaster> в App рисует список.

export type ToastKind = 'success' | 'error' | 'info'

export interface Toast {
  id: number
  kind: ToastKind
  text: string
}

const TTL_MS = 4000
let nextId = 1
const items = $state<Toast[]>([])

function push(kind: ToastKind, text: string) {
  const id = nextId++
  items.push({ id, kind, text })
  setTimeout(() => dismiss(id), TTL_MS)
}

function dismiss(id: number) {
  const i = items.findIndex((t) => t.id === id)
  if (i >= 0) items.splice(i, 1)
}

export const toasts = {
  get items(): readonly Toast[] {
    return items
  },
  success: (text: string) => push('success', text),
  error: (text: string) => push('error', text),
  info: (text: string) => push('info', text),
  dismiss,
}
