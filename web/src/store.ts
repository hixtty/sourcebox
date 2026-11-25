import { reactive } from 'vue'

import { ApiError, api } from './api'
import type { Me, Overview } from './types'

export type View = 'dashboard' | 'settings'
export type Tone = 'ok' | 'error'

interface Toast {
  id: number
  text: string
  tone: Tone
}

const storedTheme = localStorage.getItem('sourcebox-theme')

export const state = reactive({
  booting: true,
  offline: false,
  signingIn: false,
  me: null as Me | null,
  overview: null as Overview | null,
  view: 'dashboard' as View,
  refreshing: false,
  theme: (storedTheme === 'light' ? 'light' : 'dark') as 'light' | 'dark',
  toasts: [] as Toast[],
})

let toastSeq = 0

export function pushToast(text: string, tone: Tone = 'ok'): void {
  const id = ++toastSeq
  state.toasts.push({ id, text, tone })
  window.setTimeout(() => {
    state.toasts = state.toasts.filter((toast) => toast.id !== id)
  }, 4200)
}

export function setTheme(theme: 'dark' | 'light'): void {
  state.theme = theme
  localStorage.setItem('sourcebox-theme', theme)
  document.documentElement.dataset.theme = theme
}

export function toggleTheme(): void {
  setTheme(state.theme === 'dark' ? 'light' : 'dark')
}

export async function loadOverview(silent = false): Promise<void> {
  try {
    state.overview = await api.overview()
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      state.me = state.me ? { ...state.me, authed: false } : null
      return
    }
    if (!silent) {
      pushToast(error instanceof Error ? error.message : 'Could not load the workspace.', 'error')
    }
  }
}

export async function init(): Promise<void> {
  setTheme(state.theme)
  try {
    state.me = await api.me()
    state.offline = false
    if (state.me.authed) {
      await loadOverview()
    }
  } catch (error) {
    state.offline = true
    pushToast(error instanceof Error ? error.message : 'Could not reach the server.', 'error')
  } finally {
    state.booting = false
  }
}

export async function signIn(token: string): Promise<boolean> {
  state.signingIn = true
  try {
    await api.login(token)
    state.me = await api.me()
    if (state.me.authed) {
      await loadOverview()
    }
    return state.me?.authed ?? false
  } catch (error) {
    pushToast(error instanceof Error ? error.message : 'Sign in failed.', 'error')
    return false
  } finally {
    state.signingIn = false
  }
}

export async function signOut(): Promise<void> {
  try {
    await api.logout()
  } finally {
    state.me = state.me ? { ...state.me, authed: false } : null
    state.overview = null
  }
}

