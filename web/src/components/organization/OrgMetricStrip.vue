<template>
  <div class="app-metrics-grid org-metric-strip">
    <div class="app-metrics-card">
      <div class="app-metrics-card__label">{{ t('organization.market.metricEnabled') }}</div>
      <span class="app-metrics-card__value app-mono">{{ summary.enabled }}</span>
      <div class="app-metrics-card__foot">
        <span class="org-metric-strip__delta-up">{{ t('organization.market.metricEnabledDelta') }}</span>
        {{ t('organization.market.metricEnabledFootNoDelta') }}
      </div>
    </div>
    <div class="app-metrics-card">
      <div class="app-metrics-card__label">{{ t('organization.market.metricDepartments') }}</div>
      <span class="app-metrics-card__value app-mono">{{ summary.departments }}</span>
      <div class="app-metrics-card__foot">{{ t('organization.market.metricDepartmentsFoot') }}</div>
    </div>
    <div class="app-metrics-card">
      <div class="app-metrics-card__label">{{ t('organization.market.metricPositions') }}</div>
      <span class="app-metrics-card__value app-mono">{{ summary.positions }}</span>
      <div class="app-metrics-card__foot">
        {{ t('organization.market.metricPositionsFoot', { ratio: avgAgentsPerPosition }) }}
      </div>
    </div>
    <div class="app-metrics-card">
      <div class="app-metrics-card__label">{{ t('organization.market.metricAgents') }}</div>
      <span class="app-metrics-card__value app-mono">{{ summary.agents }}</span>
      <div class="app-metrics-card__foot">
        {{ t('organization.market.metricAgentsFoot', { installed: summary.installed }) }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { OrgSummary } from '../../features/organization/useOrgMarket';

const props = defineProps<{ summary: OrgSummary }>();
const { t } = useI18n();

const avgAgentsPerPosition = computed(() => {
  if (props.summary.positions === 0) return '0';
  return (props.summary.agents / props.summary.positions).toFixed(1);
});
</script>

<style lang="sass" scoped>
.org-metric-strip
  margin: 20px 0

.org-metric-strip__delta-up
  color: var(--color-success, #4CAF7C)
  font-weight: 600

.app-mono
  font-family: 'JetBrains Mono', 'SF Mono', Menlo, monospace
  font-feature-settings: 'tnum' 1
</style>
