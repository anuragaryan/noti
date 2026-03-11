import { marked } from 'marked'
import { escapeHtml } from './html'

marked.setOptions({
  gfm: true,
  breaks: false,
})

export function renderMarkdownPreview(markdown: string): string {
  if (!markdown) return ''
  const html = marked.parse(escapeHtml(markdown))
  return `<div class="markdown-body">${html}</div>`
}
