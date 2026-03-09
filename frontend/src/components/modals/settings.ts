/**
 * Settings modal — STT, LLM, Audio configuration.
 */

import state from '../../state'
import { ConfigAPI, AudioAPI, LLMAPI } from '../../api'
import { domain } from '../../../wailsjs/go/models'
import { escapeHtml } from '../../utils/html'
import { icon } from '../../utils/icons'

// ─── Icons — see utils/icons.ts ────────────────────

export async function renderSettingsModal(container: HTMLElement): Promise<void> {
  let config: domain.Config | null = state.get('config')
  if (!config) {
    try {
      config = await ConfigAPI.get()
      state.setState({ config })
    } catch {
      state.showNotification('Failed to load settings', 'error')
      return
    }
  }

  // Fetch model lists in parallel
  let sttModels: string[] = []
  let llmModels: Array<Record<string, string>> = []
  try {
    ;[sttModels, llmModels] = await Promise.all([
      AudioAPI.getSTTModels(),
      LLMAPI.getLLMModels(),
    ])
  } catch {
    // Fall back to empty lists — inputs will still render
  }

  const currentProvider = config.llm?.provider || 'local'
  const isLocal = currentProvider === 'local'

  // Build STT model options
  const sttModelOptions = sttModels.length > 0
    ? sttModels.map(m =>
        `<option value="${escapeHtml(m)}" ${config?.modelName === m ? 'selected' : ''}>${escapeHtml(m)}</option>`
      ).join('')
    : `<option value="${escapeHtml(config.modelName || 'base.en')}" selected>${escapeHtml(config.modelName || 'base.en')}</option>`

  // Build LLM local model options
  const llmLocalModelOptions = llmModels.length > 0
    ? llmModels.map(m =>
        `<option value="${escapeHtml(m.name)}" ${config?.llm?.modelName === m.name ? 'selected' : ''}>${escapeHtml(m.name)} — ${escapeHtml(m.description)}</option>`
      ).join('')
    : `<option value="${escapeHtml(config.llm?.modelName || '')}" selected>${escapeHtml(config.llm?.modelName || '')}</option>`

  container.innerHTML = `
    <div class="modal-card modal-card-settings">
      <div class="modal-header">
        <h2 class="modal-heading">Settings</h2>
        <button id="settings-close" class="btn-icon">${icon('x', 16)}</button>
      </div>

      <div class="modal-body">
        <section>
          <h3 class="form-section-title">Speech-to-Text</h3>
          <div class="settings-section">
            <label class="form-label">
              <span class="form-label-text">Model</span>
              <select id="stt-model" class="form-select">
                ${sttModelOptions}
              </select>
            </label>
          </div>
        </section>

        <section>
          <h3 class="form-section-title">AI / LLM</h3>
          <div class="settings-section">
            <label class="form-label">
              <span class="form-label-text">Provider</span>
              <select id="llm-provider" class="form-select">
                <option value="local" ${currentProvider === 'local' ? 'selected' : ''}>Local (llama.cpp)</option>
                <option value="api" ${currentProvider === 'api' ? 'selected' : ''}>API (OpenAI / compatible)</option>
              </select>
            </label>
            <label class="form-label" id="llm-model-label">
              <span class="form-label-text">Model Name</span>
              <div id="llm-model-container">
                ${isLocal
                  ? `<select id="llm-model" class="form-select">${llmLocalModelOptions}</select>`
                  : `<input id="llm-model" type="text" class="form-input" value="${escapeHtml(config.llm?.modelName || '')}" />`
                }
              </div>
            </label>
            <label id="llm-api-endpoint-label" class="form-label${isLocal ? ' hidden' : ''}">
              <span class="form-label-text">API Endpoint</span>
              <input id="llm-endpoint" type="text" class="form-input" value="${escapeHtml(config.llm?.apiEndpoint || '')}" placeholder="http://localhost:8080" />
            </label>
            <label id="llm-api-key-label" class="form-label${isLocal ? ' hidden' : ''}">
              <span class="form-label-text">API Key</span>
              <input id="llm-apikey" type="password" class="form-input" value="${escapeHtml(config.llm?.apiKey || '')}" placeholder="sk-…" />
            </label>
            <label class="form-label">
              <span class="form-label-text">Temperature: <span id="temp-val">${config.llm?.temperature ?? 0.7}</span></span>
              <input id="llm-temp" type="range" min="0" max="2" step="0.1" value="${config.llm?.temperature ?? 0.7}" class="settings-range" />
            </label>
            <label class="form-label">
              <span class="form-label-text">Max Tokens</span>
              <input id="llm-tokens" type="number" class="form-input" min="50" max="8000" value="${config.llm?.maxTokens || 1024}" />
            </label>
          </div>
        </section>

        <section>
          <h3 class="form-section-title">Audio</h3>
          <div class="settings-section">
            <label class="form-label">
              <span class="form-label-text">Default Source</span>
              <select id="audio-source" class="form-select">
                <option value="microphone" ${config.audio?.defaultSource === 'microphone' ? 'selected' : ''}>Microphone</option>
                <option value="system" ${config.audio?.defaultSource === 'system' ? 'selected' : ''}>System Audio</option>
                <option value="mixed" ${config.audio?.defaultSource === 'mixed' ? 'selected' : ''}>Mixed (Mic + System)</option>
              </select>
            </label>
            <label class="form-label">
              <span class="form-label-text">Sample Rate</span>
              <select id="audio-samplerate" class="form-select">
                ${[8000, 16000, 22050, 44100, 48000].map(r =>
                  `<option value="${r}" ${config?.audio?.sampleRate === r ? 'selected' : ''}>${r} Hz</option>`
                ).join('')}
              </select>
            </label>
          </div>
        </section>
      </div>

      <div class="modal-footer">
        <button id="settings-cancel" class="btn-secondary">Cancel</button>
        <button id="settings-save" class="btn-primary">Save Settings</button>
      </div>
    </div>
  `

  // Live slider updates
  const tempSlider = container.querySelector<HTMLInputElement>('#llm-temp')
  const tempVal = container.querySelector<HTMLElement>('#temp-val')
  tempSlider?.addEventListener('input', () => { if (tempVal && tempSlider) tempVal.textContent = tempSlider.value })

  // Provider change — toggle model input type and API field visibility
  const providerSelect = container.querySelector<HTMLSelectElement>('#llm-provider')
  const endpointLabel = container.querySelector<HTMLElement>('#llm-api-endpoint-label')
  const apiKeyLabel = container.querySelector<HTMLElement>('#llm-api-key-label')
  const modelContainer = container.querySelector<HTMLElement>('#llm-model-container')

  function updateProviderUI(provider: string): void {
    const local = provider === 'local'

    // Toggle API fields visibility
    if (local) {
      endpointLabel?.classList.add('hidden')
      apiKeyLabel?.classList.add('hidden')
    } else {
      endpointLabel?.classList.remove('hidden')
      apiKeyLabel?.classList.remove('hidden')
    }

    // Swap model input between select (local) and text input (api)
    if (modelContainer) {
      const currentValue = modelContainer.querySelector<HTMLInputElement | HTMLSelectElement>('#llm-model')?.value ?? ''
      if (local) {
        // Find the best matching option or fall back to first
        const matchedOption = llmModels.find(m => m.name === currentValue)
        const selectedValue = matchedOption ? currentValue : (llmModels[0]?.name ?? '')
        modelContainer.innerHTML = `<select id="llm-model" class="form-select">
          ${llmModels.map(m =>
            `<option value="${escapeHtml(m.name)}" ${m.name === selectedValue ? 'selected' : ''}>${escapeHtml(m.name)} — ${escapeHtml(m.description)}</option>`
          ).join('')}
        </select>`
      } else {
        modelContainer.innerHTML = `<input id="llm-model" type="text" class="form-input" value="${escapeHtml(currentValue)}" />`
      }
    }
  }

  providerSelect?.addEventListener('change', (e) => {
    updateProviderUI((e.target as HTMLSelectElement).value)
  })

  // Close buttons
  const close = () => state.closeModal()
  container.querySelector('#settings-close')?.addEventListener('click', close)
  container.querySelector('#settings-cancel')?.addEventListener('click', close)

  // Save
  container.querySelector('#settings-save')?.addEventListener('click', async () => {
    if (!config) return

    const newConfig = domain.Config.createFrom({
      ...config,
      modelName: (container.querySelector<HTMLSelectElement>('#stt-model')?.value ?? config.modelName),
      llm: {
        ...config.llm,
        provider: (container.querySelector<HTMLSelectElement>('#llm-provider')?.value ?? config.llm?.provider) as string,
        modelName: (container.querySelector<HTMLInputElement | HTMLSelectElement>('#llm-model')?.value ?? config.llm?.modelName),
        apiEndpoint: (container.querySelector<HTMLInputElement>('#llm-endpoint')?.value ?? config.llm?.apiEndpoint),
        apiKey: (container.querySelector<HTMLInputElement>('#llm-apikey')?.value ?? config.llm?.apiKey),
        temperature: parseFloat(container.querySelector<HTMLInputElement>('#llm-temp')?.value ?? '0.7'),
        maxTokens: parseInt(container.querySelector<HTMLInputElement>('#llm-tokens')?.value ?? '1024'),
      },
      audio: Object.assign(config.audio ?? {}, {
        defaultSource: (container.querySelector<HTMLSelectElement>('#audio-source')?.value ?? config.audio?.defaultSource),
        sampleRate: parseInt(container.querySelector<HTMLSelectElement>('#audio-samplerate')?.value ?? '44100'),
        mixer: config.audio?.mixer,
      }),
    })
    try {
      await ConfigAPI.save(newConfig)
      state.setState({ config: newConfig })
      state.showNotification('Settings saved', 'success')
      state.closeModal()
    } catch {
      state.showNotification('Failed to save settings', 'error')
    }
  })
}
