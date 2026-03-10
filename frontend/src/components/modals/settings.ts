/**
 * Settings modal — STT, LLM, Audio configuration.
 */

import state from '../../state'
import { ConfigAPI, AudioAPI, LLMAPI, type ModelOption } from '../../api'
import { domain } from '../../../wailsjs/go/models'
import { escapeHtml } from '../../utils/html'
import { icon } from '../../utils/icons'

const LLM_MODEL_BY_PROVIDER_KEY = 'noti-llm-model-by-provider'

function modelLabel(model: ModelOption): string {
  return model.isRecommended ? `${model.name} (recommended)` : model.name
}

function sortByID(models: ModelOption[]): ModelOption[] {
  return [...models].sort((a, b) => a.id - b.id)
}

function noteForModel(models: ModelOption[], code: string): string {
  return models.find((m) => m.code === code)?.note?.trim() || ''
}

function providerNote(provider: string): string {
  if (provider === 'api') {
    return 'Sends transcript text and prompts to your configured API endpoint. Requires network access and a valid API key.'
  }
  return 'Runs inference locally with llama.cpp. Text stays on this device and works offline after model download.'
}

function temperatureNote(value: number): string {
  if (value <= 0.3) {
    return 'Lower temperature gives more deterministic and focused outputs.'
  }
  if (value <= 0.8) {
    return 'Balanced setting for stable outputs with moderate variation.'
  }
  return 'Higher temperature gives more creative but less predictable outputs.'
}

function audioSourceNote(source: string): string {
  if (source === 'microphone') {
    return 'Captures only microphone input.'
  }
  if (source === 'system') {
    return 'Captures only system playback audio.'
  }
  return 'Captures both microphone and system audio.'
}

function loadLLMModelByProvider(): Record<string, string> {
  try {
    const raw = localStorage.getItem(LLM_MODEL_BY_PROVIDER_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object') return {}
    return Object.entries(parsed as Record<string, unknown>).reduce<Record<string, string>>((acc, [k, v]) => {
      if (typeof v === 'string') acc[k] = v
      return acc
    }, {})
  } catch {
    return {}
  }
}

function saveLLMModelByProvider(modelsByProvider: Record<string, string>): void {
  try {
    localStorage.setItem(LLM_MODEL_BY_PROVIDER_KEY, JSON.stringify(modelsByProvider))
  } catch {
    // Ignore client-side persistence failures.
  }
}

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
  let sttModels: ModelOption[] = []
  let llmModels: ModelOption[] = []
  try {
    ;[sttModels, llmModels] = await Promise.all([
      AudioAPI.getSTTModels(),
      LLMAPI.getLLMModels(),
    ])
  } catch {
    // Fall back to empty lists — inputs will still render
  }

  sttModels = sortByID(sttModels)
  llmModels = sortByID(llmModels)

  const currentProvider = config.llm?.provider || 'local'
  const isLocal = currentProvider === 'local'
  const modelsByProvider = loadLLMModelByProvider()
  const configuredModelName = config.llm?.modelName || ''
  if (configuredModelName) {
    modelsByProvider[currentProvider] = configuredModelName
    saveLLMModelByProvider(modelsByProvider)
  }
  const localModelName = modelsByProvider.local || (currentProvider === 'local' ? configuredModelName : '')
  const apiModelName = modelsByProvider.api || (currentProvider === 'api' ? configuredModelName : '')

  // Build STT model options
  const sttModelOptions = sttModels.length > 0
    ? sttModels.map(m =>
        `<option value="${escapeHtml(m.code)}" ${config?.modelName === m.code ? 'selected' : ''}>${escapeHtml(modelLabel(m))}</option>`
      ).join('')
    : `<option value="${escapeHtml(config.modelName || '')}" selected>${escapeHtml(config.modelName || '')}</option>`

  // Build LLM local model options
  const llmLocalModelOptions = llmModels.length > 0
    ? llmModels.map(m =>
        `<option value="${escapeHtml(m.code)}" ${localModelName === m.code ? 'selected' : ''}>${escapeHtml(modelLabel(m))}</option>`
      ).join('')
    : `<option value="${escapeHtml(localModelName)}" selected>${escapeHtml(localModelName)}</option>`

  const sttNote = noteForModel(sttModels, config.modelName || sttModels[0]?.code || '')
  const llmNote = noteForModel(llmModels, localModelName)

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
              <span class="form-label-text">Whisper Model</span>
              <select id="stt-model" class="form-select">
                ${sttModelOptions}
              </select>
              <p id="stt-model-note" class="form-note${sttNote ? '' : ' hidden'}">${escapeHtml(sttNote)}</p>
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
              <p id="llm-provider-note" class="form-note">${escapeHtml(providerNote(currentProvider))}</p>
            </label>
            <label class="form-label" id="llm-model-label">
              <span class="form-label-text">Model Name</span>
              <div id="llm-model-container">
                ${isLocal
                  ? `<select id="llm-model" class="form-select">${llmLocalModelOptions}</select>`
                  : `<input id="llm-model" type="text" class="form-input" value="${escapeHtml(apiModelName)}" />`
                }
              </div>
              <p id="llm-model-note" class="form-note${isLocal && llmNote ? '' : ' hidden'}">${escapeHtml(llmNote)}</p>
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
              <p id="llm-temp-note" class="form-note">${escapeHtml(temperatureNote(config.llm?.temperature ?? 0.7))}</p>
            </label>
            <label class="form-label">
              <span class="form-label-text">Max Tokens</span>
              <input id="llm-tokens" type="number" class="form-input" min="50" max="8000" value="${config.llm?.maxTokens || 1024}" />
              <p id="llm-tokens-note" class="form-note">Caps completion length per response. Increase for longer outputs; keep lower for faster replies and tighter context budgeting.</p>
            </label>
          </div>
        </section>

        <section>
          <h3 class="form-section-title">Audio</h3>
          <div class="settings-section">
            <label class="form-label">
              <select id="audio-source" class="form-select">
                <option value="microphone" ${config.audio?.defaultSource === 'microphone' ? 'selected' : ''}>Microphone</option>
                <option value="system" ${config.audio?.defaultSource === 'system' ? 'selected' : ''}>System Audio</option>
                <option value="mixed" ${config.audio?.defaultSource === 'mixed' ? 'selected' : ''}>Mixed (Mic + System)</option>
              </select>
              <p id="audio-source-note" class="form-note">${escapeHtml(audioSourceNote(config.audio?.defaultSource || 'mixed'))}</p>
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
  const tempNoteEl = container.querySelector<HTMLElement>('#llm-temp-note')
  tempSlider?.addEventListener('input', () => {
    if (tempVal && tempSlider) tempVal.textContent = tempSlider.value
    if (tempNoteEl && tempSlider) tempNoteEl.textContent = temperatureNote(parseFloat(tempSlider.value))
  })

  const audioSourceSelect = container.querySelector<HTMLSelectElement>('#audio-source')
  const audioSourceNoteEl = container.querySelector<HTMLElement>('#audio-source-note')
  audioSourceSelect?.addEventListener('change', () => {
    if (!audioSourceNoteEl || !audioSourceSelect) return
    audioSourceNoteEl.textContent = audioSourceNote(audioSourceSelect.value)
  })

  // Provider change — toggle model input type and API field visibility
  const providerSelect = container.querySelector<HTMLSelectElement>('#llm-provider')
  const endpointLabel = container.querySelector<HTMLElement>('#llm-api-endpoint-label')
  const apiKeyLabel = container.querySelector<HTMLElement>('#llm-api-key-label')
  const providerNoteEl = container.querySelector<HTMLElement>('#llm-provider-note')
  const modelContainer = container.querySelector<HTMLElement>('#llm-model-container')
  const sttNoteEl = container.querySelector<HTMLElement>('#stt-model-note')
  const llmNoteEl = container.querySelector<HTMLElement>('#llm-model-note')
  let activeProvider = currentProvider

  function renderModelNotes(provider: string): void {
    const sttCode = container.querySelector<HTMLSelectElement>('#stt-model')?.value ?? config?.modelName ?? ''
    const sttText = noteForModel(sttModels, sttCode)
    if (sttNoteEl) {
      sttNoteEl.textContent = sttText
      sttNoteEl.classList.toggle('hidden', sttText === '')
    }

    if (!llmNoteEl) return
    if (provider !== 'local') {
      llmNoteEl.textContent = ''
      llmNoteEl.classList.add('hidden')
      return
    }
    const llmCode = modelContainer?.querySelector<HTMLInputElement | HTMLSelectElement>('#llm-model')?.value ?? ''
    const llmText = noteForModel(llmModels, llmCode)
    llmNoteEl.textContent = llmText
    llmNoteEl.classList.toggle('hidden', llmText === '')
  }

  function rememberModel(provider: string, modelName: string): void {
    modelsByProvider[provider] = modelName
    saveLLMModelByProvider(modelsByProvider)
  }

  function fallbackModelFor(provider: string): string {
    if (provider === 'local') {
      return llmModels[0]?.code ?? ''
    }
    return ''
  }

  function renderModelInput(provider: string, selectedValue: string): void {
    if (!modelContainer) return
    if (provider === 'local') {
      const safeValue = selectedValue || fallbackModelFor('local')
      if (llmModels.length === 0) {
        modelContainer.innerHTML = `<select id="llm-model" class="form-select"><option value="${escapeHtml(safeValue)}" selected>${escapeHtml(safeValue)}</option></select>`
        return
      }
      modelContainer.innerHTML = `<select id="llm-model" class="form-select">
        ${llmModels.map(m =>
          `<option value="${escapeHtml(m.code)}" ${m.code === safeValue ? 'selected' : ''}>${escapeHtml(modelLabel(m))}</option>`
        ).join('')}
      </select>`
      renderModelNotes(provider)
      return
    }
    modelContainer.innerHTML = `<input id="llm-model" type="text" class="form-input" value="${escapeHtml(selectedValue)}" />`
    renderModelNotes(provider)
  }

  function updateProviderUI(provider: string): void {
    const local = provider === 'local'
    const currentModelValue = modelContainer?.querySelector<HTMLInputElement | HTMLSelectElement>('#llm-model')?.value ?? ''
    rememberModel(activeProvider, currentModelValue)
    activeProvider = provider

    // Toggle API fields visibility
    if (local) {
      endpointLabel?.classList.add('hidden')
      apiKeyLabel?.classList.add('hidden')
    } else {
      endpointLabel?.classList.remove('hidden')
      apiKeyLabel?.classList.remove('hidden')
    }
    if (providerNoteEl) {
      providerNoteEl.textContent = providerNote(provider)
    }

    // Swap model input between select (local) and text input (api)
    const rememberedValue = modelsByProvider[provider] ?? fallbackModelFor(provider)
    renderModelInput(provider, rememberedValue)
    renderModelNotes(provider)
  }

  modelContainer?.addEventListener('input', (e) => {
    if (!(e.target instanceof HTMLInputElement) || e.target.id !== 'llm-model') return
    rememberModel(activeProvider, e.target.value)
  })
  modelContainer?.addEventListener('change', (e) => {
    if (!(e.target instanceof HTMLSelectElement) || e.target.id !== 'llm-model') return
    rememberModel(activeProvider, e.target.value)
    renderModelNotes(activeProvider)
  })

  container.querySelector<HTMLSelectElement>('#stt-model')?.addEventListener('change', () => {
    renderModelNotes(activeProvider)
  })

  providerSelect?.addEventListener('change', (e) => {
    updateProviderUI((e.target as HTMLSelectElement).value)
  })

  renderModelNotes(activeProvider)

  // Close buttons
  const close = () => state.closeModal()
  container.querySelector('#settings-close')?.addEventListener('click', close)
  container.querySelector('#settings-cancel')?.addEventListener('click', close)

  // Save
  container.querySelector('#settings-save')?.addEventListener('click', async () => {
    if (!config) return
    const selectedProvider = (container.querySelector<HTMLSelectElement>('#llm-provider')?.value ?? config.llm?.provider) as string
    const selectedModelName = (container.querySelector<HTMLInputElement | HTMLSelectElement>('#llm-model')?.value ?? config.llm?.modelName)
    rememberModel(selectedProvider, selectedModelName)

    const newConfig = domain.Config.createFrom({
      ...config,
      modelName: (container.querySelector<HTMLSelectElement>('#stt-model')?.value ?? config.modelName),
      llm: {
        ...config.llm,
        provider: selectedProvider,
        modelName: selectedModelName,
        apiEndpoint: (container.querySelector<HTMLInputElement>('#llm-endpoint')?.value ?? config.llm?.apiEndpoint),
        apiKey: (container.querySelector<HTMLInputElement>('#llm-apikey')?.value ?? config.llm?.apiKey),
        temperature: parseFloat(container.querySelector<HTMLInputElement>('#llm-temp')?.value ?? '0.7'),
        maxTokens: parseInt(container.querySelector<HTMLInputElement>('#llm-tokens')?.value ?? '1024'),
      },
      audio: Object.assign(config.audio ?? {}, {
        defaultSource: (container.querySelector<HTMLSelectElement>('#audio-source')?.value ?? config.audio?.defaultSource),
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
