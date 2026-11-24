// Ссылки пропускаем целиком, в том числе если внутри URL есть разделитель тега.
const tagOrURL = /\b(?:[a-z][a-z\d+.-]*:\/\/|www\.)[^\s<>"«»]+|(?<![\p{L}\p{N}\p{M}_/#=-])#([\p{L}\p{N}_-]+)/giu

export function taskTags(note: string): string[] {
  const tags = new Set<string>()
  for (const [, tag] of note.matchAll(tagOrURL)) {
    if (tag) tags.add(tag.toLowerCase())
  }
  return [...tags]
}
