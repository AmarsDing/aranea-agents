<template>
  <div
    :class="[
      'graph-flow-node',
      `graph-flow-node--${data.nodeType}`,
      {
        'graph-flow-node--selected': selected,
        'graph-flow-node--running': data.execStatus === 'running',
        'graph-flow-node--completed': data.execStatus === 'completed',
        'graph-flow-node--failed': data.execStatus === 'error' || data.execStatus === 'failed',
        'graph-flow-node--interrupted': data.execStatus === 'interrupted',
      },
    ]"
    :style="nodeStyle"
  >
    <Handle type="target" :position="Position.Left" class="graph-flow-node__handle" />
    <div class="graph-flow-node__accent" />
    <div class="graph-flow-node__body">
      <div class="graph-flow-node__header">
        <div class="graph-flow-node__icon-wrap">
          <q-icon :name="styleConfig.icon" size="14px" />
        </div>
        <span class="graph-flow-node__type-label">{{ styleConfig.label }}</span>
        <OrchestrationStatusChip
          v-if="data.nodeType === 'agent' && showStatusChip"
          :display-status="displayStatus"
          :fine-status="fineStatusKey"
        />
        <q-badge v-else-if="data.execStatus" dense rounded class="graph-flow-node__status-badge">
          {{ statusLabel }}
        </q-badge>
      </div>
      <div class="graph-flow-node__label">{{ primaryLabel }}</div>
      <div v-if="roleLabel" class="graph-flow-node__role">{{ roleLabel }}</div>
      <div v-if="agentKeyLine" class="graph-flow-node__agent-key">{{ agentKeyLine }}</div>
      <div v-if="responsibilityLine" class="graph-flow-node__hint">{{ truncate(responsibilityLine, 56) }}</div>
      <div v-if="ioPreviewLine" class="graph-flow-node__io">{{ truncate(ioPreviewLine, 64) }}</div>
      <div v-if="fineStatusLabel && !showStatusChip" class="graph-flow-node__fine-status">{{ fineStatusLabel }}</div>
    </div>
    <Handle type="source" :position="Position.Right" class="graph-flow-node__handle" />
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { Handle, Position } from "@vue-flow/core";
import { NODE_TYPE_STYLES, EXECUTION_STATUS_STYLES, type NodeType } from "../../features/graph/types";
import { AGENT_NODE_STATUS_STYLES, DISPLAY_STATUS_STYLES } from "../../features/orchestration/agentNodeStatusStyles";
import type { AgentNodeStatus, DisplayStatus } from "../../features/orchestration/types";
import OrchestrationStatusChip from "../orchestration/OrchestrationStatusChip.vue";

const props = defineProps<{
  id: string;
  data: {
    nodeId: string;
    nodeType: NodeType;
    label: string;
    agentName?: string;
    role?: string;
    description?: string;
    instruction?: string;
    execStatus?: string;
    fineStatus?: string;
    inputPreview?: string;
    outputPreview?: string;
    currentActivity?: string;
  };
  selected?: boolean;
}>();

const styleConfig = computed(() => NODE_TYPE_STYLES[props.data.nodeType] ?? NODE_TYPE_STYLES.function);

const nodeStyle = computed(() => ({
  "--node-accent": styleConfig.value.borderColor,
}));

const statusConfig = computed(() => {
  const status = props.data.execStatus ?? "idle";
  return EXECUTION_STATUS_STYLES[status] ?? EXECUTION_STATUS_STYLES.idle;
});

const statusLabel = computed(() => statusConfig.value.label);

const showStatusChip = computed(() => props.data.nodeType === "agent");

const fineStatusKey = computed(() => (props.data.fineStatus as AgentNodeStatus | undefined) ?? "idle");

const displayStatus = computed<DisplayStatus>(() => {
  const status = props.data.execStatus ?? "waiting";
  if (status in DISPLAY_STATUS_STYLES) {
    return status as DisplayStatus;
  }
  if (status === "running" || status === "active") return "active";
  if (status === "completed" || status === "success") return "success";
  if (status === "failed" || status === "error") return "failed";
  return "waiting";
});

const primaryLabel = computed(() => props.data.label || props.data.nodeId);

const roleLabel = computed(() => {
  if (props.data.nodeType !== "agent" || !props.data.role) return "";
  return props.data.role;
});

const agentKeyLine = computed(() => {
  if (props.data.nodeType !== "agent" || !props.data.agentName) return "";
  if (props.data.agentName === primaryLabel.value) return "";
  return props.data.agentName;
});

const responsibilityLine = computed(() => {
  if (props.data.nodeType !== "agent") return props.data.instruction ?? "";
  return props.data.instruction || props.data.description || "";
});

const ioPreviewLine = computed(() => {
  if (props.data.inputPreview) {
    return `收：${props.data.inputPreview}`;
  }
  if (props.data.outputPreview) {
    return `交：${props.data.outputPreview}`;
  }
  if (props.data.currentActivity) {
    return `做：${props.data.currentActivity}`;
  }
  return "";
});

const fineStatusLabel = computed(() => {
  const key = props.data.fineStatus as AgentNodeStatus | undefined;
  if (!key) return "";
  return AGENT_NODE_STATUS_STYLES[key]?.label ?? key;
});

function truncate(text: string, max: number) {
  return text.length > max ? `${text.slice(0, max)}…` : text;
}
</script>
