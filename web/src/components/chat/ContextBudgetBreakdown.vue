<template>
  <div class="ctx-budget-breakdown">
    <!-- 口径锚点：下方明细是「首轮 prompt 构成」的估算，合计 = est_total_input；
         与 tooltip 标题的轮总 token（实测计费，含多轮工具循环累加）是两个口径。 -->
    <div class="ctx-budget-breakdown__head">
      <span class="ctx-budget-breakdown__head-label">{{ t('chat.contextBudgetFirstRoundTitle') }}</span>
      <span class="ctx-budget-breakdown__head-value">{{ formatTokens(budget.est_total_input) }}</span>
    </div>
    <!-- Stacked composition bar: segments sized against est_total_input so the
         bar reads as the prompt's composition for this turn. -->
    <div class="ctx-budget-breakdown__stack" role="img" :aria-label="t('chat.contextBudgetTitle')">
      <div
        v-for="row in rows"
        :key="row.key"
        class="ctx-budget-breakdown__stack-seg"
        :style="{ width: segWidth(row), background: row.color }"
      />
    </div>
    <div class="ctx-budget-breakdown__rows">
      <div v-for="row in rows" :key="row.key" class="ctx-budget-breakdown__row">
        <span class="ctx-budget-breakdown__dot" :style="{ background: row.color }" />
        <span class="ctx-budget-breakdown__row-label">
          {{ t(`chat.${row.labelKey}`) }}
          <span v-if="row.key === toolsSchemaKey && toolsCount > 0" class="ctx-budget-breakdown__sub">
            ({{ t('chat.contextBudgetToolsCount', { count: toolsCount }) }})
          </span>
        </span>
        <span class="ctx-budget-breakdown__row-value">{{ formatTokens(row.estTokens) }}</span>
      </div>
      <template v-if="topTools.length">
        <div v-for="tool in topTools" :key="tool.name" class="ctx-budget-breakdown__row ctx-budget-breakdown__row--sub">
          <span class="ctx-budget-breakdown__row-label ellipsis">{{ tool.name }}</span>
          <span class="ctx-budget-breakdown__row-value">{{ formatTokens(tool.est_tokens) }}</span>
        </div>
      </template>
    </div>
    <div class="ctx-budget-breakdown__estimate">{{ t('chat.contextBudgetEstimated') }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  CONTEXT_BUDGET_CATEGORY,
  buildContextBudgetRows,
  contextBudgetRowsTotal,
  type ContextBudgetRow,
} from '../../features/chat/contextBudget';
import { formatTokenCount as formatTokens } from '../../features/chat/composerUsageMetrics';
import type { ContextBudgetSnapshot } from '../../features/session/types';

const props = defineProps<{
  budget: ContextBudgetSnapshot;
}>();

const { t } = useI18n();

const rows = computed(() => buildContextBudgetRows(props.budget));
const rowsTotal = computed(() => contextBudgetRowsTotal(rows.value));
const toolsCount = computed(() => props.budget.tools_count ?? 0);
const topTools = computed(() => props.budget.top_tools ?? []);
const toolsSchemaKey = CONTEXT_BUDGET_CATEGORY.toolsSchema;

function segWidth(row: ContextBudgetRow): string {
  // Use the backend-reported est_total_input so bar segments always sum to
  // 100% even when some categories are absent from the ledger (0 values are
  // dropped by parseContextBudgetMeta).
  const total = props.budget.est_total_input > 0 ? props.budget.est_total_input : rowsTotal.value || 1;
  const pct = Math.min(100, Math.max(0.6, (row.estTokens / total) * 100));
  return `${pct}%`;
}
</script>

<style scoped lang="sass">
.ctx-budget-breakdown__head
  display: flex
  align-items: center
  justify-content: space-between
  font-size: 11px
  margin-bottom: 4px

.ctx-budget-breakdown__head-label
  color: var(--color-text-secondary)

.ctx-budget-breakdown__head-value
  color: var(--color-text-primary)
  font-variant-numeric: tabular-nums

.ctx-budget-breakdown__stack
  display: flex
  height: 6px
  border-radius: 3px
  overflow: hidden
  background: color-mix(in srgb, var(--color-text-tertiary) 18%, transparent)
  margin: 2px 0 8px

.ctx-budget-breakdown__stack-seg
  height: 100%

.ctx-budget-breakdown__rows
  display: flex
  flex-direction: column
  gap: 3px
  max-height: 180px
  overflow-y: auto

.ctx-budget-breakdown__row
  display: flex
  align-items: center
  gap: 6px
  font-size: 11px

.ctx-budget-breakdown__row--sub
  padding-left: 14px
  color: var(--color-text-tertiary)

.ctx-budget-breakdown__dot
  width: 8px
  height: 8px
  border-radius: 2px
  flex-shrink: 0

.ctx-budget-breakdown__row-label
  flex: 1
  min-width: 0
  color: var(--color-text-secondary)

.ctx-budget-breakdown__sub
  color: var(--color-text-tertiary)

.ctx-budget-breakdown__row-value
  color: var(--color-text-primary)
  font-variant-numeric: tabular-nums

.ctx-budget-breakdown__estimate
  margin-top: 6px
  font-size: 10px
  color: var(--color-text-tertiary)
</style>
