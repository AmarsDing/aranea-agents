<template>
  <section class="wb-cost" :aria-label="L.title">
    <header class="wb-cost__head">
      <span class="wb-cost__micro">{{ L.title }}</span>
      <span
        v-if="budgetState !== 'none'"
        class="wb-cost__budget"
        :class="`wb-cost__budget--${budgetState}`"
        :title="budgetTip"
      >
        <i class="wb-cost__budget-dot" />{{ budgetLabel }}
      </span>
      <span v-if="loading" class="wb-cost__loading" aria-hidden="true">…</span>
    </header>

    <div class="wb-cost__nums">
      <div class="wb-cost__cell">
        <span class="wb-cost__num" data-testid="cost-today">{{ loaded ? todayText : '—' }}</span>
        <span class="wb-cost__foot">{{ L.today }}</span>
      </div>
      <div class="wb-cost__cell">
        <span class="wb-cost__num" data-testid="cost-month">{{ loaded ? monthText : '—' }}</span>
        <span class="wb-cost__foot">{{ L.month }}</span>
      </div>
    </div>

    <div class="wb-cost__agents">
      <span class="wb-cost__micro">{{ L.topAgents }}</span>
      <template v-if="loaded && topAgents.length">
        <div v-for="a in topAgents" :key="a.agentKey" class="wb-cost__agent">
          <span class="wb-cost__agent-key">@{{ a.agentKey }}</span>
          <span class="wb-cost__agent-cost">{{ formatUsdCompact(a.costMicroUsd) }}</span>
        </div>
      </template>
      <div v-else class="wb-cost__agent wb-cost__agent--empty">{{ loaded ? L.emptyAgents : '—' }}</div>
    </div>

    <footer v-if="error" class="wb-cost__error" role="alert">
      <span class="wb-cost__error-text">{{ error }}</span>
      <button type="button" class="wb-cost__retry" @click="$emit('refresh')">{{ L.retry }}</button>
    </footer>
  </section>
</template>

<script setup lang="ts">
/**
 * 作战总台 LLM 成本卡（E阶段 W2b D6 V5 后半 / F2，验收 #8）。
 * 纯展示族：数据全部经 props 注入（页面组装 useLlmCostCard），自身零数据流。
 * 数字=真实 usage overview 口径（formatUsdCompact 与用量页同函数），禁演示数字。
 *
 * i18n 说明：cockpit 语言文件为 peer 在制品，本卡文案走 labels prop（中文默认内联）；
 * 挂载收口时在 locale 补 workbench.cost.* 键并由页面注入 t() 文案即可无缝切换。
 */
import { computed } from 'vue';
import { formatUsdCompact, formatPercent } from '../../features/usage/moneyFormat';
import type { LlmCostBudgetState, LlmCostTopAgent } from '../../features/workbench/useLlmCostCard';

export interface LlmCostCardLabels {
  title: string;
  today: string;
  month: string;
  topAgents: string;
  emptyAgents: string;
  budgetOk: string;
  budgetWarn: string;
  budgetOver: string;
  retry: string;
}

const DEFAULT_LABELS: LlmCostCardLabels = {
  title: 'LLM 成本',
  today: '今日',
  month: '本月',
  topAgents: 'TOP AGENT',
  emptyAgents: '暂无消耗',
  budgetOk: '预算正常',
  budgetWarn: '预算预警',
  budgetOver: '预算超支',
  retry: '重试',
};

const props = withDefaults(
  defineProps<{
    loaded: boolean;
    loading?: boolean;
    error?: string;
    todayCostMicroUsd: number;
    monthCostMicroUsd: number;
    topAgents: LlmCostTopAgent[];
    budgetState: LlmCostBudgetState;
    /** quota_dashboard.max_utilization_ratio（budgetState=none 时忽略） */
    quotaRatio?: number;
    labels?: Partial<LlmCostCardLabels>;
  }>(),
  { loading: false, error: '', quotaRatio: 0, labels: () => ({}) },
);

defineEmits<{ (e: 'refresh'): void }>();

const L = computed(() => ({ ...DEFAULT_LABELS, ...props.labels }));

const todayText = computed(() => formatUsdCompact(props.todayCostMicroUsd));
const monthText = computed(() => formatUsdCompact(props.monthCostMicroUsd));

const budgetLabel = computed(() => {
  switch (props.budgetState) {
    case 'ok':
      return L.value.budgetOk;
    case 'warn':
      return L.value.budgetWarn;
    case 'over':
      return L.value.budgetOver;
    default:
      return '';
  }
});

const budgetTip = computed(() =>
  props.budgetState === 'none' ? '' : `${L.value.title} · ${formatPercent(props.quotaRatio)}`,
);
</script>

<style scoped lang="sass">
.wb-cost
  display: flex
  flex-direction: column
  gap: 6px
  padding: 8px 10px
  border: 1px solid var(--wb-line-hairline)
  border-radius: var(--wb-radius)
  background: color-mix(in srgb, var(--color-text-primary) 3%, transparent)
  min-height: 0
.wb-cost__head
  display: flex
  align-items: center
  gap: 6px
.wb-cost__micro
  font-size: var(--wb-micro-label)
  letter-spacing: 0.14em
  text-transform: uppercase
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-cost__head > .wb-cost__micro
  flex: 1
.wb-cost__loading
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)
.wb-cost__budget
  display: inline-flex
  align-items: center
  gap: 4px
  font-size: var(--wb-micro-label)
  letter-spacing: 0.08em
.wb-cost__budget-dot
  width: 6px
  height: 6px
  border-radius: 50%
.wb-cost__budget--ok
  color: var(--wb-ok)
  .wb-cost__budget-dot
    background: var(--wb-ok)
.wb-cost__budget--warn
  color: var(--wb-await)
  .wb-cost__budget-dot
    background: var(--wb-await)
.wb-cost__budget--over
  color: var(--wb-block)
  .wb-cost__budget-dot
    background: var(--wb-block)
.wb-cost__nums
  display: grid
  grid-template-columns: 1fr 1fr
  gap: 8px
.wb-cost__cell
  display: flex
  flex-direction: column
  gap: 1px
.wb-cost__num
  font-size: 20px
  line-height: 1.1
  font-weight: 700
  font-variant-numeric: tabular-nums
  color: var(--color-text-primary)
.wb-cost__foot
  font-size: var(--wb-micro-label)
  color: var(--color-text-secondary)
.wb-cost__agents
  display: flex
  flex-direction: column
  gap: 2px
  border-top: 1px solid var(--wb-line-hairline)
  padding-top: 4px
.wb-cost__agent
  display: flex
  justify-content: space-between
  gap: 8px
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)
.wb-cost__agent-key
  font-weight: 600
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.wb-cost__agent-cost
  font-variant-numeric: tabular-nums
  color: var(--color-text-primary)
.wb-cost__agent--empty
  color: var(--color-text-secondary)
.wb-cost__error
  display: flex
  align-items: center
  gap: 8px
  border-top: 1px solid var(--wb-line-hairline)
  padding-top: 4px
.wb-cost__error-text
  flex: 1
  min-width: 0
  font-size: var(--wb-caption)
  color: var(--wb-block)
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.wb-cost__retry
  flex: none
  font-size: var(--wb-caption)
  color: var(--wb-flow)
  background: transparent
  border: 1px solid var(--wb-line-hairline)
  border-radius: var(--wb-radius)
  padding: 2px 8px
  cursor: pointer
  &:hover
    border-color: var(--wb-flow)
  &:focus-visible
    outline: 2px solid var(--wb-flow)
    outline-offset: 2px
</style>
