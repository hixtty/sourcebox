<script setup lang="ts">
import { computed, onMounted } from 'vue'

import Dashboard from './components/Dashboard.vue'
import LoginView from './components/LoginView.vue'
import SettingsView from './components/SettingsView.vue'
import Toasts from './components/Toasts.vue'
import { init, signIn, signOut, state, toggleTheme } from './store'

onMounted(init)

const showLogin = computed(() => state.me !== null && !state.me.authed)

function reload(): void {
  window.location.reload()
}

async function handleSignIn(token: string): Promise<void> {
  await signIn(token)
}
</script>

<template>
  <div class="app">
    <div v-if="state.booting" class="boot">
      <svg class="mark mark--lg" viewBox="0 0 24 24" aria-hidden="true">
        <rect x="2.5" y="4" width="19" height="6.2" rx="2.4" fill="currentColor" />
        <rect x="2.5" y="13.8" width="11.4" height="6.2" rx="2.4" fill="currentColor" opacity=".55" />
      </svg>
      <p class="mono muted">Loading workspace…</p>
    </div>

    <div v-else-if="state.offline" class="boot">
      <svg class="mark mark--lg" viewBox="0 0 24 24" aria-hidden="true">
        <rect x="2.5" y="4" width="19" height="6.2" rx="2.4" fill="currentColor" />
        <rect x="2.5" y="13.8" width="11.4" height="6.2" rx="2.4" fill="currentColor" opacity=".55" />
      </svg>
      <h1 class="boot__title">Cannot reach SourceBox</h1>
      <p class="muted boot__copy">
        The API did not answer. Check that the container is running, then reload this page.
      </p>
      <button class="btn btn--primary" type="button" @click="reload">Reload</button>
    </div>

    <LoginView v-else-if="showLogin" @submit="handleSignIn" />

    <template v-else>
      <header class="topbar">
        <div class="brand">
          <svg class="mark" viewBox="0 0 24 24" aria-hidden="true">
            <rect x="2.5" y="4" width="19" height="6.2" rx="2.4" fill="currentColor" />
            <rect x="2.5" y="13.8" width="11.4" height="6.2" rx="2.4" fill="currentColor" opacity=".55" />
          </svg>
          <span class="brand__name">SourceBox</span>
          <span class="brand__tag mono">v{{ state.me?.version ?? '0.1.0' }}</span>
        </div>

        <nav class="nav" aria-label="Sections">
          <button
            class="nav__item"
            :class="{ 'is-active': state.view === 'dashboard' }"
            type="button"
            @click="state.view = 'dashboard'"
          >
            Overview
          </button>
          <button
            class="nav__item"
            :class="{ 'is-active': state.view === 'settings' }"
            type="button"
            @click="state.view = 'settings'"
          >
            Settings
          </button>
        </nav>

        <div class="topbar__side">
          <button
            class="icon-btn"
            type="button"
            :aria-label="state.theme === 'dark' ? 'Switch to the light theme' : 'Switch to the dark theme'"
            @click="toggleTheme"
          >
            <svg v-if="state.theme === 'dark'" viewBox="0 0 24 24" aria-hidden="true">
              <circle cx="12" cy="12" r="4.4" fill="currentColor" />
              <g stroke="currentColor" stroke-width="1.6" stroke-linecap="round">
                <path d="M12 3v2.2M12 18.8V21M3 12h2.2M18.8 12H21M5.6 5.6l1.6 1.6M16.8 16.8l1.6 1.6M18.4 5.6l-1.6 1.6M7.2 16.8l-1.6 1.6" />
              </g>
            </svg>
            <svg v-else viewBox="0 0 24 24" aria-hidden="true">
              <path
                d="M20 14.2A8.2 8.2 0 1 1 9.8 4a6.6 6.6 0 0 0 10.2 10.2Z"
                fill="currentColor"
              />
            </svg>
          </button>
          <button v-if="state.me?.authRequired" class="btn btn--ghost btn--sm" type="button" @click="signOut">
            Sign out
          </button>
        </div>
      </header>

      <main class="page">
        <Dashboard v-if="state.view === 'dashboard'" />
        <SettingsView v-else />
      </main>
    </template>

    <Toasts />
  </div>
</template>

