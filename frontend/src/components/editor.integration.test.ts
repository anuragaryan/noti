import { beforeEach, describe, expect, it, vi } from 'vitest'
import state from '../state'

const mockNotesAPI = {
  update: vi.fn(),
}

const mockFoldersAPI = {
  getPath: vi.fn(),
}

vi.mock('../api', () => ({
  NotesAPI: mockNotesAPI,
  FoldersAPI: mockFoldersAPI,
}))

function renderDOM(): void {
  document.body.innerHTML = `
    <div id="top-bar"></div>
    <div id="editor-header"></div>
    <div id="editor-area"></div>
  `
}

function resetState(): void {
  const now = new Date().toISOString()
  const note = {
    id: 'n1',
    title: 'Note 1',
    fileStem: 'n1',
    folderId: '',
    transcriptActivated: true,
    markdownContent: 'saved markdown',
    transcriptContent: 'saved transcript',
    createdAt: now,
    updatedAt: now,
    order: 0,
  } as any

  state.setState({
    notes: [note],
    folders: [],
    currentNote: note,
    isPreviewMode: false,
    isRecording: true,
    partialTranscript: '',
    isDirty: false,
    isSaving: false,
  })
}

describe('editor integration', () => {
  beforeEach(async () => {
    renderDOM()
    resetState()
    mockNotesAPI.update.mockReset()
    mockNotesAPI.update.mockResolvedValue(undefined)
    mockFoldersAPI.getPath.mockReset()
    mockFoldersAPI.getPath.mockResolvedValue([])
    vi.useRealTimers()
  })

  it('preserves unsaved markdown while partial transcript updates', async () => {
    const { initEditor } = await import('./editor')
    initEditor()

    const textareaBefore = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
    expect(textareaBefore).not.toBeNull()
    if (!textareaBefore) return

    textareaBefore.value = 'unsaved local markdown edit'
    textareaBefore.dispatchEvent(new Event('input'))

    state.setState({ partialTranscript: 'live partial words' })

    const textareaAfter = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
    expect(textareaAfter).toBe(textareaBefore)
    expect(textareaAfter?.value).toBe('unsaved local markdown edit')

    const transcriptBody = document.getElementById('transcript-panel-body')
    expect(transcriptBody?.textContent).toContain('saved transcript')
    expect(transcriptBody?.textContent).toContain('live partial words')
  })

  it('scrolls transcript panel to end on live partial updates', async () => {
    const rafSpy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb: FrameRequestCallback) => {
      cb(0)
      return 1
    })

    const { initEditor } = await import('./editor')
    initEditor()

    const transcriptBody = document.getElementById('transcript-panel-body')
    expect(transcriptBody).not.toBeNull()
    if (!transcriptBody) return

    Object.defineProperty(transcriptBody, 'scrollHeight', { value: 420, configurable: true })

    state.setState({ partialTranscript: 'more live text' })
    expect(transcriptBody.scrollTop).toBe(420)

    rafSpy.mockRestore()
  })

  it('keeps title cursor position after autosave rerender', async () => {
    vi.useFakeTimers()

    state.setState({ isRecording: false })
    const { initEditor } = await import('./editor')
    initEditor()

    const titleInput = document.querySelector<HTMLInputElement>('#note-title-input')
    expect(titleInput).not.toBeNull()
    if (!titleInput) return

    titleInput.focus()
    titleInput.value = 'Typing in title'
    titleInput.setSelectionRange(4, 4)
    titleInput.dispatchEvent(new Event('input'))

    await vi.advanceTimersByTimeAsync(1200)

    const titleAfter = document.querySelector<HTMLInputElement>('#note-title-input')
    expect(document.activeElement).toBe(titleAfter)
    expect(titleAfter?.selectionStart).toBe(4)
    expect(titleAfter?.selectionEnd).toBe(4)
  })

  it('allows editing transcript when recording is stopped', async () => {
    vi.useFakeTimers()

    state.setState({ isRecording: false })
    const { initEditor } = await import('./editor')
    initEditor()

    const transcriptInput = document.querySelector<HTMLTextAreaElement>('#transcript-content-textarea')
    expect(transcriptInput).not.toBeNull()
    if (!transcriptInput) return

    transcriptInput.value = 'edited transcript'
    transcriptInput.dispatchEvent(new Event('input'))

    await vi.advanceTimersByTimeAsync(1200)

    expect(mockNotesAPI.update).toHaveBeenCalled()
    const lastCall = mockNotesAPI.update.mock.calls[mockNotesAPI.update.mock.calls.length - 1]
    expect(lastCall[3]).toBe('edited transcript')
  })
})
