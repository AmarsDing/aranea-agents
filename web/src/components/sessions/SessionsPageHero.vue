<template>
  <section class="sessions-hero sessions-glass-panel q-pa-md q-mb-md">
    <div>
      <div class="sessions-hero-overline text-overline">{{ overline }}</div>
      <h1 class="sessions-title">{{ title }}</h1>
      <p class="sessions-muted q-mt-xs q-mb-none">{{ description }}</p>
    </div>
    <q-btn
      flat
      round
      icon="refresh"
      class="sessions-icon-btn"
      :loading="loading"
      aria-label="刷新"
      @click="$emit('refresh')"
    />
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

<style scoped>
.sessions-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.sessions-title {
  margin: 0;
  font-size: clamp(28px, 4vw, 42px);
  line-height: 1.1;
  font-weight: 800;
  color: var(--color-text-primary);
}

.sessions-hero-overline {
  color: var(--color-accent);
}

.sessions-muted {
  color: var(--color-text-secondary);
}
</style>
