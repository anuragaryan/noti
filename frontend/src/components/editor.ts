/**
 * Editor component — top bar, editor header, content area.
 * Injects into: #top-bar, #editor-header, #editor-area
 */

import state from '../state'
import type { Note } from '../types'
import { NotesAPI, FoldersAPI } from '../api'
import { renderMarkdownPreview } from '../utils/markdown'
import { escapeHtml } from '../utils/html'
import { icon } from '../utils/icons'

// ─── Lucide icons — see utils/icons.ts ──────────────────────────────────────────────────────────

// ─── Debounce utility ────────────────────────────────────────────────────────

function debounce<T extends (...args: unknown[]) => void>(fn: T, ms: number): T {
  let timer: ReturnType<typeof setTimeout>
  return ((...args: unknown[]) => {
    clearTimeout(timer)
    timer = setTimeout(() => fn(...args), ms)
  }) as T
}

// ─── Top Bar ─────────────────────────────────────────────────────────────────

async function renderTopBar(): Promise<void> {
  const container = document.getElementById('top-bar')
  if (!container) return

  const note = state.get('currentNote')

  if (!note) {
    container.innerHTML = ''
    container.classList.add('hidden')
    return
  }

  container.classList.remove('hidden')
  // Layout defined in #top-bar CSS class

  let breadcrumbHtml = `<span class="breadcrumb-text">All Notes</span>`

  if (note?.folderId) {
    try {
      const path = await FoldersAPI.getPath(note.folderId)
      const parts = path.map((f, i) => {
        const isLast = i === path.length - 1
        return isLast
          ? `<span class="breadcrumb-current">${escapeHtml(f.name)}</span>`
          : `<span class="breadcrumb-text">${escapeHtml(f.name)}</span>`
      })
      breadcrumbHtml = parts.join(`<span class="breadcrumb-sep">${icon('chevron-right', 14)}</span>`)
    } catch {
      // fallback
    }
  }

  const isDirty = state.get('isDirty')
  const isSaving = state.get('isSaving')
  let statusHtml = ''
  if (isSaving) {
    statusHtml = `<span class="save-badge save-badge-saving"><span class="status-dot"></span>Saving…</span>`
  } else if (note && !isDirty) {
    statusHtml = `<span class="save-badge save-badge-saved"><span class="status-dot"></span>Saved</span>`
  }

  container.innerHTML = `
    <div class="top-bar-left">
      ${breadcrumbHtml}
    </div>
    <div class="top-bar-right">
      ${statusHtml}
    </div>
  `
}

// ─── Editor Header ────────────────────────────────────────────────────────────

function renderEditorHeader(): void {
  const container = document.getElementById('editor-header')
  if (!container) return

  // Layout defined in #editor-header CSS class

  const note = state.get('currentNote')
  if (!note) {
    container.innerHTML = ''
    return
  }

  const isRecording = state.get('isRecording')
  const isPreview = state.get('isPreviewMode')

  container.innerHTML = `
    <div class="editor-header-title">
      <input
        id="note-title-input"
        type="text"
        class="note-title-input"
        value="${escapeHtml(note.title || '')}"
        placeholder="Untitled"
      />
    </div>
    <div class="editor-header-actions">
      <button id="record-btn" class="record-btn ${isRecording ? 'record-btn-active' : 'record-btn-idle'}">
        ${isRecording ? `
          <span class="record-btn-recording-content">
            <span class="record-dot pulse-dot"></span>
            Recording…
          </span>
          <span class="record-btn-stop-content">
            ${icon('square', 14)}
            Stop
          </span>
        ` : `
          ${icon('mic', 16)} Record
        `}
      </button>
      <button id="preview-btn" class="preview-btn ${isRecording ? 'preview-btn-recording' : 'preview-btn-idle'}">
        ${icon(isPreview ? 'eye-off' : 'eye', 16)}
        ${isPreview ? 'Edit' : 'Preview'}
      </button>
    </div>
  `

  // Wire title input
  const titleInput = container.querySelector<HTMLInputElement>('#note-title-input')
  if (titleInput) {
    titleInput.addEventListener('input', debounce(() => {
      if (note) {
        state.setState({ isDirty: true })
        void autoSave(note.id, titleInput.value, getEditorContent())
      }
    }, 1000))
  }

  // Record button clicks are handled by recording.ts via event delegation on #editor-header

  // Wire preview button
  container.querySelector('#preview-btn')?.addEventListener('click', () => {
    state.setState({ isPreviewMode: !isPreview })
  })
}

// ─── Editor Content Area ──────────────────────────────────────────────────────

function renderEditorArea(): void {
  const editorArea = document.getElementById('editor-area')
  const note = state.get('currentNote')

  if (!editorArea) return

  if (!note) {
    editorArea.classList.add('hidden')
    return
  }

  // Show editor (empty-state visibility is managed in main.ts)
  editorArea.classList.remove('hidden')

  const isPreview = state.get('isPreviewMode')

  if (isPreview) {
    editorArea.innerHTML = `
      <div id="preview-content" class="preview-content">
        ${renderMarkdownPreview(note.content || '')}
      </div>
    `
  } else {
    editorArea.innerHTML = `
      <textarea
        id="note-content-textarea"
        class="editor-textarea"
        placeholder="Start writing…"
      >${escapeHtml(note.content || '')}</textarea>
    `

    const textarea = editorArea.querySelector<HTMLTextAreaElement>('#note-content-textarea')
    if (textarea) {
      textarea.addEventListener('input', debounce(() => {
        state.setState({ isDirty: true })
        const titleInput = document.querySelector<HTMLInputElement>('#note-title-input')
        void autoSave(note.id, titleInput?.value ?? note.title, textarea.value)
      }, 1000))
    }
  }
}

// ─── Auto-save ────────────────────────────────────────────────────────────────

let saveTimer: ReturnType<typeof setTimeout> | null = null

async function autoSave(noteId: string, title: string, content: string): Promise<void> {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(async () => {
    try {
      state.setState({ isSaving: true })
      await NotesAPI.update(noteId, title, content)
      state.setState({ isSaving: false, isDirty: false, lastSaved: new Date() })
      void renderTopBar()
    } catch (err) {
      console.error('Auto-save failed:', err)
      state.setState({ isSaving: false })
    }
  }, 100)
}

export function getEditorContent(): string {
  return document.querySelector<HTMLTextAreaElement>('#note-content-textarea')?.value ?? ''
}

export function getEditorTitle(): string {
  return document.querySelector<HTMLInputElement>('#note-title-input')?.value ?? ''
}

// ─── Manual save ─────────────────────────────────────────────────────────────

export async function saveCurrentNote(): Promise<void> {
  const note = state.get('currentNote')
  if (!note) return
  const title = getEditorTitle()
  const content = getEditorContent()
  try {
    state.setState({ isSaving: true })
    await NotesAPI.update(note.id, title, content)
    state.setState({ isSaving: false, isDirty: false, lastSaved: new Date() })
    void renderTopBar()
  } catch (err) {
    console.error('Save failed:', err)
    state.setState({ isSaving: false })
    state.showNotification('Failed to save', 'error')
  }
}

// ─── Utility — escapeHtml imported from utils/html.ts ───────────────────────

// ─── Public API ──────────────────────────────────────────────────────────────

export function initEditor(): void {
  void renderTopBar()
  renderEditorHeader()
  renderEditorArea()

  // Main content layout defined in #main-content CSS class (app.css)

  // Subscribe to state changes
  state.subscribe('currentNote', () => {
    void renderTopBar()
    renderEditorHeader()
    renderEditorArea()
  })
  state.subscribe('isPreviewMode', () => {
    renderEditorHeader()
    renderEditorArea()
  })
  state.subscribe('isRecording', () => renderEditorHeader())
  state.subscribe('isDirty', () => void renderTopBar())
  state.subscribe('isSaving', () => void renderTopBar())
  state.subscribe('theme', () => void renderTopBar())
}
