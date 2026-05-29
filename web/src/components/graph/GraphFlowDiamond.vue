<template>
  <div
    :class="['graph-flow-diamond', `graph-flow-diamond--${data.nodeType}`, { 'graph-flow-diamond--selected': selected, 'graph-flow-diamond--running': data.execStatus === 'running' }]"
    role="group"
    :aria-label="data.label || data.nodeId"
    :style="diamondStyle"
  >
    <Handle type="target" :position="Position.Top" :style="{ left: '50%', top: '0' }" />
    <div class="graph-flow-diamond__inner">
      <q-icon :name="styleConfig.icon" size="14px" />
      <span class="graph-flow-diamond__label">{{ data.label || data.nodeId }}</span>
    </div>
    <Handle type="source" :position="Position.Bottom" :style="{ left: '50%', bottom: '0' }" />
    <Handle v-if="data.nodeType === 'router'" type="source" :position="Position.Right" :style="{ right: '0', top: '50%' }" id="branch-yes" />
    <Handle v-if="data.nodeType === 'router'" type="source" :position="Position.Left" :style="{ left: '0', top: '50%' }" id="branch-no" />
    <span v-if="data.nodeType === 'router'" class="graph-flow-diamond__branch-label graph-flow-diamond__branch-label--yes">是</span>
    <span v-if="data.nodeType === 'router'" class="graph-flow-diamond__branch-label graph-flow-diamond__branch-label--no">否</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { Handle, Position } from "@vue-flow/core";
import { NODE_TYPE_STYLES, type NodeType } from "../../features/graph/types";

const props = defineProps<{
  id: string;
  data: {
    nodeId: string;
    nodeType: NodeType;
    label: string;
    execStatus?: string;
  };
  selected?: boolean;
}>();

const styleConfig = computed(() => NODE_TYPE_STYLES[props.data.nodeType] ?? NODE_TYPE_STYLES.router);

const diamondStyle = computed(() => ({
  "--node-fill": styleConfig.value.fillColor,
  "--node-border": styleConfig.value.borderColor,
}));
</script>
