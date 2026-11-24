import { Marked, type TokenizerAndRendererExtension } from 'marked'
import DOMPurify from 'dompurify'
import katex from 'katex'

export function escapeHTML(text: string): string {
  return text.replace(
    /[&<>"']/g,
    (c) =>
      ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[
        c
      ]!,
  )
}
function mathHTML(source: string, displayMode: boolean): string {
  return katex.renderToString(source, {
    displayMode,
    throwOnError: false,
    trust: false,
    strict: 'ignore',
    maxSize: 20,
    maxExpand: 1000,
    macros: {},
  })
}
const mathBlock: TokenizerAndRendererExtension = {
  name: 'mathBlock',
  level: 'block',
  start: (src) => src.search(/(?:\$\$|\\\[)/),
  tokenizer(src) {
    const m = /^(?:\$\$([^]*?)\$\$|\\\[([^]*?)\\\])(?:[ \t]*\n|$)/.exec(src)
    if (m) return { type: 'mathBlock', raw: m[0], text: m[1] ?? m[2] ?? '' }
  },
  renderer: (token) => mathHTML(String(token.text), true),
}
const mathInline: TokenizerAndRendererExtension = {
  name: 'mathInline',
  level: 'inline',
  start: (src) => src.search(/\$|\\\(|\\\[/),
  tokenizer(src) {
    const m =
      /^(?:\$\$([^]*?)\$\$|\$([^\n$]+?)\$|\\\(([^]*?)\\\)|\\\[([^]*?)\\\])/.exec(
        src,
      )
    if (m)
      return {
        type: 'mathInline',
        raw: m[0],
        text: m[1] ?? m[2] ?? m[3] ?? m[4] ?? '',
        display: !!(m[1] || m[4]),
      }
  },
  renderer: (token) => mathHTML(String(token.text), !!token.display),
}
const markdown = new Marked({
  gfm: true,
  breaks: true,
  extensions: [mathBlock, mathInline],
  renderer: { html: (token) => escapeHTML(token.text) },
})
/** The sole HTML boundary for study content: Markdown + untrusted TeX, then sanitization. */
export function renderMarkdown(source: string): string {
  return DOMPurify.sanitize(markdown.parse(source, { async: false }), {
    USE_PROFILES: { html: true, mathMl: true, svg: true },
    FORBID_TAGS: [
      'style',
      'iframe',
      'form',
      'input',
      'button',
      'video',
      'audio',
    ],
    FORBID_ATTR: ['srcset'],
  })
}
