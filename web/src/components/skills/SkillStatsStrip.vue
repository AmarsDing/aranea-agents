<template>
  <div class="app-inline-stats-strip" role="group" :aria-label="t('skillsPage.statsTitle')">
    <div class="app-inline-stats-metric">
      <span class="app-inline-stats-metric__label">{{ t('skillsPage.statsUsage') }}</span>
      <span class="app-inline-stats-metric__value">{{ formatCompactCount(skill.invoke_count) }}</span>
      <span class="app-inline-stats-metric__meta">7d {{ formatCompactCount(skill.usage_count_7d) }}</span>
    </div>
    <span class="app-inline-stats-sep" aria-hidden="true" />
    <div class="app-inline-stats-metric">
      <span class="app-inline-stats-metric__label">{{ t('skillsPage.statsSuccess') }}</span>
      <span class="app-inline-stats-metric__value text-positive">{{ formatCompactCount(skill.success_count) }}</span>
      <span class="app-inline-stats-metric__meta text-negative">
        {{ t('skillsPage.statsFailure') }} {{ formatCompactCount(skill.failure_count) }}
      </span>
    </div>
    <span class="app-inline-stats-sep" aria-hidden="true" />
    <div class="app-inline-stats-metric">
      <span class="app-inline-stats-metric__label">{{ t('skillsPage.statsDuration') }}</span>
      <span class="app-inline-stats-metric__value">{{ formatDuration(skill.avg_duration_ms) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { Skill } from '../../features/skills/types';
import { formatCompactCount } from '../../features/usage/moneyFormat';

defineProps<{
  skill: Skill;
}>();

const { t } = useI18n();

function formatDuration(value?: number | null) {
  if (value === undefined || value === null) return '-';
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(1)}s`;
}
</script>
