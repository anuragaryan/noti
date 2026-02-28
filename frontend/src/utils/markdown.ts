/**
 * Lightweight markdown → HTML parser.
 * Supports: headings, bold, italic, inline code, code blocks, lists, links, blockquotes.
 */

import { escapeHtml } from './html'
export function renderMarkdownPreview(markdown: string): string {
  if (!markdown) return '<p style="color: var(--muted-foreground); font-style: italic;">Nothing to preview.</p>'

  let html = escapeHtml(markdown)

  // Code blocks (``` ... ```)
  html = html.replace(/```[\w]*\n([\s\S]*?)```/g, (_, code) => {
    return `<pre style="background: var(--muted); border-radius: 6px; padding: 16px; overflow-x: auto; margin: 16px 0;"><code style="font-family: var(--font-primary); font-size: 13px; color: var(--foreground);">${code.trim()}</code></pre>`
  })

  // Headings
  html = html.replace(/^######\s+(.+)$/gm, '<h6 style="font-family: var(--font-primary); font-size: 13px; font-weight: 700; margin: 16px 0 8px;">$1</h6>')
  html = html.replace(/^#####\s+(.+)$/gm, '<h5 style="font-family: var(--font-primary); font-size: 14px; font-weight: 700; margin: 16px 0 8px;">$1</h5>')
  html = html.replace(/^####\s+(.+)$/gm, '<h4 style="font-family: var(--font-primary); font-size: 15px; font-weight: 700; margin: 16px 0 8px;">$1</h4>')
  html = html.replace(/^###\s+(.+)$/gm, '<h3 style="font-family: var(--font-primary); font-size: 16px; font-weight: 700; margin: 20px 0 8px;">$1</h3>')
  html = html.replace(/^##\s+(.+)$/gm, '<h2 style="font-family: var(--font-primary); font-size: 18px; font-weight: 700; margin: 24px 0 8px;">$1</h2>')
  html = html.replace(/^#\s+(.+)$/gm, '<h1 style="font-family: var(--font-primary); font-size: 22px; font-weight: 700; margin: 24px 0 8px;">$1</h1>')

  // Blockquote
  html = html.replace(/^&gt;\s+(.+)$/gm, '<blockquote style="border-left: 3px solid var(--primary); padding-left: 16px; color: var(--muted-foreground); margin: 12px 0;">$1</blockquote>')

  // Bold
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong style="font-weight: 700;">$1</strong>')
  html = html.replace(/__(.+?)__/g, '<strong style="font-weight: 700;">$1</strong>')

  // Italic
  html = html.replace(/\*(.+?)\*/g, '<em>$1</em>')
  html = html.replace(/_(.+?)_/g, '<em>$1</em>')

  // Inline code
  html = html.replace(/`([^`]+)`/g, '<code style="font-family: var(--font-primary); font-size: 13px; background: var(--muted); padding: 2px 6px; border-radius: 4px;">$1</code>')

  // Unordered lists
  html = html.replace(/^[•\-\*]\s+(.+)$/gm, '<li style="margin: 4px 0; padding-left: 4px;">$1</li>')
  html = html.replace(/(<li.*<\/li>\n?)+/g, (m) => `<ul style="margin: 8px 0; padding-left: 24px; list-style: disc;">${m}</ul>`)

  // Ordered lists
  html = html.replace(/^\d+\.\s+(.+)$/gm, '<li style="margin: 4px 0; padding-left: 4px;">$1</li>')

  // Links
  html = html.replace(/\[(.+?)\]\((.+?)\)/g, '<a href="$2" style="color: var(--primary); text-decoration: underline;">$1</a>')

  // Paragraphs — wrap consecutive non-block lines
  html = html.replace(/^(?!<[hupbco]|<li|<pre|<blockquote)(.+)$/gm, '<p style="margin: 8px 0; line-height: 1.8;">$1</p>')

  // Clean up empty paragraphs
  html = html.replace(/<p[^>]*><\/p>/g, '')

  return html
}

// escapeHtml imported from ./html.ts
