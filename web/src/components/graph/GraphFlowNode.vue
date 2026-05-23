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

<style scoped>
.graph-flow-node {
  position: relative;
  min-width: 200px;
  max-width: 260px;
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  background: var(--glass-elevated);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  box-shadow: var(--shadow-entity-panel);
  overflow: hidden;
  transition: border-color 160ms ease, box-shadow 160ms ease, transform 160ms ease;
}

.graph-flow-node__accent {
  height: 3px;
  background: var(--node-accent, var(--color-accent));
}

.graph-flow-node__body {
  padding: 10px 12px 12px;
}

.graph-flow-node__header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
}

.graph-flow-node__icon-wrap {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--node-accent, var(--color-accent)) 14%, var(--glass-surface));
  color: var(--node-accent, var(--color-accent));
}

.graph-flow-node__type-label {
  flex: 1;
  min-width: 0;
  font-weight: 700;
  text-transform: uppercase;
  font-size: 10px;
  letter-spacing: 0.08em;
  color: var(--color-text-secondary);
}

.graph-flow-node__status-badge {
  font-size: 10px;
  padding: 2px 6px;
  background: color-mix(in srgb, var(--color-accent) 12%, var(--glass-surface));
  color: var(--color-text-secondary);
}

.graph-flow-node__label {
  font-weight: 700;
  font-size: 13px;
  line-height: 1.35;
  color: var(--color-text-heading);
  overflow-wrap: anywhere;
}

.graph-flow-node__role {
  margin-top: 4px;
  display: inline-flex;
  padding: 2px 7px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--node-accent, var(--color-accent)) 10%, var(--glass-surface));
  color: var(--color-text-secondary);
  font-size: 10px;
  font-weight: 700;
  text-transform: lowercase;
}

.graph-flow-node__agent-key {
  margin-top: 4px;
  font-size: 10px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  color: var(--color-text-tertiary);
}

.graph-flow-node__hint {
  margin-top: 6px;
  color: var(--color-text-secondary);
  font-size: 11px;
  line-height: 1.45;
}

.graph-flow-node__io {
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px dashed color-mix(in srgb, var(--glass-border) 80%, transparent);
  color: var(--color-text-secondary);
  font-size: 10px;
  line-height: 1.45;
}

.graph-flow-node__fine-status {
  margin-top: 6px;
  font-size: 10px;
  font-weight: 600;
  color: var(--color-text-tertiary);
}

.graph-flow-node__handle {
  width: 10px;
  height: 10px;
  border: 2px solid var(--glass-border);
  background: var(--glass-elevated);
}

.graph-flow-node--selected {
  border-color: color-mix(in srgb, var(--color-accent) 42%, var(--glass-border));
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-accent) 28%, transparent), var(--shadow-entity-panel);
  transform: translateY(-1px);
}

.graph-flow-node--running {
  animation: graph-node-pulse 1.5s ease-in-out infinite;
}

.graph-flow-node--completed {
  border-color: color-mix(in srgb, var(--color-success) 36%, var(--glass-border));
}

.graph-flow-node--failed {
  border-color: color-mix(in srgb, var(--color-danger) 36%, var(--glass-border));
}

.graph-flow-node--interrupted {
  border-color: color-mix(in srgb, var(--color-warning) 36%, var(--glass-border));
}
</style>
