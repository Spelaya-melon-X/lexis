// Черновик ввода слов. Живёт в модуле, а не в компоненте: таб пересоздаётся при каждом
// переключении ({#key} в App), и без этого текст пропадал бы. localStorage — чтобы пережить
// перезагрузку страницы.

const KEY = 'lexis.addDraft'

function load(): string {
  try {
    return localStorage.getItem(KEY) ?? ''
  } catch {
    return ''
  }
}

let text = $state(load())

export const draft = {
  get text(): string {
    return text
  },
  set text(v: string) {
    text = v
    try {
      if (v) localStorage.setItem(KEY, v)
      else localStorage.removeItem(KEY)
    } catch {
      // нет localStorage — черновик проживёт до перезагрузки
    }
  },
}
