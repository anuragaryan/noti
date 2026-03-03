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

// ─── Live transcript in textarea ─────────────────────────────────────────────

// Content that existed in the textarea before recording started.
// The live transcript is appended after this base, separated by '\n\n'.
let preRecordingContent = ''

// Write the running transcript into the textarea, always replacing the
// previous partial so the text doesn't duplicate.
function setLiveTranscript(text: string): void {
  const textarea = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
  if (!textarea) return
  const separator = preRecordingContent ? '\n\n' : ''
  textarea.value = preRecordingContent + separator + text
  // Trigger auto-save debounce
  textarea.dispatchEvent(new Event('input'))
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
    const perms = await AudioAPI.checkPermissions(source)
    if (perms.status === 'denied') {
      state.showNotification('Microphone permission denied. Please allow in System Settings.', 'error')
      return
    }

    if (!state.get('sttAvailable')) {
      state.showNotification('STT model not available. Please configure in Settings.', 'error')
      return
    }

    // Snapshot what's already in the editor so we can append without clobbering it.
    const textarea = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
    preRecordingContent = textarea?.value ?? ''

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
      // Replace the live-preview text with the final confirmed transcript.
      setLiveTranscript(result.text)
      state.showNotification('Transcription complete', 'success')
    } else {
      // Nothing transcribed — restore pre-recording content as-is.
      const textarea = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
      if (textarea) textarea.value = preRecordingContent
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

  state.subscribe('isRecording', () => {
    renderRecordingBar()
    if (!state.get('isRecording')) stopTimer()
  })

  // Each partial event now carries the full running transcript for this session.
  // Write it directly into the textarea so the user sees words appear live.
  AppEvents.onTranscriptionPartial((result) => {
    if (result.isPartial && state.get('isRecording')) {
      state.setState({ partialTranscript: result.text })
      setLiveTranscript(result.text)
    }
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
