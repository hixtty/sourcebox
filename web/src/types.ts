export type RepoStatus = 'ok' | 'no_release' | 'no_ipa' | 'error' | 'pending'

export interface Me {
  version: string
  authRequired: boolean
  authed: boolean
  githubTokenConfigured: boolean
}

export interface SourceConfig {
  name: string
  slug: string
  subtitle: string
  description: string
  tintColor: string
  cacheSeconds: number
  url: string
}

export interface Stats {
  repositories: number
  apps: number
  errors: number
  pending: number
  lastRefresh: string | null
}

export interface Repo {
  id: number
  url: string
  owner: string
  name: string
  slug: string
  displayName: string
  tintColor: string
  iconUrl: string | null
  iconSrc: string
  status: RepoStatus
  statusLabel: string
  error: string | null
  releaseTag: string | null
  releaseDate: string | null
  refreshedAt: string | null
  appCount: number
  latestVersion: string | null
  latestSize: number | null
  description: string
  stars: number
}

export interface Overview {
  baseUrl: string
  adminPath: string
  source: SourceConfig
  stats: Stats
  repos: Repo[]
  authRequired: boolean
  githubTokenConfigured: boolean
}

export interface Version {
  version: string
  buildVersion: string
  date: string
  downloadURL: string
  localizedDescription: string
  size: number
}

export interface AppEntry {
  name: string
  bundleIdentifier: string
  developerName: string
  subtitle: string
  localizedDescription: string
  iconURL: string
  tintColor: string
  versions: Version[]
}

export interface Release {
  tag: string
  name: string
  publishedAt: string | null
  body: string
}

export interface RepoDetail {
  repo: Repo
  apps: AppEntry[]
  release: Release | null
  error: string | null
  status: RepoStatus
}

export interface SourcePayload {
  name: string
  slug: string
  subtitle: string
  description: string
  tintColor: string
  cacheSeconds: number
}

