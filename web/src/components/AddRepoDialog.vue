<script setup lang="ts">
import { ref, watch } from 'vue'

import { api } from '../api'
import { errorMessage } from '../format'
import { loadOverview, pushToast } from '../store'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: []; added: [] }>()

const url = ref('')
const tint = ref('#5B7CFA')
const busy = ref(false)
const problem = ref<string | null>(null)

watch(
  () => props.open,
  (open) => {
    if (open) {
      url.value = ''
      tint.value = '#5B7CFA'
      busy.value = false
      problem.value = null
    }
  },
)

async function submit(): Promise<void> {
  if (busy.value) {
    return
  }
  busy.value = true
  problem.value = null
  try {
    await api.addRepo(url.value.trim(), tint.value)
    await loadOverview(true)
    pushToast('Repository added to the source.')
    emit('added')
    emit('close')
  } catch (error) {
    problem.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="open" class="dialog-shell">
    <div class="drawer-backdrop" @click="emit('close')"></div>
    <form class="dialog" role="dialog" aria-modal="true" aria-label="Add repository" @submit.prevent="submit">
      <header class="dialog__head">
        <div>
          <span class="eyebrow">New entry</span>
          <h2>Add a repository</h2>
        </div>
        <button class="icon-btn" type="button" aria-label="Close" @click="emit('close')">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M6 6l12 12M18 6 6 18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
          </svg>
        </button>
      </header>

      <label class="field">
        <span>GitHub repository</span>
        <input
          v-model="url"
          class="input"
          type="text"
          placeholder="owner/repo or https://github.com/owner/repo"
          autocomplete="off"
          autofocus
          required
        />
        <small class="field__hint">The latest release must include at least one .ipa asset.</small>
      </label>

      <label class="field">
        <span>Accent color</span>
        <div class="color-field">
          <input v-model="tint" class="input" type="text" />
          <input v-model="tint" class="color-field__picker" type="color" aria-label="Pick accent color" />
        </div>
      </label>

      <p v-if="problem" class="alert alert--inline" role="alert">{{ problem }}</p>

      <div class="dialog__actions">
        <button class="btn btn--ghost" type="button" @click="emit('close')">Cancel</button>
        <button class="btn btn--primary" type="submit" :disabled="busy || !url.trim()">
          {{ busy ? 'Checking latest release…' : 'Add repository' }}
        </button>
      </div>
    </form>
  </div>
</template>

