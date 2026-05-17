<template>
  <q-banner rounded class="sessions-error-banner q-mb-md">
    {{ message }}
    <template #action>
      <q-btn flat dense label="重试" class="sessions-error-action" @click="$emit('retry')" />
    </template>
  </q-banner>
</template>

<script setup lang="ts">
defineProps<{ message: string }>();
defineEmits<{ retry: [] }>();
</script>

<style scoped>
.sessions-error-banner {
  background: color-mix(in srgb, var(--color-danger) 88%, transparent);
  color: var(--color-on-accent);
  border: 1px solid color-mix(in srgb, var(--color-danger) 55%, transparent);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.sessions-error-action {
  color: var(--color-on-accent) !important;
}
</style>
