/**
 * Editor component — top bar, editor header, content area.
 */

import state from '../state'
import type { Note } from '../types'
import { NotesAPI, FoldersAPI } from '../api'
import { renderMarkdownPreview } from '../utils/markdown'
import { escapeHtml } from '../utils/html'
import { icon } from '../utils/icons'

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

  // Title Input
  const titleInput = container.querySelector<HTMLInputElement>('#note-title-input')
  if (titleInput) {
    titleInput.addEventListener('input', debounce(() => {
      // Re-read from state to avoid stale closure if note was switched
      const latestNote = state.get('currentNote')
      if (latestNote) {
        const currentTitle = titleInput.value
        // If we're in preview mode, getEditorContent() will return '' because textarea is missing.
        // In that case, fallback to the existing note content.
        const contentEl = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
        const currentContent = contentEl ? contentEl.value : latestNote.content || ''

        state.setState({ isDirty: true })
        void autoSave(latestNote.id, currentTitle, currentContent)
      }
    }, 1000))
  }

  // Preview Button
  container.querySelector('#preview-btn')?.addEventListener('click', async () => {
    const isCurrentlyPreview = state.get('isPreviewMode')

    // If switching TO preview mode, save first
    if (!isCurrentlyPreview) {
      const currentNote = state.get('currentNote')
      if (currentNote) {
        // Cancel any pending auto-save to avoid race condition
        cancelPendingSave()

        const previewTitleInput = document.querySelector<HTMLInputElement>('#note-title-input')
        const contentInput = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
        const title = previewTitleInput?.value ?? currentNote.title ?? ''
        const content = contentInput?.value ?? currentNote.content ?? ''

        // Save immediately before switching to preview
        try {
          await NotesAPI.update(currentNote.id, title, content)

          // Update both currentNote AND the notes array so sidebar sees the change
          const notes = state.get('notes')
          const updatedNotes = notes.map(n =>
            n.id === currentNote.id ? { ...n, title, content } : n
          ) as Note[]

          state.setState({
            isDirty: false,
            lastSaved: new Date(),
            notes: updatedNotes,
            currentNote: { ...currentNote, title, content } as Note
          })
        } catch (err) {
          console.error('Preview save failed:', err)
          state.showNotification('Failed to save before preview', 'error')
          return // Don't switch to preview if save failed
        }
      }
    }

    state.setState({ isPreviewMode: !isCurrentlyPreview })
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
        // Re-read from state to avoid stale closure if note was switched
        const latestNote = state.get('currentNote')
        if (!latestNote) return
        const titleInput = document.querySelector<HTMLInputElement>('#note-title-input')
        const currentTitle = titleInput?.value ?? latestNote.title ?? ''
        const currentContent = textarea.value

        state.setState({ isDirty: true })
        void autoSave(latestNote.id, currentTitle, currentContent)
      }, 1000))
    }
  }
}

// ─── Auto-save ────────────────────────────────────────────────────────────────

let saveTimer: ReturnType<typeof setTimeout> | null = null

function cancelPendingSave(): void {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
}

async function autoSave(noteId: string, title: string, content: string): Promise<void> {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(async () => {
    saveTimer = null
    try {
      state.setState({ isSaving: true })
      await NotesAPI.update(noteId, title, content)

      const currentNote = state.get('currentNote')
      const notes = state.get('notes')

      // Update both currentNote AND the notes array so sidebar sees the change
      const updatedNotes = notes.map(n =>
        n.id === noteId ? { ...n, title, content } : n
      ) as Note[]

      state.setState({
        isSaving: false,
        isDirty: false,
        lastSaved: new Date(),
        notes: updatedNotes,
        ...(currentNote?.id === noteId ? { currentNote: { ...currentNote, title, content } as Note } : {})
      })
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

    const currentNote = state.get('currentNote')
    const notes = state.get('notes')

    // Update both currentNote AND the notes array so sidebar sees the change
    const updatedNotes = notes.map(n =>
      n.id === note.id ? { ...n, title, content } : n
    ) as Note[]

    state.setState({
      isSaving: false,
      isDirty: false,
      lastSaved: new Date(),
      notes: updatedNotes,
      ...(currentNote?.id === note.id ? { currentNote: { ...currentNote, title, content } as Note } : {})
    })
    void renderTopBar()
  } catch (err) {
    console.error('Save failed:', err)
    state.setState({ isSaving: false })
    state.showNotification('Failed to save', 'error')
  }
}

// ─── Public API ──────────────────────────────────────────────────────────────

export function initEditor(): void {
  void renderTopBar()
  renderEditorHeader()
  renderEditorArea()

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

