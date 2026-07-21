<template>
  <div class="observation-canvas">
    <VueFlow
      ref="vueFlowRef"
      :nodes="flowNodes"
      :edges="styledEdges"
      :fit-view-on-init="true"
      :node-types="nodeTypes"
      :default-zoom="0.8"
      :min-zoom="0.2"
      :max-zoom="2"
      :nodes-draggable="true"
      :nodes-connectable="false"
      :edges-updatable="false"
      @node-click="onNodeClick"
      @node-double-click="onNodeDoubleClick"
      @edge-click="onEdgeClick"
      @edge-mouse-enter="onEdgeMouseEnter"
      @edge-mouse-leave="onEdgeMouseLeave"
    >
      <Background :pattern-color="isDark ? '#333' : '#ddd'" />
    </VueFlow>
  </div>
</template>

<script setup lang="ts">
import { markRaw, provide, toRef, useTemplateRef, ref, computed } from 'vue';
import { VueFlow, type Node } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import '@vue-flow/core/dist/style.css';
import '@vue-flow/core/dist/theme-default.css';
import ObserveNode from './ObserveNode.vue';
import type { GraphStage, GraphNode } from '../../../features/chat/v2Types';
import { useObserveGraph } from '../../../features/chat/composables/useObserveGraph';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';

const props = defineProps<{
  graphStage: GraphStage | null;
  spiritSessionId: string;
  isDark: boolean;
}>();

const emit = defineEmits<{
  'select-node': [node: GraphNode];
  preview: [art: MediaArtifact];
}>();

const nodeTypes = { observe: markRaw(ObserveNode) };

const spiritSessionIdRef = toRef(props, 'spiritSessionId');
const { nodes, flowNodes, flowEdges, getDependencyChain } = useObserveGraph(spiritSessionIdRef);

// Vue Flow instance ref for programmatic control
const vueFlowRef = useTemplateRef('vueFlowRef');

// Edge highlight state: which edges/nodes are in the highlighted dependency chain
const highlightedChain = ref<{ nodeIds: Set<string>; edgeIds: Set<string> } | null>(null);

// Computed edges with highlight styling
const styledEdges = computed(() =>
  flowEdges.value.map((edge) => {
    const isHighlighted = highlightedChain.value?.edgeIds.has(edge.id) ?? false;
    return {
      ...edge,
      class: isHighlighted ? 'edge--highlighted' : '',
      style: isHighlighted ? { stroke: 'var(--q-primary)', strokeWidth: 2.5 } : {},
    };
  }),
);

function onNodeClick({ node }: { node: Node }) {
  const graphNode = nodes.value.find((n) => n.ID === node.id);
  if (graphNode) emit('select-node', graphNode);
}

/**
 * Double-click a node: center it in the viewport and zoom in slightly.
 * Uses Vue Flow's internal state to find the node position and adjust
 * the viewport accordingly.
 */
function onNodeDoubleClick({ node }: { node: Node }) {
  const vf = vueFlowRef.value;
  if (!vf) return;

  const { x, y } = node.position;
  // Get current viewport dimensions
  const viewport = vf.viewportRef.value;
  if (!viewport) return;

  const rect = viewport.getBoundingClientRect();
  const centerX = rect.width / 2;
  const centerY = rect.height / 2;

  // Calculate new transform to center the node
  const currentTransform = vf.viewport.value;
  const newZoom = Math.min(currentTransform.zoom * 1.2, 2); // Zoom in 20%, max 2x

  vf.setTransform({
    x: centerX - x * newZoom,
    y: centerY - y * newZoom,
    zoom: newZoom,
  });
}

/**
 * Edge click: highlight the dependency chain (upstream → current → downstream).
 * Clicking the same edge again clears the highlight.
 */
function onEdgeClick({ edge }: { edge: { id: string; source: string; target: string } }) {
  const chain = getDependencyChain(edge.target);
  if (chain.nodeIds.size === 0) {
    highlightedChain.value = null;
    return;
  }
  // Toggle: if same chain is already highlighted, clear it
  if (highlightedChain.value?.edgeIds.has(edge.id)) {
    highlightedChain.value = null;
  } else {
    highlightedChain.value = chain;
  }
}

/**
 * Edge mouse enter: highlight the edge and its dependency chain.
 */
function onEdgeMouseEnter({ edge }: { edge: { id: string; source: string; target: string } }) {
  const chain = getDependencyChain(edge.target);
  highlightedChain.value = chain;
}

/**
 * Edge mouse leave: clear highlight.
 */
function onEdgeMouseLeave() {
  highlightedChain.value = null;
}

// Custom-node emits do not bubble through Vue Flow; bridge preview via provide/inject.
provide('observe-media-preview', (art: MediaArtifact) => emit('preview', art));
</script>

<style scoped lang="sass">
.observation-canvas
  width: 100%
  height: 100%
  min-height: 300px

:deep(.edge--highlighted)
  path
    stroke: var(--q-primary) !important
    stroke-width: 2.5px !important
    filter: drop-shadow(0 0 3px var(--q-primary))
</style>
