<template>
  <q-banner rounded class="app-error-banner sessions-error-banner q-mb-md">
    {{ message }}
    <template #action>
      <q-btn flat dense label="重试" class="app-error-banner__action sessions-error-action" @click="$emit('retry')" />
    </template>
  </q-banner>
</template>

<script setup lang="ts">
defineProps<{ message: string }>();
defineEmits<{ retry: [] }>();
</script>
