// Re-export domain types from Wails auto-generated models
export type { domain } from '../wailsjs/go/models'
import type { domain } from '../wailsjs/go/models'

export type Note = domain.Note
export type Folder = domain.Folder
export type Prompt = domain.Prompt
export type Config = domain.Config
export type LLMConfig = domain.LLMConfig
export type AudioSettings = domain.AudioSettings
export type AudioMixerConfig = domain.AudioMixerConfig
export type AudioDevice = domain.AudioDevice
export type LLMResponse = domain.LLMResponse
export type PromptExecutionResult = domain.PromptExecutionResult
export type TranscriptionResult = domain.TranscriptionResult
export type SearchMatch = domain.SearchMatch

// UI-specific types
export type Theme = 'light' | 'dark'

export type ModalType = 'settings' | 'prompts' | 'delete-note' | 'delete-folder' | 'create-folder' | 'rename-note' | 'rename-folder' | 'move-note' | 'move-folder' | 'downloads' | null

export interface SidebarState {
  expandedFolders: Set<string>
  selectedNoteId: string | null
  selectedFolderId: string | null
}

export interface EditorState {
  isDirty: boolean
  isSaving: boolean
  lastSaved: Date | null
  isPreviewMode: boolean
}

export interface RecordingState {
  isRecording: boolean
  duration: number
  source: string
  partialTranscript: string
}

export interface StreamingState {
  isStreaming: boolean
  content: string
  selectedPromptId: string | null
}

// Event payload types
export interface StreamChunkPayload {
  text: string
  index: number
  done: boolean
  finishReason?: string
}

export interface LLMReadyPayload {
  provider: string
  modelName: string
  model?: string
}

export type DownloadKind = 'stt-model' | 'llm-model' | 'llama-server'
export type DownloadStatus = 'queued' | 'downloading' | 'completed' | 'error'

export interface DownloadEventPayload {
  id: string
  kind: DownloadKind
  label: string
  status: DownloadStatus
  bytesDownloaded?: number
  totalBytes?: number
  percent?: number
  error?: string
  timestamp?: string
}

export interface DownloadItem extends DownloadEventPayload {
  percent: number
  bytesDownloaded: number
  totalBytes: number
  completedInSession: boolean
  createdAt: string
}

// Delete confirmation context
export interface DeleteContext {
  type: 'note' | 'folder'
  id: string
  name: string
  hasNotes?: boolean // for folders
}

// Create folder context
export interface CreateFolderContext {
  parentId?: string
}

// Rename context
export interface RenameContext {
  type: 'note' | 'folder'
  id: string
  currentName: string
  /** Only present for folders — needed to preserve parentID on rename */
  parentId?: string
}

// Move context
export interface MoveContext {
  type: 'note' | 'folder'
  id: string
  name: string
  currentFolderId?: string   // for notes
  currentParentId?: string  // for folders
}
