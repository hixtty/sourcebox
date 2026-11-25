<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'

import { api } from '../api'
import { copyText, errorMessage } from '../format'
import { loadOverview, pushToast, state } from '../store'

const form = reactive({
  name: '',
  slug: '',
  subtitle: '',
  description: '',
  tintColor: '#5B7CFA',
  cacheSeconds: 600,
})

const saving = ref(false)
const copied = ref(false)

watch(
  () => state.overview?.source,
  (source) => {
    if (!source) {
      return
    }
    Object.assign(form, {
      name: source.name,
      slug: source.slug,
      subtitle: source.subtitle,
      description: source.description,
      tintColor: source.tintColor,
      cacheSeconds: source.cacheSeconds,
    })
  },
  { immediate: true },
)

const sourceUrl = computed(() => state.overview?.source.url ?? '')

async function save(): Promise<void> {
  if (saving.value) {
    return
  }
  saving.value = true
  try {
    await api.saveSource({
      name: form.name,
      slug: form.slug,
      subtitle: form.subtitle,
      description: form.description,
      tintColor: form.tintColor,
      cacheSeconds: Number(form.cacheSeconds) || 0,
    })
    await loadOverview(true)
    pushToast('Source settings saved.')
  } catch (error) {
    pushToast(errorMessage(error), 'error')
  } finally {
    saving.value = false
  }
}

async function copy(): Promise<void> {
  if (await copyText(sourceUrl.value)) {
    copied.value = true
    window.setTimeout(() => {
      copied.value = false
    }, 1600)
  }
}
</script>

<template>
  <div class="stack">
    <header class="page-head">
      <span class="eyebrow">Workspace</span>
      <h1>Settings</h1>
      <p class="muted">How the source presents itself, and how often release data is refreshed.</p>
    </header>

    <div v-if="!state.overview" class="stack">
      <div class="skel skel--row"></div>
      <div class="skel skel--row"></div>
    </div>

    <template v-else>
      <div class="settings-grid">
        <section class="panel">
          <header class="panel__head">
            <h2>Source identity</h2>
            <p class="muted">Shown at the top of the source in SideStore and AltStore.</p>
          </header>

          <div class="form-grid">
            <label class="field">
              <span>Source name</span>
              <input v-model="form.name" class="input" type="text" />
            </label>

            <label class="field">
              <span>Source slug</span>
              <input v-model="form.slug" class="input" type="text" />
              <small class="field__hint">
                The file name after <span class="mono">/</span>. <span class="mono">source</span> keeps both
                <span class="mono">/source.json</span> and <span class="mono">/{{ form.slug }}.json</span>.
              </small>
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
                <span>Cache seconds</span>
                <input v-model.number="form.cacheSeconds" class="input" type="number" min="0" step="30" />
                <small class="field__hint">How long a repository's release data stays cached.</small>
              </label>
            </div>
          </div>
        </section>

        <section class="panel">
          <header class="panel__head">
            <h2>Description</h2>
            <p class="muted">Plain text. Markdown is not rendered by the clients.</p>
          </header>

          <div class="form-grid">
            <label class="field">
              <span>Subtitle</span>
              <input v-model="form.subtitle" class="input" type="text" />
            </label>
            <label class="field">
              <span>Description</span>
              <textarea v-model="form.description" class="input textarea" rows="4"></textarea>
            </label>
          </div>
        </section>

        <section class="panel">
          <header class="panel__head">
            <h2>Endpoint</h2>
            <p class="muted">This is what you paste into SideStore or AltStore.</p>
          </header>

          <div class="endpoint">
            <span class="mono endpoint__url">{{ sourceUrl }}</span>
            <button class="btn btn--ghost btn--sm" type="button" @click="copy">
              {{ copied ? 'Copied' : 'Copy' }}
            </button>
          </div>
        </section>

        <section class="panel">
          <header class="panel__head">
            <h2>Server environment</h2>
            <p class="muted">These values come from the container environment.</p>
          </header>

          <dl class="facts">
            <div class="fact">
              <dt>Base URL</dt>
              <dd class="mono">{{ state.overview.baseUrl }}</dd>
            </div>
            <div class="fact">
              <dt>Dashboard path</dt>
              <dd class="mono">{{ state.overview.adminPath }}</dd>
            </div>
            <div class="fact">
              <dt>GitHub token</dt>
              <dd>
                <span class="chip" :data-status="state.overview.githubTokenConfigured ? 'ok' : 'pending'">
                  {{ state.overview.githubTokenConfigured ? 'Configured' : 'Not set' }}
                </span>
                <small class="field__hint">
                  Rate limit is 60 requests/hour without a token, 5000 with one. Set
                  <span class="mono">GITHUB_TOKEN</span> and restart.
                </small>
              </dd>
            </div>
            <div class="fact">
              <dt>Admin access</dt>
              <dd>
                <span class="chip" :data-status="state.overview.authRequired ? 'ok' : 'no_ipa'">
                  {{ state.overview.authRequired ? 'Token required' : 'Open' }}
                </span>
                <small class="field__hint">
                  Set <span class="mono">ADMIN_TOKEN</span> to lock this dashboard. The source JSON stays
                  public either way.
                </small>
              </dd>
            </div>
          </dl>
        </section>
      </div>

      <div class="settings-actions">
        <span class="muted">Changes apply on the next source request.</span>
        <button class="btn btn--primary" type="button" :disabled="saving" @click="save">
          {{ saving ? 'Saving…' : 'Save settings' }}
        </button>
      </div>
    </template>
  </div>
</template>

