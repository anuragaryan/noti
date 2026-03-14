import { ConfigAPI } from '../api'
import state from '../state'
import type { Config } from '../types'

export async function saveConfigWithPending(nextConfig: Config): Promise<void> {
  state.setState({ isConfigSaving: true, pendingConfig: nextConfig })
  try {
    await ConfigAPI.save(nextConfig)
    state.setState({
      config: nextConfig,
      isConfigSaving: false,
      pendingConfig: null,
    })
  } catch (err) {
    state.setState({
      isConfigSaving: false,
      pendingConfig: null,
    })
    throw err
  }
}
