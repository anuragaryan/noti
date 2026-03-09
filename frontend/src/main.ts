/**
 * noti — main entry point.
 * Initializes app: theme, state, data load, components, events, shortcuts.
 */

import './app.css'
import { initTheme } from './components/theme-toggle'
import { initSidebar } from './components/sidebar'
import { initEditor, saveCurrentNote, getEditorTitle, getEditorContent } from './components/editor'
import { initToolbar, loadPrompts } from './components/toolbar'
import { initRecording } from './components/recording'
import { renderEmptyState } from './components/empty-state'
import { renderGettingStarted } from './components/getting-started'
import { renderSettingsModal } from './components/modals/settings'
import { renderPromptsModal } from './components/modals/prompts'
import { renderDeleteConfirmModal } from './components/modals/delete-confirm'
import { renderCreateFolderModal } from './components/modals/create-folder'
import { renderRenameModal } from './components/modals/rename'
import { renderMoveModal } from './components/modals/move'
import { renderDownloadsModal } from './components/modals/downloads'
import { AppEvents } from './events'
import { NotesAPI, FoldersAPI, AudioAPI, ConfigAPI } from './api'
import state from './state'

// ─── Modal System ─────────────────────────────────────────────────────────────

function initModalSystem(): void {
  const overlay = document.getElementById('modal-overlay')!
  const content = document.getElementById('modal-content')!
  if (!overlay || !content) return

  async function renderModal(): Promise<void> {
    const modal = state.get('activeModal')

    if (!modal) {
      overlay.classList.add('hidden')
      content.innerHTML = ''
      return
    }

    overlay.classList.remove('hidden')
    content.innerHTML = ''

    switch (modal) {
      case 'settings':
        await renderSettingsModal(content)
        break
      case 'downloads':
        renderDownloadsModal(content)
        break
      case 'prompts':
        await renderPromptsModal(content)
        break
      case 'delete-note':
      case 'delete-folder':
        renderDeleteConfirmModal(content)
        break
      case 'create-folder':
        renderCreateFolderModal(content)
        break
      case 'rename-note':
      case 'rename-folder':
        renderRenameModal(content)
        break
      case 'move-note':
      case 'move-folder':
        renderMoveModal(content)
        break
    }
  }

  // Close on overlay click (not card click)
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) state.closeModal()
  })

  // Close on Escape
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && state.get('activeModal')) {
      state.closeModal()
    }
  })

  state.subscribe('activeModal', () => void renderModal())
  state.subscribe('downloads', () => {
    if (state.get('activeModal') === 'downloads') {
      renderDownloadsModal(content)
    }
  })
}

// ─── Notification Toast ───────────────────────────────────────────────────────

function initNotificationToast(): void {
  const toast = document.getElementById('notification-toast')
  if (!toast) return

  // Base positioning and typography are defined in #notification-toast CSS class (app.css).

  state.subscribe('notification', () => {
    const notif = state.get('notification')
    if (!notif) {
      toast.classList.add('hidden')
      return
    }

    const bgMap = {
      info: 'var(--card)',
      success: 'var(--color-success)',
      error: 'var(--color-error)',
    }
    const colorMap = {
      info: 'var(--foreground)',
      success: 'var(--color-success-foreground)',
      error: 'var(--color-error-foreground)',
    }

    toast.style.background = bgMap[notif.type]
    toast.style.color = colorMap[notif.type]
    toast.style.border = '' // reset; border defined in CSS
    toast.textContent = notif.message
    toast.classList.remove('hidden')
  })
}

// ─── Keyboard Shortcuts ───────────────────────────────────────────────────────

function initKeyboardShortcuts(): void {
  document.addEventListener('keydown', (e) => {
    const isMac = /mac/i.test((navigator as Navigator & { userAgentData?: { platform: string } }).userAgentData?.platform ?? navigator.userAgent)
    const mod = isMac ? e.metaKey : e.ctrlKey

    if (!mod) return

    switch (e.key.toLowerCase()) {
      case 's':
        e.preventDefault()
        void saveCurrentNote()
        break

      case ',':
        // ⌘, — Settings
        e.preventDefault()
        state.openModal('settings')
        break

      case 'k':
        e.preventDefault()
        document.getElementById('search-input')?.focus()
        break
    }
  })
}

// ─── Go Events ────────────────────────────────────────────────────────────────

function initGoEvents(): void {
  AppEvents.onMenuSettings(() => state.openModal('settings'))

  AppEvents.onConfigSaved(() => {
    void ConfigAPI.get().then(config => state.setState({ config }))
  })

  AppEvents.onLLMReady(({ provider, modelName }) => {
    state.setState({ llmAvailable: true })
    state.showNotification(`LLM ready: ${modelName} (${provider})`, 'info')
  })

  AppEvents.onSTTReady(() => {
    state.setState({ sttAvailable: true })
  })

  AppEvents.onDownloadEvent((payload) => {
    state.upsertDownload(payload)

    if (
      payload.status === 'queued' &&
      state.get('activeModal') !== 'downloads' &&
      !state.get('downloadsModalDismissed')
    ) {
      state.openModal('downloads')
    }

    if (payload.kind === 'stt-model' && payload.status === 'completed') {
      void AudioAPI.getSTTStatus().then(status => {
        state.setState({ sttAvailable: Boolean(status.available) })
      })
    }

    if (payload.status === 'error') {
      const label = payload.label || 'Download'
      const message = payload.error ? `${label}: ${payload.error}` : `${label} download failed`
      state.showNotification(message, 'error')
    }
  })
}

// ─── Initial Data Load ────────────────────────────────────────────────────────

async function loadInitialData(): Promise<void> {
  try {
    const [notes, folders, config, sttStatus, isFirstRun] = await Promise.all([
      NotesAPI.getAll(),
      FoldersAPI.getAll(),
      ConfigAPI.get(),
      AudioAPI.getSTTStatus(),
      ConfigAPI.isFirstRun(),
    ])

    state.setState({
      notes,
      folders,
      config,
      sttAvailable: Boolean(sttStatus.available),
      showGettingStarted: isFirstRun,
      // Sync recording source from config so the UI reflects the actual configured source
      recordingSource: config?.audio?.defaultSource ?? 'microphone',
    })

    await loadPrompts()
  } catch (err) {
    console.error('Failed to load initial data:', err)
  }
}

// ─── Empty State ──────────────────────────────────────────────────────────────

function initEmptyState(): void {
  const emptyEl = document.getElementById('empty-state')
  if (!emptyEl) return

  const renderPlaceholder = (): void => {
    if (state.get('showGettingStarted')) {
      void renderGettingStarted(emptyEl)
      return
    }
    renderEmptyState(emptyEl)
  }

  renderPlaceholder()

  state.subscribe('currentNote', () => {
    const emptyEl = document.getElementById('empty-state')
    const editorArea = document.getElementById('editor-area')
    const note = state.get('currentNote')
    const showGettingStarted = state.get('showGettingStarted')

    if (note && !showGettingStarted) {
      emptyEl?.classList.add('hidden')
      if (editorArea) editorArea.classList.remove('hidden')
    } else {
      emptyEl?.classList.remove('hidden')
      if (editorArea) editorArea.classList.add('hidden')
      if (emptyEl) renderPlaceholder()
    }
  })

  state.subscribe('showGettingStarted', () => {
    const emptyEl = document.getElementById('empty-state')
    const editorArea = document.getElementById('editor-area')
    const note = state.get('currentNote')
    const showGettingStarted = state.get('showGettingStarted')

    if (showGettingStarted || !note) {
      emptyEl?.classList.remove('hidden')
      editorArea?.classList.add('hidden')
      if (emptyEl) renderPlaceholder()
      return
    }

    emptyEl?.classList.add('hidden')
    editorArea?.classList.remove('hidden')
  })
}

// ─── Boot ─────────────────────────────────────────────────────────────────────

async function boot(): Promise<void> {
  // 1. Theme (must be first to avoid flash)
  initTheme()

  // 2. Initialize all UI components
  initSidebar()
  initEditor()
  initToolbar()
  initRecording()
  initEmptyState()
  initModalSystem()
  initNotificationToast()

  // 3. Wire keyboard shortcuts + Go events
  initKeyboardShortcuts()
  initGoEvents()

  // 4. Load data (triggers state updates which re-render components)
  await loadInitialData()

  // 5. Show empty state initially (no note selected)
  const emptyEl = document.getElementById('empty-state')
  const editorArea = document.getElementById('editor-area')
  if (state.get('showGettingStarted') || !state.get('currentNote')) {
    emptyEl?.classList.remove('hidden')
    editorArea?.classList.add('hidden')
  }
}

window.addEventListener('DOMContentLoaded', () => {
  void boot()
})
