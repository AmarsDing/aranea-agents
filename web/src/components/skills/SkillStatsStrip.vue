<template>
  <div class="skill-stats-strip" role="group" aria-label="使用统计">
    <div class="skill-stats-metric">
      <span class="skill-stats-metric__label">使用</span>
      <span class="skill-stats-metric__value">{{ skill.invoke_count }}</span>
      <span class="skill-stats-metric__meta">7d {{ skill.usage_count_7d ?? 0 }}</span>
    </div>
    <span class="skill-stats-sep" aria-hidden="true" />
    <div class="skill-stats-metric">
      <span class="skill-stats-metric__label">成功</span>
      <span class="skill-stats-metric__value text-positive">{{ skill.success_count }}</span>
      <span class="skill-stats-metric__meta text-negative">败 {{ skill.failure_count }}</span>
    </div>
    <span class="skill-stats-sep" aria-hidden="true" />
    <div class="skill-stats-metric">
      <span class="skill-stats-metric__label">耗时</span>
      <span class="skill-stats-metric__value">{{ formatDuration(skill.avg_duration_ms) }}</span>
      <span class="skill-stats-metric__meta">近 {{ formatDuration(skill.last_duration_ms) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Skill } from "../../features/skills/types";

defineProps<{
  skill: Skill;
}>();

function formatDuration(value?: number | null) {
  if (value === undefined || value === null) return "-";
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(1)}s`;
}
</script>

<style scoped lang="sass">
.skill-stats-strip
  display: flex
  align-items: center
  flex-wrap: nowrap
  gap: 10px
  min-width: 0
  max-width: 100%

.skill-stats-metric
  display: grid
  grid-template-columns: auto auto
  grid-template-rows: auto auto
  column-gap: 6px
  row-gap: 1px
  align-items: baseline
  min-width: 0

.skill-stats-metric__label
  grid-column: 1
  grid-row: 1 / span 2
  align-self: center
  font-size: 11px
  font-weight: 600
  color: var(--color-text-secondary)
  white-space: nowrap

.skill-stats-metric__value
  grid-column: 2
  grid-row: 1
  font-size: 13px
  font-weight: 700
  line-height: 1.2
  color: var(--color-text-primary)
  white-space: nowrap

.skill-stats-metric__meta
  grid-column: 2
  grid-row: 2
  font-size: 11px
  line-height: 1.2
  color: var(--color-text-tertiary)
  white-space: nowrap

.skill-stats-sep
  flex: 0 0 1px
  align-self: stretch
  min-height: 28px
  background: var(--glass-border)
</style>
