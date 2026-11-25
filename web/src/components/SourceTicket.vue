<script setup lang="ts">
import { ref } from 'vue'

import { copyText, timeAgo } from '../format'

const props = defineProps<{
  url: string
  qr: string
  apps: number
  repos: number
  tint: string
  lastRefresh: string | null
}>()

const copied = ref(false)

async function copy(): Promise<void> {
  if (await copyText(props.url)) {
    copied.value = true
    window.setTimeout(() => {
      copied.value = false
    }, 1600)
  }
}
</script>

<template>
  <section class="ticket" :style="{ '--tint': tint }">
    <div class="ticket__main">
      <span class="eyebrow">Source endpoint</span>
      <p class="ticket__url mono">{{ url }}</p>
      <div class="ticket__acts">
        <button class="btn btn--primary" type="button" @click="copy">
          <svg viewBox="0 0 24 24" aria-hidden="true" class="btn__icon">
            <path
              d="M9 9V5.6A1.6 1.6 0 0 1 10.6 4h7.8A1.6 1.6 0 0 1 20 5.6v7.8a1.6 1.6 0 0 1-1.6 1.6H15"
              fill="none"
              stroke="currentColor"
              stroke-width="1.7"
              stroke-linecap="round"
            />
            <rect
              x="4"
              y="9"
              width="11"
              height="11"
              rx="1.8"
              fill="none"
              stroke="currentColor"
              stroke-width="1.7"
            />
          </svg>
          {{ copied ? 'Copied' : 'Copy URL' }}
        </button>
        <a class="btn btn--ghost" :href="url" target="_blank" rel="noreferrer">Open JSON</a>
        <span class="ticket__meta">{{ apps }} apps · {{ repos }} repos · refreshed {{ timeAgo(lastRefresh) }}</span>
      </div>
    </div>

    <div class="ticket__rail">
      <img class="ticket__qr" :src="qr" width="128" height="128" alt="QR code for the source URL" />
      <span class="ticket__scan">Scan in SideStore</span>
    </div>
  </section>
</template>

