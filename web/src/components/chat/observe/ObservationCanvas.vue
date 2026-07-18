<template>
  <div class="observation-canvas">
    <VueFlow
      :nodes="flowNodes"
      :edges="flowEdges"
      :fit-view-on-init="true"
      :node-types="nodeTypes"
      :default-zoom="0.8"
      :min-zoom="0.2"
      :max-zoom="2"
      :nodes-draggable="false"
      :nodes-connectable="false"
      :edges-updatable="false"
      @node-click="onNodeClick"
    >
      <Background :pattern-color="isDark ? '#333' : '#ddd'" />
      <Controls />
      <MiniMap :pannable="true" :zoomable="true" />
    </VueFlow>
  </div>
</template>

<script setup lang="ts">
import { markRaw, provide, toRef } from 'vue';
import { VueFlow, type Node } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import { Controls } from '@vue-flow/controls';
import { MiniMap } from '@vue-flow/minimap';
import '@vue-flow/core/dist/style.css';
import '@vue-flow/core/dist/theme-default.css';
import '@vue-flow/controls/dist/style.css';
import '@vue-flow/minimap/dist/style.css';
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
const { nodes, flowNodes, flowEdges } = useObserveGraph(spiritSessionIdRef);

function onNodeClick({ node }: { node: Node }) {
  const graphNode = nodes.value.find((n) => n.ID === node.id);
  if (graphNode) emit('select-node', graphNode);
}

// Custom-node emits do not bubble through Vue Flow; bridge preview via provide/inject.
provide('observe-media-preview', (art: MediaArtifact) => emit('preview', art));
</script>

<style scoped lang="sass">
.observation-canvas
  width: 100%
  height: 100%
  min-height: 300px
</style>
