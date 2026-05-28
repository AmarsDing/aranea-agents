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
          {{ t("chat.toolLongRunning", "长任务") }}
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
      <div v-if="hasArgs" class="chat-execution-card__section">
        <div class="text-caption text-weight-medium q-mb-xs">{{ t("chat.toolArgs", "参数") }}</div>
        <pre class="chat-execution-card__pre">{{ argsText }}</pre>
      </div>
      <div v-if="hasResult" class="chat-execution-card__section q-mt-sm">
        <div class="text-caption text-weight-medium q-mb-xs">{{ t("chat.toolResult", "结果") }}</div>
        <pre class="chat-execution-card__pre">{{ resultText }}</pre>
      </div>
      <div v-if="errorText" class="text-caption text-negative q-mt-sm">{{ errorText }}</div>
      <div v-if="hasMetadata" class="chat-execution-card__section q-mt-sm">
        <div class="text-caption text-weight-medium q-mb-xs">{{ t("chat.activity.metadata", "元数据") }}</div>
        <div class="text-caption text-grey-7 column q-gutter-xs">
          <div v-if="event.run_id"><span class="text-weight-medium">run_id:</span> {{ event.run_id }}</div>
          <div v-if="event.trace_id"><span class="text-weight-medium">trace_id:</span> {{ event.trace_id }}</div>
        </div>
      </div>
      <div v-if="expanded && (hasArgs || hasResult)" class="chat-execution-card__audit text-caption text-grey-6 q-mt-sm">
        {{ t("chat.activity.copyAuditHint", "复制内容可能包含敏感信息；完整审计请前往 Monitor → Traces。") }}
      </div>
    </div>
  </q-expansion-item>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  formatDurationLabel,
  maskSensitiveJSON,
  resolveActivityIcon,
  resolveDisplayLabel,
} from "../../features/chat/activityPresentation";
import type { ToolUseEvent } from "../../features/chat/types";

const props = defineProps<{
  event: ToolUseEvent;
  showMemberLabel?: boolean;
}>();

const { t } = useI18n();
const expanded = ref(false);

const status = computed(() => props.event.status);
const isLongRunning = computed(() => Boolean(props.event.is_long_running));
const title = computed(() => resolveDisplayLabel(props.event));
const summaryText = computed(() => props.event.summary?.trim() || "");
const memberLabel = computed(() => {
  if (props.showMemberLabel === false) return "";
  const key = props.event.agent_key?.trim();
  const name = props.event.agent_name?.trim();
  if (!key && !name) return "";
  if (name && key && name !== key) return `${name} · ${key}`;
  return name || key || "";
});
const activityIcon = computed(() => resolveActivityIcon(props.event));

const isFailed = computed(
  () => status.value === "failed" || status.value === "error"
);

const statusIconColor = computed(() => {
  if (status.value === "running" || status.value === "blocked") return "warning";
  if (isFailed.value) return "negative";
  if (status.value === "cancelled") return "grey";
  return "primary";
});

const durationLabel = computed(() => formatDurationLabel(props.event.duration_ms));

const statusText = computed(() => {
  if (status.value === "running") return t("chat.activity.running", "正在执行");
  if (status.value === "blocked") return t("chat.activity.blocked", "待确认");
  if (status.value === "cancelled") return t("chat.activity.cancelled", "已取消");
  if (isFailed.value) return t("chat.activity.failed", "失败");
  return "";
});

const headerAriaLabel = computed(() => {
  const base = title.value;
  if (statusText.value) return `${base} — ${statusText.value}`;
  return base;
});

const statusTextClass = computed(() => {
  if (status.value === "running") return "text-warning";
  if (isFailed.value) return "text-negative";
  if (status.value === "blocked") return "text-warning";
  if (status.value === "cancelled") return "text-grey-6";
  return "text-grey-7";
});

const cardClass = computed(() => ({
  "chat-execution-card--running": status.value === "running",
  "chat-execution-card--failed": isFailed.value,
  "chat-execution-card--cancelled": status.value === "cancelled",
}));

function onExpanded(value: boolean) {
  expanded.value = value;
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
const errorText = computed(() => props.event.error?.trim() ?? "");
const hasArgs = computed(() => Object.keys(props.event.arguments ?? {}).length > 0);
const hasResult = computed(() => status.value !== "running" && Object.keys(props.event.result ?? {}).length > 0);
const hasMetadata = computed(() => Boolean(props.event.run_id?.trim() || props.event.trace_id?.trim()));
</script>

<style scoped lang="sass">
.chat-execution-card
  border-radius: 10px
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  overflow: hidden

.chat-execution-card--running
  border-color: color-mix(in srgb, var(--color-warning) 35%, transparent)

.chat-execution-card--failed
  border-color: color-mix(in srgb, var(--color-danger) 35%, transparent)

.chat-execution-card--cancelled
  border-color: color-mix(in srgb, var(--color-text-tertiary) 35%, transparent)
  opacity: 0.92

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
  background: var(--glass-surface)

body.body--dark .chat-execution-card__pre
  background: var(--glass-surface-hover)

.chat-execution-card__audit
  padding-top: var(--space-1)
  border-top: 1px dashed var(--glass-border)
  line-height: 1.4
</style>
