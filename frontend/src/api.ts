/**
 * Typed wrapper around Wails auto-generated Go bindings.
 * All methods in wailsjs/go/main/App.js are available via window.go at runtime.
 * We import the TypeScript declarations here for type safety.
 */

import * as App from '../wailsjs/go/main/App'
import type { Note, Folder, Prompt, Config, LLMConfig, AudioDevice, LLMResponse, PromptExecutionResult, TranscriptionResult } from './types'

// ─── Notes API ──────────────────────────────────────────────────────────────

export const NotesAPI = {
  getAll(): Promise<Note[]> {
    return App.GetAllNotes()
  },

  get(id: string): Promise<Note> {
    return App.GetNote(id)
  },

  create(title: string, content: string, folderId: string = ''): Promise<Note> {
    return App.CreateNote(title, content, folderId)
  },

  update(id: string, title: string, content: string): Promise<void> {
    return App.UpdateNote(id, title, content)
  },

  delete(id: string): Promise<void> {
    return App.DeleteNote(id)
  },

  move(noteId: string, targetFolderId: string): Promise<void> {
    return App.MoveNote(noteId, targetFolderId)
  },
}

// ─── Folders API ─────────────────────────────────────────────────────────────

export const FoldersAPI = {
  getAll(): Promise<Folder[]> {
    return App.GetAllFolders()
  },

  getPath(folderId: string): Promise<Folder[]> {
    return App.GetFolderPath(folderId)
  },

  create(name: string, parentId: string = ''): Promise<Folder> {
    return App.CreateFolder(name, parentId)
  },

  update(id: string, name: string, parentId: string = ''): Promise<void> {
    return App.UpdateFolder(id, name, parentId)
  },

  delete(id: string, deleteNotes: boolean): Promise<void> {
    return App.DeleteFolder(id, deleteNotes)
  },
}

// ─── Audio / STT API ─────────────────────────────────────────────────────────

export const AudioAPI = {
  getSTTStatus(): Promise<Record<string, unknown>> {
    return App.GetSTTStatus()
  },

  startRecording(): Promise<void> {
    return App.StartVoiceRecording()
  },

  startRecordingWithSource(source: string): Promise<void> {
    return App.StartVoiceRecordingWithSource(source)
  },

  stopRecording(): Promise<TranscriptionResult> {
    return App.StopVoiceRecording()
  },

  isRecording(): Promise<boolean> {
    return App.IsRecording()
  },

  getSources(): Promise<Array<Record<string, unknown>>> {
    return App.GetAudioSources()
  },

  getDevices(): Promise<AudioDevice[]> {
    return App.GetAudioDevices()
  },

  getCurrentSource(): Promise<string> {
    return App.GetCurrentAudioSource()
  },

  setSource(source: string): Promise<void> {
    return App.SetAudioSource(source)
  },

  checkPermissions(source: string): Promise<Record<string, unknown>> {
    return App.CheckAudioPermissions(source)
  },

  requestPermissions(source: string): Promise<void> {
    return App.RequestAudioPermissions(source)
  },

  getMixerConfig(): Promise<Record<string, unknown>> {
    return App.GetMixerConfig()
  },

  setMixerConfig(micGain: number, sysGain: number, mixMode: string): Promise<void> {
    return App.SetMixerConfig(micGain, sysGain, mixMode)
  },

  getStatus(): Promise<Record<string, unknown>> {
    return App.GetAudioStatus()
  },
}

// ─── LLM API ─────────────────────────────────────────────────────────────────

export const LLMAPI = {
  getStatus(): Promise<Record<string, unknown>> {
    return App.GetLLMStatus()
  },

  getStreamingSupport(): Promise<boolean> {
    return App.GetStreamingSupport()
  },

  generate(prompt: string, systemPrompt: string): Promise<LLMResponse> {
    return App.GenerateText(prompt, systemPrompt)
  },

  generateWithOptions(prompt: string, systemPrompt: string, temperature: number, maxTokens: number): Promise<LLMResponse> {
    return App.GenerateTextWithOptions(prompt, systemPrompt, temperature, maxTokens)
  },

  generateStream(prompt: string, systemPrompt: string): Promise<void> {
    return App.GenerateTextStream(prompt, systemPrompt)
  },

  generateStreamWithOptions(prompt: string, systemPrompt: string, temperature: number, maxTokens: number): Promise<void> {
    return App.GenerateTextStreamWithOptions(prompt, systemPrompt, temperature, maxTokens)
  },

  updateConfig(config: LLMConfig): Promise<void> {
    return App.UpdateLLMConfig(config)
  },
}

// ─── Prompts API ─────────────────────────────────────────────────────────────

export const PromptsAPI = {
  getAll(): Promise<Prompt[]> {
    return App.GetAllPrompts()
  },

  get(id: string): Promise<Prompt> {
    return App.GetPrompt(id)
  },

  create(
    name: string,
    description: string,
    systemPrompt: string,
    userPrompt: string,
    temperature: number,
    maxTokens: number,
  ): Promise<Prompt> {
    return App.CreatePrompt(name, description, systemPrompt, userPrompt, temperature, maxTokens)
  },

  update(
    id: string,
    name: string,
    description: string,
    systemPrompt: string,
    userPrompt: string,
    temperature: number,
    maxTokens: number,
  ): Promise<void> {
    return App.UpdatePrompt(id, name, description, systemPrompt, userPrompt, temperature, maxTokens)
  },

  delete(id: string): Promise<void> {
    return App.DeletePrompt(id)
  },

  executeOnNote(promptId: string, noteId: string): Promise<PromptExecutionResult> {
    return App.ExecutePromptOnNote(promptId, noteId)
  },

  executeOnNoteStream(promptId: string, noteId: string): Promise<void> {
    return App.ExecutePromptOnNoteStream(promptId, noteId)
  },

  executeOnContent(promptId: string, content: string): Promise<PromptExecutionResult> {
    return App.ExecutePromptOnContent(promptId, content)
  },

  executeOnContentStream(promptId: string, content: string): Promise<void> {
    return App.ExecutePromptOnContentStream(promptId, content)
  },
}

// ─── Config API ──────────────────────────────────────────────────────────────

export const ConfigAPI = {
  get(): Promise<Config> {
    return App.GetConfig()
  },

  save(config: Config): Promise<void> {
    return App.SaveConfig(config)
  },
}
