/**
 * Prompt management modal — list, create, edit, delete AI prompts.
 */

import state from '../../state'
import { PromptsAPI } from '../../api'
import type { Prompt } from '../../types'
import { escapeHtml } from '../../utils/html'
import { icon } from '../../utils/icons'

// ─── Icons — see utils/icons.ts ────────────────────

let selectedPrompt: Prompt | null = null

export async function renderPromptsModal(container: HTMLElement): Promise<void> {
  // Reset selection on each open (prevents stale state across open/close cycles)
  selectedPrompt = null

  let prompts = state.get('prompts')
  if (prompts.length === 0) {
    try {
      prompts = await PromptsAPI.getAll()
      state.setState({ prompts })
    } catch {
      state.showNotification('Failed to load prompts', 'error')
    }
  }

  function buildUI(): void {
    const currentPrompts = state.get('prompts')

    container.innerHTML = `
      <div class="modal-card modal-card-lg">
        <!-- Header -->
        <div class="modal-header">
          <h2 class="modal-heading">Manage Prompts</h2>
          <button id="prompts-close" class="btn-icon">${icon('x', 16)}</button>
        </div>

        <!-- Body: two-column -->
        <div class="prompts-body">

          <!-- Left: list + new button -->
          <div class="prompts-sidebar">
            <div class="prompts-sidebar-actions">
              <button id="new-prompt-btn" class="new-prompt-btn">${icon('plus', 14)} New Prompt</button>
            </div>
            <div id="prompt-list" class="prompts-list">
              ${currentPrompts.map(p => `
                <div class="prompt-list-item${selectedPrompt?.id === p.id ? ' active' : ''}" data-id="${escapeHtml(p.id)}">
                  <div class="prompt-item-name">${escapeHtml(p.name)}</div>
                  <div class="prompt-item-desc">${escapeHtml(p.description || '')}</div>
                </div>
              `).join('')}
              ${currentPrompts.length === 0 ? '<div class="prompt-no-items">No prompts yet</div>' : ''}
            </div>
          </div>

          <!-- Right: edit form -->
          <div id="prompt-form-area" class="prompt-form-area">
            ${selectedPrompt ? renderForm(selectedPrompt) : `
              <div class="prompt-empty-hint">
                Select a prompt or create a new one
              </div>
            `}
          </div>
        </div>
      </div>
    `

    // Wire close
    container.querySelector('#prompts-close')?.addEventListener('click', () => state.closeModal())

    // Wire list item clicks
    container.querySelectorAll('.prompt-list-item').forEach(item => {
      item.addEventListener('click', () => {
        const id = (item as HTMLElement).dataset.id
        selectedPrompt = state.get('prompts').find(p => p.id === id) ?? null
        buildUI()
      })
    })

    // Wire new prompt
    container.querySelector('#new-prompt-btn')?.addEventListener('click', () => {
      selectedPrompt = null
      const formArea = container.querySelector<HTMLElement>('#prompt-form-area')
      if (formArea) formArea.innerHTML = renderForm(null)
      wireFormButtons(formArea, null)
    })

    if (selectedPrompt) {
      wireFormButtons(container.querySelector('#prompt-form-area'), selectedPrompt)
    }
  }

  function renderForm(prompt: Prompt | null): string {
    return `
      <div class="pf-form">
        <label class="pf-label">
          <span class="form-label-text">Name</span>
          <input id="pf-name" type="text" value="${escapeHtml(prompt?.name || '')}" placeholder="e.g. Summarize" class="form-input" />
        </label>
        <label class="pf-label">
          <span class="form-label-text">Description</span>
          <input id="pf-desc" type="text" value="${escapeHtml(prompt?.description || '')}" placeholder="Short description" class="form-input" />
        </label>
        <label class="pf-label">
          <span class="form-label-text">System Prompt</span>
          <textarea id="pf-system" rows="3" placeholder="You are a helpful assistant..." class="pf-textarea">${escapeHtml(prompt?.systemPrompt || '')}</textarea>
        </label>
        <label class="pf-label">
          <span class="form-label-text">User Prompt <span class="pf-hint">(use {{content}} for note text)</span></span>
          <textarea id="pf-user" rows="4" placeholder="Summarize the following:\n\n{{content}}" class="pf-textarea">${escapeHtml(prompt?.userPrompt || '')}</textarea>
        </label>
        <div class="pf-actions">
          ${prompt ? `
            <button id="pf-delete" class="pf-delete-btn">${icon('trash-2', 14)} Delete</button>
          ` : ''}
          <button id="pf-save" class="btn-primary">${prompt ? 'Save Changes' : 'Create Prompt'}</button>
        </div>
      </div>
    `
  }

  function wireFormButtons(formEl: Element | null, prompt: Prompt | null): void {
    if (!formEl) return

    formEl.querySelector('#pf-save')?.addEventListener('click', async () => {
      const name = (formEl.querySelector<HTMLInputElement>('#pf-name')?.value ?? '').trim()
      if (!name) { state.showNotification('Prompt name required', 'error'); return }
      const desc = formEl.querySelector<HTMLInputElement>('#pf-desc')?.value ?? ''
      const system = formEl.querySelector<HTMLTextAreaElement>('#pf-system')?.value ?? ''
      const user = formEl.querySelector<HTMLTextAreaElement>('#pf-user')?.value ?? ''

      try {
        if (prompt) {
          await PromptsAPI.update(prompt.id, name, desc, system, user)
        } else {
          await PromptsAPI.create(name, desc, system, user)
        }
        const updated = await PromptsAPI.getAll()
        state.setState({ prompts: updated })
        selectedPrompt = updated.find(p => p.name === name) ?? null
        state.showNotification(prompt ? 'Prompt updated' : 'Prompt created', 'success')
        buildUI()
      } catch {
        state.showNotification('Failed to save prompt', 'error')
      }
    })

    formEl.querySelector('#pf-delete')?.addEventListener('click', async () => {
      if (!prompt) return
      try {
        await PromptsAPI.delete(prompt.id)
        const updated = await PromptsAPI.getAll()
        state.setState({ prompts: updated })
        selectedPrompt = null
        state.showNotification('Prompt deleted', 'success')
        buildUI()
      } catch {
        state.showNotification('Failed to delete prompt', 'error')
      }
    })
  }

  buildUI()
}
