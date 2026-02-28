/**
 * Move modal — lets user pick a destination folder for a note or a folder.
 * For notes:   calls NotesAPI.move(noteId, targetFolderId)
 * For folders: calls FoldersAPI.update(folderId, sameName, newParentId)
 */

import state from '../../state'
import { NotesAPI, FoldersAPI } from '../../api'
import { escapeHtml } from '../../utils/html'
import { icon } from '../../utils/icons'
import type { Folder } from '../../types'

// Build a flat list of folders with indentation info so the user can pick one.
function buildFlatList(
  folders: Folder[],
  excludeId: string,
): Array<{ folder: Folder; depth: number }> {
  const tree = new Map<string, Folder[]>()
  tree.set('', [])
  for (const f of folders) {
    if (f.id === excludeId) continue // skip self (can't move into self)
    const parentId = f.parentId ?? ''
    if (!tree.has(parentId)) tree.set(parentId, [])
    tree.get(parentId)!.push(f)
  }

  const result: Array<{ folder: Folder; depth: number }> = []

  function walk(parentId: string, depth: number): void {
    const children = tree.get(parentId) ?? []
    const sorted = [...children].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }))
    for (const f of sorted) {
      result.push({ folder: f, depth })
      walk(f.id, depth + 1)
    }
  }

  walk('', 0)
  return result
}

export function renderMoveModal(container: HTMLElement): void {
  const ctx = state.get('moveContext')
  if (!ctx) { state.closeModal(); return }

  const isNote = ctx.type === 'note'
  const label = isNote ? 'Note' : 'Folder'
  const currentDestId = isNote ? (ctx.currentFolderId ?? '') : (ctx.currentParentId ?? '')

  // For folder moves we must exclude the folder itself (can't nest into self)
  const excludeId = isNote ? '' : ctx.id

  const allFolders = state.get('folders')
  const flatList = buildFlatList(allFolders, excludeId)

  // Track selected destination
  let selectedId = currentDestId

  function buildItems(): string {
    const rootItem = `
      <button
        class="move-list-item${selectedId === '' ? ' selected' : ''}"
        data-folder-id=""
        style="padding-left: 10px;"
      >
        <span class="move-list-item-icon">${icon('home', 14)}</span>
        Root (no folder)
      </button>
    `

    const folderItems = flatList.map(({ folder, depth }) => {
      const indent = 10 + depth * 16
      return `
        <button
          class="move-list-item${selectedId === folder.id ? ' selected' : ''}"
          data-folder-id="${escapeHtml(folder.id)}"
          style="padding-left: ${indent}px;"
        >
          <span class="move-list-item-icon">${icon('folder', 14)}</span>
          ${escapeHtml(folder.name)}
        </button>
      `
    }).join('')

    return rootItem + folderItems
  }

  const render = () => {
    const list = container.querySelector<HTMLElement>('#move-list')
    if (!list) return
    list.innerHTML = buildItems()
    list.querySelectorAll<HTMLButtonElement>('.move-list-item').forEach(btn => {
      btn.addEventListener('click', () => {
        selectedId = btn.dataset.folderId ?? ''
        render()
      })
    })
  }

  container.innerHTML = `
    <div class="modal-card modal-card-xs">
      <!-- Header -->
      <div class="modal-header">
        <div class="modal-header-icon">
          <span class="modal-header-icon-muted">${icon('folder-input', 18)}</span>
          <h2 class="modal-heading">Move ${label}</h2>
        </div>
        <button id="move-close" class="btn-icon">${icon('x', 16)}</button>
      </div>

      <!-- Body -->
      <div class="modal-body" style="gap: 12px;">
        <p style="font-family: var(--font-secondary); font-size: 13px; color: var(--muted-foreground); margin: 0;">
          Move <strong>${escapeHtml(ctx.name)}</strong> to:
        </p>
        <div class="move-list" id="move-list">
          ${flatList.length === 0 && isNote ? '<p class="move-empty">No folders yet. Item will be moved to root.</p>' : ''}
        </div>
      </div>

      <!-- Footer -->
      <div class="modal-footer">
        <button id="move-cancel" class="btn-secondary">Cancel</button>
        <button id="move-confirm" class="btn-primary">Move Here</button>
      </div>
    </div>
  `

  render()

  const close = () => state.closeModal()
  container.querySelector('#move-close')?.addEventListener('click', close)
  container.querySelector('#move-cancel')?.addEventListener('click', close)

  container.querySelector('#move-confirm')?.addEventListener('click', async () => {
    if (selectedId === currentDestId) { close(); return }

    try {
      if (ctx.type === 'note') {
        await NotesAPI.move(ctx.id, selectedId)
        const [notes, folders] = await Promise.all([NotesAPI.getAll(), FoldersAPI.getAll()])
        const currentNote = state.get('currentNote')
        state.setState({
          notes,
          folders,
          currentNote: currentNote?.id === ctx.id
            ? Object.assign(currentNote, { folderId: selectedId })
            : currentNote,
        })
        if (selectedId) {
          const expanded = new Set(state.get('expandedFolders'))
          expanded.add(selectedId)
          state.setState({ expandedFolders: expanded })
        }
      } else {
        // Moving a folder: keep the same name, change parentId
        const folder = allFolders.find(f => f.id === ctx.id)
        if (!folder) throw new Error('Folder not found')
        await FoldersAPI.update(ctx.id, folder.name, selectedId)
        const folders = await FoldersAPI.getAll()
        state.setState({ folders })
        if (selectedId) {
          const expanded = new Set(state.get('expandedFolders'))
          expanded.add(selectedId)
          state.setState({ expandedFolders: expanded })
        }
      }
      state.showNotification(`${label} moved`, 'success')
      close()
    } catch (err) {
      console.error('Move failed:', err)
      state.showNotification('Move failed', 'error')
    }
  })
}
