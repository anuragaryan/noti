/**
 * Empty state — shown in #editor-area when no note is selected.
 */

import state from '../state'
import { NotesAPI } from '../api'
import { icon } from '../utils/icons'
import { openAIChat } from './ai-chat'

// ─── Lucide icons — see utils/icons.ts ─────────────────────────────────────────────────────────

export function renderEmptyState(container: HTMLElement): void {
  // Layout defined in #empty-state CSS class (app.css)

  container.innerHTML = `
    <div class="empty-state-icon">
      ${icon('file-text', 64)}
    </div>
    <div class="empty-state-text">
      <h2 class="empty-state-heading">
        No note selected
      </h2>
      <p class="empty-state-body">
        Select a note from the sidebar or create a new one to get started.
      </p>
    </div>
    <div class="empty-state-actions">
      <button id="empty-new-note" class="btn-primary btn-primary-lg">
        ${icon('plus', 16)}
        New Note
      </button>
      <button id="empty-new-folder" class="btn-secondary btn-primary-lg">
        ${icon('folder-plus', 16)}
        New Folder
      </button>
      <button id="empty-ai-chat" class="btn-secondary btn-primary-lg">
        ${icon('sparkles', 16)}
        AI Chat
      </button>
    </div>
  `

  container.querySelector('#empty-new-note')?.addEventListener('click', async () => {
    try {
      const note = await NotesAPI.create('Untitled', '', '')
      const notes = await NotesAPI.getAll()
      state.setState({ notes, currentNote: note })
    } catch {
      state.showNotification('Failed to create note', 'error')
    }
  })

  container.querySelector('#empty-new-folder')?.addEventListener('click', () => {
    state.openModal('create-folder')
  })

  container.querySelector('#empty-ai-chat')?.addEventListener('click', () => {
    openAIChat()
  })
}
