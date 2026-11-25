<script setup lang="ts">
import { formatBytes, timeAgo } from '../format'
import type { Repo } from '../types'

defineProps<{ repo: Repo }>()

const emit = defineEmits<{ open: [id: number]; refresh: [id: number] }>()
</script>

<template>
  <article class="card" :style="{ '--tint': repo.tintColor }">
    <div class="card__top">
      <img class="card__icon" :src="repo.iconSrc" :alt="`${repo.displayName} icon`" loading="lazy" />
      <div class="card__id">
        <h3 class="card__name">{{ repo.displayName }}</h3>
        <p class="card__handle">{{ repo.owner }}/{{ repo.name }}</p>
      </div>
      <span class="chip" :data-status="repo.status">{{ repo.statusLabel }}</span>
    </div>

    <p class="card__desc">
      {{ repo.description || 'This repository has no description on GitHub.' }}
    </p>

    <div class="card__foot">
      <span class="card__version">{{ repo.latestVersion ? `v${repo.latestVersion}` : '—' }}</span>
      <span>{{ formatBytes(repo.latestSize) }}</span>
      <span>{{ timeAgo(repo.refreshedAt) }}</span>
      <div class="card__tools">
        <button
          class="icon-btn icon-btn--sm"
          type="button"
          :aria-label="`Refresh ${repo.displayName}`"
          title="Refresh"
          @click="emit('refresh', repo.id)"
        >
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path
              d="M19 12a7 7 0 1 1-2.4-5.3M19 4.6V9h-4.4"
              fill="none"
              stroke="currentColor"
              stroke-width="1.7"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </button>
        <button class="btn btn--ghost btn--sm" type="button" @click="emit('open', repo.id)">Details</button>
      </div>
    </div>
  </article>
</template>

