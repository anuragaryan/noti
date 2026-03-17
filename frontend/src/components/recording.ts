/**
 * Voice recording component.
 * Manages: record button state (in editor header), recording bar (bottom of main).
 */

import state from '../state'
import { AudioAPI, NotesAPI } from '../api'
import { AppEvents } from '../events'

// ─── Timer ───────────────────────────────────────────────────────────────────

let timerInterval: ReturnType<typeof setInterval> | null = null

function startTimer(): void {
  state.setState({ recordingDuration: 0 })
  timerInterval = setInterval(() => {
    state.setState({ recordingDuration: state.get('recordingDuration') + 1 })
    updateTimerDisplay()
  }, 1000)
}

function stopTimer(): void {
  if (timerInterval) {
    clearInterval(timerInterval)
    timerInterval = null
  }
}

function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60).toString().padStart(2, '0')
  const s = (seconds % 60).toString().padStart(2, '0')
  return `${m}:${s}`
}

function updateTimerDisplay(): void {
  const el = document.getElementById('recording-timer')
  if (el) el.textContent = formatDuration(state.get('recordingDuration'))
}

// ─── Live transcript state ───────────────────────────────────────────────────

let preRecordingTranscript = ''

function extractErrorMessage(err: unknown): string {
  if (typeof err === 'string') return err
  if (err instanceof Error && err.message) return err.message
  if (err && typeof err === 'object' && 'message' in err) {
    const msg = (err as { message?: unknown }).message
    if (typeof msg === 'string' && msg.trim()) return msg
  }
  return 'Failed to start recording'
}

function accessDeniedMessage(source: string): string {
  if (source === 'system') {
    return 'Screen Recording permission denied. Please allow in System Settings.'
  }
  if (source === 'mixed') {
    return 'Microphone and Screen Recording permissions are required for mixed capture. Please allow in System Settings.'
  }
  return 'Microphone permission denied. Please allow in System Settings.'
}

function looksLikePermissionError(message: string): boolean {
  const normalized = message.toLowerCase()
  return normalized.includes('permission') ||
    normalized.includes('not granted') ||
    normalized.includes('denied') ||
    normalized.includes('screen recording') ||
    normalized.includes('microphone')
}

function mergedTranscript(base: string, live: string): string {
  const trimmedLive = live.trim()
  if (!trimmedLive) return base
  const separator = base.trim() ? '\n\n' : ''
  return `${base}${separator}${trimmedLive}`
}

// ─── Recording Bar ────────────────────────────────────────────────────────────

function renderRecordingBar(): void {
  const bar = document.getElementById('recording-bar')
  if (!bar) return

  const isRecording = state.get('isRecording')

  if (!isRecording) {
    bar.classList.add('hidden')
    return
  }

  bar.classList.remove('hidden')

  const source = state.get('recordingSource') || 'microphone'
  const sourceLabel = source === 'microphone' ? 'Microphone' : source === 'system' ? 'System Audio' : 'Mixed'

  // Waveform bar heights defined via CSS nth-child rules in app.css
  const waveformBars = Array.from({ length: 8 }, () =>`<div class="waveform-bar"></div>`).join('')

  bar.innerHTML = `
    <div class="recording-dot pulse-dot"></div>
    <span class="recording-label">Recording…</span>
    <span id="recording-timer" class="recording-timer">
      ${formatDuration(state.get('recordingDuration'))}
    </span>
    <div class="recording-spacer"></div>
    <div class="recording-waveform">
      ${waveformBars}
    </div>
    <div class="recording-spacer"></div>
    <span class="recording-source">Source: ${sourceLabel}</span>
  `
}

// ─── Start / Stop ─────────────────────────────────────────────────────────────

export async function startRecording(): Promise<void> {
  const source = state.get('recordingSource') || 'microphone'

  try {
    const initialPerms = await AudioAPI.checkPermissions(source)
    let permissionStatus = String(initialPerms.status ?? 'unknown')

    if (permissionStatus !== 'granted') {
      try {
        await AudioAPI.requestPermissions(source)
      } catch {
        // Ignore and rely on a follow-up permission check for final status.
      }

      const updatedPerms = await AudioAPI.checkPermissions(source)
      permissionStatus = String(updatedPerms.status ?? 'unknown')
    }

    if (permissionStatus !== 'granted') {
      state.showNotification(accessDeniedMessage(source), 'error')
      state.openModal('settings')
      return
    }

    if (!state.get('sttAvailable')) {
      state.showNotification('STT model not available. Please configure in Settings.', 'error')
      return
    }

    const note = state.get('currentNote')
    preRecordingTranscript = note?.transcriptContent ?? ''

    if (note && !note.transcriptActivated) {
      try {
        await NotesAPI.markTranscriptActivated(note.id)
      } catch (err) {
        console.error('Failed to persist transcript activation:', err)
      }

      const notes = state.get('notes').map((n) =>
        n.id === note.id ? { ...n, transcriptActivated: true } : n,
      ) as any
      state.setState({
        notes,
        currentNote: {
          ...note,
          transcriptActivated: true,
        } as any,
      })
    }

    await AudioAPI.startRecordingWithSource(source)
    state.setState({ isRecording: true, partialTranscript: '' })
    startTimer()
    renderRecordingBar()
  } catch (err) {
    console.error('Failed to start recording:', err)
    const message = extractErrorMessage(err)
    state.showNotification(message, 'error')
    if (looksLikePermissionError(message)) {
      state.openModal('settings')
    }
  }
}

export async function stopRecording(): Promise<void> {
  try {
    // Stop recording - returns immediately, transcription happens async
    await AudioAPI.stopRecording()
    
    // Update UI immediately - no more lag!
    stopTimer()
    state.setState({ isRecording: false, partialTranscript: '' })
    renderRecordingBar()
    
    // Clear the partial transcript display but keep preRecordingContent
    // The actual final text will come via transcription:done event
  } catch (err) {
    console.error('Failed to stop recording:', err)
    stopTimer()
    state.setState({ isRecording: false })
    renderRecordingBar()
    state.showNotification('Recording stopped with error', 'error')
  }
}

async function persistTranscript(liveTranscript: string): Promise<void> {
  const note = state.get('currentNote')
  if (!note) return

  const nextTranscript = mergedTranscript(preRecordingTranscript, liveTranscript)
  const titleInput = document.querySelector<HTMLInputElement>('#note-title-input')
  const markdownInput = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
  const title = titleInput?.value ?? note.title ?? ''
  const markdownContent = markdownInput?.value ?? note.markdownContent ?? ''

  await NotesAPI.update(note.id, title, markdownContent, nextTranscript)

  const notes = state.get('notes').map((n) =>
    n.id === note.id
      ? { ...n, title, markdownContent, transcriptContent: nextTranscript }
      : n
  ) as any

  state.setState({
    notes,
    currentNote: {
      ...note,
      title,
      markdownContent,
      transcriptContent: nextTranscript,
    } as any,
  })
}

// ─── Public API ──────────────────────────────────────────────────────────────

export function initRecording(): void {
  renderRecordingBar()

  state.subscribe('isRecording', () => {
    renderRecordingBar()
    if (!state.get('isRecording')) stopTimer()
  })

  // Each partial event now carries the full running transcript for this session.
  // Write it directly into the textarea so the user sees words appear live.
  AppEvents.onTranscriptionPartial((result) => {
    if (result.isPartial && state.get('isRecording')) {
      state.setState({ partialTranscript: result.text })
    }
  })

  // Handle final transcription result from async background processing
  AppEvents.onTranscriptionDone((result) => {
    if (!result.text) {
      return
    }

    void persistTranscript(result.text)
      .then(() => {
        state.showNotification('Transcription complete', 'success')
      })
      .catch((err) => {
        console.error('Failed to persist transcript:', err)
        state.showNotification('Failed to save transcript', 'error')
      })
  })

  // Wire the record button in editor header via event delegation.
  document.getElementById('editor-header')?.addEventListener('click', (e) => {
    const target = e.target as HTMLElement
    if (target.closest('#record-btn')) {
      if (state.get('isRecording')) {
        void stopRecording()
      } else {
        void startRecording()
      }
      e.stopPropagation()
    }
  })
}
