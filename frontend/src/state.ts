/**
 * Global application state with a simple pub/sub mechanism.
 * No external libraries — just a typed singleton with subscriber callbacks.
 */

import type {
  Note,
  Folder,
  Prompt,
  Config,
  Theme,
  ModalType,
  RecordingState,
  StreamingState,
  DeleteContext,
  CreateFolderContext,
  RenameContext,
  MoveContext,
} from './types'

export interface AppState {
  // Data
  notes: Note[]
  folders: Folder[]
  prompts: Prompt[]
  config: Config | null

  // Editor
  currentNote: Note | null
  currentFolderId: string | null
  isDirty: boolean
  isSaving: boolean
  lastSaved: Date | null
  isPreviewMode: boolean

  // Sidebar
  expandedFolders: Set<string>

  // Recording
  isRecording: boolean
  recordingDuration: number
  recordingSource: string
  partialTranscript: string

  // AI / Streaming
  isStreaming: boolean
  streamingContent: string
  selectedPromptId: string | null
  showAIPanel: boolean

  // Availability
  sttAvailable: boolean
  llmAvailable: boolean

  // Theme
  theme: Theme
  sortOrder: 'asc' | 'desc'

  // Modals
  activeModal: ModalType
  deleteContext: DeleteContext | null
  createFolderContext: CreateFolderContext | null
  renameContext: RenameContext | null
  moveContext: MoveContext | null

  // Notifications
  notification: { message: string; type: 'info' | 'success' | 'error' } | null
}

type Listener = () => void
type Subscribers = Map<keyof AppState | '__any__', Set<Listener>>

class StateManager {
  private _state: AppState = {
    notes: [],
    folders: [],
    prompts: [],
    config: null,

    currentNote: null,
    currentFolderId: null,
    isDirty: false,
    isSaving: false,
    lastSaved: null,
    isPreviewMode: false,

    expandedFolders: new Set(),

    isRecording: false,
    recordingDuration: 0,
    recordingSource: 'microphone',
    partialTranscript: '',

    isStreaming: false,
    streamingContent: '',
    selectedPromptId: null,
    showAIPanel: false,

    sttAvailable: false,
    llmAvailable: false,

    theme: 'light',
    sortOrder: 'asc',

    activeModal: null,
    deleteContext: null,
    createFolderContext: null,
    renameContext: null,
    moveContext: null,

    notification: null,
  }

  private _subscribers: Subscribers = new Map()

  get<K extends keyof AppState>(key: K): AppState[K] {
    return this._state[key]
  }

  getAll(): Readonly<AppState> {
    return this._state
  }

  setState(partial: Partial<AppState>): void {
    const changedKeys = Object.keys(partial) as Array<keyof AppState>
    Object.assign(this._state, partial)

    // Notify key-specific subscribers
    for (const key of changedKeys) {
      const subs = this._subscribers.get(key)
      if (subs) {
        for (const cb of subs) cb()
      }
    }

    // Notify wildcard subscribers
    const anySubs = this._subscribers.get('__any__')
    if (anySubs) {
      for (const cb of anySubs) cb()
    }
  }

  subscribe(key: keyof AppState | '__any__', cb: Listener): () => void {
    if (!this._subscribers.has(key)) {
      this._subscribers.set(key, new Set())
    }
    this._subscribers.get(key)!.add(cb)

    // Return unsubscribe function
    return () => {
      this._subscribers.get(key)?.delete(cb)
    }
  }

  // ─── Convenience helpers ─────────────────────────────────────────────────

  openModal(modal: ModalType): void {
    this.setState({ activeModal: modal })
  }

  closeModal(): void {
    this.setState({
      activeModal: null,
      deleteContext: null,
      createFolderContext: null,
      renameContext: null,
      moveContext: null,
    })
  }

  showNotification(message: string, type: 'info' | 'success' | 'error' = 'info', durationMs = 3000): void {
    this.setState({ notification: { message, type } })
    setTimeout(() => {
      this.setState({ notification: null })
    }, durationMs)
  }

  toggleFolder(folderId: string): void {
    const expanded = new Set(this._state.expandedFolders)
    if (expanded.has(folderId)) {
      expanded.delete(folderId)
    } else {
      expanded.add(folderId)
    }
    this.setState({ expandedFolders: expanded })
  }
}

// Export singleton
export const state = new StateManager()
export default state
