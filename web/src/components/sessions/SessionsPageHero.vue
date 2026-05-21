<template>
  <section class="app-page-hero">
    <div>
      <div class="app-page-kicker">{{ overline }}</div>
      <h1 class="app-page-title">{{ title }}</h1>
      <p class="app-page-subtitle">{{ description }}</p>
    </div>
    <div class="app-actions-bar">
      <q-btn outline rounded no-caps color="primary" icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
    </div>
  </section>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string;
    overline?: string;
    description?: string;
    loading?: boolean;
  }>(),
  {
    title: "会话历史",
    overline: "Session history",
    description: "按 Agent、Team、状态与上下文消耗追踪任务运行实例。"
  }
);

defineEmits<{ refresh: [] }>();
</script>
