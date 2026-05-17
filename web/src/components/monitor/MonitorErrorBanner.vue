<template>
  <q-banner rounded class="monitor-error-banner q-mb-md">
    {{ message }}
    <template #action>
      <q-btn flat dense label="重试" class="monitor-error-banner__action" @click="$emit('retry')" />
    </template>
  </q-banner>
</template>

<script setup lang="ts">
defineProps<{ message: string }>();
defineEmits<{ retry: [] }>();
</script>

<style scoped lang="sass">
.monitor-error-banner
  background: color-mix(in srgb, var(--color-danger) 88%, transparent)
  color: var(--color-on-accent)
  border: 1px solid color-mix(in srgb, var(--color-danger) 55%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.monitor-error-banner__action
  color: var(--color-on-accent) !important

body.body--dark .monitor-error-banner
  background: color-mix(in srgb, var(--color-danger) 28%, transparent)
  color: var(--color-text-primary)
  border-color: color-mix(in srgb, var(--color-danger) 45%, transparent)

body.body--dark .monitor-error-banner__action
  color: var(--color-text-primary) !important
</style>
