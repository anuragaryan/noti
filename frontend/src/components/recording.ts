/**
 * Voice recording component.
 * Manages: record button state (in editor header), recording bar (bottom of main).
 */

import state from '../state'
import { AudioAPI } from '../api'
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
  // Layout defined in #recording-bar CSS class (app.css)

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
    <button id="stop-recording-btn" class="stop-recording-btn">■ Stop</button>
  `

  bar.querySelector('#stop-recording-btn')?.addEventListener('click', () => {
    void stopRecording()
  })
}

// ─── Start / Stop ─────────────────────────────────────────────────────────────

export async function startRecording(): Promise<void> {
  const source = state.get('recordingSource') || 'microphone'

  try {
    // Check permissions first
    const perms = await AudioAPI.checkPermissions(source)
    if (perms.status === 'denied') {
      state.showNotification('Microphone permission denied. Please allow in System Settings.', 'error')
      return
    }

    if (!state.get('sttAvailable')) {
      state.showNotification('STT model not available. Please configure in Settings.', 'error')
      return
    }

    await AudioAPI.startRecordingWithSource(source)
    state.setState({ isRecording: true, partialTranscript: '' })
    startTimer()
    renderRecordingBar()
  } catch (err) {
    console.error('Failed to start recording:', err)
    state.showNotification('Failed to start recording', 'error')
  }
}

export async function stopRecording(): Promise<void> {
  try {
    const result = await AudioAPI.stopRecording()
    stopTimer()
    state.setState({ isRecording: false, partialTranscript: '' })
    renderRecordingBar()

    if (result?.text) {
      // Append transcription to current note content
      const textarea = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
      if (textarea) {
        const separator = textarea.value ? '\n\n' : ''
        textarea.value = textarea.value + separator + result.text
        textarea.dispatchEvent(new Event('input'))
      }
      state.showNotification('Transcription complete', 'success')
    }
  } catch (err) {
    console.error('Failed to stop recording:', err)
    stopTimer()
    state.setState({ isRecording: false })
    renderRecordingBar()
    state.showNotification('Recording stopped with error', 'error')
  }
}

// ─── Public API ──────────────────────────────────────────────────────────────

export function initRecording(): void {
  renderRecordingBar()

  // Subscribe to state changes
  state.subscribe('isRecording', () => {
    renderRecordingBar()
    if (!state.get('isRecording')) stopTimer()
  })

  // Wire transcription partial events
  AppEvents.onTranscriptionPartial((result) => {
    if (result.isPartial) {
      state.setState({ partialTranscript: result.text })
    }
  })

  // Wire the record button in editor header
  // The editor header renders a #record-btn — we intercept it via event delegation
  document.getElementById('editor-header')?.addEventListener('click', (e) => {
    const target = e.target as HTMLElement
    if (target.closest('#record-btn')) {
      if (state.get('isRecording')) {
        void stopRecording()
      } else {
        void startRecording()
      }
      // Prevent state.setState from doubling up — recording.ts owns this
      e.stopPropagation()
    }
  })
}
