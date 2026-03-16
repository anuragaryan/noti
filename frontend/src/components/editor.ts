/**
 * Editor component — top bar, editor header, content area.
 */

import state from '../state'
import type { Note } from '../types'
import { NotesAPI, FoldersAPI } from '../api'
import { renderMarkdownPreview } from '../utils/markdown'
import { escapeHtml } from '../utils/html'
import { icon } from '../utils/icons'
import { scrollToBottom } from '../utils/scroll'

// ─── Constants ─────────────────────────────────────────────────────────────────

const DEBOUNCE_DELAY_MS = 1000
const AUTO_SAVE_DELAY_MS = 100

// ─── Debounce utility ────────────────────────────────────────────────────────

function debounce<T extends (...args: unknown[]) => void>(fn: T, ms: number): T {
  let timer: ReturnType<typeof setTimeout>
  return ((...args: unknown[]) => {
    clearTimeout(timer)
    timer = setTimeout(() => fn(...args), ms)
  }) as T
}

// ─── Note State Helper ────────────────────────────────────────────────────────

function updateNoteInState(
  noteId: string,
  title: string,
  markdownContent: string,
  transcriptContent: string,
  options?: { updateCurrentNote?: boolean }
): void {
  const currentNote = state.get('currentNote')
  const notes = state.get('notes')
  const shouldUpdateCurrentNote = options?.updateCurrentNote ?? true

  const updatedNotes = notes.map(n =>
    n.id === noteId ? { ...n, title, markdownContent, transcriptContent } : n
  ) as Note[]

  const shouldSyncCurrentNoteTitleOnly =
    !shouldUpdateCurrentNote &&
    currentNote?.id === noteId &&
    currentNote.title !== title

  state.setState({
    isDirty: false,
    lastSaved: new Date(),
      notes: updatedNotes,
      ...(shouldUpdateCurrentNote && currentNote?.id === noteId
      ? { currentNote: { ...currentNote, title, markdownContent, transcriptContent } as Note }
      : shouldSyncCurrentNoteTitleOnly
        ? { currentNote: { ...currentNote, title } as Note }
      : {})
  })
}

// ─── Top Bar ─────────────────────────────────────────────────────────────────

async function renderTopBar(): Promise<void> {
  const container = document.getElementById('top-bar')
  if (!container) return

  const note = state.get('currentNote')
  const mainView = state.get('mainView')

  if (!note || mainView === 'ai-chat') {
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
  const mainView = state.get('mainView')
  if (!note || mainView === 'ai-chat') {
    container.classList.add('hidden')
    container.innerHTML = ''
    return
  }

  container.classList.remove('hidden')

  const existingTitleInput = document.querySelector<HTMLInputElement>('#note-title-input')
  const hasMatchingInput = existingTitleInput?.dataset.noteId === note.id
  const titleValue = hasMatchingInput ? (existingTitleInput?.value ?? '') : (note.title || '')
  const wasFocused = document.activeElement === existingTitleInput
  const selectionStart = existingTitleInput?.selectionStart ?? null
  const selectionEnd = existingTitleInput?.selectionEnd ?? null

  const isRecording = state.get('isRecording')
  const isPreview = state.get('isPreviewMode')

  container.innerHTML = `
    <div class="editor-header-title">
      <input
        id="note-title-input"
        type="text"
        class="note-title-input"
        data-note-id="${escapeHtml(note.id)}"
        value="${escapeHtml(titleValue)}"
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

  attachTitleInputHandler()
  attachPreviewButtonHandler()

  if (wasFocused) {
    const newInput = document.querySelector<HTMLInputElement>('#note-title-input')
    if (newInput) {
      newInput.focus()
      if (selectionStart !== null && selectionEnd !== null) {
        newInput.setSelectionRange(selectionStart, selectionEnd)
      }
    }
  }
}

function attachTitleInputHandler(): void {
  const titleInput = document.querySelector<HTMLInputElement>('#note-title-input')
  if (!titleInput) return

  titleInput.addEventListener('input', debounce(() => {
    const latestNote = state.get('currentNote')
    if (!latestNote) return

    const currentTitle = titleInput.value
    const contentEl = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
    const currentContent = contentEl ? contentEl.value : latestNote.markdownContent || ''

    state.setState({ isDirty: true })
    void autoSave(latestNote.id, currentTitle, currentContent, getTranscriptEditorContent())
  }, DEBOUNCE_DELAY_MS))
}

function attachPreviewButtonHandler(): void {
  document.querySelector('#preview-btn')?.addEventListener('click', async () => {
    const isCurrentlyPreview = state.get('isPreviewMode')

    // If switching TO preview mode, save first
    if (!isCurrentlyPreview) {
      const currentNote = state.get('currentNote')
      if (!currentNote) return

      cancelPendingSave()

      const previewTitleInput = document.querySelector<HTMLInputElement>('#note-title-input')
      const contentInput = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
      const transcriptInput = document.querySelector<HTMLTextAreaElement>('#transcript-content-textarea')
      const title = previewTitleInput?.value ?? currentNote.title ?? ''
      const markdownContent = contentInput?.value ?? currentNote.markdownContent ?? ''
      const transcriptContent = transcriptInput?.value ?? currentNote.transcriptContent ?? ''

      try {
        await NotesAPI.update(currentNote.id, title, markdownContent, transcriptContent)
        updateNoteInState(currentNote.id, title, markdownContent, transcriptContent)
      } catch (err) {
        console.error('Preview save failed:', err)
        state.showNotification('Failed to save before preview', 'error')
        return
      }
    }

    state.setState({ isPreviewMode: !isCurrentlyPreview })
  })
}

// ─── Editor Content Area ──────────────────────────────────────────────────────

type TranscriptPanelState = {
	isRecording: boolean
	subtitle: string
	bodyHtml: string
}

function getTranscriptEditorContent(): string {
	const transcriptEl = document.querySelector<HTMLTextAreaElement>('#transcript-content-textarea')
	if (transcriptEl) {
		return transcriptEl.value
	}
	return state.get('currentNote')?.transcriptContent || ''
}

function buildTranscriptPanelState(note: Note): TranscriptPanelState {
	const isRecording = state.get('isRecording')
	const partialTranscript = isRecording ? (state.get('partialTranscript') || '') : ''
	const transcriptSuffix = partialTranscript.trim()
	const transcriptBase = note.transcriptContent || ''
	const transcriptDisplay = transcriptSuffix
		? `${transcriptBase}${transcriptBase.trim() ? '\n\n' : ''}${transcriptSuffix}`
		: transcriptBase

	return {
		isRecording,
		subtitle: isRecording ? '.transcript.txt · live capture' : '.transcript.txt · captured',
		bodyHtml: isRecording
			? (transcriptDisplay.trim()
				? `<pre class="transcript-content">${escapeHtml(transcriptDisplay)}</pre>`
				: `<div class="transcript-empty">Transcript will appear here after recording starts.</div>`)
			: `<textarea id="transcript-content-textarea" class="transcript-textarea" placeholder="Transcript…">${escapeHtml(transcriptBase)}</textarea>`,
	}
}

function updateTranscriptPanel(): void {
	if (!state.get('isRecording')) return

	const note = state.get('currentNote')
	if (!note) return

	const panel = document.getElementById('transcript-panel')
	if (!panel) return

	const subtitleEl = document.getElementById('transcript-panel-subtitle')
	const liveChipEl = document.getElementById('transcript-panel-live-chip')
	const bodyEl = document.getElementById('transcript-panel-body')
	if (!subtitleEl || !liveChipEl || !bodyEl) return

	const transcriptState = buildTranscriptPanelState(note)
	panel.classList.toggle('content-panel-transcript-live', transcriptState.isRecording)
	panel.classList.toggle('content-panel-transcript-captured', !transcriptState.isRecording)
	subtitleEl.textContent = transcriptState.subtitle
	liveChipEl.innerHTML = transcriptState.isRecording ? '<span class="transcript-live-chip">LIVE</span>' : ''
	bodyEl.innerHTML = transcriptState.bodyHtml
	scrollToBottom(bodyEl)
}

function renderEditorArea(): void {
  const editorArea = document.getElementById('editor-area')
  const note = state.get('currentNote')
  const mainView = state.get('mainView')

  if (!editorArea) return

  if (!note || mainView === 'ai-chat') {
    editorArea.classList.add('hidden')
    return
  }

  editorArea.classList.remove('hidden')

  const isPreview = state.get('isPreviewMode')
	const isRecording = state.get('isRecording')
	const transcriptActivated = Boolean(note.transcriptActivated)
	const shouldShowTranscript = transcriptActivated || isRecording
	const transcriptState = buildTranscriptPanelState(note)

	if (!shouldShowTranscript) {
		editorArea.innerHTML = isPreview
			? `
	      <div id="preview-content" class="preview-content">
	        ${renderMarkdownPreview(note.markdownContent || '')}
	      </div>
	    `
			: `
	      <textarea
	        id="note-content-textarea"
	        class="editor-textarea"
	        placeholder="Start writing…"
	      >${escapeHtml(note.markdownContent || '')}</textarea>
	    `
	} else {
		editorArea.innerHTML = `
	    <div class="content-workspace ${isRecording ? 'content-workspace-recording' : 'content-workspace-stopped'}">
	      <section class="content-panel content-panel-markdown">
	        <header class="content-panel-header">
	          <div class="content-panel-title-group">
	            <div class="content-panel-title">Final Note</div>
	            <div class="content-panel-subtitle">.md · primary workspace</div>
	          </div>
	        </header>
	        <div class="content-panel-body">
	          ${isPreview
			? `<div id="preview-content" class="preview-content preview-content-panel">${renderMarkdownPreview(note.markdownContent || '')}</div>`
			: `<textarea id="note-content-textarea" class="editor-textarea editor-textarea-panel" placeholder="Start writing…">${escapeHtml(note.markdownContent || '')}</textarea>`
		}
	        </div>
	      </section>

	      <section id="transcript-panel" class="content-panel content-panel-transcript ${transcriptState.isRecording ? 'content-panel-transcript-live' : 'content-panel-transcript-captured'}">
	        <header class="content-panel-header">
	          <div class="content-panel-title-group">
	            <div class="content-panel-title">Transcript</div>
	            <div id="transcript-panel-subtitle" class="content-panel-subtitle">${transcriptState.subtitle}</div>
	          </div>
	          <div id="transcript-panel-live-chip">${transcriptState.isRecording ? '<span class="transcript-live-chip">LIVE</span>' : ''}</div>
	        </header>
	        <div id="transcript-panel-body" class="content-panel-body transcript-body">
	          ${transcriptState.bodyHtml}
	        </div>
	      </section>
	    </div>
	  `
	}

	if (shouldShowTranscript && isRecording) {
		const transcriptBody = document.getElementById('transcript-panel-body')
		if (transcriptBody) {
			scrollToBottom(transcriptBody)
		}
	}

	const textarea = editorArea.querySelector<HTMLTextAreaElement>('#note-content-textarea')
	if (textarea) {
		const targetLine = state.get('editorFocusLine')
		if (targetLine && targetLine > 0) {
			requestAnimationFrame(() => {
				focusTextareaAtLine(textarea, targetLine)
				state.setState({ editorFocusLine: null })
			})
		}

		textarea.addEventListener('input', debounce(() => {
			const latestNote = state.get('currentNote')
			if (!latestNote) return

			const titleInput = document.querySelector<HTMLInputElement>('#note-title-input')
			const currentTitle = titleInput?.value ?? latestNote.title ?? ''
			const currentContent = textarea.value

			state.setState({ isDirty: true })
			void autoSave(latestNote.id, currentTitle, currentContent, getTranscriptEditorContent())
		}, DEBOUNCE_DELAY_MS))
	}

	const transcriptTextarea = editorArea.querySelector<HTMLTextAreaElement>('#transcript-content-textarea')
	if (transcriptTextarea) {
		transcriptTextarea.addEventListener('input', debounce(() => {
			const latestNote = state.get('currentNote')
			if (!latestNote) return

			const titleInput = document.querySelector<HTMLInputElement>('#note-title-input')
			const markdownInput = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
			const currentTitle = titleInput?.value ?? latestNote.title ?? ''
			const currentMarkdown = markdownInput?.value ?? latestNote.markdownContent ?? ''
			const currentTranscript = transcriptTextarea.value

			state.setState({ isDirty: true })
			void autoSave(latestNote.id, currentTitle, currentMarkdown, currentTranscript)
		}, DEBOUNCE_DELAY_MS))
	}
}

function focusTextareaAtLine(textarea: HTMLTextAreaElement, lineNumber: number): void {
  const value = textarea.value
  if (!value) {
    textarea.focus()
    return
  }

  let targetIndex = 0
  let currentLine = 1
  while (currentLine < lineNumber && targetIndex < value.length) {
    const nextNewline = value.indexOf('\n', targetIndex)
    if (nextNewline === -1) {
      targetIndex = value.length
      break
    }
    targetIndex = nextNewline + 1
    currentLine++
  }

  textarea.focus()
  textarea.setSelectionRange(targetIndex, targetIndex)

  const lineHeight = Number.parseFloat(getComputedStyle(textarea).lineHeight) || 20
  textarea.scrollTop = Math.max(0, (currentLine - 3) * lineHeight)
}

// ─── Auto-save ───────────────────────────────────────────────────────────────

let saveTimer: ReturnType<typeof setTimeout> | null = null

function cancelPendingSave(): void {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
}

async function autoSave(noteId: string, title: string, markdownContent: string, transcriptContent: string): Promise<void> {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(async () => {
    saveTimer = null
    try {
      state.setState({ isSaving: true })
      await NotesAPI.update(noteId, title, markdownContent, transcriptContent)
      updateNoteInState(noteId, title, markdownContent, transcriptContent, { updateCurrentNote: false })
      state.setState({ isSaving: false })
      void renderTopBar()
    } catch (err) {
      console.error('Auto-save failed:', err)
      state.setState({ isSaving: false })
    }
  }, AUTO_SAVE_DELAY_MS)
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
	const markdownContent = getEditorContent()
	const transcriptContent = getTranscriptEditorContent()

  try {
    state.setState({ isSaving: true })
    await NotesAPI.update(note.id, title, markdownContent, transcriptContent)
    state.setState({ isSaving: false })
    updateNoteInState(note.id, title, markdownContent, transcriptContent)
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
  state.subscribe('isRecording', () => {
    renderEditorHeader()
    renderEditorArea()
  })
	state.subscribe('partialTranscript', () => updateTranscriptPanel())
  state.subscribe('isDirty', () => void renderTopBar())
  state.subscribe('isSaving', () => void renderTopBar())
  state.subscribe('theme', () => void renderTopBar())
  state.subscribe('mainView', () => {
    void renderTopBar()
    renderEditorHeader()
    renderEditorArea()
  })
}
