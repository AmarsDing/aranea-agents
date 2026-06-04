<template>
  <div class="app-metrics-grid industry-metric-strip">
    <div class="app-metrics-card">
      <div class="app-metrics-card__label">{{ t('industries.market.metricEnabled') }}</div>
      <span class="app-metrics-card__value app-mono">{{ summary.enabled }}</span>
      <div class="app-metrics-card__foot">
        <span class="industry-metric-strip__delta-up">{{ t('industries.market.metricEnabledDelta') }}</span>
        {{ t('industries.market.metricEnabledFootNoDelta') }}
      </div>
    </div>
    <div class="app-metrics-card">
      <div class="app-metrics-card__label">{{ t('industries.market.metricDepartments') }}</div>
      <span class="app-metrics-card__value app-mono">{{ summary.departments }}</span>
      <div class="app-metrics-card__foot">{{ t('industries.market.metricDepartmentsFoot') }}</div>
    </div>
    <div class="app-metrics-card">
      <div class="app-metrics-card__label">{{ t('industries.market.metricPositions') }}</div>
      <span class="app-metrics-card__value app-mono">{{ summary.positions }}</span>
      <div class="app-metrics-card__foot">
        {{ t('industries.market.metricPositionsFoot', { ratio: avgAgentsPerPosition }) }}
      </div>
    </div>
    <div class="app-metrics-card">
      <div class="app-metrics-card__label">{{ t('industries.market.metricAgents') }}</div>
      <span class="app-metrics-card__value app-mono">{{ summary.agents }}</span>
      <div class="app-metrics-card__foot">
        {{ t('industries.market.metricAgentsFoot', { installed: summary.installed }) }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { IndustrySummary } from '../../features/industries/useIndustryMarket';

const props = defineProps<{ summary: IndustrySummary }>();
const { t } = useI18n();

const avgAgentsPerPosition = computed(() => {
  if (props.summary.positions === 0) return '0';
  return (props.summary.agents / props.summary.positions).toFixed(1);
});
</script>

<style lang="sass" scoped>
.industry-metric-strip
  margin: 20px 0

.industry-metric-strip__delta-up
  color: var(--color-success, #4CAF7C)
  font-weight: 600

.app-mono
  font-family: 'JetBrains Mono', 'SF Mono', Menlo, monospace
  font-feature-settings: 'tnum' 1
</style>
