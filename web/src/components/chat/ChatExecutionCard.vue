<template>
  <q-expansion-item
    v-model="expanded"
    class="chat-execution-card"
    :class="cardClass"
    dense
    expand-separator
    header-class="chat-execution-card__header"
    :aria-label="headerAriaLabel"
    :aria-expanded="expanded"
    :aria-controls="bodyId"
    @update:model-value="onExpanded"
  >
    <template #header>
      <div class="row items-center no-wrap full-width q-gutter-xs">
        <agent-avatar-q v-if="agentInitials" :icon="agentIcon" size="24px" avatar-class="chat-execution-card__avatar" />
        <q-icon v-else :name="activityIcon" :color="statusIconColor" size="20px" />
        <div class="col ellipsis">
          <div class="text-weight-medium ellipsis">{{ title }}</div>
          <div v-if="memberLabel" class="text-caption text-accent ellipsis">{{ memberLabel }}</div>
          <div v-else-if="summaryText" class="text-caption text-grey ellipsis">{{ summaryText }}</div>
        </div>
        <q-chip v-if="isLongRunning" dense size="sm" color="warning" text-color="white" icon="schedule">
          {{ t('chat.toolLongRunning', '长任务') }}
        </q-chip>
        <q-space />
        <span v-if="showElapsed" class="text-caption" :style="{ color: elapsedColor }">{{ elapsedLabel }}</span>
        <span v-else-if="durationLabel" class="text-caption text-grey">{{ durationLabel }}</span>
        <q-icon v-if="status === 'running'" name="hourglass_top" color="warning" size="18px" aria-hidden="true" />
        <span v-if="status === 'running'" class="chat-execution-card__pulse" aria-hidden="true" />
        <q-icon v-else-if="isFailed" name="error" color="negative" size="18px" aria-hidden="true" />
        <q-icon v-else-if="status === 'blocked'" name="warning" color="warning" size="18px" aria-hidden="true" />
        <q-icon v-else-if="status === 'cancelled'" name="cancel" color="grey" size="18px" aria-hidden="true" />
        <q-icon v-else name="check_circle" color="positive" size="18px" aria-hidden="true" />
        <span class="text-caption" :class="statusTextClass">{{ statusText }}</span>
        <span role="status" aria-live="polite" class="sr-only">{{ statusAnnouncement }}</span>
      </div>
    </template>

    <div
      :id="bodyId"
      role="region"
      :aria-label="t('chat.activity.detailRegion', '执行详情')"
      class="chat-execution-card__body"
    >
      <ChatDiffViewer
        v-if="isFileEdit"
        :file-name="diffFileName"
        :hunks="diffHunks"
        :tool-name="event.tool_name"
        :applied-count="appliedCount"
        :is-dark="isDark"
        :show-actions="showDiffActions"
        @apply="$emit('apply-diff', { toolName: event.tool_name, fileName: diffFileName })"
        @reject="$emit('reject-diff', { toolName: event.tool_name, fileName: diffFileName })"
      />
      <template v-else>
        <div v-if="hasArgs" class="chat-execution-card__section" role="group" :aria-label="t('chat.toolArgs', '参数')">
          <div class="text-caption text-weight-medium q-mb-xs">{{ t('chat.toolArgs', '参数') }}</div>
          <pre class="chat-execution-card__pre">{{ argsText }}</pre>
        </div>
        <div
          v-if="hasResult"
          class="chat-execution-card__section q-mt-sm"
          role="group"
          :aria-label="t('chat.toolResult', '结果')"
        >
          <div class="text-caption text-weight-medium q-mb-xs">{{ t('chat.toolResult', '结果') }}</div>
          <pre class="chat-execution-card__pre">{{ resultText }}</pre>
        </div>
      </template>
      <div v-if="errorText" role="alert" class="text-caption text-negative q-mt-sm">{{ errorText }}</div>
      <div v-if="hasMetadata" class="chat-execution-card__section q-mt-sm">
        <div class="text-caption text-weight-medium q-mb-xs">{{ t('chat.activity.metadata', '元数据') }}</div>
        <div class="text-caption text-grey column q-gutter-xs">
          <div v-if="event.run_id"><span class="text-weight-medium">run_id:</span> {{ event.run_id }}</div>
          <div v-if="event.trace_id"><span class="text-weight-medium">trace_id:</span> {{ event.trace_id }}</div>
        </div>
      </div>
      <div v-if="expanded && (hasArgs || hasResult)" class="chat-execution-card__audit text-caption text-grey q-mt-sm">
        {{ t('chat.activity.copyAuditHint', '复制内容可能包含敏感信息；完整审计请前往 Monitor → Traces。') }}
      </div>
    </div>
  </q-expansion-item>
</template>

<script lang="ts">
/** Module-level counter for generating unique fallback IDs across ChatExecutionCard instances. */

let _cardInstanceCounter = 0;
</script>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import {
  formatDurationLabel,
  maskSensitiveJSON,
  resolveActivityIcon,
  resolveDisplayLabel,
} from '../../features/chat/activityPresentation';
import type { ToolUseEvent, FileEditResult } from '../../features/chat/types';
import { EXECUTION_COLLAPSE_CONTROL_KEY, generateSummaryFallback } from '../../features/chat/executionCardHelpers';
import { isFileEditTool, extractDiffHunks, extractFileName } from '../../features/chat/diffEditHelpers';
import ChatDiffViewer from './ChatDiffViewer.vue';
import AgentAvatarQ from '../avatar/AgentAvatarQ.vue';

const props = withDefaults(
  defineProps<{
    event: ToolUseEvent;
    showMemberLabel?: boolean;
    initialCollapsed?: boolean;
    autoCollapse?: boolean;
  }>(),
  {
    showMemberLabel: undefined,
    initialCollapsed: true,
    autoCollapse: true,
  },
);

defineEmits<{
  'apply-diff': [payload: { toolName: string; fileName: string }];
  'reject-diff': [payload: { toolName: string; fileName: string }];
}>();

const { t } = useI18n();
const $q = useQuasar();
const expanded = ref(!props.initialCollapsed);
/** Instance-level unique ID for aria-controls. Deterministic: prefers event.id, falls back to instance counter. */
// eslint-disable-next-line no-useless-assignment
const _instanceSeq = ++_cardInstanceCounter;
const bodyId = computed(() => `exec-card-body-${props.event.id ?? `gen-${_instanceSeq}`}`);
/** Tracks whether the user manually expanded this card — prevents auto-collapse from overriding. */
const userManuallyExpanded = ref(false);

// ── SP-FE-27: 5s elapsed timer ──
const status = computed(() => props.event.status);
const elapsedSeconds = ref(0);
let elapsedInterval: ReturnType<typeof setInterval> | null = null;

/** Resolve effective start time: started_at → occurred_at → Date.now() */
function resolveEffectiveStartTime(): number {
  const started = props.event.started_at;
  if (started) return new Date(started).getTime();
  const occurred = props.event.occurred_at;
  if (occurred) return new Date(occurred).getTime();
  return Date.now();
}

function startElapsedTimer() {
  stopElapsedTimer();
  const startMs = resolveEffectiveStartTime();
  elapsedSeconds.value = Math.max(0, Math.floor((Date.now() - startMs) / 1000));
  elapsedInterval = setInterval(() => {
    elapsedSeconds.value = Math.max(0, Math.floor((Date.now() - startMs) / 1000));
  }, 1000);
}

function stopElapsedTimer() {
  if (elapsedInterval !== null) {
    clearInterval(elapsedInterval);
    elapsedInterval = null;
  }
}

/** Show elapsed only when running AND ≥5s */
const showElapsed = computed(() => status.value === 'running' && elapsedSeconds.value >= 5);

/** Format elapsed: <60s → "Ns...", ≥60s → "Nm Ns..." (running) or "Ns"/"Nm Ns" (not running) */
const elapsedLabel = computed(() => {
  const s = elapsedSeconds.value;
  const isRunning = status.value === 'running';
  if (s < 60) return `${s}s${isRunning ? '...' : ''}`;
  const m = Math.floor(s / 60);
  const rem = s % 60;
  return `${m}m ${rem}s${isRunning ? '...' : ''}`;
});

/** Elapsed color: <60s → tertiary, ≥60s → warning */
const elapsedColor = computed(() =>
  elapsedSeconds.value >= 60 ? 'var(--color-warning)' : 'var(--color-text-tertiary)',
);

// ── SP-FE-30: Provide/Inject global control ──
const collapseControl = inject(EXECUTION_COLLAPSE_CONTROL_KEY, null);

watch(
  () => props.event.status,
  (newStatus) => {
    if (!props.autoCollapse) return;
    if (newStatus === 'running') {
      expanded.value = true;
      userManuallyExpanded.value = false;
      startElapsedTimer();
    } else if ((newStatus === 'success' || newStatus === 'cancelled') && !userManuallyExpanded.value) {
      expanded.value = false;
      stopElapsedTimer();
    } else if (newStatus === 'failed' || newStatus === 'error') {
      // U1: Failed tools stay expanded so the error message is immediately
      // visible. Auto-collapsing a failed card hides the error summary,
      // forcing the user to manually expand to see what went wrong.
      stopElapsedTimer();
    } else {
      stopElapsedTimer();
    }
  },
);

// Watch global expand/collapse signals
if (collapseControl) {
  watch(
    () => collapseControl.expandAllSignal.value,
    () => {
      expanded.value = true;
      userManuallyExpanded.value = true;
    },
  );
  watch(
    () => collapseControl.collapseAllSignal.value,
    () => {
      // Running tools are immune to collapseAll
      if (status.value !== 'running') {
        expanded.value = false;
        userManuallyExpanded.value = false;
      }
    },
  );
}

// Start timer if already running on mount
if (props.event.status === 'running') {
  startElapsedTimer();
}

onBeforeUnmount(() => {
  stopElapsedTimer();
});

const isDark = computed(() => $q.dark.isActive);

const isFileEdit = computed(() => isFileEditTool(props.event.tool_name));
const diffArgs = computed<Record<string, unknown> | undefined>(
  () => (props.event.arguments ?? undefined) as Record<string, unknown> | undefined,
);
const diffHunks = computed(() => extractDiffHunks(props.event.tool_name, diffArgs.value));
const diffFileName = computed(() => extractFileName(diffArgs.value));
const appliedCount = computed(() => {
  const result = props.event.result as FileEditResult | undefined;
  return result?.applied_edits ?? result?.applied_hunks ?? 0;
});
const showDiffActions = computed(() => isFileEdit.value && props.event.status === 'success');

const isLongRunning = computed(() => Boolean(props.event.is_long_running));
const title = computed(() => resolveDisplayLabel(props.event));
const agentInitials = computed(() => {
  const name = props.event.agent_name?.trim();
  if (!name) return '';
  return name.charAt(0);
});
// icon_key 由调用方（消息适配层 / 父组件）从 agent 配置解析后注入。
// 空值由 AgentAvatarQ 回退到 smart_toy 图标。
const agentIcon = computed(() => props.event.icon_key?.trim() ?? '');
const summaryText = computed(() => props.event.summary?.trim() || generateSummaryFallback(props.event));
const memberLabel = computed(() => {
  if (props.showMemberLabel === false) return '';
  const key = props.event.agent_key?.trim();
  const name = props.event.agent_name?.trim();
  if (!key && !name) return '';
  if (name && key && name !== key) return `${name} · ${key}`;
  return name || key || '';
});
const activityIcon = computed(() => resolveActivityIcon(props.event));

const isFailed = computed(() => status.value === 'failed' || status.value === 'error');

const statusIconColor = computed(() => {
  if (status.value === 'running' || status.value === 'blocked') return 'warning';
  if (isFailed.value) return 'negative';
  if (status.value === 'cancelled') return 'grey';
  return 'accent';
});

const durationLabel = computed(() => formatDurationLabel(props.event.duration_ms));

const statusText = computed(() => {
  if (status.value === 'running') return t('chat.activity.running', '正在执行');
  if (status.value === 'blocked') return t('chat.activity.blocked', '待确认');
  if (status.value === 'cancelled') return t('chat.activity.cancelled', '已取消');
  if (isFailed.value) return t('chat.activity.failed', '失败');
  return '';
});

const headerAriaLabel = computed(() => {
  const base = title.value;
  if (statusText.value) return `${base} — ${statusText.value}`;
  return base;
});

/** Screen-reader-only announcement for status transitions (running → completed/failed). */
const statusAnnouncement = computed(() => {
  if (status.value === 'success')
    return t('chat.activity.completedAnnouncement', { tool: title.value }, '{tool} 已完成');
  if (isFailed.value) return t('chat.activity.failedAnnouncement', { tool: title.value }, '{tool} 执行失败');
  if (status.value === 'cancelled')
    return t('chat.activity.cancelledAnnouncement', { tool: title.value }, '{tool} 已取消');
  return '';
});

const statusTextClass = computed(() => {
  if (status.value === 'running') return 'text-warning';
  if (isFailed.value) return 'text-negative';
  if (status.value === 'blocked') return 'text-warning';
  if (status.value === 'cancelled') return 'text-grey';
  return 'text-grey';
});

const cardClass = computed(() => ({
  'chat-execution-card--running': status.value === 'running',
  'chat-execution-card--failed': isFailed.value,
  'chat-execution-card--cancelled': status.value === 'cancelled',
  'chat-execution-card--collapsed-failed': !expanded.value && isFailed.value,
}));

function onExpanded(value: boolean) {
  if (value) {
    userManuallyExpanded.value = true;
  }
}

function prettyJSON(value: unknown): string {
  try {
    return JSON.stringify(maskSensitiveJSON(value), null, 2);
  } catch {
    return String(value);
  }
}

const argsText = computed(() => prettyJSON(props.event.arguments ?? {}));
const resultText = computed(() => prettyJSON(props.event.result ?? {}));
const errorText = computed(() => props.event.error?.trim() ?? '');
const hasArgs = computed(() => Object.keys(props.event.arguments ?? {}).length > 0);
const hasResult = computed(() => status.value !== 'running' && Object.keys(props.event.result ?? {}).length > 0);
const hasMetadata = computed(() => Boolean(props.event.run_id?.trim() || props.event.trace_id?.trim()));
</script>

<style scoped lang="sass">
.chat-execution-card
  border-radius: 10px
  border: 1px solid color-mix(in srgb, var(--glass-border) 65%, transparent)
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  overflow: hidden
  transition: border-color 0.2s ease

.chat-execution-card--running
  border-color: color-mix(in srgb, var(--color-warning) 30%, transparent)

.chat-execution-card--failed
  border-color: color-mix(in srgb, var(--color-danger) 30%, transparent)

.chat-execution-card--cancelled
  border-color: color-mix(in srgb, var(--color-text-tertiary) 30%, transparent)
  opacity: 0.88

.chat-execution-card--collapsed-failed
  border-color: color-mix(in srgb, var(--color-danger) 50%, transparent)
  background: color-mix(in srgb, var(--color-danger) 6%, transparent)

.chat-execution-card__avatar
  flex-shrink: 0
  font-size: 11px

.chat-execution-card__body
  padding: 0 var(--space-3) var(--space-3)

.chat-execution-card__pre
  margin: 0
  padding: var(--space-2)
  border-radius: 6px
  font-size: var(--text-xs)
  line-height: 1.45
  max-height: 280px
  overflow: auto
  white-space: pre-wrap
  word-break: break-word
  background: color-mix(in srgb, var(--glass-surface) 70%, transparent)

body.body--dark .chat-execution-card__pre
  background: var(--glass-surface-hover)

.chat-execution-card__audit
  padding-top: var(--space-1)
  border-top: 1px dashed color-mix(in srgb, var(--glass-border) 60%, transparent)
  line-height: 1.4

.sr-only
  position: absolute
  width: 1px
  height: 1px
  padding: 0
  margin: -1px
  overflow: hidden
  clip: rect(0, 0, 0, 0)
  white-space: nowrap
  border: 0

.chat-execution-card__pulse
  display: inline-block
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-accent)
  vertical-align: middle
  margin-left: 4px
  animation: exec-pulse 1s ease-in-out infinite

@keyframes exec-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3
</style>
