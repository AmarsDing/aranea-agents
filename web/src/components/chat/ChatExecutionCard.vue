<template>
  <q-expansion-item
    class="chat-execution-card"
    :class="cardClass"
    dense
    expand-separator
    header-class="chat-execution-card__header"
    :aria-expanded="expanded"
    :aria-label="headerAriaLabel"
    @update:model-value="onExpanded"
  >
    <template #header>
      <div class="row items-center no-wrap full-width q-gutter-xs">
        <q-icon :name="activityIcon" :color="statusIconColor" size="20px" />
        <div class="col ellipsis">
          <div class="text-weight-medium ellipsis">{{ title }}</div>
          <div v-if="memberLabel" class="text-caption text-primary ellipsis">{{ memberLabel }}</div>
          <div v-else-if="summaryText" class="text-caption text-grey-7 ellipsis">{{ summaryText }}</div>
        </div>
        <q-chip v-if="isLongRunning" dense size="sm" color="orange" text-color="white" icon="schedule">
          {{ t('chat.toolLongRunning', '长任务') }}
        </q-chip>
        <q-space />
        <span v-if="durationLabel" class="text-caption text-grey-7">{{ durationLabel }}</span>
        <q-icon v-if="status === 'running'" name="hourglass_top" color="warning" size="18px" aria-hidden="true" />
        <q-icon v-else-if="isFailed" name="error" color="negative" size="18px" aria-hidden="true" />
        <q-icon v-else-if="status === 'blocked'" name="warning" color="warning" size="18px" aria-hidden="true" />
        <q-icon v-else-if="status === 'cancelled'" name="cancel" color="grey" size="18px" aria-hidden="true" />
        <q-icon v-else name="check_circle" color="positive" size="18px" aria-hidden="true" />
        <span class="text-caption" :class="statusTextClass">{{ statusText }}</span>
      </div>
    </template>

    <div class="chat-execution-card__body">
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
        <div v-if="hasArgs" class="chat-execution-card__section">
          <div class="text-caption text-weight-medium q-mb-xs">{{ t('chat.toolArgs', '参数') }}</div>
          <pre class="chat-execution-card__pre">{{ argsText }}</pre>
        </div>
        <div v-if="hasResult" class="chat-execution-card__section q-mt-sm">
          <div class="text-caption text-weight-medium q-mb-xs">{{ t('chat.toolResult', '结果') }}</div>
          <pre class="chat-execution-card__pre">{{ resultText }}</pre>
        </div>
      </template>
      <div v-if="errorText" class="text-caption text-negative q-mt-sm">{{ errorText }}</div>
      <div v-if="hasMetadata" class="chat-execution-card__section q-mt-sm">
        <div class="text-caption text-weight-medium q-mb-xs">{{ t('chat.activity.metadata', '元数据') }}</div>
        <div class="text-caption text-grey-7 column q-gutter-xs">
          <div v-if="event.run_id"><span class="text-weight-medium">run_id:</span> {{ event.run_id }}</div>
          <div v-if="event.trace_id"><span class="text-weight-medium">trace_id:</span> {{ event.trace_id }}</div>
        </div>
      </div>
      <div
        v-if="expanded && (hasArgs || hasResult)"
        class="chat-execution-card__audit text-caption text-grey-6 q-mt-sm"
      >
        {{ t('chat.activity.copyAuditHint', '复制内容可能包含敏感信息；完整审计请前往 Monitor → Traces。') }}
      </div>
    </div>
  </q-expansion-item>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import {
  formatDurationLabel,
  maskSensitiveJSON,
  resolveActivityIcon,
  resolveDisplayLabel,
} from '../../features/chat/activityPresentation';
import type { ToolUseEvent, FileEditResult } from '../../features/chat/types';
import { isFileEditTool, extractDiffHunks, extractFileName } from '../../features/chat/diffEditHelpers';
import ChatDiffViewer from './ChatDiffViewer.vue';

const props = withDefaults(
  defineProps<{
    event: ToolUseEvent;
    showMemberLabel?: boolean;
    initialCollapsed?: boolean;
    autoCollapse?: boolean;
  }>(),
  {
    showMemberLabel: undefined,
    initialCollapsed: false,
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
/** Tracks whether the user manually expanded this card — prevents auto-collapse from overriding. */
const userManuallyExpanded = ref(false);

watch(
  () => props.event.status,
  (newStatus) => {
    if (!props.autoCollapse) return;
    if (newStatus === 'running') {
      expanded.value = true;
      userManuallyExpanded.value = false;
    } else if (
      (newStatus === 'success' || newStatus === 'failed' || newStatus === 'cancelled') &&
      !userManuallyExpanded.value
    ) {
      expanded.value = false;
    }
  },
);

const isDark = computed(() => $q.dark.isActive);

const isFileEdit = computed(() => isFileEditTool(props.event.tool_name));
const diffHunks = computed(() => extractDiffHunks(props.event.tool_name, props.event.arguments));
const diffFileName = computed(() => extractFileName(props.event.arguments));
const appliedCount = computed(() => {
  const result = props.event.result as FileEditResult | undefined;
  return result?.applied_edits ?? result?.applied_hunks ?? 0;
});
const showDiffActions = computed(() => isFileEdit.value && props.event.status === 'success');

const status = computed(() => props.event.status);
const isLongRunning = computed(() => Boolean(props.event.is_long_running));
const title = computed(() => resolveDisplayLabel(props.event));
const summaryText = computed(() => props.event.summary?.trim() || '');
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
  return 'primary';
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

const statusTextClass = computed(() => {
  if (status.value === 'running') return 'text-warning';
  if (isFailed.value) return 'text-negative';
  if (status.value === 'blocked') return 'text-warning';
  if (status.value === 'cancelled') return 'text-grey-6';
  return 'text-grey-7';
});

const cardClass = computed(() => ({
  'chat-execution-card--running': status.value === 'running',
  'chat-execution-card--failed': isFailed.value,
  'chat-execution-card--cancelled': status.value === 'cancelled',
  'chat-execution-card--collapsed-failed': !expanded.value && isFailed.value,
}));

function onExpanded(value: boolean) {
  expanded.value = value;
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
</style>
