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
	DownloadEventPayload,
	DownloadItem,
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
  aiMode: 'preset' | 'custom'
  customPromptText: string

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
		downloads: DownloadItem[]
		downloadsModalDismissed: boolean
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
    aiMode: 'preset',
    customPromptText: '',

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
			downloads: [],
			downloadsModalDismissed: false,
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
		const partial: Partial<AppState> = { activeModal: modal }
		if (modal === 'downloads') {
			partial.downloadsModalDismissed = false
		}
		this.setState(partial)
	}

	closeModal(): void {
		const closingDownloads = this._state.activeModal === 'downloads'
		const hasActiveDownloads = this._state.downloads.some((d) => d.status === 'queued' || d.status === 'downloading')
		const partial: Partial<AppState> = {
			activeModal: null,
			deleteContext: null,
			createFolderContext: null,
			renameContext: null,
			moveContext: null,
		}
		if (closingDownloads && hasActiveDownloads) {
			partial.downloadsModalDismissed = true
		}
		this.setState(partial)
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

	upsertDownload(event: DownloadEventPayload): void {
		const downloads = [...this._state.downloads]
		const idx = downloads.findIndex((d) => d.id === event.id)
		const base: DownloadItem = idx >= 0 ? { ...downloads[idx] } : {
			id: event.id,
			kind: event.kind,
			label: event.label,
			status: event.status,
			bytesDownloaded: 0,
			totalBytes: 0,
			percent: 0,
			error: event.error,
			completedInSession: false,
			createdAt: new Date().toISOString(),
		}

		base.kind = event.kind
		base.label = event.label
		base.status = event.status
		if (event.status === 'queued') {
			base.bytesDownloaded = 0
			base.totalBytes = 0
			base.percent = 0
			base.error = undefined
		}
		base.bytesDownloaded = typeof event.bytesDownloaded === 'number' && event.bytesDownloaded >= 0 ? event.bytesDownloaded : base.bytesDownloaded
		base.totalBytes = typeof event.totalBytes === 'number' && event.totalBytes >= 0 ? event.totalBytes : base.totalBytes
		const percent = typeof event.percent === 'number' ? event.percent : base.percent
		base.percent = Math.min(100, Math.max(0, percent || 0))
		base.error = event.error
		if (event.timestamp) {
			base.createdAt = event.timestamp
		}
		if (event.status === 'completed' || event.status === 'error') {
			base.completedInSession = true
		}

		if (idx >= 0) {
			downloads[idx] = base
		} else {
			downloads.push(base)
		}

		const hasActive = downloads.some((d) => d.status === 'queued' || d.status === 'downloading')
		const partial: Partial<AppState> = { downloads }
		if (!hasActive && this._state.downloadsModalDismissed) {
			partial.downloadsModalDismissed = false
		}

		this.setState(partial)
	}
}

// Export singleton
export const state = new StateManager()
export default state
