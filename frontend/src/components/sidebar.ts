/**
 * Sidebar component — renders folders, notes, search, action bar, and footer.
 * Injects into: #sidebar-header, #sidebar-search, #sidebar-actions,
 *               #sidebar-folders, #sidebar-notes, #sidebar-footer
 */

import { renderThemeToggle } from './theme-toggle'
import { escapeHtml } from '../utils/html'
import { icon } from '../utils/icons'
import state from '../state'
import type { Folder, Note } from '../types'
import { NotesAPI, FoldersAPI } from '../api'

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
  // Layout defined in #sidebar-actions CSS class

  const makeActionBtn = (iconName: string, title: string, onClick: () => void) => {
    const btn = el('button', { title, class: 'action-btn' })
    btn.innerHTML = icon(iconName, 14)
    btn.addEventListener('click', onClick)
    return btn
  }

  container.append(
    makeActionBtn('file-plus', 'New Note', handleNewNote),
    makeActionBtn('folder-plus', 'New Folder', handleNewFolder),
    makeActionBtn('arrow-up-down', 'Sort', () => { /* TODO: implement sort */ }),
  )
}

// ─── Folder Tree ──────────────────────────────────────────────────────────────

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

function renderFolderItem(folder: Folder, tree: Map<string, Folder[]>, depth: number): HTMLElement {
  const expanded = state.get('expandedFolders').has(folder.id)
  const currentNote = state.get('currentNote')
  const notesInFolder = state.get('notes').filter(n => n.folderId === folder.id)

  // Check if a note in this folder is currently active
  const hasActiveNote = currentNote && currentNote.folderId === folder.id

  const wrapper = el('div', { class: 'folder-wrapper' })

  // Folder row — depth-based padding is dynamic, keep as inline style
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

  wrapper.appendChild(row)

  // Sub items (notes + child folders) when expanded
  if (expanded) {
    // Depth-based padding is dynamic, keep as inline style
    const subContainer = el('div', {
      style: `padding-left: ${28 + depth * 16}px;`,
    })

    // Child folders
    for (const child of tree.get(folder.id) ?? []) {
      subContainer.appendChild(renderFolderItem(child, tree, depth + 1))
    }

    // Notes in this folder
    for (const note of notesInFolder) {
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
  return row
}

function renderFolderList(): void {
  const container = document.getElementById('sidebar-folders')
  if (!container) return
  // Layout defined in #sidebar-folders CSS class
  container.innerHTML = ''

  const folders = state.get('folders')

  const tree = buildFolderTree(folders)
  const rootFolders = tree.get('') ?? []
  for (const folder of rootFolders) {
    container.appendChild(renderFolderItem(folder, tree, 0))
  }
}

// ─── Note List ────────────────────────────────────────────────────────────────

function renderNoteList(): void {
  const container = document.getElementById('sidebar-notes')
  if (!container) return
  // Layout defined in #sidebar-notes CSS class
  container.innerHTML = ''

  const notes = state.get('notes').filter(n => !n.folderId)
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

    container.appendChild(row)
  }

  if (notes.length === 0 && state.get('folders').length === 0) {
    const empty = el('div', { class: 'notes-empty' })
    empty.textContent = 'No notes yet'
    container.appendChild(empty)
  }
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
  state.subscribe('currentNote', () => {
    renderNoteList()
    renderFolderList()
  })
  state.subscribe('expandedFolders', () => renderFolderList())
}
