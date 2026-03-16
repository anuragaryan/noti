import state from '../state'
import { LLMAPI } from '../api'
import { AppEvents } from '../events'
import { icon } from '../utils/icons'
import { escapeHtml } from '../utils/html'
import { renderMarkdownPreview } from '../utils/markdown'
import { isNearBottom, scrollToBottom } from '../utils/scroll'
import type { ChatMessage } from '../types'

function createMessageId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function buildSystemPrompt(): string {
  const note = state.get('currentNote')
  if (!note) {
    return 'You are a helpful AI assistant inside Noti. Respond clearly and concisely.'
  }

  const markdownContent = note.markdownContent || ''
  const transcriptContent = note.transcriptContent || ''
  const noteContext = `## Markdown\n${markdownContent}\n\n## Transcript\n${transcriptContent}`

  return `You are a helpful AI assistant. Analyze the user's note below and respond to their request.\n\nNote content:\n${noteContext}`
}

function updateLastAssistantMessage(chunkText: string): void {
  if (!chunkText) return

  const messages = [...state.get('chatMessages')]
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === 'assistant') {
      messages[i] = { ...messages[i], content: messages[i].content + chunkText }
      state.setState({ chatMessages: messages })
      return
    }
  }
}

function updateLastAssistantReasoning(reasoningText: string): void {
  if (!reasoningText) return

  const messages = [...state.get('chatMessages')]
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === 'assistant') {
      const previousReasoning = messages[i].reasoning || ''
      messages[i] = {
        ...messages[i],
        reasoning: previousReasoning + reasoningText,
        showThinking: messages[i].showThinking ?? true,
      }
      state.setState({ chatMessages: messages })
      return
    }
  }
}

function markReasoningCompletedIfNeeded(onDone: boolean): void {
  const messages = [...state.get('chatMessages')]
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role !== 'assistant') continue

    const hasReasoning = Boolean(messages[i].reasoning?.trim())
    const shouldComplete = hasReasoning && onDone
    if (!shouldComplete) return

    messages[i] = {
      ...messages[i],
      reasoningComplete: true,
      showThinking: false,
    }
    state.setState({ chatMessages: messages })
    return
  }
}

function closeAIChat(): void {
  const isChatStreaming = state.get('activeStreamTarget') === 'ai-chat' && state.get('chatIsStreaming')
  if (isChatStreaming) {
    void LLMAPI.stopStream().catch((err) => {
      console.error('Failed to stop AI chat stream:', err)
    })
  }

  state.setState({
    mainView: 'default',
    chatIsStreaming: false,
    activeStreamTarget: isChatStreaming ? null : state.get('activeStreamTarget'),
  })
}

function clearAIChat(): void {
  const isChatStreaming = state.get('activeStreamTarget') === 'ai-chat' && state.get('chatIsStreaming')
  if (isChatStreaming) {
    void LLMAPI.stopStream().catch((err) => {
      console.error('Failed to stop AI chat stream:', err)
    })
  }

  state.setState({
    chatInput: '',
    chatMessages: [],
    chatIsStreaming: false,
    activeStreamTarget: state.get('activeStreamTarget') === 'ai-chat' ? null : state.get('activeStreamTarget'),
  })
}

function renderAIChat(): void {
  const container = document.getElementById('ai-chat-screen')
  if (!container) return

  const note = state.get('currentNote')
  const isStreaming = state.get('chatIsStreaming')
  const chatInput = state.get('chatInput')
  const messages = state.get('chatMessages')
  const isBusy = isStreaming || (state.get('activeStreamTarget') === 'toolbar' && state.get('isStreaming'))

  const breadcrumb = note
    ? `<span class="chat-breadcrumb-muted">All Notes</span><span class="chat-breadcrumb-sep">${icon('chevron-right', 14)}</span><span class="chat-breadcrumb-current">${escapeHtml(note.title || 'Untitled')}</span>`
    : `<span class="chat-breadcrumb-muted">All Notes</span>`

  const contextText = note
    ? `Using current note: ${note.title || 'Untitled'}`
    : 'No note selected'

  const threadHtml = messages.length > 0
    ? messages.map((message) => {
      if (message.role === 'user') {
        return `
          <div class="chat-message-row chat-message-row-user">
            <div class="chat-bubble chat-bubble-user">${renderMarkdownPreview(message.content)}</div>
          </div>
        `
      }

      return `
        <div class="chat-message-block">
          <div class="chat-message-head">
            <span class="chat-message-role">Assistant</span>
          </div>
          ${(message.reasoning?.trim() || '') ? `
            <div class="chat-thinking">
              <button class="chat-thinking-toggle" data-chat-thinking-toggle-id="${message.id}" type="button">
                <span class="chat-thinking-title-left">${icon(message.reasoningComplete ? 'check' : 'loader', 14)} ${message.reasoningComplete ? 'Reasoning complete' : 'Thinking'}</span>
                <span class="chat-thinking-chevron ${message.showThinking === false ? 'collapsed' : ''}">${icon('chevron-down', 14)}</span>
              </button>
              <div class="chat-thinking-body ${message.showThinking === false ? 'hidden' : ''}">${renderMarkdownPreview(message.reasoning || '')}</div>
            </div>
          ` : ''}
          ${!message.reasoning?.trim() || message.reasoningComplete
            ? `<div class="chat-bubble chat-bubble-assistant">${renderMarkdownPreview(message.content)}</div>`
            : ''}
        </div>
      `
    }).join('')
    : '<div class="chat-thread-empty">Ask anything to start the conversation.</div>'

  container.innerHTML = `
    <div class="ai-chat-topbar">
      ${breadcrumb}
    </div>
    <div class="ai-chat-header">
      <div class="ai-chat-title-wrap">
        <h2 class="ai-chat-title">AI Chat</h2>
        <p class="ai-chat-subtitle">Ask anything about your notes in a scrollable chat</p>
      </div>
      <div class="ai-chat-header-actions">
        <button id="ai-chat-clear" class="ai-chat-clear-btn">${icon('trash-2', 14)} Clear</button>
        <button id="ai-chat-close" class="ai-chat-close-btn">${icon('x', 16)} Close</button>
      </div>
    </div>
    <div class="ai-chat-context-bar">
      <div class="ai-chat-context-label">${icon('sparkles', 16)} AI Chat</div>
      <div class="ai-chat-context-pill">${escapeHtml(contextText)}</div>
    </div>
    <div class="ai-chat-workspace">
      <div id="ai-chat-thread" class="ai-chat-thread">${threadHtml}</div>
      <div class="ai-chat-composer">
        <input id="ai-chat-input" class="ai-chat-input" type="text" placeholder="Ask AI about this note..." value="${escapeHtml(chatInput)}" ${isBusy ? 'disabled' : ''} />
        <button id="ai-chat-send" class="ai-chat-send-btn" ${(!chatInput.trim() || isBusy) ? 'disabled' : ''}>${icon('send', 14)} Send</button>
      </div>
    </div>
  `

  const thread = container.querySelector<HTMLElement>('#ai-chat-thread')
  if (thread) {
    scrollToBottom(thread)
  }

  container.querySelector('#ai-chat-close')?.addEventListener('mousedown', (e) => {
    e.preventDefault()
    closeAIChat()
  })

  container.querySelector('#ai-chat-close')?.addEventListener('click', (e) => {
    e.preventDefault()
    closeAIChat()
  })

  container.querySelector<HTMLInputElement>('#ai-chat-input')?.addEventListener('input', (e) => {
    const val = (e.target as HTMLInputElement).value
    state.setState({ chatInput: val })
    const sendBtn = container.querySelector<HTMLButtonElement>('#ai-chat-send')
    if (sendBtn) {
      const busy = state.get('chatIsStreaming') || (state.get('activeStreamTarget') === 'toolbar' && state.get('isStreaming'))
      sendBtn.disabled = !val.trim() || busy
    }
  })

  container.querySelector<HTMLInputElement>('#ai-chat-input')?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      void sendChatMessage()
    }
  })

  container.querySelector('#ai-chat-send')?.addEventListener('click', () => {
    void sendChatMessage()
  })

  container.querySelector('#ai-chat-clear')?.addEventListener('click', () => {
    clearAIChat()
  })

  container.querySelectorAll<HTMLButtonElement>('[data-chat-thinking-toggle-id]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const id = btn.dataset.chatThinkingToggleId
      if (!id) return

      const messages = state.get('chatMessages').map((m) => {
        if (m.id !== id || m.role !== 'assistant') return m
        return { ...m, showThinking: !(m.showThinking ?? true) }
      })
      state.setState({ chatMessages: messages })
    })
  })
}

async function sendChatMessage(): Promise<void> {
  const input = state.get('chatInput').trim()
  if (!input) return

  const activeStreamTarget = state.get('activeStreamTarget')
  if (activeStreamTarget && activeStreamTarget !== 'ai-chat') {
    state.showNotification('Another AI task is already running', 'info')
    return
  }

  if (state.get('chatIsStreaming')) return

  const nextMessages: ChatMessage[] = [
    ...state.get('chatMessages'),
    { id: createMessageId(), role: 'user', content: input },
    { id: createMessageId(), role: 'assistant', content: '', reasoning: '', showThinking: true, reasoningComplete: false },
  ]

  state.setState({
    chatInput: '',
    chatMessages: nextMessages,
    chatIsStreaming: true,
    activeStreamTarget: 'ai-chat',
  })

  try {
    await LLMAPI.generateStream(input, buildSystemPrompt())
  } catch (err) {
    console.error('AI chat failed:', err)
    state.setState({
      chatIsStreaming: false,
      activeStreamTarget: null,
    })
    state.showNotification('AI chat failed', 'error')
  }
}

export function openAIChat(): void {
  state.setState({ mainView: 'ai-chat' })
}

export function initAIChat(): void {
  renderAIChat()

  AppEvents.onStreamChunk((chunk) => {
    if (state.get('activeStreamTarget') !== 'ai-chat') return

    const thread = document.getElementById('ai-chat-thread')
    const shouldAutoScroll = thread ? isNearBottom(thread) : false

    updateLastAssistantMessage(chunk.text || '')
    updateLastAssistantReasoning(chunk.reasoningText || '')

    if (shouldAutoScroll && thread) {
      scrollToBottom(thread)
    }
  })

  AppEvents.onStreamDone(() => {
    if (state.get('activeStreamTarget') !== 'ai-chat') return
    markReasoningCompletedIfNeeded(true)
    state.setState({
      chatIsStreaming: false,
      activeStreamTarget: null,
    })
  })

  AppEvents.onStreamError((err) => {
    if (state.get('activeStreamTarget') !== 'ai-chat') return
    console.error('AI chat stream error:', err)
    state.setState({
      chatIsStreaming: false,
      activeStreamTarget: null,
    })
    state.showNotification(`AI error: ${err}`, 'error')
  })

  state.subscribe('mainView', renderAIChat)
  state.subscribe('currentNote', renderAIChat)
  state.subscribe('chatMessages', renderAIChat)
  state.subscribe('chatIsStreaming', renderAIChat)
}
