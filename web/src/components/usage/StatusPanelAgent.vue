<template>
  <q-card flat class="command-center-stat-panel">
    <q-card-section class="q-pb-sm">
      <div class="command-center-stat-panel__header">
        <q-icon
          name="smart_toy"
          size="16px"
          class="command-center-stat-panel__icon command-center-stat-panel__icon--agent"
        />
        <span class="command-center-stat-panel__title">Agent 状态</span>
      </div>
    </q-card-section>
    <q-card-section class="q-pt-none">
      <div v-if="loading" class="row justify-center q-py-md">
        <q-skeleton type="circle" size="64px" />
      </div>
      <template v-else>
        <div class="command-center-stat-panel__ring-wrap">
          <svg class="command-center-stat-panel__ring" viewBox="0 0 36 36">
            <circle
              class="command-center-stat-panel__ring-track"
              cx="18"
              cy="18"
              r="15.9"
              fill="none"
              stroke-width="3"
            />
            <circle
              class="command-center-stat-panel__ring-fill"
              cx="18"
              cy="18"
              r="15.9"
              fill="none"
              stroke-width="3"
              :stroke-dasharray="`${activePercent} ${100 - activePercent}`"
              stroke-dashoffset="25"
            />
          </svg>
          <div class="command-center-stat-panel__ring-text">
            <span class="command-center-stat-panel__ring-value">{{ active }}</span>
            <span class="command-center-stat-panel__ring-label">/ {{ total }}</span>
          </div>
        </div>
        <div class="command-center-stat-panel__detail">
          <span class="command-center-stat-panel__dot command-center-stat-panel__dot--active" />
          <span>在线 {{ active }}</span>
          <span class="command-center-stat-panel__dot command-center-stat-panel__dot--inactive q-ml-sm" />
          <span>离线 {{ total - active }}</span>
        </div>
      </template>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  active: number;
  total: number;
  loading: boolean;
}>();

const activePercent = computed(() => {
  if (props.total === 0) return 0;
  return Math.round((props.active / props.total) * 100);
});
</script>

<style lang="sass">
.command-center-stat-panel__icon--agent
  color: var(--color-accent-indigo, #4F46E5)

body.body--dark .command-center-stat-panel__icon--agent
  color: var(--color-accent-indigo, #818CF8)

.command-center-stat-panel__ring-fill
  stroke: var(--color-accent-indigo, #4F46E5)

body.body--dark .command-center-stat-panel__ring-fill
  stroke: var(--color-accent-indigo, #818CF8)

.command-center-stat-panel__ring-track
  stroke: rgba(128, 128, 128, 0.12)

body.body--dark .command-center-stat-panel__ring-track
  stroke: var(--glass-border, rgba(255, 255, 255, 0.08))
</style>
