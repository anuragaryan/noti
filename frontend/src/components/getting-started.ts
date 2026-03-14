import { domain } from '../../wailsjs/go/models'
import { AudioAPI, ConfigAPI, LLMAPI, type ModelOption } from '../api'
import state from '../state'
import { escapeHtml } from '../utils/html'
import { icon } from '../utils/icons'
import { saveConfigWithPending } from '../utils/config-save'

const RECOMMENDED = {
  llmProvider: 'local',
  source: 'mixed',
} as const

type SourceOption = 'microphone' | 'system' | 'mixed'

const STT_LANGUAGES: Array<{ code: string; name: string }> = [
  { code: 'auto', name: 'Auto Detect' },
  { code: 'en', name: 'English' },
  { code: 'es', name: 'Spanish' },
  { code: 'fr', name: 'French' },
  { code: 'de', name: 'German' },
  { code: 'it', name: 'Italian' },
  { code: 'pt', name: 'Portuguese' },
  { code: 'nl', name: 'Dutch' },
  { code: 'ru', name: 'Russian' },
  { code: 'hi', name: 'Hindi' },
  { code: 'ja', name: 'Japanese' },
  { code: 'ko', name: 'Korean' },
  { code: 'zh', name: 'Chinese' },
  { code: 'ar', name: 'Arabic' },
]

function labelWithRecommendation(value: string, recommended: string): string {
  return value === recommended ? `${value} (recommended)` : value
}

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

function llmProviderLabel(provider: string): string {
  if (provider === 'local') return 'Local'
  if (provider === 'api') return 'API'
  return provider || 'Local'
}

function sourceLabel(source: SourceOption): string {
  if (source === 'microphone') return 'Mic only'
  if (source === 'system') return 'System only'
  return 'Mixed'
}

function sourceHint(source: SourceOption): string {
  if (source === 'microphone') {
    return 'Captures your microphone only. Best for personal voice notes and dictation.'
  }
  if (source === 'system') {
    return 'Captures system playback audio only. Best for meetings, calls, and media playback.'
  }
  return 'Captures microphone and system audio together. Best when you need both your voice and playback audio.'
}

function renderSourceOptions(selectedSource: SourceOption): string {
  const options: SourceOption[] = ['microphone', 'system', 'mixed']

  return options.map((option) => {
    const active = option === selectedSource
    const label = labelWithRecommendation(sourceLabel(option), sourceLabel(RECOMMENDED.source))
    return `
      <button
        type="button"
        class="gs-source-pill${active ? ' active' : ''}"
        data-source="${option}"
      >
        ${escapeHtml(label)}
      </button>
    `
  }).join('')
}

function sttOptionsHtml(sttModels: ModelOption[], selectedModel: string): string {
  const models = sttModels.length > 0
    ? sttModels
    : [{ id: 0, code: selectedModel, name: selectedModel, isRecommended: false, note: '' }]
  return models.map((model) => {
    return `<option value="${escapeHtml(model.code)}" ${model.code === selectedModel ? 'selected' : ''}>${escapeHtml(modelLabel(model))}</option>`
  }).join('')
}

function sttLanguageOptionsHtml(selectedLanguage: string): string {
  return STT_LANGUAGES.map((language) => {
    return `<option value="${escapeHtml(language.code)}" ${language.code === selectedLanguage ? 'selected' : ''}>${escapeHtml(language.name)}</option>`
  }).join('')
}

function localModelOptionsHtml(models: ModelOption[], selectedModel: string): string {
  if (models.length === 0) {
    return `<option value="${escapeHtml(selectedModel)}" selected>${escapeHtml(selectedModel || 'No models available')}</option>`
  }

  const exists = models.some((m) => m.code === selectedModel)
  const withFallback = exists || !selectedModel
    ? models
    : [{ id: 0, code: selectedModel, name: selectedModel, isRecommended: false, note: '' }, ...models]

  return withFallback.map((m) => {
    return `<option value="${escapeHtml(m.code)}" ${m.code === selectedModel ? 'selected' : ''}>${escapeHtml(modelLabel(m))}</option>`
  }).join('')
}

function renderScreen(
  container: HTMLElement,
  config: domain.Config,
  selectedSource: SourceOption,
  sttModels: ModelOption[],
  llmModels: ModelOption[],
  initialAPIModels: ModelOption[],
): void {
  const sortedSTTModels = sortByID(sttModels)
  const sortedLLMModels = sortByID(llmModels)
  const currentProvider = config.llm?.provider === 'api' ? 'api' : 'local'
  const currentSTTModel = config.modelName || sortedSTTModels[0]?.code || ''
  const currentSTTLanguage = config.sttLanguage || 'en'
  const fallbackLocalModel = sortedLLMModels[0]?.code || config.llm?.modelName || ''
  const sortedAPIModels = sortByID(initialAPIModels)
  const rememberedModels: Record<string, string> = {
    local: currentProvider === 'local' ? (config.llm?.modelName || fallbackLocalModel) : fallbackLocalModel,
    api: currentProvider === 'api'
      ? (config.llm?.modelName || sortedAPIModels[0]?.code || '')
      : (sortedAPIModels[0]?.code || ''),
  }
  let activeProvider = currentProvider
  let apiModels = sortedAPIModels
  let apiModelMessage = currentProvider === 'api' && (config.llm?.apiEndpoint || '').trim() === ''
    ? 'Enter API endpoint, then click refresh to load hosted models.'
    : ''
  let apiModelLoading = false

  container.innerHTML = `
    <div class="getting-started-wrap">
      <h2 class="getting-started-title">Getting Started in 30 seconds</h2>
      <p class="getting-started-desc">
        These settings control transcription quality, speed, and privacy. Start with recommended defaults - you can change everything later in Settings.
      </p>

      <div class="getting-started-card">
        <section class="getting-started-section">
          <h3 class="getting-started-heading">Speech-to-Text</h3>
          <div class="getting-started-field">
            <span class="getting-started-label">Model</span>
            <select id="gs-stt-model" class="form-select">
              ${sttOptionsHtml(sortedSTTModels, currentSTTModel)}
            </select>
            <p id="gs-stt-note" class="getting-started-help"></p>
          </div>
          <div class="getting-started-field">
            <span class="getting-started-label">Language</span>
            <select id="gs-stt-language" class="form-select">
              ${sttLanguageOptionsHtml(currentSTTLanguage)}
            </select>
          </div>
        </section>

        <div class="getting-started-divider"></div>

        <section class="getting-started-section">
          <h3 class="getting-started-heading">AI Processing</h3>
          <div class="getting-started-row">
            <div class="getting-started-field">
              <span class="getting-started-label">Provider</span>
              <select id="gs-llm-provider" class="form-select">
                <option value="local" ${currentProvider === 'local' ? 'selected' : ''}>${escapeHtml(labelWithRecommendation('Local (llama.cpp)', 'Local (llama.cpp)'))}</option>
                <option value="api" ${currentProvider === 'api' ? 'selected' : ''}>API (OpenAI / compatible)</option>
              </select>
              <p id="gs-provider-note" class="getting-started-help">${escapeHtml(providerNote(currentProvider))}</p>
            </div>
            <div class="getting-started-field">
              <span class="getting-started-label">Model</span>
              <div id="gs-llm-model-container">
                ${currentProvider === 'local'
                  ? `<select id="gs-llm-model" class="form-select">${localModelOptionsHtml(sortedLLMModels, rememberedModels.local)}</select>`
                  : `<div class="model-select-row"><select id="gs-llm-model" class="form-select">${localModelOptionsHtml(apiModels, rememberedModels.api)}</select><button id="gs-llm-model-refresh" type="button" class="btn-icon model-refresh-btn" title="Refresh models">${icon('refresh-cw', 14)}</button></div>`
                }
              </div>
              <p id="gs-llm-note" class="getting-started-help"></p>
              <p id="gs-api-model-status" class="getting-started-help${currentProvider === 'api' && apiModelMessage ? '' : ' hidden'}">${escapeHtml(apiModelMessage)}</p>
            </div>
          </div>
          <div id="gs-api-extra" class="getting-started-row${currentProvider === 'api' ? '' : ' hidden'}">
            <label class="getting-started-field">
              <span class="getting-started-label">API Endpoint</span>
              <input id="gs-api-endpoint" type="text" class="form-input" value="${escapeHtml(config.llm?.apiEndpoint || '')}" placeholder="http://localhost:8080" />
            </label>
            <label class="getting-started-field">
              <span class="getting-started-label">Security Key</span>
              <input id="gs-api-key" type="password" class="form-input" value="${escapeHtml(config.llm?.apiKey || '')}" placeholder="sk-..." />
            </label>
          </div>
        </section>

        <div class="getting-started-divider"></div>

        <section class="getting-started-section">
          <h3 class="getting-started-heading">Input Source</h3>
          <div class="getting-started-source-row">
            ${renderSourceOptions(selectedSource)}
          </div>
          <p class="getting-started-source-hint" id="gs-source-hint">${escapeHtml(sourceHint(selectedSource))}</p>
        </section>

        <div class="getting-started-divider"></div>

        <p class="getting-started-note">You can tune all values anytime in Settings.</p>

        <div class="getting-started-actions">
          <button id="gs-start" class="btn-primary">Start transcribing</button>
        </div>
      </div>
    </div>
  `

  const providerSelect = container.querySelector<HTMLSelectElement>('#gs-llm-provider')
  const modelContainer = container.querySelector<HTMLElement>('#gs-llm-model-container')
  const apiExtra = container.querySelector<HTMLElement>('#gs-api-extra')
  const providerNoteEl = container.querySelector<HTMLElement>('#gs-provider-note')
  const sttNote = container.querySelector<HTMLElement>('#gs-stt-note')
  const llmNote = container.querySelector<HTMLElement>('#gs-llm-note')
  const apiModelStatusEl = container.querySelector<HTMLElement>('#gs-api-model-status')
  const startButton = container.querySelector<HTMLButtonElement>('#gs-start')

  const setStartButtonLoading = (loading: boolean): void => {
    if (!startButton) return
    startButton.disabled = loading
    if (loading) {
      startButton.classList.add('icon-spin')
      startButton.innerHTML = `${icon('loader', 14)} Applying...`
      return
    }
    startButton.classList.remove('icon-spin')
    startButton.textContent = 'Start transcribing'
  }
  setStartButtonLoading(state.get('isConfigSaving'))

  const unsubscribeConfigSaving = state.subscribe('isConfigSaving', () => {
    if (!startButton?.isConnected) return
    setStartButtonLoading(state.get('isConfigSaving'))
  })
  const unsubscribeGettingStarted = state.subscribe('showGettingStarted', () => {
    if (state.get('showGettingStarted')) return
    unsubscribeConfigSaving()
    unsubscribeGettingStarted()
  })

  const renderNotes = (): void => {
    const sttCode = container.querySelector<HTMLSelectElement>('#gs-stt-model')?.value ?? currentSTTModel
    const sttText = noteForModel(sortedSTTModels, sttCode)
    if (sttNote) {
      sttNote.textContent = sttText
      sttNote.classList.toggle('hidden', sttText === '')
    }

    if (llmNote) {
      if (activeProvider !== 'local') {
        llmNote.textContent = ''
        llmNote.classList.add('hidden')
      } else {
        const llmCode = container.querySelector<HTMLInputElement | HTMLSelectElement>('#gs-llm-model')?.value ?? rememberedModels.local
        const llmText = noteForModel(sortedLLMModels, llmCode)
        llmNote.textContent = llmText
        llmNote.classList.toggle('hidden', llmText === '')
      }
    }

    if (!apiModelStatusEl) return
    if (activeProvider !== 'api') {
      apiModelStatusEl.textContent = ''
      apiModelStatusEl.classList.add('hidden')
      return
    }
    apiModelStatusEl.textContent = apiModelMessage
    apiModelStatusEl.classList.toggle('hidden', apiModelMessage.trim() === '')
  }

  const renderModelInput = (provider: string): void => {
    if (!modelContainer) return
    const remembered = rememberedModels[provider] || (provider === 'local' ? fallbackLocalModel : '')
    if (provider === 'local') {
      modelContainer.innerHTML = `<select id="gs-llm-model" class="form-select">${localModelOptionsHtml(sortedLLMModels, remembered)}</select>`
      renderNotes()
      return
    }
    const modelControl = apiModels.length > 0
      ? `<select id="gs-llm-model" class="form-select">${localModelOptionsHtml(apiModels, remembered)}</select>`
      : `<input id="gs-llm-model" type="text" class="form-input" value="${escapeHtml(remembered)}" placeholder="gpt-4o-mini" />`
    modelContainer.innerHTML = `<div class="model-select-row">${modelControl}<button id="gs-llm-model-refresh" type="button" class="btn-icon model-refresh-btn${apiModelLoading ? ' icon-spin' : ''}" title="Refresh models" ${apiModelLoading ? 'disabled' : ''}>${icon('refresh-cw', 14)}</button></div>`
    renderNotes()
  }

  const refreshAPIModels = async (): Promise<void> => {
    const endpoint = container.querySelector<HTMLInputElement>('#gs-api-endpoint')?.value ?? ''
    const apiKey = container.querySelector<HTMLInputElement>('#gs-api-key')?.value ?? ''
    if (endpoint.trim() === '') {
      apiModelMessage = 'Enter API endpoint, then click refresh to load hosted models.'
      renderNotes()
      return
    }

    apiModelLoading = true
    apiModelMessage = 'Loading models...'
    const selectedValue = modelContainer?.querySelector<HTMLSelectElement>('#gs-llm-model')?.value ?? rememberedModels.api ?? ''
    if (activeProvider === 'api') {
      renderModelInput('api')
    }

    try {
      const fetched = sortByID(await LLMAPI.getAPILLMModels(endpoint, apiKey))
      apiModels = fetched
      const nextValue = fetched.some((m) => m.code === selectedValue)
        ? selectedValue
        : (fetched[0]?.code ?? selectedValue)
      rememberedModels.api = nextValue
      apiModelMessage = fetched.length > 0
        ? `Loaded ${fetched.length} hosted model${fetched.length === 1 ? '' : 's'}.`
        : 'No models were returned by this endpoint. Enter a model ID manually.'
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      apiModels = []
      apiModelMessage = `Failed to load API models: ${message}. Enter a model ID manually.`
    } finally {
      apiModelLoading = false
      if (activeProvider === 'api') {
        renderModelInput('api')
      }
    }
  }

  providerSelect?.addEventListener('change', () => {
    const currentModelValue = modelContainer?.querySelector<HTMLInputElement | HTMLSelectElement>('#gs-llm-model')?.value ?? ''
    rememberedModels[activeProvider] = currentModelValue
    activeProvider = providerSelect.value === 'api' ? 'api' : 'local'
    if (providerNoteEl) providerNoteEl.textContent = providerNote(activeProvider)
    apiExtra?.classList.toggle('hidden', activeProvider !== 'api')
    renderModelInput(activeProvider)
    if (activeProvider === 'api' && apiModels.length === 0) {
      void refreshAPIModels()
    }
  })

  container.querySelector<HTMLSelectElement>('#gs-stt-model')?.addEventListener('change', () => {
    renderNotes()
  })

  modelContainer?.addEventListener('input', (e) => {
    if (!(e.target instanceof HTMLInputElement) || e.target.id !== 'gs-llm-model') return
    rememberedModels[activeProvider] = e.target.value
  })

  modelContainer?.addEventListener('change', (e) => {
    if (!(e.target instanceof HTMLSelectElement) || e.target.id !== 'gs-llm-model') return
    rememberedModels[activeProvider] = e.target.value
    renderNotes()
  })

  modelContainer?.addEventListener('click', (e) => {
    if (!(e.target instanceof Element)) return
    const refreshButton = e.target.closest<HTMLButtonElement>('#gs-llm-model-refresh')
    if (!refreshButton) return
    void refreshAPIModels()
  })

  renderNotes()
  if (activeProvider === 'api' && apiModels.length === 0 && (config.llm?.apiEndpoint || '').trim() !== '') {
    void refreshAPIModels()
  }

  container.querySelectorAll<HTMLButtonElement>('[data-source]').forEach((button) => {
    button.addEventListener('click', () => {
      if (!(button.dataset.source === 'microphone' || button.dataset.source === 'system' || button.dataset.source === 'mixed')) return
      const source = button.dataset.source as SourceOption
      container.querySelectorAll<HTMLButtonElement>('[data-source]').forEach((node) => node.classList.remove('active'))
      button.classList.add('active')
      const hint = container.querySelector<HTMLElement>('#gs-source-hint')
      if (hint) hint.textContent = sourceHint(source)
    })
  })

  startButton?.addEventListener('click', async () => {
    if (state.get('isConfigSaving')) return
    const source = (container.querySelector<HTMLButtonElement>('.gs-source-pill.active')?.dataset.source as SourceOption | undefined) ?? selectedSource
    const sttModel = container.querySelector<HTMLSelectElement>('#gs-stt-model')?.value ?? currentSTTModel
    const sttLanguage = container.querySelector<HTMLSelectElement>('#gs-stt-language')?.value ?? currentSTTLanguage
    const selectedProvider = container.querySelector<HTMLSelectElement>('#gs-llm-provider')?.value === 'api' ? 'api' : 'local'
    const selectedModel = container.querySelector<HTMLInputElement | HTMLSelectElement>('#gs-llm-model')?.value ?? rememberedModels[selectedProvider]
    const endpoint = container.querySelector<HTMLInputElement>('#gs-api-endpoint')?.value ?? config.llm?.apiEndpoint ?? ''
    const apiKey = container.querySelector<HTMLInputElement>('#gs-api-key')?.value ?? config.llm?.apiKey ?? ''

    const nextConfig = domain.Config.createFrom({
      ...config,
      modelName: sttModel,
      sttLanguage,
      llm: {
        ...(config.llm ?? {}),
        provider: selectedProvider,
        modelName: selectedModel,
        apiEndpoint: selectedProvider === 'api' ? endpoint : '',
        apiKey: selectedProvider === 'api' ? apiKey : '',
      },
      audio: {
        ...(config.audio ?? {}),
        defaultSource: source,
        mixer: config.audio?.mixer,
      },
    })

    try {
      await saveConfigWithPending(nextConfig)
      state.setState({
        recordingSource: source,
        showGettingStarted: false,
      })
      state.showNotification('Setup complete', 'success')
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      state.showNotification(`Failed to save setup: ${message}`, 'error')
    }
  })
}

export async function renderGettingStarted(container: HTMLElement): Promise<void> {
  const config = state.get('config')
  if (!config) return

  let sttModels: ModelOption[] = []
  let llmModels: ModelOption[] = []
  let apiModels: ModelOption[] = []
  const apiProviderSelected = (config.llm?.provider || 'local') === 'api'
  const hasEndpoint = (config.llm?.apiEndpoint || '').trim() !== ''
  try {
    ;[sttModels, llmModels] = await Promise.all([
      AudioAPI.getSTTModels(),
      LLMAPI.getLLMModels(),
    ])
  } catch {
    sttModels = []
    llmModels = []
  }

  if (apiProviderSelected && hasEndpoint) {
    try {
      apiModels = await LLMAPI.getAPILLMModels(config.llm?.apiEndpoint || '', config.llm?.apiKey || '')
    } catch {
      apiModels = []
    }
  }

  const configuredSource = (config.audio?.defaultSource || 'mixed') as SourceOption
  const selectedSource: SourceOption = configuredSource === 'microphone' || configuredSource === 'system' || configuredSource === 'mixed'
    ? configuredSource
    : 'mixed'

  renderScreen(container, config, selectedSource, sttModels, llmModels, apiModels)
}
