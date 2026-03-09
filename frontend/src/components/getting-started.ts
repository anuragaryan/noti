import { domain } from '../../wailsjs/go/models'
import { AudioAPI, ConfigAPI, LLMAPI } from '../api'
import state from '../state'
import { escapeHtml } from '../utils/html'

const RECOMMENDED = {
  sttModel: 'large-v3-turbo-q5_0',
  llmProvider: 'local',
  llmModel: 'Qwen3.5-4B-UD-Q4_K_XL',
  source: 'mixed',
} as const

type SourceOption = 'microphone' | 'system' | 'mixed'

function labelWithRecommendation(value: string, recommended: string): string {
  return value === recommended ? `${value} (recommended)` : value
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
    return 'For the selected source (Mic only): captures your microphone. Best for personal voice notes and dictation.'
  }
  if (source === 'system') {
    return 'For the selected source (System only): captures system playback audio. Best for recorded meetings and media.'
  }
  return 'For the selected source (Mixed): captures microphone and system audio. Best for meetings, calls, and recordings.'
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

function sttOptionsHtml(sttModels: string[], selectedModel: string): string {
  const models = sttModels.length > 0 ? sttModels : [selectedModel]
  return models.map((model) => {
    const label = labelWithRecommendation(model, RECOMMENDED.sttModel)
    return `<option value="${escapeHtml(model)}" ${model === selectedModel ? 'selected' : ''}>${escapeHtml(label)}</option>`
  }).join('')
}

function localModelOptionsHtml(models: Array<Record<string, string>>, selectedModel: string): string {
  if (models.length === 0) {
    const label = labelWithRecommendation(selectedModel, RECOMMENDED.llmModel)
    return `<option value="${escapeHtml(selectedModel)}" selected>${escapeHtml(label)}</option>`
  }

  return models.map((m) => {
    const name = String(m.name || '')
    const label = labelWithRecommendation(name, RECOMMENDED.llmModel)
    return `<option value="${escapeHtml(name)}" ${name === selectedModel ? 'selected' : ''}>${escapeHtml(label)}</option>`
  }).join('')
}

function renderScreen(
  container: HTMLElement,
  config: domain.Config,
  selectedSource: SourceOption,
  sttModels: string[],
  llmModels: Array<Record<string, string>>,
): void {
  const currentProvider = config.llm?.provider === 'api' ? 'api' : 'local'
  const currentSTTModel = config.modelName || sttModels[0] || RECOMMENDED.sttModel
  const fallbackLocalModel = llmModels[0]?.name || config.llm?.modelName || RECOMMENDED.llmModel
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
              ${sttOptionsHtml(sttModels, currentSTTModel)}
            </select>
            <p class="getting-started-help">For the selected model: strong accuracy with good speed on most devices.</p>
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
              <p class="getting-started-help">For the selected API: processing stays on-device and can work offline.</p>
            </div>
            <div class="getting-started-field">
              <span class="getting-started-label">Model</span>
              <div id="gs-llm-model-container">
                <select id="gs-llm-model" class="form-select">
                  ${localModelOptionsHtml(llmModels, rememberedModels.local)}
                </select>
              </div>
              <p class="getting-started-help">For the selected model: balanced quality and performance for first-time setup.</p>
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
          <p class="getting-started-help">This helper text updates when you switch Mic only, System only, or Mixed.</p>
        </section>

        <div class="getting-started-divider"></div>

        <p class="getting-started-note">Better models improve quality but may be slower. You can tune all values anytime in Settings.</p>

        <div class="getting-started-actions">
          <button id="gs-start" class="btn-primary">Start transcribing</button>
        </div>
      </div>
    </div>
  `

  const providerSelect = container.querySelector<HTMLSelectElement>('#gs-llm-provider')
  const modelContainer = container.querySelector<HTMLElement>('#gs-llm-model-container')
  const apiExtra = container.querySelector<HTMLElement>('#gs-api-extra')

  const renderModelInput = (provider: string): void => {
    if (!modelContainer) return
    const remembered = rememberedModels[provider] || (provider === 'local' ? fallbackLocalModel : '')
    if (provider === 'local') {
      modelContainer.innerHTML = `<select id="gs-llm-model" class="form-select">${localModelOptionsHtml(llmModels, remembered)}</select>`
      return
    }
    modelContainer.innerHTML = `<input id="gs-llm-model" type="text" class="form-input" value="${escapeHtml(remembered)}" placeholder="gpt-4o-mini" />`
  }

  providerSelect?.addEventListener('change', () => {
    const currentModelValue = modelContainer?.querySelector<HTMLInputElement | HTMLSelectElement>('#gs-llm-model')?.value ?? ''
    rememberedModels[activeProvider] = currentModelValue
    activeProvider = providerSelect.value === 'api' ? 'api' : 'local'
    apiExtra?.classList.toggle('hidden', activeProvider !== 'api')
    renderModelInput(activeProvider)
  })

  modelContainer?.addEventListener('input', (e) => {
    if (!(e.target instanceof HTMLInputElement) || e.target.id !== 'gs-llm-model') return
    rememberedModels[activeProvider] = e.target.value
  })

  modelContainer?.addEventListener('change', (e) => {
    if (!(e.target instanceof HTMLSelectElement) || e.target.id !== 'gs-llm-model') return
    rememberedModels[activeProvider] = e.target.value
  })

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

  let sttModels: string[] = []
  let llmModels: Array<Record<string, string>> = []
  try {
    ;[sttModels, llmModels] = await Promise.all([
      AudioAPI.getSTTModels(),
      LLMAPI.getLLMModels(),
    ])
  } catch {
    sttModels = config.modelName ? [config.modelName] : [RECOMMENDED.sttModel]
    llmModels = config.llm?.modelName ? [{ name: config.llm.modelName, description: '' }] : [{ name: RECOMMENDED.llmModel, description: '' }]
  }

  const configuredSource = (config.audio?.defaultSource || 'mixed') as SourceOption
  const selectedSource: SourceOption = configuredSource === 'microphone' || configuredSource === 'system' || configuredSource === 'mixed'
    ? configuredSource
    : 'mixed'

  renderScreen(container, config, selectedSource, sttModels, llmModels)
}
