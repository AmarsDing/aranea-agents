<template>
  <div
    :class="['graph-flow-diamond', `graph-flow-diamond--${data.nodeType}`, { 'graph-flow-diamond--selected': selected, 'graph-flow-diamond--running': data.execStatus === 'running' }]"
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

<style scoped>
.graph-flow-diamond {
  width: 100px;
  height: 100px;
  transform: rotate(45deg);
  border: 2px solid var(--node-border);
  border-radius: 8px;
  background: var(--node-fill);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  transition: box-shadow 0.2s;
  position: relative;
}

.graph-flow-diamond--selected {
  box-shadow: 0 0 0 2px var(--color-accent, var(--color-accent));
}

.graph-flow-diamond--running {
  animation: pulse-glow 1.5s ease-in-out infinite;
}

.graph-flow-diamond__inner {
  transform: rotate(-45deg);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 4px;
  padding: 4px;
}

.graph-flow-diamond__label {
  font-size: 10px;
  font-weight: 600;
  text-align: center;
  overflow-wrap: break-word;
  line-height: 1.2;
}

@keyframes pulse-glow {
  0%, 100% { box-shadow: 0 0 4px rgb(33 150 243 / 30%); }
  50% { box-shadow: 0 0 16px rgb(33 150 243 / 60%); }
}
</style>
