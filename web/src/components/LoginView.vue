<script setup lang="ts">
import { ref } from 'vue'

import { state } from '../store'

const emit = defineEmits<{ submit: [token: string] }>()

const token = ref('')

function submit(): void {
  if (!token.value.trim() || state.signingIn) {
    return
  }
  emit('submit', token.value.trim())
}
</script>

<template>
  <div class="login">
    <form class="login__card" @submit.prevent="submit">
      <div class="login__brand">
        <svg class="mark" viewBox="0 0 24 24" aria-hidden="true">
          <rect x="2.5" y="4" width="19" height="6.2" rx="2.4" fill="currentColor" />
          <rect x="2.5" y="13.8" width="11.4" height="6.2" rx="2.4" fill="currentColor" opacity=".55" />
        </svg>
        <span class="brand__name">SourceBox</span>
      </div>

      <div class="login__copy">
        <h1>Unlock the workspace</h1>
        <p class="muted">Enter the admin token you set as <code class="inline">ADMIN_TOKEN</code> on the server.</p>
      </div>

      <label class="field">
        <span>Admin token</span>
        <input
          v-model="token"
          class="input"
          type="password"
          autocomplete="current-password"
          placeholder="••••••••••••"
          autofocus
        />
      </label>

      <button class="btn btn--primary" type="submit" :disabled="state.signingIn || !token.trim()">
        {{ state.signingIn ? 'Checking…' : 'Continue' }}
      </button>

      <p class="login__hint">Sessions last 30 days on this browser.</p>
    </form>
  </div>
</template>

