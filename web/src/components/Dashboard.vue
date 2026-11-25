<script setup lang="ts">
import { computed, ref } from 'vue'

import { api } from '../api'
import { errorMessage, timeAgo } from '../format'
import { loadOverview, pushToast, state } from '../store'
import AddRepoDialog from './AddRepoDialog.vue'
import RepoCard from './RepoCard.vue'
import RepoDrawer from './RepoDrawer.vue'
import SourceTicket from './SourceTicket.vue'

const query = ref('')
const showAdd = ref(false)
const activeId = ref<number | null>(null)

const overview = computed(() => state.overview)
const isEmpty = computed(() => (overview.value?.stats.repositories ?? 0) === 0)
const problemCount = computed(() => overview.value?.stats.errors ?? 0)
const qr = computed(() => (overview.value ? api.qr(overview.value.source.url) : ''))

const filtered = computed(() => {
  const list = overview.value?.repos ?? []
  const needle = query.value.trim().toLowerCase()
  if (!needle) {
    return list
  }
  return list.filter((repo) =>
    `${repo.displayName} ${repo.owner}/${repo.name} ${repo.slug}`.toLowerCase().includes(needle),
  )
})

async function refreshAll(): Promise<void> {
  if (state.refreshing) {
    return
  }
  state.refreshing = true
  try {
    const result = await api.refreshAll()
    await loadOverview(true)
    pushToast(
      result.refreshed
        ? `Refreshed ${result.refreshed} ${result.refreshed === 1 ? 'repository' : 'repositories'}.`
        : 'Nothing to refresh yet.',
    )
  } catch (error) {
    pushToast(errorMessage(error), 'error')
  } finally {
    state.refreshing = false
  }
}

async function refreshOne(id: number): Promise<void> {
  try {
    await api.refreshRepo(id)
    await loadOverview(true)
    pushToast('Repository refreshed.')
  } catch (error) {
    pushToast(errorMessage(error), 'error')
  }
}
</script>

<template>
  <div class="stack">
    <div v-if="!overview" class="stack">
      <div class="skel skel--ticket"></div>
      <div class="grid">
        <div v-for="index in 3" :key="index" class="skel skel--card"></div>
      </div>
    </div>

    <template v-else>
      <SourceTicket
        :url="overview.source.url"
        :qr="qr"
        :apps="overview.stats.apps"
        :repos="overview.stats.repositories"
        :tint="overview.source.tintColor"
        :last-refresh="overview.stats.lastRefresh"
      />

      <section class="metrics" aria-label="Source totals">
        <div class="metric">
          <span class="metric__value">{{ overview.stats.repositories }}</span>
          <span class="metric__label">Repositories</span>
        </div>
        <div class="metric">
          <span class="metric__value">{{ overview.stats.apps }}</span>
          <span class="metric__label">Apps in source</span>
        </div>
        <div class="metric">
          <span class="metric__value">{{ timeAgo(overview.stats.lastRefresh) }}</span>
          <span class="metric__label">Last refresh</span>
        </div>
      </section>

      <section class="stack stack--tight">
        <div class="section-head">
          <div>
            <h2>Repositories</h2>
            <p>One app entry per .ipa asset on each repository's latest release.</p>
          </div>
          <div class="section-head__tools">
            <input
              v-model="query"
              class="input input--search"
              type="search"
              placeholder="Filter repositories"
              aria-label="Filter repositories"
            />
            <button
              class="btn btn--ghost"
              type="button"
              :disabled="state.refreshing || isEmpty"
              @click="refreshAll"
            >
              {{ state.refreshing ? 'Refreshing…' : 'Refresh all' }}
            </button>
            <button class="btn btn--primary" type="button" @click="showAdd = true">Add repository</button>
          </div>
        </div>

        <p v-if="problemCount" class="notice">
          {{ problemCount }}
          {{ problemCount === 1 ? 'repository needs' : 'repositories need' }} attention. Open a card to read
          the exact GitHub response.
        </p>

        <div v-if="isEmpty" class="empty">
          <svg class="empty__art" viewBox="0 0 120 72" aria-hidden="true">
            <rect x="10" y="16" width="100" height="18" rx="6" fill="none" stroke="currentColor" stroke-width="2" />
            <rect x="10" y="42" width="62" height="18" rx="6" fill="none" stroke="currentColor" stroke-width="2" />
            <path d="M88 51h22" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-dasharray="4 5" />
          </svg>
          <h2>No repositories yet</h2>
          <p>
            Point SourceBox at a GitHub repository that publishes <span class="mono">.ipa</span> assets on its
            latest release. The source stays in sync from then on.
          </p>
          <button class="btn btn--primary" type="button" @click="showAdd = true">Add your first repository</button>
        </div>

        <div v-else-if="!filtered.length" class="empty empty--quiet">
          <h2>No matches</h2>
          <p>Nothing matches “{{ query }}”. Try the owner name or the repository name.</p>
          <button class="btn btn--ghost" type="button" @click="query = ''">Clear filter</button>
        </div>

        <div v-else class="grid">
          <RepoCard
            v-for="repo in filtered"
            :key="repo.id"
            :repo="repo"
            @open="activeId = $event"
            @refresh="refreshOne"
          />
        </div>
      </section>

      <p class="footnote">
        Add this source in SideStore or AltStore under
        <span class="mono">Sources → +</span>. Clients cache the JSON on their own schedule.
      </p>
    </template>

    <RepoDrawer :repo-id="activeId" @close="activeId = null" @changed="loadOverview(true)" />
    <AddRepoDialog :open="showAdd" @close="showAdd = false" @added="loadOverview(true)" />
  </div>
</template>

