import { beforeEach, describe, expect, it, vi } from 'vitest'
import state from '../state'

const mockAudioAPI = {
  checkPermissions: vi.fn(),
  requestPermissions: vi.fn(),
  startRecordingWithSource: vi.fn(),
  stopRecording: vi.fn(),
}

const mockNotesAPI = {
  update: vi.fn(),
  markTranscriptActivated: vi.fn(),
}

const recordingListeners = {
  partial: [] as Array<(payload: { isPartial: boolean; text: string }) => void>,
  done: [] as Array<(payload: { text: string }) => void>,
}

vi.mock('../api', () => ({
  AudioAPI: mockAudioAPI,
  NotesAPI: mockNotesAPI,
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
    currentNote: {
      id: 'n1',
      title: 'Note 1',
      markdownContent: 'existing text',
      transcriptContent: '',
      fileStem: 'n1',
      folderId: '',
      transcriptActivated: false,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      order: 0,
    } as any,
    notes: [
      {
        id: 'n1',
        title: 'Note 1',
        markdownContent: 'existing text',
        transcriptContent: '',
        fileStem: 'n1',
        folderId: '',
        transcriptActivated: false,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        order: 0,
      } as any,
    ],
    notification: null,
    activeModal: null,
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
    mockNotesAPI.update.mockReset()
    mockNotesAPI.markTranscriptActivated.mockReset()
  })

  it('starts recording and updates live transcript lifecycle', async () => {
    const { initRecording, startRecording, stopRecording } = await import('./recording')

    mockAudioAPI.checkPermissions.mockResolvedValue({ status: 'granted' })
    mockAudioAPI.startRecordingWithSource.mockResolvedValue(undefined)
    mockAudioAPI.stopRecording.mockResolvedValue({})

    initRecording()

    await startRecording()

    expect(state.get('isRecording')).toBe(true)
    expect(mockAudioAPI.startRecordingWithSource).toHaveBeenCalledWith('microphone')
    expect(mockNotesAPI.markTranscriptActivated).toHaveBeenCalledWith('n1')

    recordingListeners.partial[0]({ isPartial: true, text: 'partial words' })
    expect(state.get('partialTranscript')).toContain('partial words')

    await stopRecording()
    expect(state.get('isRecording')).toBe(false)

    recordingListeners.done[0]({ text: 'final words' })
    await Promise.resolve()
    expect(mockNotesAPI.update).toHaveBeenCalled()
  })

  it('shows error and does not start when permission denied', async () => {
    const { startRecording } = await import('./recording')

    mockAudioAPI.checkPermissions.mockResolvedValue({ status: 'denied' })
    mockAudioAPI.requestPermissions.mockResolvedValue(undefined)

    await startRecording()

    expect(state.get('isRecording')).toBe(false)
    expect(mockAudioAPI.startRecordingWithSource).not.toHaveBeenCalled()
    expect(state.get('notification')?.type).toBe('error')
    expect(state.get('activeModal')).toBe('settings')
  })

  it('surfaces backend start error and opens settings for permission issues', async () => {
    const { startRecording } = await import('./recording')

    mockAudioAPI.checkPermissions.mockResolvedValue({ status: 'granted' })
    mockAudioAPI.startRecordingWithSource.mockRejectedValue(new Error('screen recording permission not granted'))

    await startRecording()

    expect(state.get('isRecording')).toBe(false)
    expect(state.get('notification')?.message).toContain('screen recording permission not granted')
    expect(state.get('activeModal')).toBe('settings')
  })
})
