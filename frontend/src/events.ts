/**
 * Typed event subscription wrapper for all Go→Frontend Wails events.
 * Centralizes all EventsOn/EventsOff calls.
 */

import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import type { StreamChunkPayload, LLMReadyPayload, TranscriptionResult, DownloadEventPayload } from './types'

// Track active listeners for cleanup
const activeListeners: string[] = []

function on<T>(event: string, callback: (data: T) => void): void {
  EventsOn(event, callback)
  if (!activeListeners.includes(event)) {
    activeListeners.push(event)
  }
}

export const AppEvents = {
  // ─── LLM Streaming ──────────────────────────────────────────────────────
  onStreamChunk(cb: (chunk: StreamChunkPayload) => void): void {
    on<StreamChunkPayload>('llm:stream:chunk', cb)
  },

  onStreamDone(cb: (chunk: StreamChunkPayload) => void): void {
    on<StreamChunkPayload>('llm:stream:done', cb)
  },

  onStreamError(cb: (error: string) => void): void {
    on<string>('llm:stream:error', cb)
  },

  onLLMReady(cb: (payload: LLMReadyPayload) => void): void {
    on<LLMReadyPayload>('llm:ready', cb)
  },

  // ─── STT / Transcription ────────────────────────────────────────────────
  onSTTReady(cb: () => void): void {
    EventsOn('stt:ready', cb)
    if (!activeListeners.includes('stt:ready')) activeListeners.push('stt:ready')
  },

  onTranscriptionPartial(cb: (result: TranscriptionResult) => void): void {
    on<TranscriptionResult>('transcription:partial', cb)
  },

  onTranscriptionDone(cb: (result: TranscriptionResult) => void): void {
    on<TranscriptionResult>('transcription:done', cb)
  },

  // ─── Downloads ────────────────────────────────────────────────────────────
  onDownloadEvent(cb: (payload: DownloadEventPayload) => void): void {
    on<DownloadEventPayload>('download:event', cb)
  },

  // ─── Config ──────────────────────────────────────────────────────────────
  onConfigSaved(cb: () => void): void {
    EventsOn('config:saved', cb)
    if (!activeListeners.includes('config:saved')) activeListeners.push('config:saved')
  },

  // ─── Native Menu ─────────────────────────────────────────────────────────
  onMenuSettings(cb: () => void): void {
    EventsOn('menu:settings', cb)
    if (!activeListeners.includes('menu:settings')) activeListeners.push('menu:settings')
  },

  // ─── Cleanup ─────────────────────────────────────────────────────────────
  removeAll(): void {
    for (const event of activeListeners) {
      EventsOff(event)
    }
    activeListeners.length = 0
  },
}
