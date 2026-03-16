import { beforeEach, describe, expect, it, vi } from 'vitest'
import state from '../state'

const mockNotesAPI = {
  create: vi.fn(),
  getAll: vi.fn(),
}

vi.mock('../api', () => ({
  NotesAPI: mockNotesAPI,
}))

function resetState(): void {
  state.setState({
    notes: [],
    currentNote: null,
    mainView: 'default',
    notification: null,
  })
}

describe('empty state integration', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="empty-state"></div>'
    mockNotesAPI.create.mockReset()
    mockNotesAPI.getAll.mockReset()
    resetState()
  })

  it('opens AI chat from empty state CTA', async () => {
    const { renderEmptyState } = await import('./empty-state')

    const container = document.getElementById('empty-state')
    if (!container) throw new Error('empty-state container missing')
    renderEmptyState(container)

    const aiChatBtn = document.querySelector<HTMLButtonElement>('#empty-ai-chat')
    if (!aiChatBtn) throw new Error('empty ai chat button missing')

    aiChatBtn.click()
    expect(state.get('mainView')).toBe('ai-chat')
  })
})
