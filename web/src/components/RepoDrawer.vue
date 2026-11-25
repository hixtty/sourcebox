<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { api } from '../api'
import { errorMessage, formatBytes, formatDate } from '../format'
import { loadOverview, pushToast } from '../store'
import type { RepoDetail } from '../types'

const props = defineProps<{ repoId: number | null }>()
const emit = defineEmits<{ close: []; changed: [] }>()

const detail = ref<RepoDetail | null>(null)
const loading = ref(false)
const saving = ref(false)
const removing = ref(false)
const confirmDelete = ref(false)
const form = ref({ displayName: '', tintColor: '#5B7CFA', iconUrl: '' })

const versions = computed(() => {
  const apps = detail.value?.apps ?? []
  return apps.flatMap((app) => app.versions.map((version) => ({ app, version })))
})

watch(
  () => props.repoId,
  async (id) => {
    confirmDelete.value = false
    if (id === null) {
      detail.value = null
      return
    }
    loading.value = true
    detail.value = null
    try {
      const data = await api.repoDetail(id)
      detail.value = data
      form.value = {
        displayName: data.repo.displayName,
        tintColor: data.repo.tintColor,
        iconUrl: data.repo.iconUrl ?? '',
      }
    } catch (error) {
      pushToast(errorMessage(error), 'error')
      emit('close')
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    emit('close')
  }
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))

async function save(): Promise<void> {
  if (!props.repoId || saving.value) {
    return
  }
  saving.value = true
  try {
    await api.patchRepo(props.repoId, {
      displayName: form.value.displayName,
      tintColor: form.value.tintColor,
      iconUrl: form.value.iconUrl,
    })
    detail.value = await api.repoDetail(props.repoId)
    await loadOverview(true)
    emit('changed')
    pushToast('仓库已更新。')
  } catch (error) {
    pushToast(errorMessage(error), 'error')
  } finally {
    saving.value = false
  }
}

async function refresh(): Promise<void> {
  if (!props.repoId) {
    return
  }
  try {
    await api.refreshRepo(props.repoId)
    detail.value = await api.repoDetail(props.repoId)
    await loadOverview(true)
    emit('changed')
    pushToast('仓库已刷新。')
  } catch (error) {
    pushToast(errorMessage(error), 'error')
  }
}

async function remove(): Promise<void> {
  if (!props.repoId) {
    return
  }
  if (!confirmDelete.value) {
    confirmDelete.value = true
    return
  }
  removing.value = true
  try {
    await api.deleteRepo(props.repoId)
    await loadOverview(true)
    emit('changed')
    emit('close')
    pushToast('仓库已从源中移除。')
  } catch (error) {
    pushToast(errorMessage(error), 'error')
  } finally {
    removing.value = false
  }
}
</script>

<template>
  <div v-if="repoId !== null" class="drawer-shell">
    <div class="drawer-backdrop" @click="emit('close')"></div>

    <aside class="drawer" role="dialog" aria-modal="true" aria-label="仓库详情">
      <header class="drawer__head">
        <div>
          <h2>{{ detail?.repo.displayName ?? '正在加载仓库…' }}</h2>
          <p v-if="detail" class="mono muted">{{ detail.repo.owner }}/{{ detail.repo.name }}</p>
        </div>
        <button class="icon-btn" type="button" aria-label="关闭详情" @click="emit('close')">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M6 6l12 12M18 6 6 18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
          </svg>
        </button>
      </header>

      <div v-if="loading" class="stack">
        <div class="skel skel--row"></div>
        <div class="skel skel--row"></div>
        <div class="skel skel--row"></div>
      </div>

      <template v-else-if="detail">
        <div class="drawer__hero" :style="{ '--tint': detail.repo.tintColor }">
          <img class="drawer__icon" :src="detail.repo.iconSrc" alt="" />
          <div class="drawer__hero-copy">
            <span class="chip" :data-status="detail.repo.status">{{ detail.repo.statusLabel }}</span>
            <p class="drawer__rel">
              <span v-if="detail.release?.tag">Release {{ detail.release.tag }}</span>
              <span v-else>未检测到 Release</span>
              <span v-if="detail.repo.latestSize"> · {{ formatBytes(detail.repo.latestSize) }}</span>
            </p>
            <p class="drawer__meta mono">
              {{ detail.repo.appCount }} 个条目 · {{ detail.repo.stars }} 星
            </p>
          </div>
        </div>

        <div v-if="detail.error" class="alert" role="alert">
          <strong>{{ detail.repo.statusLabel }}</strong>
          <p>{{ detail.error }}</p>
          <a :href="detail.repo.url" target="_blank" rel="noreferrer">在 GitHub 上打开</a>
        </div>

        <section v-if="versions.length" class="drawer__section">
          <h3>版本</h3>
          <ul class="versions">
            <li v-for="row in versions" :key="row.version.downloadURL" class="version">
              <div class="version__main">
                <p class="version__name">{{ row.app.name }}</p>
                <p class="version__meta mono">
                  v{{ row.version.version }} · {{ formatDate(row.version.date) }} ·
                  {{ formatBytes(row.version.size) }}
                </p>
              </div>
              <a class="btn btn--ghost btn--sm" :href="row.version.downloadURL" target="_blank" rel="noreferrer">
                下载
              </a>
            </li>
          </ul>
        </section>

        <section v-if="detail.release?.body" class="drawer__section">
          <h3>Release 说明</h3>
          <pre class="notes">{{ detail.release.body }}</pre>
        </section>

        <section class="drawer__section">
          <h3>外观</h3>
          <div class="form-grid">
            <label class="field">
              <span>显示名称</span>
              <input v-model="form.displayName" class="input" type="text" />
            </label>
            <div class="form-row">
              <label class="field">
                <span>强调色</span>
                <div class="color-field">
                  <input v-model="form.tintColor" class="input" type="text" />
                  <input
                    v-model="form.tintColor"
                    class="color-field__picker"
                    type="color"
                    aria-label="选择强调色"
                  />
                </div>
              </label>
              <label class="field">
                <span>图标覆盖</span>
                <input v-model="form.iconUrl" class="input" type="text" placeholder="https://…" />
              </label>
            </div>
          </div>
          <div class="drawer__actions">
            <button class="btn btn--ghost" type="button" @click="refresh">立即刷新</button>
            <button class="btn btn--primary" type="button" :disabled="saving" @click="save">
              {{ saving ? '保存中…' : '保存修改' }}
            </button>
          </div>
        </section>

        <section class="drawer__section drawer__section--danger">
          <h3>移除</h3>
          <p class="muted">只把仓库里的应用从源里去掉，GitHub 上的仓库本身不受影响。</p>
          <button class="btn btn--danger" type="button" :disabled="removing" @click="remove">
            {{ confirmDelete ? '确认移除' : '从源中移除' }}
          </button>
        </section>
      </template>
    </aside>
  </div>
</template>

