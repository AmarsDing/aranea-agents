<template>
  <div class="chat-tool-card" :class="cardClass">
    <div class="chat-tool-card__head row items-center no-wrap q-gutter-xs">
      <q-icon :name="statusIcon" :color="statusColor" size="20px" />
      <span class="text-weight-medium ellipsis">{{ label }}</span>
      <q-chip v-if="isLongRunning" dense size="sm" color="orange" text-color="white" icon="schedule">
        {{ t("chat.toolLongRunning", "长任务") }}
      </q-chip>
      <q-space />
      <span v-if="durationLabel" class="text-caption text-grey-7">{{ durationLabel }}</span>
      <q-badge :color="statusColor" :label="statusLabel" />
    </div>
    <details v-if="hasArgs" class="chat-tool-card__block q-mt-xs" open>
      <summary class="text-caption text-weight-medium">{{ t("chat.toolArgs", "参数") }}</summary>
      <pre class="chat-tool-card__pre">{{ argsText }}</pre>
    </details>
    <details v-if="hasResult" class="chat-tool-card__block q-mt-xs" :open="status !== 'running'">
      <summary class="text-caption text-weight-medium">{{ t("chat.toolResult", "结果") }}</summary>
      <pre class="chat-tool-card__pre">{{ resultText }}</pre>
    </details>
    <div v-if="errorText" class="text-caption text-negative q-mt-xs">{{ errorText }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { ToolUseEvent } from "../../features/chat/types";

const props = defineProps<{
  event: ToolUseEvent;
}>();

const { t } = useI18n();

const status = computed(() => props.event.status);
const isLongRunning = computed(() => Boolean(props.event.is_long_running));
const label = computed(() => props.event.tool_label || props.event.tool_name || "tool");

const statusColor = computed(() => {
  if (status.value === "running") return "orange";
  if (status.value === "failed" || status.value === "error" || status.value === "blocked") return "negative";
  return "positive";
});

const statusIcon = computed(() => {
  if (status.value === "running") return "hourglass_top";
  if (status.value === "failed" || status.value === "error") return "error";
  return "check_circle";
});

const statusLabel = computed(() => status.value);

const durationLabel = computed(() => {
  const ms = props.event.duration_ms;
  if (!ms || ms <= 0) return "";
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`;
});

const cardClass = computed(() => ({
  "chat-tool-card--running": status.value === "running",
  "chat-tool-card--failed": status.value === "failed" || status.value === "error",
}));

function prettyJSON(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

const argsText = computed(() => prettyJSON(props.event.arguments ?? {}));
const resultText = computed(() => prettyJSON(props.event.result ?? {}));
const errorText = computed(() => props.event.error?.trim() ?? "");
const hasArgs = computed(() => Object.keys(props.event.arguments ?? {}).length > 0);
const hasResult = computed(() => status.value !== "running" && Object.keys(props.event.result ?? {}).length > 0);
</script>

<style scoped lang="sass">
.chat-tool-card
  border-radius: 10px
  padding: 10px 12px
  border: 1px solid var(--color-border-subtle, rgba(0, 0, 0, 0.08))
  background: var(--color-surface-elevated, rgba(255, 255, 255, 0.6))

.chat-tool-card--running
  border-color: rgba(255, 152, 0, 0.35)

.chat-tool-card--failed
  border-color: rgba(244, 67, 54, 0.35)

.chat-tool-card__pre
  margin: 6px 0 0
  padding: 8px
  border-radius: 6px
  font-size: 12px
  line-height: 1.45
  max-height: 280px
  overflow: auto
  white-space: pre-wrap
  word-break: break-word
  background: rgba(0, 0, 0, 0.04)

body.body--dark .chat-tool-card__pre
  background: rgba(255, 255, 255, 0.06)

.chat-tool-card__block summary
  cursor: pointer
  user-select: none
</style>
