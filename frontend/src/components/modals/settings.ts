/**
 * Settings modal — STT, LLM, Audio configuration.
 */

import state from '../../state'
import { ConfigAPI } from '../../api'
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
              <input id="stt-model" type="text" class="form-input" value="${escapeHtml(config.modelName || 'base.en')}" />
            </label>
            <label class="form-label">
              <span class="form-label-text">Realtime chunk (seconds): <span id="chunk-val">${config.realtimeTranscriptionChunkSeconds || 5}</span></span>
              <input id="stt-chunk" type="range" min="1" max="30" value="${config.realtimeTranscriptionChunkSeconds || 5}" class="settings-range" />
            </label>
          </div>
        </section>

        <section>
          <h3 class="form-section-title">AI / LLM</h3>
          <div class="settings-section">
            <label class="form-label">
              <span class="form-label-text">Provider</span>
              <select id="llm-provider" class="form-select">
                <option value="local" ${config.llm?.provider === 'local' ? 'selected' : ''}>Local (llama.cpp)</option>
                <option value="api" ${config.llm?.provider === 'api' ? 'selected' : ''}>API (OpenAI / compatible)</option>
              </select>
            </label>
            <label class="form-label">
              <span class="form-label-text">Model Name</span>
              <input id="llm-model" type="text" class="form-input" value="${escapeHtml(config.llm?.modelName || '')}" />
            </label>
            <label id="llm-api-endpoint-label" class="form-label">
              <span class="form-label-text">API Endpoint</span>
              <input id="llm-endpoint" type="text" class="form-input" value="${escapeHtml(config.llm?.apiEndpoint || '')}" placeholder="http://localhost:8080" />
            </label>
            <label id="llm-api-key-label" class="form-label">
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
  const chunkSlider = container.querySelector<HTMLInputElement>('#stt-chunk')
  const chunkVal = container.querySelector<HTMLElement>('#chunk-val')
  chunkSlider?.addEventListener('input', () => { if (chunkVal && chunkSlider) chunkVal.textContent = chunkSlider.value })

  const tempSlider = container.querySelector<HTMLInputElement>('#llm-temp')
  const tempVal = container.querySelector<HTMLElement>('#temp-val')
  tempSlider?.addEventListener('input', () => { if (tempVal && tempSlider) tempVal.textContent = tempSlider.value })

  // Close buttons
  const close = () => state.closeModal()
  container.querySelector('#settings-close')?.addEventListener('click', close)
  container.querySelector('#settings-cancel')?.addEventListener('click', close)

  // Save
  container.querySelector('#settings-save')?.addEventListener('click', async () => {
    if (!config) return

    const newConfig = domain.Config.createFrom({
      ...config,
      modelName: (container.querySelector<HTMLInputElement>('#stt-model')?.value ?? config.modelName),
      realtimeTranscriptionChunkSeconds: parseInt(container.querySelector<HTMLInputElement>('#stt-chunk')?.value ?? '5'),
      llm: {
        ...config.llm,
        provider: (container.querySelector<HTMLSelectElement>('#llm-provider')?.value ?? config.llm?.provider) as string,
        modelName: (container.querySelector<HTMLInputElement>('#llm-model')?.value ?? config.llm?.modelName),
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
