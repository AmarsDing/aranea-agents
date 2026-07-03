<!-- web/src/components/chat/v2/PlanDAG.vue -->
<template>
  <svg :width="width" :height="height" class="plan-dag">
    <!-- Dependency edges -->
    <line
      v-for="edge in edges"
      :key="`${edge.from}-${edge.to}`"
      :x1="edge.x1"
      :y1="edge.y1"
      :x2="edge.x2"
      :y2="edge.y2"
      stroke="#bbb"
      stroke-width="1.5"
      marker-end="url(#arrowhead)"
    />
    <!-- Nodes -->
    <PlanStepNode
      v-for="step in steps"
      :key="step.ID"
      :step="step"
      :pos="positions.get(step.ID) || { x: 0, y: 0 }"
      :node-width="nodeWidth"
      :node-height="nodeHeight"
      :is-selected="selectedId === step.ID"
      @select="selectedId = $event"
    />
    <defs>
      <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
        <polygon points="0 0, 10 3.5, 0 7" fill="#bbb" />
      </marker>
    </defs>
  </svg>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { usePlanDAGLayout } from '../../../features/chat/composables/usePlanDAGLayout';
import type { PlanStep } from '../../../features/chat/v2Types';
import PlanStepNode from './PlanStepNode.vue';

const props = defineProps<{
  steps: PlanStep[];
  width?: number;
}>();

const nodeWidth = 120;
const nodeHeight = 60;
const gapX = 40;
const gapY = 30;
const svgWidth = computed(() => props.width || 600);
const { layoutDAG } = usePlanDAGLayout();
const positions = computed(() => layoutDAG(props.steps, { width: svgWidth.value, nodeWidth, nodeHeight, gapX, gapY }));
const height = computed(() => {
  const max = Math.max(0, ...Array.from(positions.value.values()).map((p) => p.y));
  return max + nodeHeight + 20;
});

const selectedId = ref<string | null>(null);

interface Edge {
  from: string;
  to: string;
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}
const edges = computed<Edge[]>(() => {
  const out: Edge[] = [];
  for (const step of props.steps) {
    const toPos = positions.value.get(step.ID);
    if (!toPos) continue;
    for (const depId of step.DependsOn) {
      const fromPos = positions.value.get(depId);
      if (!fromPos) continue;
      out.push({
        from: depId,
        to: step.ID,
        x1: fromPos.x + nodeWidth / 2,
        y1: fromPos.y + nodeHeight,
        x2: toPos.x + nodeWidth / 2,
        y2: toPos.y,
      });
    }
  }
  return out;
});
</script>
