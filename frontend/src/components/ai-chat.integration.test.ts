import { beforeEach, describe, expect, it, vi } from 'vitest'
import state from '../state'

const mockLLMAPI = {
  generateChatStream: vi.fn(),
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

function requestChars(messages: Array<{ role: string; content: string }>): number {
  return messages.reduce((sum, m) => sum + m.role.length + m.content.length + 8, 0)
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
    mockLLMAPI.generateChatStream.mockReset()
    mockLLMAPI.generateChatStream.mockResolvedValue(undefined)
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

    expect(mockLLMAPI.generateChatStream).toHaveBeenCalledTimes(1)
    const [requestMessages, systemPrompt] = mockLLMAPI.generateChatStream.mock.calls[0]
    expect(requestMessages).toEqual([
      { role: 'user', content: 'help me summarize this week' },
    ])
    expect(typeof systemPrompt).toBe('string')
    expect(state.get('chatMessages').filter(m => m.role === 'user')).toHaveLength(1)
    expect(state.get('chatIsStreaming')).toBe(true)

    streamListeners.chunk[0]({ text: 'Sure, here is a summary.' })
    const messages = state.get('chatMessages')
    expect(messages[messages.length - 1]?.content).toContain('Sure, here is a summary.')

    streamListeners.done[0]()
    expect(state.get('chatIsStreaming')).toBe(false)
    expect(state.get('activeStreamTarget')).toBeNull()
  })

  it('rolls back optimistic messages when chat request fails', async () => {
    mockLLMAPI.generateChatStream.mockRejectedValueOnce(new Error('network down'))

    const { initAIChat, openAIChat } = await import('./ai-chat')
    state.setState({
      chatMessages: [
        { id: 'u0', role: 'user', content: 'existing question' },
        { id: 'a0', role: 'assistant', content: 'existing answer' },
      ],
    })
    initAIChat()
    openAIChat()

    const input = document.querySelector<HTMLInputElement>('#ai-chat-input')
    if (!input) throw new Error('chat input missing')
    input.value = 'new question'
    input.dispatchEvent(new Event('input'))

    const sendBtn = document.querySelector<HTMLButtonElement>('#ai-chat-send')
    if (!sendBtn) throw new Error('chat send button missing')
    sendBtn.click()

    await Promise.resolve()

    expect(state.get('chatMessages')).toEqual([
      { id: 'u0', role: 'user', content: 'existing question' },
      { id: 'a0', role: 'assistant', content: 'existing answer' },
    ])
    expect(state.get('chatIsStreaming')).toBe(false)
    expect(state.get('activeStreamTarget')).toBeNull()
    expect(state.get('notification')?.message).toBe('AI chat failed')
    expect(state.get('notification')?.type).toBe('error')
  })

  it('sends threaded messages and trims to context budget', async () => {
    const { initAIChat, openAIChat } = await import('./ai-chat')

    const longHistory = Array.from({ length: 80 }, (_, idx) => ({
      id: `u-${idx}`,
      role: idx % 2 === 0 ? 'user' as const : 'assistant' as const,
      content: `${idx % 2 === 0 ? 'Q' : 'A'}${idx}-` + 'x'.repeat(1200),
    }))

    state.setState({ chatMessages: longHistory })
    initAIChat()
    openAIChat()

    const input = document.querySelector<HTMLInputElement>('#ai-chat-input')
    if (!input) throw new Error('chat input missing')
    input.value = 'latest question in thread'
    input.dispatchEvent(new Event('input'))

    const sendBtn = document.querySelector<HTMLButtonElement>('#ai-chat-send')
    if (!sendBtn) throw new Error('chat send button missing')
    sendBtn.click()

    expect(mockLLMAPI.generateChatStream).toHaveBeenCalledTimes(1)
    const [requestMessages] = mockLLMAPI.generateChatStream.mock.calls[0]
    expect(requestMessages[requestMessages.length - 1]).toEqual({
      role: 'user',
      content: 'latest question in thread',
    })
    expect(requestChars(requestMessages)).toBeLessThanOrEqual(62000)
    expect(requestMessages.length).toBeLessThan(longHistory.length + 1)
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

  it('rolls back optimistic messages on stream error event', async () => {
    const { initAIChat, openAIChat } = await import('./ai-chat')
    state.setState({
      chatMessages: [
        { id: 'u0', role: 'user', content: 'existing question' },
        { id: 'a0', role: 'assistant', content: 'existing answer' },
      ],
    })
    initAIChat()
    openAIChat()

    const input = document.querySelector<HTMLInputElement>('#ai-chat-input')
    if (!input) throw new Error('chat input missing')
    input.value = 'new question'
    input.dispatchEvent(new Event('input'))

    const sendBtn = document.querySelector<HTMLButtonElement>('#ai-chat-send')
    if (!sendBtn) throw new Error('chat send button missing')
    sendBtn.click()

    streamListeners.chunk[0]({ text: 'partial response' })
    streamListeners.error[0]('provider disconnected')

    expect(state.get('chatMessages')).toEqual([
      { id: 'u0', role: 'user', content: 'existing question' },
      { id: 'a0', role: 'assistant', content: 'existing answer' },
    ])
    expect(state.get('chatIsStreaming')).toBe(false)
    expect(state.get('activeStreamTarget')).toBeNull()
    expect(state.get('notification')?.message).toBe('AI error: provider disconnected')
    expect(state.get('notification')?.type).toBe('error')
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
