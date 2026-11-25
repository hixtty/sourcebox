<script setup lang="ts">
import { state } from '../store'
</script>

<template>
  <div class="toasts" role="status" aria-live="polite">
    <div v-for="toast in state.toasts" :key="toast.id" class="toast" :data-tone="toast.tone">
      <span class="toast__dot" aria-hidden="true"></span>
      <span>{{ toast.text }}</span>
    </div>
  </div>
</template>

