<script setup lang="ts">
// 经营脉冲带：效率与规模指标（转化 / 供应商覆盖 / 成本 / 活跃会话）。
// 与 Hero 的四个业务主数字互补，不重复；所有数值来自 /v1/ceoboard/company。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { CompanySnapshot, OpsMetric } from '../../features/ceoboard/types';

const props = defineProps<{ snap: CompanySnapshot }>();
const emit = defineEmits<{ drill: [path: string] }>();
const { t } = useI18n();

const ids = ['conversion_rate', 'supply_coverage', 'cost_today', 'active_sessions'] as const;

const metrics = computed<OpsMetric[]>(() =>
  ids.map((id) => props.snap.kpis.find((k) => k.id === id)).filter((m): m is OpsMetric => !!m),
);

const facts = computed(() => props.snap.company);
const unready = computed(() => props.snap.coverage.filter((c) => !c.ready).length);

function name(m: OpsMetric): string {
  const key = `ceoboard.ops.kpis.${m.id}`;
  const text = t(key);
  return text === key ? m.name : text;
}
function windowLabel(w: string): string {
  const key = `ceoboard.ops.windows.${w}`;
  const text = t(key);
  return text === key ? w : text;
}
function go(m: OpsMetric) {
  if (m.drill?.path) emit('drill', m.drill.path);
}
</script>

<template>
  <section class="ceo-pulse" :aria-label="t('ceoboard.ops.pulseAria')">
    <button
      v-for="m in metrics"
      :key="m.id"
      type="button"
      class="ceo-pulse__item"
      :class="[m.health, { pending: m.pending, link: !!m.drill?.path }]"
      :title="[m.source, m.rule].filter(Boolean).join(' · ')"
      @click="go(m)"
    >
      <span class="ceo-pulse__label">{{ name(m) }}</span>
      <strong class="ceo-pulse__num">{{ m.display }}</strong>
      <span
        v-if="m.unit === '%' && !m.pending"
        class="ceo-pulse__bar"
        :style="{ '--v': `${Math.max(0, Math.min(100, m.value))}%` }"
        aria-hidden="true"
      />
      <span class="ceo-pulse__meta">
        {{ windowLabel(m.window) }}<template v-if="m.pending"> · {{ t('ceoboard.ops.noDenominator') }}</template>
      </span>
    </button>
    <div class="ceo-pulse__facts">
      <span>{{ t('ceoboard.ops.factDepartments', { n: facts.departments }) }}</span>
      <span>{{ t('ceoboard.ops.factPositions', { n: facts.positions }) }}</span>
      <span>{{ t('ceoboard.ops.factAgents', { n: facts.agents }) }}</span>
      <span>{{ t('ceoboard.ops.factTeams', { n: facts.teams }) }}</span>
      <span v-if="unready" class="ceo-pulse__coverage">{{ t('ceoboard.ops.coverageShort', { n: unready }) }}</span>
    </div>
  </section>
</template>
