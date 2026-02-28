/**
 * Delete confirmation modal — for notes and folders.
 */

import state from '../../state'
import { NotesAPI, FoldersAPI } from '../../api'
import { escapeHtml } from '../../utils/html'
import { icon } from '../../utils/icons'
// ─── Icons — see utils/icons.ts ────────────────────

export function renderDeleteConfirmModal(container: HTMLElement): void {
  const ctx = state.get('deleteContext')
  if (!ctx) { state.closeModal(); return }

  const isFolder = ctx.type === 'folder'

  container.innerHTML = `
    <div class="modal-card modal-card-sm">
      <!-- Header -->
      <div class="modal-header">
        <div class="modal-header-icon">
          <span class="modal-header-icon-destructive">${icon('alert-triangle', 20)}</span>
          <h2 class="modal-heading">
            Delete ${isFolder ? 'Folder' : 'Note'}
          </h2>
        </div>
        <button id="delete-close" class="btn-icon">${icon('x', 16)}</button>
      </div>

      <!-- Body -->
      <div class="modal-body">
        <p class="modal-body-text">
          Are you sure you want to delete <strong>"${escapeHtml(ctx.name)}"</strong>?
          This action cannot be undone.
        </p>
        ${isFolder ? `
          <label class="checkbox-label">
            <input type="checkbox" id="delete-notes-checkbox" class="checkbox-input" />
            <span class="checkbox-text">
              Also delete all notes inside this folder
            </span>
          </label>
        ` : ''}
      </div>

      <!-- Footer -->
      <div class="modal-footer">
        <button id="delete-cancel" class="btn-secondary">Cancel</button>
        <button id="delete-confirm" class="btn-danger">Delete</button>
      </div>
    </div>
  `

  const close = () => state.closeModal()
  container.querySelector('#delete-close')?.addEventListener('click', close)
  container.querySelector('#delete-cancel')?.addEventListener('click', close)

  container.querySelector('#delete-confirm')?.addEventListener('click', async () => {
    const deleteNotes = (container.querySelector<HTMLInputElement>('#delete-notes-checkbox')?.checked) ?? false
    try {
      if (ctx.type === 'note') {
        await NotesAPI.delete(ctx.id)
        const notes = await NotesAPI.getAll()
        const currentNote = state.get('currentNote')
        state.setState({
          notes,
          currentNote: currentNote?.id === ctx.id ? null : currentNote,
        })
      } else {
        await FoldersAPI.delete(ctx.id, deleteNotes)
        const [folders, notes] = await Promise.all([FoldersAPI.getAll(), NotesAPI.getAll()])
        state.setState({ folders, notes })
      }
      state.showNotification(`${ctx.type === 'note' ? 'Note' : 'Folder'} deleted`, 'success')
      state.closeModal()
    } catch (err) {
      console.error('Delete failed:', err)
      state.showNotification('Delete failed', 'error')
    }
  })
}
