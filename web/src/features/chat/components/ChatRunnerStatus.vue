// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <div v-if="visible" class="chat-runner-status row items-center no-wrap q-gutter-x-sm">
    <q-badge :color="badgeColor" :label="statusLabel" class="chat-runner-status__badge" />
    <span v-if="agentName" class="text-caption ellipsis chat-runner-status__agent">{{ agentName }}</span>
    <span v-if="elapsedLabel" class="text-caption text-grey-7">{{ elapsedLabel }}</span>
    <span v-if="eventCount != null && eventCount > 0" class="text-caption text-grey-7">
      {{ eventCount }} {{ t("chat.runnerEvents", "events") }}
    </span>
    <q-btn
      v-if="showCancel"
      flat
      dense
      round
      size="sm"
      color="negative"
      icon="stop"
      :aria-label="t('chat.stop')"
      @click="emit('cancel')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { RunStatusValue } from "../types";

const props = defineProps<{
  status: RunStatusValue;
  agentName?: string;
  startedAt?: string;
  eventCount?: number;
  showCancel?: boolean;
}>();

const emit = defineEmits<{ cancel: [] }>();

const { t } = useI18n();

const visible = computed(() => {
  const s = props.status;
  return s === "running" || s === "pending" || s === "awaiting_user";
});

const badgeColor = computed(() => {
  switch (props.status) {
    case "running":
      return "primary";
    case "awaiting_user":
      return "warning";
    case "pending":
      return "grey-7";
    default:
      return "grey";
  }
});

const statusLabel = computed(() => {
  const key = `chat.runStatus.${props.status}`;
  const fallback =
    props.status === "running"
      ? "Running"
      : props.status === "awaiting_user"
        ? "Awaiting"
        : props.status;
  return t(key, fallback);
});

const elapsedLabel = computed(() => {
  if (!props.startedAt) return "";
  const start = Date.parse(props.startedAt);
  if (Number.isNaN(start)) return "";
  const sec = Math.max(0, Math.floor((Date.now() - start) / 1000));
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
});

const showCancel = computed(() => props.showCancel !== false && props.status === "running");
</script>

<style scoped lang="sass">
.chat-runner-status
  max-width: 100%
  &__badge
    flex-shrink: 0
  &__agent
    max-width: 8rem
</style>
