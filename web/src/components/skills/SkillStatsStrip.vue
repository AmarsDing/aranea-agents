<template>
  <div class="skill-stats-strip">
    <div class="skill-stat">
      <span class="skill-stat__label">使用</span>
      <strong>{{ skill.invoke_count }}</strong>
      <span class="skill-stat__hint">近 7 日 {{ skill.usage_count_7d ?? 0 }}</span>
    </div>
    <div class="skill-stat">
      <span class="skill-stat__label">成功 / 失败</span>
      <strong class="text-positive">{{ skill.success_count }}</strong>
      <span class="skill-stat__hint text-negative">{{ skill.failure_count }}</span>
    </div>
    <div class="skill-stat">
      <span class="skill-stat__label">平均耗时</span>
      <strong>{{ formatDuration(skill.avg_duration_ms) }}</strong>
      <span class="skill-stat__hint">最近 {{ formatDuration(skill.last_duration_ms) }}</span>
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
// 玻璃与边：`app-global.sass` 在 `.skills-page` 下处理 `.skill-stat`
.skill-stats-strip
  display: flex
  flex-wrap: wrap
  gap: 10px

.skill-stat
  min-width: 84px

.skill-stat__label,
.skill-stat__hint
  display: block
  font-size: 11px
  color: var(--color-text-secondary)

.skill-stat strong
  display: inline-block
  margin-top: 2px
  font-size: 15px
</style>
