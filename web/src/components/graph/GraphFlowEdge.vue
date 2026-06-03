<template>
  <g>
    <path
      :id="edgeId"
      :d="path"
      class="vue-flow__edge-path"
      :style="edgeStyle"
      :class="edgeClass"
      :marker-end="markerEnd"
    />
    <circle v-if="showDot" r="3" :fill="dotColor" :style="dotStyle" class="graph-edge-dot">
      <animateMotion :dur="dotDuration" repeatCount="indefinite">
        <mpath :href="`#${edgeId}`" />
      </animateMotion>
    </circle>
    <circle v-if="showDot" r="6" :fill="dotColor" opacity="0.3" class="graph-edge-dot-glow">
      <animateMotion :dur="dotDuration" repeatCount="indefinite">
        <mpath :href="`#${edgeId}`" />
      </animateMotion>
    </circle>
  </g>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { getBezierPath, type EdgeProps } from '@vue-flow/core';

const props = defineProps<EdgeProps>();

const edgeId = computed(() => props.id ?? 'edge');

const path = computed(() => {
  const [p] = getBezierPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    sourcePosition: props.sourcePosition,
    targetX: props.targetX,
    targetY: props.targetY,
    targetPosition: props.targetPosition,
  });
  return p;
});

const edgeKind = computed(() => {
  const cls = (props.data?.edgeClass as string) ?? '';
  if (cls.includes('transfer')) return 'transfer';
  if (cls.includes('dispatch')) return 'dispatch';
  if (cls.includes('conditional')) return 'conditional';
  return 'normal';
});

const edgeClass = computed(() => {
  const classes: Record<string, boolean> = {};
  if (edgeKind.value === 'conditional') classes['graph-edge--conditional'] = true;
  if (edgeKind.value === 'transfer') classes['graph-edge--transfer'] = true;
  if (edgeKind.value === 'dispatch') classes['graph-edge--dispatch'] = true;
  return classes;
});

const edgeStyle = computed(() => props.style ?? {});

const showDot = computed(() => edgeKind.value !== 'normal');

const dotColor = computed(() => {
  switch (edgeKind.value) {
    case 'conditional':
      return 'var(--graph-edge-conditional)';
    case 'transfer':
      return 'var(--graph-edge-transfer)';
    case 'dispatch':
      return 'var(--graph-edge-dispatch)';
    default:
      return 'var(--graph-edge-normal)';
  }
});

const dotDuration = computed(() => {
  switch (edgeKind.value) {
    case 'conditional':
      return '1.6s';
    case 'transfer':
      return '1.8s';
    case 'dispatch':
      return '1.4s';
    default:
      return '2.5s';
  }
});

const dotStyle = computed(() => ({
  filter: `drop-shadow(0 0 4px currentColor)`,
}));
</script>
