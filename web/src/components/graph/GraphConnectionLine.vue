<script setup lang="ts">
import { computed, inject, ref, type Ref } from 'vue';
import { getBezierPath } from '@vue-flow/core';
import type { ConnectionLineProps } from '@vue-flow/core';
import { STATE_FIELD_TYPE_COLORS } from '../../features/graph/portTypes';

const props = defineProps<ConnectionLineProps>();

const path = computed(() => {
  const [edgePath] = getBezierPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    sourcePosition: props.sourcePosition,
    targetX: props.targetX,
    targetY: props.targetY,
    targetPosition: props.targetPosition,
  });
  return edgePath;
});

// Inject connection field info to get the type color
const connectingField = inject<Ref<{ field: string; direction: string; fieldType?: string } | null>>(
  'graphConnectingField',
  ref(null),
);

const accentColor = computed(() => {
  const fieldType = connectingField.value?.fieldType;
  if (fieldType && STATE_FIELD_TYPE_COLORS[fieldType]) {
    return STATE_FIELD_TYPE_COLORS[fieldType];
  }
  return 'var(--color-accent)';
});
</script>

<template>
  <g>
    <path
      fill="none"
      :stroke="accentColor"
      stroke-width="2"
      stroke-dasharray="5 5"
      class="graph-connection-line__path"
      :d="path"
    />
    <circle :cx="targetX" :cy="targetY" fill="var(--glass-surface)" r="5" :stroke="accentColor" stroke-width="1.5" />
  </g>
</template>

<style scoped>
.graph-connection-line__path {
  animation: graph-connection-flow 0.6s linear infinite;
}

@keyframes graph-connection-flow {
  from {
    stroke-dashoffset: 10;
  }

  to {
    stroke-dashoffset: 0;
  }
}
</style>
