// Разбор полей формы правки слова. Нормализацию (регистр, #, дубли) делает бэкенд
// теми же правилами §4, что и ввод, — здесь только разбиение на элементы.

/** "food, #verbs  nouns; x" → ["food", "#verbs", "nouns", "x"] — как разделители тегов в §4. */
export function splitTags(s: string): string[] {
  return s.split(/[\s,;]+/).filter(Boolean)
}

/** Примеры в textarea — по одному на строку; пустые строки не считаются. */
export function splitLines(s: string): string[] {
  return s
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
}
