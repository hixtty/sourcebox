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
    pushToast('Repository updated.')
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
    pushToast('Repository refreshed.')
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
    pushToast('Repository removed from the source.')
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

    <aside class="drawer" role="dialog" aria-modal="true" aria-label="Repository details">
      <header class="drawer__head">
        <div>
          <h2>{{ detail?.repo.displayName ?? 'Loading repository…' }}</h2>
          <p v-if="detail" class="mono muted">{{ detail.repo.owner }}/{{ detail.repo.name }}</p>
        </div>
        <button class="icon-btn" type="button" aria-label="Close details" @click="emit('close')">
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
              <span v-else>No release detected</span>
              <span v-if="detail.repo.latestSize"> · {{ formatBytes(detail.repo.latestSize) }}</span>
            </p>
            <p class="drawer__meta mono">
              {{ detail.repo.appCount }} {{ detail.repo.appCount === 1 ? 'entry' : 'entries' }} ·
              {{ detail.repo.stars }} stars
            </p>
          </div>
        </div>

        <div v-if="detail.error" class="alert" role="alert">
          <strong>{{ detail.repo.statusLabel }}</strong>
          <p>{{ detail.error }}</p>
          <a :href="detail.repo.url" target="_blank" rel="noreferrer">Open on GitHub</a>
        </div>

        <section v-if="versions.length" class="drawer__section">
          <h3>Versions</h3>
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
                Download
              </a>
            </li>
          </ul>
        </section>

        <section v-if="detail.release?.body" class="drawer__section">
          <h3>Release notes</h3>
          <pre class="notes">{{ detail.release.body }}</pre>
        </section>

        <section class="drawer__section">
          <h3>Appearance</h3>
          <div class="form-grid">
            <label class="field">
              <span>Display name</span>
              <input v-model="form.displayName" class="input" type="text" />
            </label>
            <div class="form-row">
              <label class="field">
                <span>Accent color</span>
                <div class="color-field">
                  <input v-model="form.tintColor" class="input" type="text" />
                  <input
                    v-model="form.tintColor"
                    class="color-field__picker"
                    type="color"
                    aria-label="Pick accent color"
                  />
                </div>
              </label>
              <label class="field">
                <span>Icon override</span>
                <input v-model="form.iconUrl" class="input" type="text" placeholder="https://…" />
              </label>
            </div>
          </div>
          <div class="drawer__actions">
            <button class="btn btn--ghost" type="button" @click="refresh">Refresh now</button>
            <button class="btn btn--primary" type="button" :disabled="saving" @click="save">
              {{ saving ? 'Saving…' : 'Save changes' }}
            </button>
          </div>
        </section>

        <section class="drawer__section drawer__section--danger">
          <h3>Remove</h3>
          <p class="muted">Drops this repository's apps from the source. The GitHub repository is untouched.</p>
          <button class="btn btn--danger" type="button" :disabled="removing" @click="remove">
            {{ confirmDelete ? 'Confirm removal' : 'Remove from source' }}
          </button>
        </section>
      </template>
    </aside>
  </div>
</template>

