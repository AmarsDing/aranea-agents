<template>
  <AppStatusChip
    :status="chipStatus"
    class="agent-status-label"
    :class="{ 'agent-status-label--animated': config.animated }"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { AgentNodeStatusLabel } from '../../features/spirit/spiritUi';
import { STATUS_LABEL_CONFIG } from '../../features/spirit/spiritUi';
import AppStatusChip from '../common/AppStatusChip.vue';

const props = defineProps<{
  label: AgentNodeStatusLabel;
}>();

const config = computed(() => STATUS_LABEL_CONFIG[props.label] ?? STATUS_LABEL_CONFIG.queued);

/** 聚合标签 → AppStatusChip 收录的通用状态（appStatusMeta，未收录则兜底显示原文）。 */
const CHIP_STATUS: Record<AgentNodeStatusLabel, string> = {
  queued: 'queued',
  active: 'running',
  suspended: 'pending',
  tool_blocked: 'awaiting_confirmation',
  interrupted: 'interrupted',
  done: 'completed',
  partial_failure: 'partial',
  failed: 'failed',
  skipped: 'skipped',
  cancelled: 'cancelled',
};

const chipStatus = computed(() => CHIP_STATUS[props.label] ?? props.label);
</script>

<style lang="sass" scoped>
.agent-status-label
  min-width: 72px
  max-width: 88px
  justify-content: center

.agent-status-label--animated
  animation: agent-status-breathe 2s ease-in-out infinite

@keyframes agent-status-breathe
  0%, 100%
    border-left-width: 3px
    border-left-color: var(--color-accent)
  50%
    border-left-width: 1px
    border-left-color: color-mix(in srgb, var(--color-accent) 40%, transparent)
</style>
