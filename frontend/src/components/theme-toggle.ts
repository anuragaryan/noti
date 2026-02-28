/**
 * Theme toggle component — switches dark/light mode.
 * Persists preference to localStorage.
 * Mounts into any container element passed to it.
 */

import state from '../state'
import type { Theme } from '../types'
import { icon } from '../utils/icons'

const STORAGE_KEY = 'noti-theme'

// ─── Lucide icons — see utils/icons.ts ─────────────────────────────────────────────────────────

export function applyTheme(theme: Theme): void {
  if (theme === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
  localStorage.setItem(STORAGE_KEY, theme)
  state.setState({ theme })
}

export function initTheme(): Theme {
  const stored = localStorage.getItem(STORAGE_KEY) as Theme | null
  if (stored === 'dark' || stored === 'light') {
    applyTheme(stored)
    return stored
  }
  // Fallback to system preference
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  const theme: Theme = prefersDark ? 'dark' : 'light'
  applyTheme(theme)
  return theme
}

export function renderThemeToggle(container: HTMLElement): void {
  const currentTheme = state.get('theme')

  container.innerHTML = `
    <div id="theme-toggle" class="theme-toggle-wrap">
      <button id="theme-dark-btn" title="Dark mode" class="theme-btn ${currentTheme === 'dark' ? 'theme-btn-active' : 'theme-btn-inactive'}">
        ${icon('moon', 14)}
      </button>
      <button id="theme-light-btn" title="Light mode" class="theme-btn ${currentTheme === 'light' ? 'theme-btn-active' : 'theme-btn-inactive'}">
        ${icon('sun', 14)}
      </button>
    </div>
  `

  container.querySelector('#theme-dark-btn')?.addEventListener('click', () => {
    applyTheme('dark')
    renderThemeToggle(container) // re-render to update active state
  })

  container.querySelector('#theme-light-btn')?.addEventListener('click', () => {
    applyTheme('light')
    renderThemeToggle(container) // re-render to update active state
  })
}
