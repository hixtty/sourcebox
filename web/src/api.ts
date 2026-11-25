import type { Me, Overview, Repo, RepoDetail, SourceConfig, SourcePayload } from './types'

const BASE = (window as unknown as { __SOURCEBOX_BASE__?: string }).__SOURCEBOX_BASE__ ?? ''

function withBase(path: string): string {
  return `${BASE}${path}`
}

export class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {}
  if (init.body) {
    headers['Content-Type'] = 'application/json'
  }
  const response = await fetch(withBase(path), {
    credentials: 'same-origin',
    ...init,
    headers: { ...headers, ...(init.headers as Record<string, string> | undefined) },
  })

  if (!response.ok) {
    let message = `Request failed (${response.status})`
    try {
      const data = (await response.json()) as { detail?: unknown }
      if (typeof data?.detail === 'string') {
        message = data.detail
      }
    } catch {
      // Response had no JSON body; keep the status message.
    }
    throw new ApiError(message, response.status)
  }

  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

export const api = {
  me: () => request<Me>('/api/me'),

  login: (token: string) =>
    request<{ ok: boolean }>('/api/login', {
      method: 'POST',
      body: JSON.stringify({ token }),
    }),

  logout: () => request<{ ok: boolean }>('/api/logout', { method: 'POST' }),

  overview: () => request<Overview>('/api/overview'),

  addRepo: (url: string, tintColor: string) =>
    request<Repo>('/api/repos', {
      method: 'POST',
      body: JSON.stringify({ url, tintColor }),
    }),

  patchRepo: (id: number, patch: { displayName?: string; tintColor?: string; iconUrl?: string }) =>
    request<Repo>(`/api/repos/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),

  deleteRepo: (id: number) => request<{ ok: boolean }>(`/api/repos/${id}`, { method: 'DELETE' }),

  refreshRepo: (id: number) => request<Repo>(`/api/repos/${id}/refresh`, { method: 'POST' }),

  refreshAll: () => request<{ refreshed: number; repos: Repo[] }>('/api/repos/refresh', { method: 'POST' }),

  repoDetail: (id: number) => request<RepoDetail>(`/api/repos/${id}`),

  reorder: (ids: number[]) =>
    request<{ ok: boolean }>('/api/repos/reorder', {
      method: 'POST',
      body: JSON.stringify({ ids }),
    }),

  getSource: () => request<SourceConfig>('/api/source'),

  saveSource: (payload: SourcePayload) =>
    request<SourceConfig>('/api/source', {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),

  preview: () => request<Record<string, unknown>>('/api/preview'),

  qr: (text: string) => `${BASE}/api/qr.svg?data=${encodeURIComponent(text)}`,
}

