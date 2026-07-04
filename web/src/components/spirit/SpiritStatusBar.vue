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
        v-if="quotaMax > 0"
        class="spirit-status-bar__item spirit-status-bar__item--hide-sm spirit-status-bar__item--shift-md"
      >
        <q-icon name="bar_chart" size="14px" :style="{ color: 'var(--color-text-tertiary)' }" />
        <span>{{ t('spirit.quotaLabel', { used: quotaUsed, max: quotaMax }) }}</span>
        <q-linear-progress
          :value="quotaUsed / quotaMax"
          size="3px"
          rounded
          :color="quotaColor"
          class="spirit-status-bar__quota-bar"
        />
      </div>
      <div
        v-if="hasContextInfo"
        class="spirit-status-bar__item spirit-status-bar__item--hide-sm spirit-status-bar__item--hoverable"
        @mouseenter="onContextEnter"
        @mouseleave="onContextLeave"
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
        >
          <div class="spirit-context-popup__inner">
            <div class="spirit-context-popup__header">
              <div class="text-weight-medium">{{ t('chat.contextPromptUse') }} {{ contextPctLabel }}</div>
              <div v-if="contextTokenLabel" class="text-caption spirit-context-popup__sub">
                {{ t('chat.contextTokensHint') }}: {{ contextTokenLabel }}
              </div>
              <div v-if="tokenLabel" class="text-caption spirit-context-popup__sub">
                {{ tokenLabel }}
              </div>
            </div>
            <q-separator class="spirit-context-popup__sep" />
            <div class="spirit-context-popup__title">{{ t('chat.modelTokenChartTitle') }}</div>
            <ModelTokenChart :data="modelTokensData" :loading="modelTokensLoading" />
          </div>
        </q-menu>
      </div>
      <div v-if="dqScore != null" class="spirit-status-bar__item spirit-status-bar__item--hide-sm">
        <q-icon name="verified" size="14px" :style="{ color: dqScoreColor }" />
        <span :style="{ color: dqScoreColor }">DQ: {{ dqScore.toFixed(2) }}</span>
        <q-tooltip :delay="300">{{ t('spirit.dqScoreTooltip') }}</q-tooltip>
      </div>
      <div
        v-if="lastEvent"
        class="spirit-status-bar__item spirit-status-bar__last-event spirit-status-bar__item--clickable"
        @click="emit('click-last-event')"
      >
        <q-icon
          :name="lastEvent.type === 'completed' ? 'check_circle' : 'error'"
          :style="{ color: lastEvent.type === 'completed' ? 'var(--color-success)' : 'var(--color-danger)' }"
          size="14px"
        />
        <span class="ellipsis">{{ lastEvent.teamName }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, toRef } from 'vue';
import { useI18n } from 'vue-i18n';
import { COMPLEXITY_CONFIG, dqScoreColor as getDqScoreColor, formatTokenCount } from '../../features/spirit/spiritUi';
import { formatTokenCount as formatCtxTokens } from '../../features/chat/composerUsageMetrics';
import { useSessionModelTokens } from '../../features/chat/useSessionModelTokens';
import ModelTokenChart from '../chat/ModelTokenChart.vue';

const { t } = useI18n();

const props = defineProps<{
  runningTeamCount: number;
  interruptedTeamCount: number;
  quotaUsed: number;
  quotaMax: number;
  tokenUsage?: { in: number; out: number } | null;
  /** Context usage ratio (0-1). */
  contextRatio?: number | null;
  /** Current context tokens used. */
  contextUsedTokens?: number | null;
  /** Model context window size in tokens. */
  contextWindow?: number | null;
  /** Current session id — used to fetch per-model token breakdown. */
  sessionId?: string | null;
  lastEvent?: { type: 'completed' | 'failed'; teamName: string; teamId?: string } | null;
  /** Complexity level from spirit_plan_created event (simple/moderate/complex). */
  complexityLevel?: string | null;
  /** Strategy reason from spirit_plan_created event. */
  complexityReason?: string | null;
  /** Current orchestration checkpoint step from spirit_orchestration_checkpoint event. */
  checkpointStep?: string | null;
  /** Last DQ score from spirit_team_completed event. */
  dqScore?: number | null;
}>();

const emit = defineEmits<{
  'click-running': [];
  'click-interrupted': [];
  'click-last-event': [];
}>();

const visible = computed(
  () =>
    props.runningTeamCount > 0 ||
    props.interruptedTeamCount > 0 ||
    props.quotaMax > 0 ||
    !!props.tokenUsage ||
    props.contextRatio != null ||
    !!props.lastEvent ||
    !!props.complexityLevel ||
    !!props.checkpointStep ||
    props.dqScore != null,
);

const tokenLabel = computed(() => formatTokenCount(props.tokenUsage?.in, props.tokenUsage?.out));

// Compact input/output label for the status bar: ↑1.2k ↓0.8k
const inOutLabel = computed(() => {
  const tin = props.tokenUsage?.in ?? 0;
  const tout = props.tokenUsage?.out ?? 0;
  if (!tin && !tout) return '';
  const fmt = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : `${n}`);
  return `↑${fmt(tin)} ↓${fmt(tout)}`;
});

// 上下文圆环功能（原 ChatComposer 中的圆环）：百分比 + 当前/模型上下文 token 量
const clampedRatio = computed(() => Math.min(1, Math.max(0, props.contextRatio ?? 0)));
const contextPctLabel = computed(() => `${Math.round(clampedRatio.value * 100)}%`);
const contextTokenLabel = computed(() => {
  const used = props.contextUsedTokens;
  const win = props.contextWindow;
  const usedStr = used != null && used > 0 ? formatCtxTokens(used) : '';
  const winStr = win != null && win > 0 ? formatCtxTokens(win) : '';
  if (usedStr && winStr) return `${usedStr} / ${winStr}`;
  return usedStr || winStr;
});
const contextLabel = computed(() => {
  if (!contextTokenLabel.value) return contextPctLabel.value;
  return `${contextPctLabel.value} · ${contextTokenLabel.value}`;
});
const hasContextInfo = computed(() => props.contextRatio != null || !!contextTokenLabel.value || !!props.tokenUsage);

// Per-model token usage for the popup chart.
const sessionIdRef = toRef(props, 'sessionId');
const { data: modelTokensData, loading: modelTokensLoading } = useSessionModelTokens(sessionIdRef);

// Hover-controlled popup for the context item.
const contextMenuOpen = ref(false);
let hideTimer: ReturnType<typeof setTimeout> | null = null;

function onContextEnter() {
  if (hideTimer) {
    clearTimeout(hideTimer);
    hideTimer = null;
  }
  contextMenuOpen.value = true;
}

function onContextLeave() {
  // Defer close so moving the cursor into the popup doesn't lose it.
  hideTimer = setTimeout(() => {
    contextMenuOpen.value = false;
    hideTimer = null;
  }, 200);
}

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

const quotaColor = computed(() => {
  if (props.quotaUsed >= props.quotaMax) return 'negative';
  if (props.quotaUsed >= props.quotaMax * 0.8) return 'warning';
  return 'accent';
});
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

.spirit-status-bar__item--hoverable
  cursor: default
  border-radius: 4px
  padding: 1px 4px
  transform: translateY(3px)
  transition: background 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--color-accent) 8%, transparent)

.spirit-status-bar__inout
  color: var(--color-text-tertiary)
  font-variant-numeric: tabular-nums

.spirit-status-bar__last-event
  margin-left: auto
  max-width: 160px

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
    gap: 2px

  .spirit-context-popup__sub
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
