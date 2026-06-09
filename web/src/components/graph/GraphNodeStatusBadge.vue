<template>
  <div :class="['graph-node-status-badge', `graph-node-status-badge--${status}`]">
    <q-icon :name="statusConfig.icon" size="12px" />
    <span v-if="showLabel" class="graph-node-status-badge__label">{{ statusConfig.label }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

export type NodeRuntimeStatus = 'idle' | 'running' | 'success' | 'error' | 'interrupted';

const STATUS_CONFIG: Record<NodeRuntimeStatus, { icon: string; label: string }> = {
  idle: { icon: 'radio_button_unchecked', label: '等待' },
  running: { icon: 'sync', label: '运行中' },
  success: { icon: 'check_circle', label: '完成' },
  error: { icon: 'error', label: '失败' },
  interrupted: { icon: 'pause_circle', label: '中断' },
};

const props = defineProps<{
  status: NodeRuntimeStatus;
  showLabel?: boolean;
}>();

const statusConfig = computed(() => STATUS_CONFIG[props.status] ?? STATUS_CONFIG.idle);
</script>
