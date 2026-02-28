/**
 * AI Toolbar component — prompt select + run button + streaming result panel.
 * Injects into: #ai-toolbar, #ai-panel
 */

import state from '../state'
import { PromptsAPI } from '../api'
import { AppEvents } from '../events'
import { escapeHtml } from '../utils/html'
import { icon } from '../utils/icons'

// ─── Lucide icons — see utils/icons.ts ─────────────────────────────────────────────────────────

// escapeHtml imported from utils/html.ts

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
  // Layout defined in #ai-toolbar CSS class (app.css)

  const prompts = state.get('prompts')
  const selectedId = state.get('selectedPromptId')
  const isStreaming = state.get('isStreaming')

  const promptOptions = prompts.map(p =>
    `<option value="${escapeHtml(p.id)}" ${p.id === selectedId ? 'selected' : ''}>${escapeHtml(p.name)}</option>`
  ).join('')

  container.innerHTML = `
    <div class="ai-label">
      ${icon('sparkles', 16)}
      <span class="ai-label-text">AI</span>
    </div>
    <select id="prompt-select" class="ai-prompt-select">
      <option value="">Select prompt…</option>
      ${promptOptions}
    </select>
    <button id="run-prompt-btn" class="ai-run-btn" ${(!selectedId || !note || isStreaming) ? 'disabled' : ''}>
      ${icon('play', 14)}
      ${isStreaming ? 'Running…' : 'Run'}
    </button>
  `

  container.querySelector<HTMLSelectElement>('#prompt-select')?.addEventListener('change', (e) => {
    const val = (e.target as HTMLSelectElement).value
    state.setState({ selectedPromptId: val || null })
    renderToolbar()
  })

  container.querySelector('#run-prompt-btn')?.addEventListener('click', () => {
    void runPrompt()
  })
}

// ─── Run prompt ──────────────────────────────────────────────────────────────

async function runPrompt(): Promise<void> {
  const note = state.get('currentNote')
  const promptId = state.get('selectedPromptId')
  if (!note || !promptId) return

  state.setState({ isStreaming: true, streamingContent: '', showAIPanel: true })
  renderToolbar()
  renderAIPanel()

  try {
    await PromptsAPI.executeOnNoteStream(promptId, note.id)
  } catch (err) {
    console.error('Prompt execution failed:', err)
    state.setState({ isStreaming: false })
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
  const isStreaming = state.get('isStreaming')

  if (!show) {
    panel.classList.add('hidden')
    return
  }

  panel.classList.remove('hidden')
  // Layout defined in #ai-panel CSS class (app.css)

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
    <div id="ai-result-text" class="ai-result-text ${isStreaming ? 'streaming-cursor' : ''}">${escapeHtml(content)}</div>
  `

  panel.querySelector('#ai-close-btn')?.addEventListener('click', () => {
    state.setState({ showAIPanel: false, streamingContent: '', isStreaming: false })
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
    const current = state.get('streamingContent')
    state.setState({ streamingContent: current + chunk.text })
    // Update just the text node for efficiency
    const el = document.getElementById('ai-result-text')
    if (el) el.textContent = state.get('streamingContent')
  })

  AppEvents.onStreamDone(() => {
    state.setState({ isStreaming: false })
    renderToolbar()
    renderAIPanel()
  })

  AppEvents.onStreamError((err) => {
    console.error('Stream error:', err)
    state.setState({ isStreaming: false })
    state.showNotification(`AI error: ${err}`, 'error')
    renderToolbar()
    renderAIPanel()
  })

  state.subscribe('prompts', () => renderToolbar())
  state.subscribe('currentNote', () => renderToolbar())
  state.subscribe('selectedPromptId', () => renderToolbar())
  state.subscribe('showAIPanel', () => renderAIPanel())
}

export async function loadPrompts(): Promise<void> {
  try {
    const prompts = await PromptsAPI.getAll()
    state.setState({ prompts })
  } catch (err) {
    console.error('Failed to load prompts:', err)
  }
}
