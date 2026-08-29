<template>
  <div v-if="visible" class="chat-runner-status row items-center no-wrap q-gutter-x-sm">
    <AppStatusChip :status="status" class="chat-runner-status__badge" />
    <span v-if="agentName" class="text-caption ellipsis chat-runner-status__agent">{{ agentName }}</span>
    <span v-if="elapsedLabel" class="text-caption text-grey">{{ elapsedLabel }}</span>
    <span v-if="eventCount != null && eventCount > 0" class="text-caption text-grey"> {{ eventCount }} 个事件 </span>
    <q-btn
      v-if="showCancel"
      flat
      dense
      round
      size="sm"
      color="negative"
      icon="stop"
      aria-label="停止生成"
      @click="emit('cancel')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import type { RunStatusValue } from '../../features/chat/types';
import AppStatusChip from '../common/AppStatusChip.vue';

const props = defineProps<{
  status: RunStatusValue;
  agentName?: string;
  startedAt?: string;
  eventCount?: number;
  showCancel?: boolean;
}>();

const emit = defineEmits<{ cancel: [] }>();

const visible = computed(() => {
  const s = props.status;
  return s === 'running' || s === 'pending' || s === 'awaiting_user' || s === 'durable';
});

// ── Reactive elapsed timer ──
// Without this, elapsedLabel uses Date.now() in a computed, which is NOT
// reactive — the displayed "12s" never ticks up, making the user think the
// system is frozen even though run_heartbeat events are arriving every 10s.
// The timer is started/stopped on visible change to avoid wasting CPU when
// the status bar is hidden.
const now = ref(Date.now());
let elapsedTimer: ReturnType<typeof setInterval> | null = null;

function startElapsedTimer() {
  stopElapsedTimer();
  elapsedTimer = setInterval(() => {
    now.value = Date.now();
  }, 1000);
}

function stopElapsedTimer() {
  if (elapsedTimer !== null) {
    clearInterval(elapsedTimer);
    elapsedTimer = null;
  }
}

watch(
  visible,
  (isVisible) => {
    if (isVisible) {
      startElapsedTimer();
    } else {
      stopElapsedTimer();
    }
  },
  { immediate: true },
);

onUnmounted(() => {
  stopElapsedTimer();
});

const elapsedLabel = computed(() => {
  if (!props.startedAt) return '';
  const start = Date.parse(props.startedAt);
  if (Number.isNaN(start)) return '';
  const sec = Math.max(0, Math.floor((now.value - start) / 1000));
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
});

const showCancel = computed(
  () =>
    props.showCancel !== false &&
    (props.status === 'running' || props.status === 'pending' || props.status === 'durable'),
);
</script>

<style scoped lang="sass">
.chat-runner-status
  max-width: 100%
  &__badge
    flex-shrink: 0
  &__agent
    max-width: 8rem
</style>
