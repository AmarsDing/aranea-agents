<template>
  <div v-if="visible" class="spirit-status-bar">
    <div class="row items-center no-wrap q-gutter-sm spirit-status-bar__inner">
      <span class="spirit-status-bar__dot" />
      <div v-if="complexityLevel" class="spirit-status-bar__item">
        <q-icon :name="complexityIcon" size="14px" :style="{ color: complexityColor }" />
        <span>{{ complexityLabel }}</span>
        <q-tooltip v-if="complexityReason" :delay="300">{{ complexityReason }}</q-tooltip>
      </div>
      <div
        v-if="runningTeamCount > 0"
        class="spirit-status-bar__item spirit-status-bar__item--clickable"
        @click="emit('click-running')"
      >
        <q-icon name="bolt" size="14px" :style="{ color: 'var(--color-accent)' }" />
        <span>{{ t('spirit.runningCount', { count: runningTeamCount }) }}</span>
      </div>
      <div
        v-if="interruptedTeamCount > 0"
        class="spirit-status-bar__item spirit-status-bar__item--clickable"
        @click="emit('click-interrupted')"
      >
        <q-icon name="pause_circle" size="14px" :style="{ color: 'var(--color-warning)' }" />
        <span>{{ t('spirit.interruptedCount', { count: interruptedTeamCount }) }}</span>
      </div>
      <div
        v-if="checkpointStep"
        class="spirit-status-bar__item spirit-status-bar__item--hide-sm spirit-status-bar__item--shift-md"
      >
        <q-icon name="flag" size="14px" :style="{ color: 'var(--color-text-tertiary)' }" />
        <span class="ellipsis">{{ checkpointStep }}</span>
      </div>
      <div
        v-if="totalTeamCount > 0"
        class="spirit-status-bar__item spirit-status-bar__item--hide-sm spirit-status-bar__item--shift-md"
      >
        <q-icon
          :name="allCompleted ? 'task_alt' : 'bar_chart'"
          size="14px"
          :style="{ color: allCompleted ? 'var(--color-success)' : 'var(--color-text-tertiary)' }"
        />
        <span>{{ t('spirit.teamProgressLabel', { done: completedTeamCount, total: totalTeamCount }) }}</span>
        <q-linear-progress
          :value="totalTeamCount > 0 ? completedTeamCount / totalTeamCount : 0"
          size="3px"
          rounded
          :color="progressColor"
          class="spirit-status-bar__quota-bar"
        />
      </div>
      <div
        v-if="hasContextInfo"
        class="spirit-status-bar__item spirit-status-bar__item--hide-sm spirit-status-bar__item--shift-md spirit-status-bar__item--clickable"
        @click.stop="contextMenuOpen = !contextMenuOpen"
      >
        <q-icon name="data_usage" size="14px" :style="{ color: 'var(--color-text-tertiary)' }" />
        <span>{{ contextLabel }}</span>
        <span v-if="inOutLabel" class="spirit-status-bar__inout"> · {{ inOutLabel }}</span>
        <q-menu
          v-model="contextMenuOpen"
          anchor="bottom middle"
          self="top middle"
          :offset="[0, 6]"
          transition-show="fade"
          transition-hide="fade"
          :content-style="popupContentStyle"
          class="spirit-context-popup"
          @click.stop
        >
          <div class="spirit-context-popup__inner">
            <div class="spirit-context-popup__header">
              <div class="spirit-context-popup__header-row">
                <span class="text-weight-medium">{{ t('chat.contextBudgetTitle') }}</span>
                <q-icon name="close" size="14px" class="spirit-context-popup__close" @click="contextMenuOpen = false" />
              </div>
              <div class="spirit-context-popup__totals">
                <span>{{ t('chat.contextBudgetFull', { pct: contextPctLabel }) }}</span>
                <span v-if="contextTokenLabel" class="spirit-context-popup__sub">~{{ contextTokenLabel }}</span>
              </div>
            </div>

            <template v-if="budgetRows.length">
              <!-- Stacked composition bar: segments sized against the context window. -->
              <div class="spirit-context-popup__stack" role="img" :aria-label="t('chat.contextBudgetTitle')">
                <div
                  v-for="row in budgetRows"
                  :key="row.key"
                  class="spirit-context-popup__stack-seg"
                  :style="{ width: stackSegWidth(row), background: row.color }"
                />
              </div>
              <div class="spirit-context-popup__rows">
                <div v-for="row in budgetRows" :key="row.key" class="spirit-context-popup__row">
                  <span class="spirit-context-popup__dot" :style="{ background: row.color }" />
                  <span class="spirit-context-popup__row-label">
                    {{ t(`chat.${row.labelKey}`) }}
                    <span v-if="row.key === toolsSchemaKey && budgetToolsCount > 0" class="spirit-context-popup__sub">
                      ({{ t('chat.contextBudgetToolsCount', { count: budgetToolsCount }) }})
                    </span>
                  </span>
                  <span class="spirit-context-popup__row-value">{{ formatCtxTokens(row.estTokens) }}</span>
                </div>
                <template v-if="budgetTopTools.length">
                  <div
                    v-for="tool in budgetTopTools"
                    :key="tool.name"
                    class="spirit-context-popup__row spirit-context-popup__row--sub"
                  >
                    <span class="spirit-context-popup__row-label ellipsis">{{ tool.name }}</span>
                    <span class="spirit-context-popup__row-value">{{ formatCtxTokens(tool.est_tokens) }}</span>
                  </div>
                </template>
              </div>
              <div class="spirit-context-popup__estimate">{{ t('chat.contextBudgetEstimated') }}</div>
            </template>

            <template v-else>
              <q-separator class="spirit-context-popup__sep" />
              <div class="spirit-context-popup__title">{{ t('chat.modelTokenChartTitle') }}</div>
              <ModelTokenChart :data="modelTokensData" :loading="modelTokensLoading" />
            </template>
          </div>
        </q-menu>
      </div>
      <div v-if="dqScore != null" class="spirit-status-bar__item spirit-status-bar__item--hide-sm">
        <q-icon name="verified" size="14px" :style="{ color: dqScoreColor }" />
        <span :style="{ color: dqScoreColor }">DQ: {{ dqScore.toFixed(2) }}</span>
        <q-tooltip :delay="300">{{ t('spirit.dqScoreTooltip') }}</q-tooltip>
      </div>
      <!-- View toggle button -->
      <div class="spirit-status-bar__item spirit-status-bar__item--clickable" @click="emit('toggle-view')">
        <q-icon
          :name="viewMode === 'observe' ? 'chat' : 'visibility'"
          size="14px"
          :style="{ color: viewMode === 'observe' ? 'var(--color-primary)' : 'var(--color-text-tertiary)' }"
        />
        <span>{{ viewMode === 'observe' ? t('spirit.backToChat') : t('spirit.observeView') }}</span>
      </div>

      <!-- Composer toggle (only in observe mode) -->
      <div
        v-if="viewMode === 'observe'"
        class="spirit-status-bar__item spirit-status-bar__item--clickable"
        @click="emit('toggle-composer')"
      >
        <q-icon
          :name="composerVisible ? 'keyboard' : 'keyboard_hide'"
          size="14px"
          :style="{ color: 'var(--color-text-tertiary)' }"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, toRef, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { COMPLEXITY_CONFIG, dqScoreColor as getDqScoreColor } from '../../features/spirit/spiritUi';
import { formatTokenCount as formatCtxTokens } from '../../features/chat/composerUsageMetrics';
import {
  CONTEXT_BUDGET_CATEGORY,
  buildContextBudgetRows,
  type ContextBudgetRow,
} from '../../features/chat/contextBudget';
import type { ContextBudgetSnapshot } from '../../features/session/types';
import { CHAT_CONTEXT_WINDOW_TOKENS, chatContextRatio } from '../../features/session/contextMetrics';
import { useSessionModelTokens } from '../../features/chat/useSessionModelTokens';
import ModelTokenChart from '../chat/ModelTokenChart.vue';

const { t } = useI18n();

const props = defineProps<{
  runningTeamCount: number;
  interruptedTeamCount: number;
  /** Number of teams that have reached a terminal "completed" state. */
  completedTeamCount: number;
  /** Total number of teams in the current orchestration. */
  totalTeamCount: number;
  tokenUsage?: { in: number; out: number } | null;
  /** Context usage ratio (0-1). */
  contextRatio?: number | null;
  /** Current context tokens used. */
  contextUsedTokens?: number | null;
  /** Chat context window size in tokens (product 256K standard). */
  contextWindow?: number | null;
  /** Latest turn's prompt-assembly breakdown (WS context_usage push only). */
  contextBudget?: ContextBudgetSnapshot | null;
  /** Current session id — used to fetch per-model token breakdown. */
  sessionId?: string | null;
  /** Complexity level from spirit_plan_created event (simple/moderate/complex). */
  complexityLevel?: string | null;
  /** Strategy reason from spirit_plan_created event. */
  complexityReason?: string | null;
  /** Current orchestration checkpoint step from spirit_orchestration_checkpoint event. */
  checkpointStep?: string | null;
  /** Last DQ score from spirit_team_completed event. */
  dqScore?: number | null;
  /** Current view mode (chat / observe). */
  viewMode?: 'chat' | 'observe';
  /** Whether the composer is visible. */
  composerVisible?: boolean;
}>();

const emit = defineEmits<{
  'click-running': [];
  'click-interrupted': [];
  'toggle-view': [];
  'toggle-composer': [];
}>();

const visible = computed(
  () =>
    props.runningTeamCount > 0 ||
    props.interruptedTeamCount > 0 ||
    props.totalTeamCount > 0 ||
    !!props.tokenUsage ||
    props.contextRatio != null ||
    !!props.complexityLevel ||
    !!props.checkpointStep ||
    props.dqScore != null,
);

// Compact input/output label for the status bar: ↑1.2k ↓0.8k
const inOutLabel = computed(() => {
  const tin = props.tokenUsage?.in ?? 0;
  const tout = props.tokenUsage?.out ?? 0;
  if (!tin && !tout) return '';
  const fmt = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : `${n}`);
  return `↑${fmt(tin)} ↓${fmt(tout)}`;
});

// 上下文标识：百分比 + 当前 / 产品 256K chat context（不采用 provider 窗口）
const displayWindow = CHAT_CONTEXT_WINDOW_TOKENS;
const displayRatio = computed(() => {
  const used = props.contextUsedTokens;
  if (used != null && used > 0) {
    return chatContextRatio(used) ?? 0;
  }
  return props.contextRatio ?? 0;
});
const clampedRatio = computed(() => Math.min(1, Math.max(0, displayRatio.value)));
const contextPctLabel = computed(() => `${Math.round(clampedRatio.value * 100)}%`);
const contextTokenLabel = computed(() => {
  const used = props.contextUsedTokens;
  const usedStr = used != null && used > 0 ? formatCtxTokens(used) : '';
  if (!usedStr) return '';
  return `${usedStr} / ${formatCtxTokens(displayWindow)}`;
});
const contextLabel = computed(() => {
  if (!contextTokenLabel.value) return contextPctLabel.value;
  return `${contextPctLabel.value} · ${contextTokenLabel.value}`;
});
const hasContextInfo = computed(() => props.contextRatio != null || !!contextTokenLabel.value || !!props.tokenUsage);

// ── Prompt-assembly breakdown (Cursor 风格分项可观测性) ────────────────
// Rows come from the backend context_budget ledger pushed with the
// context_usage WS event. When absent (older backend / no ledger), the popup
// falls back to the legacy per-model token chart below.
const budgetRows = computed(() => buildContextBudgetRows(props.contextBudget));
const budgetToolsCount = computed(() => props.contextBudget?.tools_count ?? 0);
const budgetTopTools = computed(() => props.contextBudget?.top_tools ?? []);
const toolsSchemaKey = CONTEXT_BUDGET_CATEGORY.toolsSchema;

// Stacked-bar segment widths are fractions of the chat context window so the
// bar reads as "how full the window is", matching the header percentage.
function stackSegWidth(row: ContextBudgetRow): string {
  const window = displayWindow > 0 ? displayWindow : 1;
  const pct = Math.min(100, Math.max(0.4, (row.estTokens / window) * 100));
  return `${pct}%`;
}

// Per-model token usage for the popup chart (fallback when no budget ledger).
const sessionIdRef = toRef(props, 'sessionId');
const {
  data: modelTokensData,
  loading: modelTokensLoading,
  reload: reloadModelTokens,
} = useSessionModelTokens(sessionIdRef);

// Click-controlled popup for the context item (点击固定, dismissed via the
// close icon or q-menu's default outside-click).
const contextMenuOpen = ref(false);

// Fallback chart only: refresh when the popup opens without a budget ledger.
watch(contextMenuOpen, (open) => {
  if (open && !budgetRows.value.length) void reloadModelTokens();
});

// Theme-consistent popup styling.
const popupContentStyle = {
  background: 'var(--color-surface-elevated)',
  color: 'var(--color-text-primary)',
  border: '1px solid var(--glass-border)',
  borderRadius: '10px',
  boxShadow: '0 8px 24px rgba(0,0,0,0.18)',
};

const complexityLabel = computed(() => {
  if (!props.complexityLevel) return '';
  return COMPLEXITY_CONFIG[props.complexityLevel]?.label ?? props.complexityLevel;
});

const complexityIcon = computed(() => {
  if (!props.complexityLevel) return 'tune';
  return COMPLEXITY_CONFIG[props.complexityLevel]?.icon ?? 'tune';
});

const complexityColor = computed(() => {
  if (!props.complexityLevel) return 'var(--color-text-tertiary)';
  return COMPLEXITY_CONFIG[props.complexityLevel]?.color ?? 'var(--color-text-tertiary)';
});

const dqScoreColor = computed(() => getDqScoreColor(props.dqScore));

const allCompleted = computed(() => props.totalTeamCount > 0 && props.completedTeamCount >= props.totalTeamCount);

const progressColor = computed(() => (allCompleted.value ? 'positive' : 'accent'));
</script>

<style scoped lang="sass">
.spirit-status-bar
  height: 28px
  flex-shrink: 0
  position: sticky
  bottom: 0
  z-index: 10
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.spirit-status-bar__inner
  height: 28px
  padding: 0 var(--space-3)
  font-size: 11px
  color: var(--color-text-secondary)
  overflow: hidden

.spirit-status-bar__dot
  width: 5px
  height: 5px
  border-radius: 50%
  background: var(--color-primary)
  flex-shrink: 0

.spirit-status-bar__item
  display: flex
  align-items: center
  gap: 3px
  white-space: nowrap
  flex-shrink: 0

.spirit-status-bar__item--clickable
  cursor: pointer
  border-radius: 4px
  padding: 1px 4px
  transition: background 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--color-accent) 12%, transparent)

.spirit-status-bar__item--shift-md
  transform: translateY(3px)

.spirit-status-bar__inout
  color: var(--color-text-tertiary)
  font-variant-numeric: tabular-nums

.spirit-status-bar__item--hide-sm
  @media (max-width: 600px)
    display: none

.spirit-status-bar__quota-bar
  width: 32px
  flex-shrink: 0
</style>

<style lang="sass">
// Global styles for the q-menu popup (scoped styles don't reach q-menu content).
.spirit-context-popup
  .spirit-context-popup__inner
    padding: 10px 12px
    min-width: 320px

  .spirit-context-popup__header
    display: flex
    flex-direction: column
    gap: 4px
    margin-bottom: 8px

  .spirit-context-popup__header-row
    display: flex
    align-items: center
    justify-content: space-between
    font-size: 13px

  .spirit-context-popup__close
    color: var(--color-text-tertiary)
    cursor: pointer
    border-radius: 4px
    padding: 2px

    &:hover
      color: var(--color-text-primary)
      background: color-mix(in srgb, var(--color-accent) 10%, transparent)

  .spirit-context-popup__totals
    display: flex
    align-items: baseline
    justify-content: space-between
    font-size: 12px

  .spirit-context-popup__sub
    color: var(--color-text-tertiary)

  .spirit-context-popup__stack
    display: flex
    height: 6px
    border-radius: 3px
    overflow: hidden
    background: color-mix(in srgb, var(--color-text-tertiary) 18%, transparent)
    margin-bottom: 10px

  .spirit-context-popup__stack-seg
    height: 100%
    flex-shrink: 0

    & + .spirit-context-popup__stack-seg
      margin-left: 1px

  .spirit-context-popup__rows
    display: flex
    flex-direction: column
    gap: 5px

  .spirit-context-popup__row
    display: flex
    align-items: center
    gap: 8px
    font-size: 12px

  .spirit-context-popup__row--sub
    padding-left: 18px
    font-size: 11px
    color: var(--color-text-tertiary)

    .spirit-context-popup__row-value
      color: var(--color-text-tertiary)

  .spirit-context-popup__dot
    width: 10px
    height: 10px
    border-radius: 3px
    flex-shrink: 0

  .spirit-context-popup__row-label
    flex: 1
    min-width: 0
    color: var(--color-text-secondary)

  .spirit-context-popup__row-value
    margin-left: auto
    font-variant-numeric: tabular-nums
    color: var(--color-text-primary)

  .spirit-context-popup__estimate
    margin-top: 8px
    font-size: 10px
    color: var(--color-text-tertiary)

  .spirit-context-popup__sep
    background: var(--glass-border)
    margin: 8px 0

  .spirit-context-popup__title
    font-size: 12px
    font-weight: 600
    color: var(--color-text-secondary)
    margin-bottom: 6px
</style>
