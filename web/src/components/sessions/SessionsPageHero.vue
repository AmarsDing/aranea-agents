<template>
  <AppPageHero :kicker="overline" :title="title" :subtitle="description">
    <template #actions>
      <q-btn
        outline
        rounded
        no-caps
        color="primary"
        icon="refresh"
        label="刷新"
        :loading="loading"
        @click="$emit('refresh')"
      />
    </template>
  </AppPageHero>
</template>

<script setup lang="ts">
import AppPageHero from '../layout/AppPageHero.vue';

withDefaults(
  defineProps<{
    title?: string;
    overline?: string;
    description?: string;
    loading?: boolean;
  }>(),
  {
    title: '会话历史',
    overline: 'Session history',
    description: '按 Agent、Team、状态与上下文消耗追踪任务运行实例。',
  },
);

defineEmits<{ refresh: [] }>();
</script>
