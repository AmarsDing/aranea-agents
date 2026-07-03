<!-- web/src/components/chat/v2/PlanStepNode.vue -->
<template>
  <g :transform="`translate(${pos.x}, ${pos.y})`" @click="$emit('select', step.ID)">
    <rect
      :width="nodeWidth" :height="nodeHeight" rx="8"
      :fill="statusColor" :stroke="isSelected ? '#1976d2' : '#ccc'" :stroke-width="isSelected ? 2 : 1"
      class="plan-step-node"
    />
    <text :x="nodeWidth / 2" :y="20" text-anchor="middle" fill="white" font-size="12">
      {{ step.Label }}
    </text>
    <text :x="nodeWidth / 2" :y="40" text-anchor="middle" fill="white" font-size="10">
      {{ step.Status }}
    </text>
  </g>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { PlanStep } from '../../../features/chat/v2Types';
import type { NodePosition } from '../../../features/chat/composables/usePlanDAGLayout';

const props = defineProps<{
  step: PlanStep;
  pos: NodePosition;
  nodeWidth: number;
  nodeHeight: number;
  isSelected?: boolean;
}>();

defineEmits<{ select: [id: string] }>();

const statusColor = computed(() => ({
  pending: '#9e9e9e', running: '#2196f3', completed: '#4caf50',
  failed: '#f44336', skipped: '#ff9800', partial_failure: '#ff5722',
}[props.step.Status] || '#9e9e9e'));
</script>
