<template>
  <div class="app-inline-stats-strip" role="group" aria-label="使用统计">
    <div class="app-inline-stats-metric">
      <span class="app-inline-stats-metric__label">使用</span>
      <span class="app-inline-stats-metric__value">{{ skill.invoke_count }}</span>
      <span class="app-inline-stats-metric__meta">7d {{ skill.usage_count_7d }}</span>
    </div>
    <span class="app-inline-stats-sep" aria-hidden="true" />
    <div class="app-inline-stats-metric">
      <span class="app-inline-stats-metric__label">成功</span>
      <span class="app-inline-stats-metric__value text-positive">{{ skill.success_count }}</span>
      <span class="app-inline-stats-metric__meta text-negative">败 {{ skill.failure_count }}</span>
    </div>
    <span class="app-inline-stats-sep" aria-hidden="true" />
    <div class="app-inline-stats-metric">
      <span class="app-inline-stats-metric__label">耗时</span>
      <span class="app-inline-stats-metric__value">{{ formatDuration(skill.avg_duration_ms) }}</span>
      <span class="app-inline-stats-metric__meta">近 {{ formatDuration(skill.last_duration_ms) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Skill } from '../../features/skills/types';

defineProps<{
  skill: Skill;
}>();

function formatDuration(value?: number | null) {
  if (value === undefined || value === null) return '-';
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(1)}s`;
}
</script>
