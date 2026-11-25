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
        ? `已刷新 ${result.refreshed} 个仓库。`
        : '还没有可刷新的仓库。',
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
    pushToast('仓库已刷新。')
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

      <section class="metrics" aria-label="源统计">
        <div class="metric">
          <span class="metric__value">{{ overview.stats.repositories }}</span>
          <span class="metric__label">仓库</span>
        </div>
        <div class="metric">
          <span class="metric__value">{{ overview.stats.apps }}</span>
          <span class="metric__label">源内应用</span>
        </div>
        <div class="metric">
          <span class="metric__value">{{ timeAgo(overview.stats.lastRefresh) }}</span>
          <span class="metric__label">最近刷新</span>
        </div>
      </section>

      <section class="stack stack--tight">
        <div class="section-head">
          <div>
            <h2>仓库</h2>
            <p>每个仓库的最新 Release 里，每有一个 .ipa 文件就生成一个应用条目。</p>
          </div>
          <div class="section-head__tools">
            <input
              v-model="query"
              class="input input--search"
              type="search"
              placeholder="筛选仓库"
              aria-label="筛选仓库"
            />
            <button
              class="btn btn--ghost"
              type="button"
              :disabled="state.refreshing || isEmpty"
              @click="refreshAll"
            >
              {{ state.refreshing ? '刷新中…' : '全部刷新' }}
            </button>
            <button class="btn btn--primary" type="button" @click="showAdd = true">添加仓库</button>
          </div>
        </div>

        <p v-if="problemCount" class="notice">
          有 {{ problemCount }} 个仓库需要处理。点开卡片可以看到 GitHub 返回的原始信息。
        </p>

        <div v-if="isEmpty" class="empty">
          <svg class="empty__art" viewBox="0 0 120 72" aria-hidden="true">
            <rect x="10" y="16" width="100" height="18" rx="6" fill="none" stroke="currentColor" stroke-width="2" />
            <rect x="10" y="42" width="62" height="18" rx="6" fill="none" stroke="currentColor" stroke-width="2" />
            <path d="M88 51h22" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-dasharray="4 5" />
          </svg>
          <h2>还没有仓库</h2>
          <p>
            添加一个在最新 Release 里发布 <span class="mono">.ipa</span> 的 GitHub 仓库，之后源会自动保持同步。
          </p>
          <button class="btn btn--primary" type="button" @click="showAdd = true">添加第一个仓库</button>
        </div>

        <div v-else-if="!filtered.length" class="empty empty--quiet">
          <h2>没有匹配结果</h2>
          <p>没有匹配“{{ query }}”的仓库，试试仓库作者或仓库名。</p>
          <button class="btn btn--ghost" type="button" @click="query = ''">清除筛选</button>
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
        在 SideStore 或 AltStore 的 <span class="mono">Sources → +</span> 里填入上面的地址即可。
        客户端会按自己的节奏缓存 JSON。
      </p>
    </template>

    <RepoDrawer :repo-id="activeId" @close="activeId = null" @changed="loadOverview(true)" />
    <AddRepoDialog :open="showAdd" @close="showAdd = false" @added="loadOverview(true)" />
  </div>
</template>

