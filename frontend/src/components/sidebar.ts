/**
 * Sidebar component — renders folders, notes, search, action bar, and footer.
 * Injects into: #sidebar-header, #sidebar-search, #sidebar-actions,
 *               #sidebar-folders, #sidebar-notes, #sidebar-footer
 */

import { renderThemeToggle } from './theme-toggle'
import { escapeHtml } from '../utils/html'
import { icon } from '../utils/icons'
import { NotesAPI, FoldersAPI } from '../api'
import state from '../state'
import type { Folder, Note } from '../types'

// ─── Context Menu ────────────────────────────────────────────────────────────

/** Dismisses any currently open context menu. */
function dismissContextMenu(): void {
  document.getElementById('__context-menu__')?.remove()
}

/** Shows a context menu at the given page coordinates. */
function showContextMenu(
  x: number,
  y: number,
  items: Array<{ label: string; iconName: string; danger?: boolean; action: () => void } | 'separator'>,
): void {
  dismissContextMenu()

  const menu = document.createElement('div')
  menu.id = '__context-menu__'
  menu.className = 'context-menu'

  for (const item of items) {
    if (item === 'separator') {
      const sep = document.createElement('div')
      sep.className = 'context-menu-separator'
      menu.appendChild(sep)
      continue
    }

    const btn = document.createElement('button')
    btn.className = `context-menu-item${item.danger ? ' danger' : ''}`
    btn.innerHTML = `${icon(item.iconName, 14)}<span>${item.label}</span>`
    btn.addEventListener('click', (e) => {
      e.stopPropagation()
      dismissContextMenu()
      item.action()
    })
    menu.appendChild(btn)
  }

  // Position: keep within viewport
  document.body.appendChild(menu)
  const rect = menu.getBoundingClientRect()
  const safeX = Math.min(x, window.innerWidth - rect.width - 8)
  const safeY = Math.min(y, window.innerHeight - rect.height - 8)
  menu.style.left = `${safeX}px`
  menu.style.top = `${safeY}px`

  // Dismiss on outside click / scroll / Escape
  const dismiss = (e: Event) => {
    if (!menu.contains(e.target as Node)) {
      dismissContextMenu()
      cleanup()
    }
  }
  const escDismiss = (e: KeyboardEvent) => {
    if (e.key === 'Escape') { dismissContextMenu(); cleanup() }
  }
  const cleanup = () => {
    document.removeEventListener('click', dismiss, true)
    document.removeEventListener('scroll', dismiss, true)
    document.removeEventListener('keydown', escDismiss, true)
  }
  // Use timeout so the triggering click doesn't immediately dismiss the menu
  setTimeout(() => {
    document.addEventListener('click', dismiss, true)
    document.addEventListener('scroll', dismiss, true)
    document.addEventListener('keydown', escDismiss, true)
  }, 0)
}
// ─── Drag state ──────────────────────────────────────────────────────────────

type DragPayload =
  | { kind: 'note'; id: string; currentFolderId: string }
  | { kind: 'folder'; id: string; name: string; currentParentId: string }

let dragPayload: DragPayload | null = null
// ─── Helper: create element ─────────────────────────────────────────────────

function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs: Record<string, string> = {},
  children: (HTMLElement | string)[] = [],
): HTMLElementTagNameMap[K] {
  const elem = document.createElement(tag)
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'class') elem.className = v
    else elem.setAttribute(k, v)
  }
  for (const child of children) {
    if (typeof child === 'string') elem.appendChild(document.createTextNode(child))
    else elem.appendChild(child)
  }
  return elem
}

// ─── Lucide SVG icons — see utils/icons.ts ─────────────────────────────────────────────────────────

// ─── Sidebar Header ──────────────────────────────────────────────────────────

function renderHeader(): void {
  const container = document.getElementById('sidebar-header')
  if (!container) return
  container.innerHTML = `
    <div class="sidebar-logo-lockup">
      <div class="sidebar-app-badge">${icon('audio-waveform', 16)}</div>
      <span class="sidebar-logo">noti</span>
    </div>
  `
}

// ─── Sidebar Search ───────────────────────────────────────────────────────────

function renderSearch(): void {
  const container = document.getElementById('sidebar-search')
  if (!container) return
  container.innerHTML = `
    <div class="search-bar" id="search-input-row">
      <span class="search-icon">${icon('search', 16)}</span>
      <span class="search-placeholder">Search notes…</span>
    </div>
  `
  container.querySelector('#search-input-row')?.addEventListener('click', () => {
    // Focus search — placeholder for now
  })
}

// ─── Action Bar ───────────────────────────────────────────────────────────────

function renderActions(): void {
  const container = document.getElementById('sidebar-actions')
  if (!container) return
  container.innerHTML = ''
  // Layout defined in #sidebar-actions CSS class

  const sortOrder = state.get('sortOrder')
  const sortTitle = sortOrder === 'asc' ? 'Sort: A→Z (click to reverse)' : 'Sort: Z→A (click to reverse)'

  const makeActionBtn = (iconName: string, title: string, onClick: () => void, active = false) => {
    const btn = el('button', { title, class: `action-btn${active ? ' active' : ''}` })
    btn.innerHTML = icon(iconName, 14)
    btn.addEventListener('click', onClick)
    return btn
  }

  container.append(
    makeActionBtn('file-plus', 'New Note', handleNewNote),
    makeActionBtn('folder-plus', 'New Folder', handleNewFolder),
    makeActionBtn('arrow-up-down', sortTitle, handleSort, true),
  )
}

// ─── Folder Tree ──────────────────────────────────────────────────────────────

// ─── Sort Helpers ─────────────────────────────────────────────────────────────

function sortByName<T extends { name: string }>(items: T[], order: 'asc' | 'desc'): T[] {
  return [...items].sort((a, b) => {
    const cmp = a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
    return order === 'asc' ? cmp : -cmp
  })
}

function sortNotesByTitle(notes: Note[], order: 'asc' | 'desc'): Note[] {
  return [...notes].sort((a, b) => {
    const titleA = a.title || 'Untitled'
    const titleB = b.title || 'Untitled'
    const cmp = titleA.localeCompare(titleB, undefined, { sensitivity: 'base' })
    return order === 'asc' ? cmp : -cmp
  })
}

function handleSort(): void {
  const current = state.get('sortOrder')
  state.setState({ sortOrder: current === 'asc' ? 'desc' : 'asc' })
}

function buildFolderTree(folders: Folder[]): Map<string, Folder[]> {
  const tree = new Map<string, Folder[]>()
  tree.set('', []) // root
  for (const folder of folders) {
    const parentId = folder.parentId || ''
    if (!tree.has(parentId)) tree.set(parentId, [])
    tree.get(parentId)!.push(folder)
  }
  return tree
}

function applyDropHighlight(el: HTMLElement, active: boolean): void {
  el.classList.toggle('drop-target-active', active)
}

function setupDragSource(el: HTMLElement, payload: DragPayload): void {
  el.setAttribute('draggable', 'true')
  el.addEventListener('dragstart', e => {
    dragPayload = payload
    e.dataTransfer!.effectAllowed = 'move'
    setTimeout(() => el.classList.add('dragging'), 0)
  })
  el.addEventListener('dragend', () => {
    dragPayload = null
    el.classList.remove('dragging')
  })
}

function setupDropTarget(target: HTMLElement, onDrop: (payload: DragPayload) => Promise<void>): void {
  target.addEventListener('dragover', e => {
    if (!dragPayload) return
    e.preventDefault()
    e.dataTransfer!.dropEffect = 'move'
    applyDropHighlight(target, true)
  })
  target.addEventListener('dragleave', e => {
    if (!target.contains(e.relatedTarget as Node)) {
      applyDropHighlight(target, false)
    }
  })
  target.addEventListener('drop', async e => {
    e.preventDefault()
    applyDropHighlight(target, false)
    const p = dragPayload
    dragPayload = null
    if (!p) return
    await onDrop(p)
  })
}

async function executeDrop(payload: DragPayload, targetFolderId: string): Promise<void> {
  try {
    if (payload.kind === 'note') {
      if (payload.currentFolderId === targetFolderId) return
      await NotesAPI.move(payload.id, targetFolderId)
      const notes = await NotesAPI.getAll()
      const folders = await FoldersAPI.getAll()
      const currentNote = state.get('currentNote')
      if (currentNote?.id === payload.id) {
        currentNote.folderId = targetFolderId
        state.setState({ notes, folders, currentNote })
      } else {
        state.setState({ notes, folders })
      }
      if (targetFolderId) {
        const expanded = new Set(state.get('expandedFolders'))
        expanded.add(targetFolderId)
        state.setState({ expandedFolders: expanded })
      }
    } else {
      if (payload.currentParentId === targetFolderId) return
      await FoldersAPI.update(payload.id, payload.name, targetFolderId)
      const folders = await FoldersAPI.getAll()
      state.setState({ folders })
      if (targetFolderId) {
        const expanded = new Set(state.get('expandedFolders'))
        expanded.add(targetFolderId)
        state.setState({ expandedFolders: expanded })
      }
    }
  } catch (err) {
    console.error('Drop failed:', err)
    state.showNotification('Failed to move item', 'error')
  }
}

function renderFolderItem(folder: Folder, tree: Map<string, Folder[]>, depth: number): HTMLElement {
  const expanded = state.get('expandedFolders').has(folder.id)
  const currentNote = state.get('currentNote')
  const notesInFolder = state.get('notes').filter(n => n.folderId === folder.id)

  const hasActiveNote = currentNote && currentNote.folderId === folder.id

  const wrapper = el('div', { class: 'folder-wrapper' })

  const row = el('div', {
    class: `sidebar-row${hasActiveNote ? ' active' : ''}`,
    style: `padding-left: ${10 + depth * 16}px;`,
  })

  row.innerHTML = `
    <span class="folder-chevron ${expanded ? 'expanded' : 'collapsed'}">
      ${icon('chevron-down', 14)}
    </span>
    <span class="folder-icon ${hasActiveNote ? 'active' : 'inactive'}">
      ${icon(expanded ? 'folder-open' : 'folder', 16)}
    </span>
    <span class="folder-name ${hasActiveNote ? 'active' : 'inactive'}">
      ${escapeHtml(folder.name)}
    </span>
  `

  row.addEventListener('click', () => {
    state.toggleFolder(folder.id)
    state.setState({ currentFolderId: folder.id })
    renderFolderList()
  })

  // Context menu: right-click on folder row
  row.addEventListener('contextmenu', (e) => {
    e.preventDefault()
    e.stopPropagation()
    showContextMenu(e.clientX, e.clientY, [
      {
        label: 'Rename',
        iconName: 'pencil',
        action: () => {
          state.setState({
            renameContext: {
              type: 'folder',
              id: folder.id,
              currentName: folder.name,
              parentId: folder.parentId ?? '',
            },
          })
          state.openModal('rename-folder')
        },
      },
      {
        label: 'Move',
        iconName: 'folder-input',
        action: () => {
          state.setState({
            moveContext: {
              type: 'folder',
              id: folder.id,
              name: folder.name,
              currentParentId: folder.parentId ?? '',
            },
          })
          state.openModal('move-folder')
        },
      },
      'separator',
      {
        label: 'Delete',
        iconName: 'trash-2',
        danger: true,
        action: () => {
          const hasNotes = state.get('notes').some(n => n.folderId === folder.id)
          state.setState({
            deleteContext: {
              type: 'folder',
              id: folder.id,
              name: folder.name,
              hasNotes,
            },
          })
          state.openModal('delete-folder')
        },
      },
    ])
  })

  // Drag: this folder can be dragged to reparent it
  setupDragSource(row, {
    kind: 'folder',
    id: folder.id,
    name: folder.name,
    currentParentId: folder.parentId ?? '',
  })

  // Drop: items can be dropped onto this folder's wrapper
  setupDropTarget(wrapper, async payload => {
    if (payload.kind === 'folder' && payload.id === folder.id) return
    await executeDrop(payload, folder.id)
  })

  wrapper.appendChild(row)

  if (expanded) {
    const subContainer = el('div', {
      style: `padding-left: ${28 + depth * 16}px;`,
    })

    for (const child of sortByName(tree.get(folder.id) ?? [], state.get('sortOrder'))) {
      subContainer.appendChild(renderFolderItem(child, tree, depth + 1))
    }

    for (const note of sortNotesByTitle(notesInFolder, state.get('sortOrder'))) {
      subContainer.appendChild(renderNoteInFolderItem(note))
    }

    wrapper.appendChild(subContainer)
  }

  return wrapper
}

function renderNoteInFolderItem(note: Note): HTMLElement {
  const currentNote = state.get('currentNote')
  const isActive = currentNote?.id === note.id

  const row = el('div', { class: `sidebar-row${isActive ? ' active' : ''}` })

  row.innerHTML = `
    <span class="note-folder-icon ${isActive ? 'active' : 'inactive'}">
      ${icon('file-text', 14)}
    </span>
    <span class="note-folder-name ${isActive ? 'active' : 'inactive'}">
      ${escapeHtml(note.title || 'Untitled')}
    </span>
  `

  row.addEventListener('click', () => handleNoteSelect(note))

  // Drag: note inside a folder can be dragged out or to another folder
  setupDragSource(row, {
    kind: 'note',
    id: note.id,
    currentFolderId: note.folderId ?? '',
  })

  // Context menu: right-click on note inside folder
  row.addEventListener('contextmenu', (e) => {
    e.preventDefault()
    e.stopPropagation()
    showContextMenu(e.clientX, e.clientY, [
      {
        label: 'Rename',
        iconName: 'pencil',
        action: () => {
          state.setState({
            renameContext: {
              type: 'note',
              id: note.id,
              currentName: note.title || 'Untitled',
            },
          })
          state.openModal('rename-note')
        },
      },
      {
        label: 'Move',
        iconName: 'folder-input',
        action: () => {
          state.setState({
            moveContext: {
              type: 'note',
              id: note.id,
              name: note.title || 'Untitled',
              currentFolderId: note.folderId ?? '',
            },
          })
          state.openModal('move-note')
        },
      },
      'separator',
      {
        label: 'Delete',
        iconName: 'trash-2',
        danger: true,
        action: () => {
          state.setState({
            deleteContext: {
              type: 'note',
              id: note.id,
              name: note.title || 'Untitled',
            },
          })
          state.openModal('delete-note')
        },
      },
    ])
  })

  return row
}

function renderFolderList(): void {
  const container = document.getElementById('sidebar-folders')
  if (!container) return
  container.innerHTML = ''

  const folders = state.get('folders')

  const tree = buildFolderTree(folders)
  const rootFolders = sortByName(tree.get('') ?? [], state.get('sortOrder'))
  for (const folder of rootFolders) {
    container.appendChild(renderFolderItem(folder, tree, 0))
  }

  // The folders section itself is a drop zone for moving items to root
  setupDropTarget(container as HTMLElement, async payload => {
    await executeDrop(payload, '')
  })
}

// ─── Note List ────────────────────────────────────────────────────────────────

function renderNoteList(): void {
  const container = document.getElementById('sidebar-notes')
  if (!container) return
  container.innerHTML = ''

  const notes = sortNotesByTitle(state.get('notes').filter(n => !n.folderId), state.get('sortOrder'))
  const currentNote = state.get('currentNote')

  for (const note of notes) {
    const isActive = currentNote?.id === note.id
    const row = el('div', { class: `note-row${isActive ? ' active' : ''}` })

    const updatedAt = note.updatedAt ? new Date(note.updatedAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : ''

    row.innerHTML = `
      <span class="note-row-icon ${isActive ? 'active' : 'inactive'}">
        ${icon('file-text', 14)}
      </span>
      <div class="note-row-body">
        <div class="note-row-title ${isActive ? 'active' : 'inactive'}">
          ${escapeHtml(note.title || 'Untitled')}
        </div>
        <div class="note-meta">
          ${updatedAt}
        </div>
      </div>
    `

    row.addEventListener('mouseenter', () => {
      row.querySelector('.note-meta')?.classList.add('visible')
    })
    row.addEventListener('mouseleave', () => {
      row.querySelector('.note-meta')?.classList.remove('visible')
    })
    row.addEventListener('click', () => handleNoteSelect(note))

    // Drag: top-level note can be dragged into a folder
    setupDragSource(row, {
      kind: 'note',
      id: note.id,
      currentFolderId: '',
    })

    // Context menu: right-click on top-level note
    row.addEventListener('contextmenu', (e) => {
      e.preventDefault()
      e.stopPropagation()
      showContextMenu(e.clientX, e.clientY, [
        {
          label: 'Rename',
          iconName: 'pencil',
          action: () => {
            state.setState({
              renameContext: {
                type: 'note',
                id: note.id,
                currentName: note.title || 'Untitled',
              },
            })
            state.openModal('rename-note')
          },
        },
        {
          label: 'Move',
          iconName: 'folder-input',
          action: () => {
            state.setState({
              moveContext: {
                type: 'note',
                id: note.id,
                name: note.title || 'Untitled',
                currentFolderId: '',
              },
            })
            state.openModal('move-note')
          },
        },
        'separator',
        {
          label: 'Delete',
          iconName: 'trash-2',
          danger: true,
          action: () => {
            state.setState({
              deleteContext: {
                type: 'note',
                id: note.id,
                name: note.title || 'Untitled',
              },
            })
            state.openModal('delete-note')
          },
        },
      ])
    })

    container.appendChild(row)
  }

  if (notes.length === 0 && state.get('folders').length === 0) {
    const empty = el('div', { class: 'notes-empty' })
    empty.textContent = 'No notes yet'
    container.appendChild(empty)
  }

  // The notes section itself is a drop zone for moving items to root
  setupDropTarget(container as HTMLElement, async payload => {
    await executeDrop(payload, '')
  })
}

// ─── Sidebar Footer ──────────────────────────────────────────────────────────

function renderFooter(): void {
  const container = document.getElementById('sidebar-footer')
  if (!container) return
  // Layout defined in #sidebar-footer CSS class

  const settingsBtn = el('button', { id: 'settings-btn', class: 'settings-btn' })
  settingsBtn.innerHTML = `${icon('settings', 18)} Settings`
  settingsBtn.addEventListener('click', () => state.openModal('settings'))

  const themeContainer = document.createElement('div')
  renderThemeToggle(themeContainer)

  container.appendChild(settingsBtn)
  container.appendChild(themeContainer)
  // Re-render theme toggle when theme changes
  state.subscribe('theme', () => renderThemeToggle(themeContainer))
}

// ─── Event Handlers ──────────────────────────────────────────────────────────

async function handleNoteSelect(note: Note): Promise<void> {
  try {
    const fullNote = await NotesAPI.get(note.id)
    state.setState({
      currentNote: fullNote,
      isDirty: false,
      isPreviewMode: false,
      showAIPanel: false,
      streamingContent: '',
    })
  } catch (err) {
    console.error('Failed to load note:', err)
    state.showNotification('Failed to load note', 'error')
  }
}

async function handleNewNote(): Promise<void> {
  try {
    const folderId = state.get('currentFolderId') ?? ''
    const note = await NotesAPI.create('Untitled', '', folderId)
    const notes = await NotesAPI.getAll()
    state.setState({ notes, currentNote: note })
    // Expand folder if note was created inside one
    if (folderId) state.get('expandedFolders').add(folderId)
  } catch (err) {
    console.error('Failed to create note:', err)
    state.showNotification('Failed to create note', 'error')
  }
}

function handleNewFolder(): void {
  state.openModal('create-folder')
}

// ─── Utility — escapeHtml imported from utils/html.ts ───────────────────────

// ─── Public API ──────────────────────────────────────────────────────────────

export function initSidebar(): void {
  renderHeader()
  renderSearch()
  renderActions()
  renderFolderList()
  renderNoteList()
  renderFooter()

  // Sidebar layout defined in #sidebar CSS class (app.css)

  // Subscribe to state changes
  state.subscribe('notes', () => {
    renderNoteList()
    renderFolderList()
  })
  state.subscribe('folders', () => renderFolderList())
  state.subscribe('sortOrder', () => {
    renderFolderList()
    renderNoteList()
    renderActions()
  })
  state.subscribe('currentNote', () => {
    renderNoteList()
    renderFolderList()
  })
  state.subscribe('expandedFolders', () => renderFolderList())
}
