import { beforeEach, describe, expect, it, vi } from 'vitest'
import state from '../state'

const mockAudioAPI = {
  checkPermissions: vi.fn(),
  requestPermissions: vi.fn(),
  startRecordingWithSource: vi.fn(),
  stopRecording: vi.fn(),
}

const recordingListeners = {
  partial: [] as Array<(payload: { isPartial: boolean; text: string }) => void>,
  done: [] as Array<(payload: { text: string }) => void>,
}

vi.mock('../api', () => ({
  AudioAPI: mockAudioAPI,
}))

vi.mock('../events', () => ({
  AppEvents: {
    onTranscriptionPartial: (cb: (payload: { isPartial: boolean; text: string }) => void) => {
      recordingListeners.partial.push(cb)
    },
    onTranscriptionDone: (cb: (payload: { text: string }) => void) => {
      recordingListeners.done.push(cb)
    },
  },
}))

function renderDOM(): void {
  document.body.innerHTML = `
    <div id="editor-header"><button id="record-btn">Record</button></div>
    <div id="recording-bar" class="hidden"></div>
    <textarea id="note-content-textarea"></textarea>
  `
}

function resetState(): void {
  state.setState({
    isRecording: false,
    recordingDuration: 0,
    recordingSource: 'microphone',
    partialTranscript: '',
    sttAvailable: true,
    notification: null,
  })
}

describe('recording integration', () => {
  beforeEach(async () => {
    renderDOM()
    resetState()
    recordingListeners.partial = []
    recordingListeners.done = []
    mockAudioAPI.checkPermissions.mockReset()
    mockAudioAPI.requestPermissions.mockReset()
    mockAudioAPI.startRecordingWithSource.mockReset()
    mockAudioAPI.stopRecording.mockReset()
  })

  it('starts recording and updates live transcript lifecycle', async () => {
    const { initRecording, startRecording, stopRecording } = await import('./recording')

    mockAudioAPI.checkPermissions.mockResolvedValue({ status: 'granted' })
    mockAudioAPI.startRecordingWithSource.mockResolvedValue(undefined)
    mockAudioAPI.stopRecording.mockResolvedValue({})

    const textarea = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
    if (!textarea) throw new Error('textarea missing')
    textarea.value = 'existing text'

    initRecording()

    await startRecording()

    expect(state.get('isRecording')).toBe(true)
    expect(mockAudioAPI.startRecordingWithSource).toHaveBeenCalledWith('microphone')

    recordingListeners.partial[0]({ isPartial: true, text: 'partial words' })
    expect(textarea.value).toContain('partial words')

    await stopRecording()
    expect(state.get('isRecording')).toBe(false)

    recordingListeners.done[0]({ text: '' })
    expect(textarea.value).toBe('existing text')
  })

  it('shows error and does not start when permission denied', async () => {
    const { startRecording } = await import('./recording')

    mockAudioAPI.checkPermissions.mockResolvedValue({ status: 'denied' })
    mockAudioAPI.requestPermissions.mockResolvedValue(undefined)

    await startRecording()

    expect(state.get('isRecording')).toBe(false)
    expect(mockAudioAPI.startRecordingWithSource).not.toHaveBeenCalled()
    expect(state.get('notification')?.type).toBe('error')
  })
})
