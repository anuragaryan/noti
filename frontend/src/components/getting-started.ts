import { domain } from '../../wailsjs/go/models'
import { AudioAPI, ConfigAPI, LLMAPI, type ModelOption } from '../api'
import state from '../state'
import { escapeHtml } from '../utils/html'

const RECOMMENDED = {
  llmProvider: 'local',
  source: 'mixed',
} as const

type SourceOption = 'microphone' | 'system' | 'mixed'

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

function localModelOptionsHtml(models: ModelOption[], selectedModel: string): string {
  if (models.length === 0) {
    return `<option value="${escapeHtml(selectedModel)}" selected>${escapeHtml(selectedModel)}</option>`
  }

  return models.map((m) => {
    return `<option value="${escapeHtml(m.code)}" ${m.code === selectedModel ? 'selected' : ''}>${escapeHtml(modelLabel(m))}</option>`
  }).join('')
}

function renderScreen(
  container: HTMLElement,
  config: domain.Config,
  selectedSource: SourceOption,
  sttModels: ModelOption[],
  llmModels: ModelOption[],
): void {
  const sortedSTTModels = sortByID(sttModels)
  const sortedLLMModels = sortByID(llmModels)
  const currentProvider = config.llm?.provider === 'api' ? 'api' : 'local'
  const currentSTTModel = config.modelName || sortedSTTModels[0]?.code || ''
  const fallbackLocalModel = sortedLLMModels[0]?.code || config.llm?.modelName || ''
  const rememberedModels: Record<string, string> = {
    local: currentProvider === 'local' ? (config.llm?.modelName || fallbackLocalModel) : fallbackLocalModel,
    api: currentProvider === 'api' ? (config.llm?.modelName || '') : '',
  }
  let activeProvider = currentProvider

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
                <select id="gs-llm-model" class="form-select">
                  ${localModelOptionsHtml(sortedLLMModels, rememberedModels.local)}
                </select>
              </div>
              <p id="gs-llm-note" class="getting-started-help"></p>
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

  const renderNotes = (): void => {
    const sttCode = container.querySelector<HTMLSelectElement>('#gs-stt-model')?.value ?? currentSTTModel
    const sttText = noteForModel(sortedSTTModels, sttCode)
    if (sttNote) {
      sttNote.textContent = sttText
      sttNote.classList.toggle('hidden', sttText === '')
    }

    if (!llmNote) return
    if (activeProvider !== 'local') {
      llmNote.textContent = ''
      llmNote.classList.add('hidden')
      return
    }
    const llmCode = container.querySelector<HTMLInputElement | HTMLSelectElement>('#gs-llm-model')?.value ?? rememberedModels.local
    const llmText = noteForModel(sortedLLMModels, llmCode)
    llmNote.textContent = llmText
    llmNote.classList.toggle('hidden', llmText === '')
  }

  const renderModelInput = (provider: string): void => {
    if (!modelContainer) return
    const remembered = rememberedModels[provider] || (provider === 'local' ? fallbackLocalModel : '')
    if (provider === 'local') {
      modelContainer.innerHTML = `<select id="gs-llm-model" class="form-select">${localModelOptionsHtml(sortedLLMModels, remembered)}</select>`
      renderNotes()
      return
    }
    modelContainer.innerHTML = `<input id="gs-llm-model" type="text" class="form-input" value="${escapeHtml(remembered)}" placeholder="gpt-4o-mini" />`
    renderNotes()
  }

  providerSelect?.addEventListener('change', () => {
    const currentModelValue = modelContainer?.querySelector<HTMLInputElement | HTMLSelectElement>('#gs-llm-model')?.value ?? ''
    rememberedModels[activeProvider] = currentModelValue
    activeProvider = providerSelect.value === 'api' ? 'api' : 'local'
    if (providerNoteEl) providerNoteEl.textContent = providerNote(activeProvider)
    apiExtra?.classList.toggle('hidden', activeProvider !== 'api')
    renderModelInput(activeProvider)
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

  renderNotes()

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

  container.querySelector<HTMLButtonElement>('#gs-start')?.addEventListener('click', async () => {
    const source = (container.querySelector<HTMLButtonElement>('.gs-source-pill.active')?.dataset.source as SourceOption | undefined) ?? selectedSource
    const sttModel = container.querySelector<HTMLSelectElement>('#gs-stt-model')?.value ?? currentSTTModel
    const selectedProvider = container.querySelector<HTMLSelectElement>('#gs-llm-provider')?.value === 'api' ? 'api' : 'local'
    const selectedModel = container.querySelector<HTMLInputElement | HTMLSelectElement>('#gs-llm-model')?.value ?? rememberedModels[selectedProvider]
    const endpoint = container.querySelector<HTMLInputElement>('#gs-api-endpoint')?.value ?? config.llm?.apiEndpoint ?? ''
    const apiKey = container.querySelector<HTMLInputElement>('#gs-api-key')?.value ?? config.llm?.apiKey ?? ''

    const nextConfig = domain.Config.createFrom({
      ...config,
      modelName: sttModel,
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
      await ConfigAPI.save(nextConfig)
      state.setState({
        config: nextConfig,
        recordingSource: source,
        showGettingStarted: false,
      })
      state.showNotification('Setup complete', 'success')
    } catch {
      state.showNotification('Failed to save setup', 'error')
    }
  })
}

export async function renderGettingStarted(container: HTMLElement): Promise<void> {
  const config = state.get('config')
  if (!config) return

  let sttModels: ModelOption[] = []
  let llmModels: ModelOption[] = []
  try {
    ;[sttModels, llmModels] = await Promise.all([
      AudioAPI.getSTTModels(),
      LLMAPI.getLLMModels(),
    ])
  } catch {
    sttModels = []
    llmModels = []
  }

  const configuredSource = (config.audio?.defaultSource || 'mixed') as SourceOption
  const selectedSource: SourceOption = configuredSource === 'microphone' || configuredSource === 'system' || configuredSource === 'mixed'
    ? configuredSource
    : 'mixed'

  renderScreen(container, config, selectedSource, sttModels, llmModels)
}
