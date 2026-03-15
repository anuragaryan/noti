import { beforeEach, describe, expect, it, vi } from 'vitest'
import state from '../state'

const mockPromptsAPI = {
  executeOnNoteStream: vi.fn(),
  getAll: vi.fn(),
}

const mockLLMAPI = {
  generateStream: vi.fn(),
  stopStream: vi.fn(),
}

const streamListeners = {
  chunk: [] as Array<(payload: { text?: string; reasoningText?: string }) => void>,
  done: [] as Array<() => void>,
  error: [] as Array<(msg: string) => void>,
}

vi.mock('../api', () => ({
  PromptsAPI: mockPromptsAPI,
  LLMAPI: mockLLMAPI,
}))

vi.mock('../events', () => ({
  AppEvents: {
    onStreamChunk: (cb: (payload: { text?: string; reasoningText?: string }) => void) => {
      streamListeners.chunk.push(cb)
    },
    onStreamDone: (cb: () => void) => {
      streamListeners.done.push(cb)
    },
    onStreamError: (cb: (msg: string) => void) => {
      streamListeners.error.push(cb)
    },
  },
}))

function renderDOM(): void {
  document.body.innerHTML = `
    <div id="ai-toolbar"></div>
    <div id="ai-panel"></div>
    <textarea id="note-content-textarea"></textarea>
  `
}

function resetState(): void {
  state.setState({
    currentNote: {
      id: 'n1',
      title: 'Note 1',
      content: 'existing body',
      folderId: '',
      nameOnDisk: 'note-1.md',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      order: 0,
    } as any,
    prompts: [
      {
        id: 'p1',
        name: 'Summarize',
        description: 'desc',
        systemPrompt: 'system',
        userPrompt: 'Summarize {{content}}',
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      } as any,
    ] as any,
    selectedPromptId: 'p1',
    aiMode: 'preset',
    customPromptText: '',
    isStreaming: false,
    streamingStatus: 'idle',
    streamingContent: '',
    streamingReasoning: '',
    streamingReasoningComplete: false,
    showThinkingWidget: true,
    showAIPanel: false,
    notification: null,
  })
}

describe('toolbar integration', () => {
  beforeEach(() => {
    renderDOM()
    resetState()
    streamListeners.chunk = []
    streamListeners.done = []
    streamListeners.error = []
    mockPromptsAPI.executeOnNoteStream.mockReset()
    mockPromptsAPI.getAll.mockReset()
    mockLLMAPI.generateStream.mockReset()
    mockLLMAPI.stopStream.mockReset()
  })

  it('runs preset prompt stream and consumes chunk/done events', async () => {
    const { initToolbar } = await import('./toolbar')
    mockPromptsAPI.executeOnNoteStream.mockResolvedValue(undefined)

    initToolbar()

    const runBtn = document.querySelector<HTMLButtonElement>('#run-prompt-btn')
    if (!runBtn) throw new Error('run button missing')

    runBtn.click()

    expect(mockPromptsAPI.executeOnNoteStream).toHaveBeenCalledWith('p1', 'n1')
    expect(state.get('isStreaming')).toBe(true)

    streamListeners.chunk[0]({ text: 'Hello stream' })
    expect(state.get('streamingContent')).toContain('Hello stream')

    streamListeners.done[0]()
    expect(state.get('isStreaming')).toBe(false)
    expect(state.get('streamingStatus')).toBe('done')
  })

  it('stops active stream when panel close is clicked', async () => {
    const { initToolbar } = await import('./toolbar')
    mockPromptsAPI.executeOnNoteStream.mockResolvedValue(undefined)
    mockLLMAPI.stopStream.mockResolvedValue(undefined)

    initToolbar()

    const runBtn = document.querySelector<HTMLButtonElement>('#run-prompt-btn')
    if (!runBtn) throw new Error('run button missing')
    runBtn.click()

    const closeBtn = document.querySelector<HTMLButtonElement>('#ai-close-btn')
    if (!closeBtn) throw new Error('close button missing')

    closeBtn.click()

    expect(mockLLMAPI.stopStream).toHaveBeenCalledTimes(1)
    expect(state.get('streamingStatus')).toBe('cancelled')
  })
})
