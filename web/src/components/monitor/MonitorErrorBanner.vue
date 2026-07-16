<template>
  <q-banner rounded class="app-error-banner monitor-error-banner q-mb-md">
    {{ message }}
    <template #action>
      <q-btn
        flat
        dense
        :label="actionLabel"
        class="app-error-banner__action monitor-error-banner__action"
        @click="$emit('retry')"
      />
    </template>
  </q-banner>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ message: string; actionLabel?: string }>(), {
  actionLabel: '重试',
});
defineEmits<{ retry: [] }>();
</script>
