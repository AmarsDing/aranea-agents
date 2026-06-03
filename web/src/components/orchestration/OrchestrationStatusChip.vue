<template>
  <q-chip
    dense
    :class="['orch-status-chip', `orch-status-chip--${display}`]"
    :icon="style.icon"
    :color="style.color"
    text-color="white"
  >
    <span>{{ style.label }}</span>
    <span v-if="fineLabel" class="orch-status-chip__fine">{{ fineLabel }}</span>
  </q-chip>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { AgentNodeStatus, DisplayStatus } from '../../features/orchestration/types';
import { AGENT_NODE_STATUS_STYLES, DISPLAY_STATUS_STYLES } from '../../features/orchestration/agentNodeStatusStyles';

const props = defineProps<{
  displayStatus: DisplayStatus | string;
  fineStatus?: AgentNodeStatus | string;
}>();

const display = computed(() => props.displayStatus || 'waiting');
const style = computed(() => DISPLAY_STATUS_STYLES[display.value as DisplayStatus] ?? DISPLAY_STATUS_STYLES.waiting);
const fineLabel = computed(() => {
  const key = props.fineStatus;
  if (!key || key === display.value) return '';
  return AGENT_NODE_STATUS_STYLES[key as AgentNodeStatus]?.label ?? key;
});
</script>
