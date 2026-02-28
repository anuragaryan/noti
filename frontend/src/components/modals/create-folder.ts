/**
 * Create folder modal — simple input for folder name.
 */

import state from '../../state'
import { FoldersAPI } from '../../api'
import { icon } from '../../utils/icons'

// ─── Icons — see utils/icons.ts ────────────────────

export function renderCreateFolderModal(container: HTMLElement): void {
  const ctx = state.get('createFolderContext')
  const parentId = ctx?.parentId ?? ''

  container.innerHTML = `
    <div class="modal-card modal-card-xs">
      <!-- Header -->
      <div class="modal-header">
        <div class="modal-header-icon">
          <span class="modal-header-icon-muted">${icon('folder-plus', 18)}</span>
          <h2 class="modal-heading">New Folder</h2>
        </div>
        <button id="cf-close" class="btn-icon">${icon('x', 16)}</button>
      </div>

      <!-- Body -->
      <div class="modal-body">
        <label class="cf-label">
          <span class="cf-label-text">Folder Name</span>
          <input
            id="cf-name"
            type="text"
            placeholder="e.g. Work, Personal, Archive"
            autofocus
            class="cf-name-input"
          />
        </label>
      </div>

      <!-- Footer -->
      <div class="modal-footer">
        <button id="cf-cancel" class="btn-secondary">Cancel</button>
        <button id="cf-create" class="btn-primary">Create Folder</button>
      </div>
    </div>
  `

  const nameInput = container.querySelector<HTMLInputElement>('#cf-name')
  setTimeout(() => nameInput?.focus(), 50)

  const close = () => state.closeModal()
  container.querySelector('#cf-close')?.addEventListener('click', close)
  container.querySelector('#cf-cancel')?.addEventListener('click', close)

  const create = async () => {
    const name = nameInput?.value.trim()
    if (!name) { state.showNotification('Folder name required', 'error'); return }
    try {
      await FoldersAPI.create(name, parentId)
      const folders = await FoldersAPI.getAll()
      state.setState({ folders })
      state.showNotification('Folder created', 'success')
      state.closeModal()
    } catch {
      state.showNotification('Failed to create folder', 'error')
    }
  }

  container.querySelector('#cf-create')?.addEventListener('click', create)
  nameInput?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') void create()
    if (e.key === 'Escape') close()
  })
}
