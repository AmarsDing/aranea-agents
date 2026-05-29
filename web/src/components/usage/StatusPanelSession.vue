<template>
  <q-card flat class="command-center-stat-panel">
    <q-card-section class="q-pb-sm">
      <div class="command-center-stat-panel__header">
        <q-icon name="chat" size="16px" class="command-center-stat-panel__icon command-center-stat-panel__icon--session" />
        <span class="command-center-stat-panel__title">会话活跃</span>
      </div>
    </q-card-section>
    <q-card-section class="q-pt-none">
      <div v-if="loading" class="row justify-center q-py-md">
        <q-skeleton type="text" width="60px" />
      </div>
      <template v-else>
        <div class="command-center-stat-panel__big-value">{{ activeCount }}</div>
        <div class="command-center-stat-panel__caption">今日活跃会话</div>
        <div v-if="sparkline.length > 1" class="command-center-stat-panel__sparkline">
          <svg viewBox="0 0 100 24" preserveAspectRatio="none" class="command-center-stat-panel__sparkline-svg">
            <polyline :points="sparklinePoints" fill="none" stroke="var(--color-accent-blue, #2563EB)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </div>
      </template>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  activeCount: number;
  sparkline: number[];
  loading: boolean;
}>();

const sparklinePoints = computed(() => {
  if (props.sparkline.length < 2) return "";
  const max = Math.max(...props.sparkline, 1);
  return props.sparkline
    .map((v, i) => {
      const x = (i / (props.sparkline.length - 1)) * 100;
      const y = 24 - (v / max) * 20;
      return `${x},${y}`;
    })
    .join(" ");
});
</script>

<style lang="sass">
.command-center-stat-panel__icon--session
  color: var(--color-accent-blue, #2563EB)

body.body--dark .command-center-stat-panel__icon--session
  color: var(--color-accent-blue, #3B82F6)
</style>
