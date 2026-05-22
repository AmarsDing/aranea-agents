<template>
  <div
    :class="['graph-flow-node', `graph-flow-node--${data.nodeType}`, { 'graph-flow-node--selected': selected, 'graph-flow-node--running': data.execStatus === 'running', 'graph-flow-node--completed': data.execStatus === 'completed', 'graph-flow-node--failed': data.execStatus === 'error' || data.execStatus === 'failed', 'graph-flow-node--interrupted': data.execStatus === 'interrupted' }]"
    :style="nodeStyle"
  >
    <Handle type="target" :position="Position.Left" />
    <div class="graph-flow-node__header">
      <q-icon :name="styleConfig.icon" size="16px" />
      <span class="graph-flow-node__type-label">{{ styleConfig.label }}</span>
    </div>
    <div class="graph-flow-node__label">{{ data.label || data.nodeId }}</div>
    <div v-if="data.instruction" class="graph-flow-node__hint">{{ truncate(data.instruction, 32) }}</div>
    <div v-if="data.execStatus" class="graph-flow-node__status">
      <q-icon :name="statusIcon" size="12px" />
      <span>{{ statusLabel }}</span>
      <span v-if="fineStatusLabel" class="graph-flow-node__fine-status">· {{ fineStatusLabel }}</span>
    </div>
    <Handle type="source" :position="Position.Right" />
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { Handle, Position } from "@vue-flow/core";
import { NODE_TYPE_STYLES, EXECUTION_STATUS_STYLES, type NodeType } from "../../features/graph/types";
import { AGENT_NODE_STATUS_STYLES } from "../../features/orchestration/agentNodeStatusStyles";
import type { AgentNodeStatus } from "../../features/orchestration/types";

const props = defineProps<{
  id: string;
  data: {
    nodeId: string;
    nodeType: NodeType;
    label: string;
    instruction?: string;
    execStatus?: string;
    fineStatus?: string;
  };
  selected?: boolean;
}>();

const styleConfig = computed(() => NODE_TYPE_STYLES[props.data.nodeType] ?? NODE_TYPE_STYLES.function);

const nodeStyle = computed(() => ({
  "--node-fill": styleConfig.value.fillColor,
  "--node-border": styleConfig.value.borderColor,
}));

const statusConfig = computed(() => {
  const status = props.data.execStatus ?? "idle";
  return EXECUTION_STATUS_STYLES[status] ?? EXECUTION_STATUS_STYLES.idle;
});

const statusIcon = computed(() => statusConfig.value.icon);
const statusLabel = computed(() => statusConfig.value.label);

const fineStatusLabel = computed(() => {
  const key = props.data.fineStatus as AgentNodeStatus | undefined;
  if (!key) return "";
  return AGENT_NODE_STATUS_STYLES[key]?.label ?? key;
});

function truncate(text: string, max: number) {
  return text.length > max ? text.slice(0, max) + "…" : text;
}
</script>

<style scoped>
.graph-flow-node {
  min-width: 160px;
  max-width: 220px;
  padding: 10px 14px;
  border: 2px solid var(--node-border);
  border-radius: 14px;
  background: var(--node-fill);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  font-size: 12px;
  transition: box-shadow 0.2s, border-color 0.2s;
}

.graph-flow-node--selected {
  box-shadow: 0 0 0 2px var(--color-accent, var(--color-accent));
}

.graph-flow-node--running {
  animation: pulse-glow 1.5s ease-in-out infinite;
}

.graph-flow-node--completed {
  border-color: var(--color-success, var(--color-success));
}

.graph-flow-node--failed {
  border-color: var(--color-danger, var(--color-danger));
}

.graph-flow-node--interrupted {
  border-color: var(--color-warning);
}

.graph-flow-node__header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
  opacity: 70%;
}

.graph-flow-node__type-label {
  font-weight: 700;
  text-transform: uppercase;
  font-size: 10px;
  letter-spacing: 0.06em;
}

.graph-flow-node__label {
  font-weight: 600;
  font-size: 13px;
  line-height: 1.3;
  overflow-wrap: break-word;
}

.graph-flow-node__hint {
  margin-top: 4px;
  color: var(--color-text-secondary, var(--color-text-tertiary));
  font-size: 11px;
  line-height: 1.3;
}

.graph-flow-node__status {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 6px;
  font-size: 10px;
  font-weight: 600;
}

.graph-flow-node__fine-status {
  opacity: 0.75;
  font-weight: 500;
}

@keyframes pulse-glow {
  0%, 100% { box-shadow: 0 0 4px rgb(33 150 243 / 30%); }
  50% { box-shadow: 0 0 16px rgb(33 150 243 / 60%); }
}
</style>
