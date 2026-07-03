<!-- web/src/components/chat/v2/GraphNode.vue
  2026-07-04 补齐：GraphStage 中的单个节点，对应一个 PlanStep。
  节点状态由 PlanStep.Status 通过 MapPlanStepToGraphNodeStatus 映射得到。
  设计：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.7.5
-->
<template>
  <g :transform="`translate(${pos.x}, ${pos.y})`" class="graph-node" @click="$emit('select', node.ID)">
    <rect
      :width="nodeWidth"
      :height="nodeHeight"
      rx="8"
      :fill="statusColor"
      :stroke="isSelected ? '#1976d2' : '#ccc'"
      :stroke-width="isSelected ? 2 : 1"
      :class="['graph-node__rect', { 'graph-node__rect--running': node.Status === 'running' }]"
    />
    <text :x="nodeWidth / 2" :y="22" text-anchor="middle" fill="white" font-size="12" font-weight="600">
      {{ node.Label }}
    </text>
    <text :x="nodeWidth / 2" :y="42" text-anchor="middle" fill="white" font-size="10">
      {{ statusIcon }} {{ node.Status }}
    </text>
  </g>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { GraphNode } from '../../../features/chat/v2Types';
import type { NodePosition } from '../../../features/chat/composables/usePlanDAGLayout';

const props = defineProps<{
  node: GraphNode;
  pos: NodePosition;
  nodeWidth: number;
  nodeHeight: number;
  isSelected?: boolean;
}>();

defineEmits<{ select: [id: string] }>();

// 节点状态色（设计文档 §3.7.5 节点状态色）：
//   pending → 灰色 (#9e9e9e)
//   running → 青色脉冲 (#26c6da，CSS animation)
//   completed → 绿色 ✓ (#4caf50)
//   failed → 红色 ✗ (#f44336)
//   interrupted → 黄色 ⏸ (#ffc107)
const statusColor = computed(
  () =>
    ({
      pending: '#9e9e9e',
      running: '#26c6da',
      completed: '#4caf50',
      failed: '#f44336',
      interrupted: '#ffc107',
    })[props.node.Status] || '#9e9e9e',
);

const statusIcon = computed(
  () =>
    ({
      pending: '○',
      running: '◐',
      completed: '✓',
      failed: '✗',
      interrupted: '⏸',
    })[props.node.Status] || '○',
);
</script>

<style scoped>
.graph-node {
  cursor: pointer;
}
.graph-node__rect {
  transition: fill 0.2s ease;
}
.graph-node__rect--running {
  animation: graph-node-pulse 1.4s ease-in-out infinite;
}
@keyframes graph-node-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.55;
  }
}
</style>
