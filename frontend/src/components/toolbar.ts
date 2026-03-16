/**
 * AI Toolbar component — prompt select + run button + streaming result panel.
 * Injects into: #ai-toolbar, #ai-panel
 */

import state from '../state'
import { PromptsAPI, LLMAPI } from '../api'
import { AppEvents } from '../events'
import { escapeHtml } from '../utils/html'
import { icon } from '../utils/icons'
import { renderMarkdownPreview } from '../utils/markdown'
import { isNearBottom, scrollToBottom } from '../utils/scroll'

const MANAGE_PROMPTS_VALUE = '__manage_prompts__'

// ─── Render toolbar ──────────────────────────────────────────────────────────

function renderToolbar(): void {
  const container = document.getElementById('ai-toolbar')
  if (!container) return

  const note = state.get('currentNote')
  if (!note) {
    container.classList.add('hidden')
    container.innerHTML = ''
    return
  }

  container.classList.remove('hidden')

  const prompts = state.get('prompts')
  const selectedId = state.get('selectedPromptId')
  const isStreaming = state.get('isStreaming')
  const aiMode = state.get('aiMode')
  const customText = state.get('customPromptText')

  const promptOptions = prompts.map(p =>
    `<option value="${escapeHtml(p.id)}" ${p.id === selectedId ? 'selected' : ''}>${escapeHtml(p.name)}</option>`
  ).join('')

  const isPresetMode = aiMode === 'preset'
  const isCustomMode = aiMode === 'custom'

  const canRun = isStreaming
    ? false
    : isPresetMode
      ? Boolean(selectedId && note)
      : Boolean(customText.trim() && note)

  container.innerHTML = `
    <div class="ai-label">
      ${icon('sparkles', 16)}
      <span class="ai-label-text">AI</span>
    </div>
    <div class="ai-mode-toggle">
      <button id="ai-preset-btn" class="ai-mode-btn ${isPresetMode ? 'active' : ''}">Preset</button>
      <button id="ai-custom-btn" class="ai-mode-btn ${isCustomMode ? 'active' : ''}">Custom</button>
    </div>
    ${isPresetMode ? `
      <select id="prompt-select" class="ai-prompt-select">
        <option value="">Select prompt…</option>
        <optgroup label="Presets">
          ${promptOptions}
        </optgroup>
        <optgroup label="Actions">
          <option value="${MANAGE_PROMPTS_VALUE}">⚙ Manage prompts…</option>
        </optgroup>
      </select>
    ` : `
      <input
        type="text"
        id="custom-prompt-input"
        class="ai-custom-input"
        placeholder="Enter custom prompt..."
        value="${escapeHtml(customText)}"
      />
    `}
    <button id="run-prompt-btn" class="ai-run-btn" ${!canRun ? 'disabled' : ''}>
      ${icon('play', 14)}
      ${isStreaming ? 'Running…' : 'Run'}
    </button>
  `

  container.querySelector<HTMLSelectElement>('#prompt-select')?.addEventListener('change', (e) => {
    const val = (e.target as HTMLSelectElement).value

    if (val === MANAGE_PROMPTS_VALUE) {
      state.openModal('prompts')
      renderToolbar()
      return
    }

    state.setState({ selectedPromptId: val || null })
    renderToolbar()
  })

  container.querySelector<HTMLInputElement>('#custom-prompt-input')?.addEventListener('input', (e) => {
    const val = (e.target as HTMLInputElement).value
    state.setState({ customPromptText: val })
    const runBtn = container.querySelector<HTMLButtonElement>('#run-prompt-btn')
    if (runBtn) {
      runBtn.disabled = !val.trim() || isStreaming
    }
  })

  container.querySelector('#ai-preset-btn')?.addEventListener('click', () => {
    state.setState({ aiMode: 'preset' })
    renderToolbar()
  })

  container.querySelector('#ai-custom-btn')?.addEventListener('click', () => {
    state.setState({ aiMode: 'custom' })
    renderToolbar()
  })

  container.querySelector('#run-prompt-btn')?.addEventListener('click', () => {
    void runPrompt()
  })
}

// ─── Run prompt ──────────────────────────────────────────────────────────────

async function runPrompt(): Promise<void> {
  const note = state.get('currentNote')
  const aiMode = state.get('aiMode')

  if (!note) return

  state.setState({
    isStreaming: true,
    streamingStatus: 'running',
    streamingContent: '',
    streamingReasoning: '',
    streamingReasoningComplete: false,
    showThinkingWidget: true,
    showAIPanel: true,
  })
  renderToolbar()
  renderAIPanel()

  try {
    if (aiMode === 'preset') {
      const promptId = state.get('selectedPromptId')
      if (!promptId) {
        state.setState({ isStreaming: false, streamingStatus: 'idle' })
        return
      }
      await PromptsAPI.executeOnNoteStream(promptId, note.id)
    } else {
      const customText = state.get('customPromptText')
      if (!customText.trim()) {
        state.setState({ isStreaming: false, streamingStatus: 'idle' })
        return
      }

      const markdownContent = note.markdownContent || ''
      const transcriptContent = note.transcriptContent || ''
      const noteContext = `## Markdown\n${markdownContent}\n\n## Transcript\n${transcriptContent}`
      const systemPrompt = `You are a helpful AI assistant. Analyze the user's note below and respond to their request.\n\nNote content:\n${noteContext}`

      await LLMAPI.generateStream(customText, systemPrompt)
    }
  } catch (err) {
    console.error('Prompt execution failed:', err)
    state.setState({ isStreaming: false, streamingStatus: 'error' })
    state.showNotification('AI prompt failed', 'error')
    renderToolbar()
    renderAIPanel()
  }
}

// ─── AI Result Panel ─────────────────────────────────────────────────────────

function renderAIPanel(): void {
  const panel = document.getElementById('ai-panel')
  if (!panel) return

  const show = state.get('showAIPanel')
  const content = state.get('streamingContent')
  const reasoning = state.get('streamingReasoning')
  const showThinkingWidget = state.get('showThinkingWidget')
  const isStreaming = state.get('isStreaming')
  const streamingStatus = state.get('streamingStatus')

  if (!show) {
    panel.classList.add('hidden')
    return
  }

  panel.classList.remove('hidden')
  // Layout defined in #ai-panel CSS class (app.css)

  const showThinking = Boolean(reasoning.trim())
  const isReasoningComplete = showThinking
    && (state.get('streamingReasoningComplete') || streamingStatus === 'done')
  const isReasoningInterrupted = showThinking
    && !isReasoningComplete
    && (streamingStatus === 'error' || streamingStatus === 'cancelled')
  const thinkingBodyClass = showThinkingWidget ? '' : 'hidden'
  const thinkingStatusIcon = isReasoningComplete ? 'check' : (isReasoningInterrupted ? 'alert-circle' : 'loader')
  const thinkingStatusLabel = isReasoningComplete ? 'Reasoning complete' : (isReasoningInterrupted ? 'Reasoning interrupted' : 'Thinking')

  panel.innerHTML = `
    <div class="ai-panel-header">
      <span class="ai-panel-title">AI Result</span>
      <div class="ai-panel-actions">
        ${!isStreaming ? `
          <button id="ai-copy-btn" class="ai-small-btn ai-copy-btn">${icon('copy', 14)} Copy</button>
          <button id="ai-accept-btn" class="ai-small-btn ai-accept-btn">${icon('check', 14)} Append to note</button>
        ` : ''}
        <button id="ai-close-btn" class="ai-close-btn">${icon('x', 14)}</button>
      </div>
    </div>
    ${showThinking ? `
      <div id="ai-thinking" class="ai-thinking">
        <button id="ai-thinking-toggle" class="ai-thinking-title" type="button">
          <span class="ai-thinking-title-left">${icon(thinkingStatusIcon, 14)} ${thinkingStatusLabel}</span>
          <span class="ai-thinking-chevron ${showThinkingWidget ? '' : 'collapsed'}">${icon('chevron-down', 14)}</span>
        </button>
        <div id="ai-thinking-body" class="ai-thinking-body ${thinkingBodyClass}">${renderMarkdownPreview(reasoning)}</div>
      </div>
    ` : ''}
    <div id="ai-result-markdown" class="ai-result-text ${isStreaming ? 'streaming-cursor' : ''}">${renderMarkdownPreview(content)}</div>
  `

  panel.querySelector('#ai-thinking-toggle')?.addEventListener('click', () => {
    state.setState({ showThinkingWidget: !state.get('showThinkingWidget') })
    renderAIPanel()
  })

  panel.querySelector('#ai-close-btn')?.addEventListener('click', () => {
    if (isStreaming) {
      void LLMAPI.stopStream().catch((err) => {
        console.error('Failed to stop stream:', err)
      })
    }
    state.setState({
      showAIPanel: false,
      streamingContent: '',
      streamingReasoning: '',
      streamingReasoningComplete: false,
      showThinkingWidget: true,
      isStreaming: false,
      streamingStatus: isStreaming ? 'cancelled' : 'idle',
    })
    renderAIPanel()
    renderToolbar()
  })

  panel.querySelector('#ai-copy-btn')?.addEventListener('click', () => {
    void navigator.clipboard.writeText(content)
    state.showNotification('Copied to clipboard', 'success')
  })

  panel.querySelector('#ai-accept-btn')?.addEventListener('click', () => {
    const textarea = document.querySelector<HTMLTextAreaElement>('#note-content-textarea')
    if (textarea) {
      textarea.value += '\n\n' + content
      textarea.dispatchEvent(new Event('input'))
    }
    state.setState({ showAIPanel: false, streamingContent: '' })
    renderAIPanel()
  })
}

// ─── Public API ──────────────────────────────────────────────────────────────

export function initToolbar(): void {
  renderToolbar()
  renderAIPanel()

  // Wire streaming events
  AppEvents.onStreamChunk((chunk) => {
    const panelEl = document.getElementById('ai-panel')
    const shouldAutoScroll = panelEl ? isNearBottom(panelEl) : false

    const currentContent = state.get('streamingContent')
    const currentReasoning = state.get('streamingReasoning')
    const currentReasoningComplete = state.get('streamingReasoningComplete')
    const nextContent = currentContent + (chunk.text || '')
    const nextReasoning = currentReasoning + (chunk.reasoningText || '')
    const hasReasoning = Boolean(nextReasoning.trim())
    const hasReasoningChunk = Boolean(chunk.reasoningText?.trim())
    const hasOutputChunk = Boolean(chunk.text)
    const nextReasoningComplete = hasReasoning && (currentReasoningComplete || (!hasReasoningChunk && hasOutputChunk))

    state.setState({
      streamingContent: nextContent,
      streamingReasoning: nextReasoning,
      streamingReasoningComplete: nextReasoningComplete,
    })

    const resultEl = document.getElementById('ai-result-markdown')
    if (resultEl) {
      resultEl.innerHTML = renderMarkdownPreview(nextContent)
    }

    const thinkingBodyEl = document.getElementById('ai-thinking-body')
    if (thinkingBodyEl) {
      thinkingBodyEl.innerHTML = renderMarkdownPreview(nextReasoning)
    } else if (chunk.reasoningText) {
      renderAIPanel()
    }

    if (nextReasoningComplete !== currentReasoningComplete) {
      renderAIPanel()
    }

    if (panelEl && shouldAutoScroll) {
      scrollToBottom(panelEl)
    }
  })

  AppEvents.onStreamDone(() => {
    state.setState({ isStreaming: false, streamingStatus: 'done' })
    renderToolbar()
    renderAIPanel()
  })

  AppEvents.onStreamError((err) => {
    console.error('Stream error:', err)
    state.setState({ isStreaming: false, streamingStatus: 'error' })
    state.showNotification(`AI error: ${err}`, 'error')
    renderToolbar()
    renderAIPanel()
  })

  state.subscribe('prompts', () => renderToolbar())
  state.subscribe('currentNote', () => renderToolbar())
  state.subscribe('selectedPromptId', () => renderToolbar())
  state.subscribe('showAIPanel', () => renderAIPanel())
  state.subscribe('aiMode', () => renderToolbar())
  state.subscribe('showThinkingWidget', () => renderAIPanel())
}

export async function loadPrompts(): Promise<void> {
  try {
    const prompts = await PromptsAPI.getAll()
    state.setState({ prompts })
  } catch (err) {
    console.error('Failed to load prompts:', err)
  }
}
