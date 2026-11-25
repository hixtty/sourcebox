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
    pushToast('仓库已添加到源。')
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
    <form class="dialog" role="dialog" aria-modal="true" aria-label="添加仓库" @submit.prevent="submit">
      <header class="dialog__head">
        <div>
          <span class="eyebrow">新增条目</span>
          <h2>添加仓库</h2>
        </div>
        <button class="icon-btn" type="button" aria-label="关闭" @click="emit('close')">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M6 6l12 12M18 6 6 18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
          </svg>
        </button>
      </header>

      <label class="field">
        <span>GitHub 仓库</span>
        <input
          v-model="url"
          class="input"
          type="text"
          placeholder="owner/repo 或 https://github.com/owner/repo"
          autocomplete="off"
          autofocus
          required
        />
        <small class="field__hint">最新 Release 至少需要包含一个 .ipa 文件。</small>
      </label>

      <label class="field">
        <span>强调色</span>
        <div class="color-field">
          <input v-model="tint" class="input" type="text" />
          <input v-model="tint" class="color-field__picker" type="color" aria-label="选择强调色" />
        </div>
      </label>

      <p v-if="problem" class="alert alert--inline" role="alert">{{ problem }}</p>

      <div class="dialog__actions">
        <button class="btn btn--ghost" type="button" @click="emit('close')">取消</button>
        <button class="btn btn--primary" type="submit" :disabled="busy || !url.trim()">
          {{ busy ? '正在检查最新 Release…' : '添加仓库' }}
        </button>
      </div>
    </form>
  </div>
</template>

