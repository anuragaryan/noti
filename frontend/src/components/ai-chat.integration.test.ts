import { beforeEach, describe, expect, it, vi } from 'vitest'
import state from '../state'

const mockLLMAPI = {
  generateStream: vi.fn(),
  stopStream: vi.fn(),
}

const streamListeners = {
  chunk: [] as Array<(payload: { text?: string; reasoningText?: string }) => void>,
  done: [] as Array<() => void>,
  error: [] as Array<(msg: string) => void>,
}

function lastAssistantMessage() {
  const messages = state.get('chatMessages')
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === 'assistant') return messages[i]
  }
  return null
}

vi.mock('../api', () => ({
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

function resetState(): void {
  state.setState({
    currentNote: null,
    mainView: 'default',
    chatInput: '',
    chatMessages: [],
    chatIsStreaming: false,
    activeStreamTarget: null,
    isStreaming: false,
    notification: null,
  })
}

describe('ai chat integration', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="ai-chat-screen"></div>'
    streamListeners.chunk = []
    streamListeners.done = []
    streamListeners.error = []
    mockLLMAPI.generateStream.mockReset()
    mockLLMAPI.generateStream.mockResolvedValue(undefined)
    mockLLMAPI.stopStream.mockReset()
    mockLLMAPI.stopStream.mockResolvedValue(undefined)
    resetState()
  })

  it('streams assistant response for no-note chat', async () => {
    const { initAIChat, openAIChat } = await import('./ai-chat')
    initAIChat()
    openAIChat()

    const input = document.querySelector<HTMLInputElement>('#ai-chat-input')
    if (!input) throw new Error('chat input missing')
    input.value = 'help me summarize this week'
    input.dispatchEvent(new Event('input'))

    const sendBtn = document.querySelector<HTMLButtonElement>('#ai-chat-send')
    if (!sendBtn) throw new Error('chat send button missing')
    sendBtn.click()

    expect(mockLLMAPI.generateStream).toHaveBeenCalledTimes(1)
    expect(state.get('chatMessages').filter(m => m.role === 'user')).toHaveLength(1)
    expect(state.get('chatIsStreaming')).toBe(true)

    streamListeners.chunk[0]({ text: 'Sure, here is a summary.' })
    const messages = state.get('chatMessages')
    expect(messages[messages.length - 1]?.content).toContain('Sure, here is a summary.')

    streamListeners.done[0]()
    expect(state.get('chatIsStreaming')).toBe(false)
    expect(state.get('activeStreamTarget')).toBeNull()
  })

  it('keeps response hidden until reasoning is complete', async () => {
    const { initAIChat, openAIChat } = await import('./ai-chat')
    initAIChat()
    openAIChat()

    const input = document.querySelector<HTMLInputElement>('#ai-chat-input')
    if (!input) throw new Error('chat input missing')
    input.value = 'what are my action items'
    input.dispatchEvent(new Event('input'))

    const sendBtn = document.querySelector<HTMLButtonElement>('#ai-chat-send')
    if (!sendBtn) throw new Error('chat send button missing')
    sendBtn.click()

    streamListeners.chunk[0]({ reasoningText: 'Thinking through context...' })
    let assistant = lastAssistantMessage()
    expect(assistant?.reasoning).toContain('Thinking through context...')
    expect(assistant?.content).toBe('')
    expect(assistant?.showThinking).toBe(true)

    streamListeners.chunk[0]({ text: 'Action items: 1) ...' })
    assistant = lastAssistantMessage()
    expect(assistant?.content).toContain('Action items: 1) ...')
    expect(assistant?.reasoning).toContain('Thinking through context...')
    expect(assistant?.showThinking).toBe(true)
    expect(assistant?.reasoningComplete).toBe(false)

    streamListeners.done[0]()
    assistant = lastAssistantMessage()
    expect(assistant?.showThinking).toBe(false)
    expect(assistant?.reasoningComplete).toBe(true)
  })

  it('closes chat while streaming and stops stream', async () => {
    const { initAIChat, openAIChat } = await import('./ai-chat')
    initAIChat()
    openAIChat()

    const input = document.querySelector<HTMLInputElement>('#ai-chat-input')
    if (!input) throw new Error('chat input missing')
    input.value = 'draft an action list'
    input.dispatchEvent(new Event('input'))

    const sendBtn = document.querySelector<HTMLButtonElement>('#ai-chat-send')
    if (!sendBtn) throw new Error('chat send button missing')
    sendBtn.click()

    const closeBtn = document.querySelector<HTMLButtonElement>('#ai-chat-close')
    if (!closeBtn) throw new Error('chat close button missing')
    closeBtn.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))

    expect(state.get('mainView')).toBe('default')
    expect(state.get('chatIsStreaming')).toBe(false)
    expect(state.get('activeStreamTarget')).toBeNull()
    expect(mockLLMAPI.stopStream).toHaveBeenCalledTimes(1)
  })

  it('clears chat screen and context', async () => {
    const { initAIChat, openAIChat } = await import('./ai-chat')
    initAIChat()
    openAIChat()

    state.setState({
      chatInput: 'draft follow-up actions',
      chatMessages: [
        { id: 'u1', role: 'user', content: 'summarize this note' },
        { id: 'a1', role: 'assistant', content: 'Here is a short summary.' },
      ],
    })

    const clearBtn = document.querySelector<HTMLButtonElement>('#ai-chat-clear')
    if (!clearBtn) throw new Error('chat clear button missing')
    clearBtn.click()

    expect(state.get('chatInput')).toBe('')
    expect(state.get('chatMessages')).toHaveLength(0)
    expect(state.get('chatIsStreaming')).toBe(false)

    const thread = document.querySelector<HTMLElement>('#ai-chat-thread')
    expect(thread?.textContent).toContain('Ask anything to start the conversation.')
  })

  it('clears chat while streaming and stops stream', async () => {
    const { initAIChat, openAIChat } = await import('./ai-chat')
    initAIChat()
    openAIChat()

    const input = document.querySelector<HTMLInputElement>('#ai-chat-input')
    if (!input) throw new Error('chat input missing')
    input.value = 'generate tasks from transcript'
    input.dispatchEvent(new Event('input'))

    const sendBtn = document.querySelector<HTMLButtonElement>('#ai-chat-send')
    if (!sendBtn) throw new Error('chat send button missing')
    sendBtn.click()

    const clearBtn = document.querySelector<HTMLButtonElement>('#ai-chat-clear')
    if (!clearBtn) throw new Error('chat clear button missing')
    clearBtn.click()

    expect(state.get('mainView')).toBe('ai-chat')
    expect(state.get('chatInput')).toBe('')
    expect(state.get('chatMessages')).toHaveLength(0)
    expect(state.get('chatIsStreaming')).toBe(false)
    expect(state.get('activeStreamTarget')).toBeNull()
    expect(mockLLMAPI.stopStream).toHaveBeenCalledTimes(1)
  })
})
