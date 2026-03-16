/**
 * Rename modal — for notes and folders.
 * Uses the existing UpdateNote / UpdateFolder APIs which handle both rename + disk ops.
 */

import state from '../../state'
import { NotesAPI, FoldersAPI } from '../../api'
import { escapeHtml } from '../../utils/html'
import { icon } from '../../utils/icons'

export function renderRenameModal(container: HTMLElement): void {
  const ctx = state.get('renameContext')
  if (!ctx) { state.closeModal(); return }

  const isFolder = ctx.type === 'folder'
  const label = isFolder ? 'Folder' : 'Note'

  container.innerHTML = `
    <div class="modal-card modal-card-xs">
      <!-- Header -->
      <div class="modal-header">
        <div class="modal-header-icon">
          <span class="modal-header-icon-muted">${icon(isFolder ? 'folder' : 'file-text', 18)}</span>
          <h2 class="modal-heading">Rename ${label}</h2>
        </div>
        <button id="rename-close" class="btn-icon">${icon('x', 16)}</button>
      </div>

      <!-- Body -->
      <div class="modal-body">
        <label class="rename-label">
          <span class="rename-label-text">${label} Name</span>
          <input
            id="rename-input"
            type="text"
            value="${escapeHtml(ctx.currentName)}"
            class="rename-input"
            autocomplete="off"
            spellcheck="false"
          />
        </label>
      </div>

      <!-- Footer -->
      <div class="modal-footer">
        <button id="rename-cancel" class="btn-secondary">Cancel</button>
        <button id="rename-confirm" class="btn-primary">Rename</button>
      </div>
    </div>
  `

  const input = container.querySelector<HTMLInputElement>('#rename-input')
  // Focus and select all text for quick overwrite
  setTimeout(() => {
    input?.focus()
    input?.select()
  }, 50)

  const close = () => state.closeModal()
  container.querySelector('#rename-close')?.addEventListener('click', close)
  container.querySelector('#rename-cancel')?.addEventListener('click', close)

  const doRename = async () => {
    const newName = input?.value.trim()
    if (!newName) { state.showNotification(`${label} name cannot be empty`, 'error'); return }
    if (newName === ctx.currentName) { close(); return }

    try {
      if (ctx.type === 'note') {
        // Keep current markdown/transcript while updating title.
        const fullNote = await NotesAPI.get(ctx.id)
        await NotesAPI.update(
          ctx.id,
          newName,
          fullNote.markdownContent ?? '',
          fullNote.transcriptContent ?? '',
        )
        const notes = await NotesAPI.getAll()
        const currentNote = state.get('currentNote')
        state.setState({
          notes,
          currentNote: currentNote?.id === ctx.id ? Object.assign(currentNote, { title: newName }) : currentNote,
        })
      } else {
        // UpdateFolder: keep existing parentId, change name only
        await FoldersAPI.update(ctx.id, newName, ctx.parentId ?? '')
        const folders = await FoldersAPI.getAll()
        state.setState({ folders })
      }
      state.showNotification(`${label} renamed`, 'success')
      close()
    } catch (err) {
      console.error('Rename failed:', err)
      state.showNotification('Rename failed', 'error')
    }
  }

  container.querySelector('#rename-confirm')?.addEventListener('click', () => void doRename())
  input?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') void doRename()
    if (e.key === 'Escape') close()
  })
}
