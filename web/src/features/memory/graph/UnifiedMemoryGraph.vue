// Container: approved — 跨层关联图谱 Tab：左过滤栏 + Vue Flow 画布（去 Controls/MiniMap，保留缩放拖拽）+ 右详情抽屉 +
底部状态栏。
<template>
  <div class="column q-gutter-md">
    <q-banner v-if="error" rounded class="bg-negative text-white">
      {{ error }}
      <template #action>
        <q-btn flat color="white" :label="t('memory.error.retry')" @click="load()" />
      </template>
    </q-banner>

    <q-card v-if="!agentId" flat class="memory-card text-center text-grey-7 q-pa-xl">
      {{ t('memory.unifiedGraph.selectAgent') }}
    </q-card>

    <div v-else class="row q-gutter-md graph-wrap">
      <graph-filter-rail
        :enabled-layers="enabledLayers"
        :min-weight="minWeight"
        :hops="hops"
        :loading="loading"
        :search-nodes="searchNodes"
        @toggle-layer="toggleLayer"
        @update:min-weight="minWeight = $event"
        @update:hops="hops = $event"
        @locate="onLocate"
        @refresh="load()"
      />

      <div class="col graph-canvas-col">
        <div class="graph-canvas memory-card">
          <VueFlow
            ref="vueFlowRef"
            :nodes="flowNodes"
            :edges="flowEdges"
            :node-types="nodeTypes"
            :fit-view-on-init="true"
            :default-zoom="0.9"
            :min-zoom="0.2"
            :max-zoom="2"
            :nodes-draggable="false"
            :nodes-connectable="false"
            :edges-updatable="false"
            @node-click="onNodeClick"
            @pane-click="onPaneClick"
          >
            <Background :pattern-color="$q.dark.isActive ? '#333' : '#ddd'" :gap="22" />
          </VueFlow>

          <q-inner-loading :showing="loading">
            <q-spinner-dots size="40px" color="primary" />
          </q-inner-loading>

          <div v-if="!loading && !nodes.length" class="graph-empty column items-center q-gutter-sm">
            <q-icon name="bubble_chart" size="44px" color="grey-5" />
            <div class="text-body2 text-grey-7 text-center">{{ emptyText }}</div>
          </div>
        </div>

        <div class="graph-status text-caption row items-center q-gutter-md">
          <span>
            {{
              t('memory.unifiedGraph.statusBar', { nodes: nodeCount, edges: edgeCount, filtered: filteredEdgeCount })
            }}
          </span>
          <span v-if="focusLabel" class="graph-status__focus">
            {{ t('memory.unifiedGraph.focusPrefix') }} · {{ focusLabel }}
          </span>
        </div>
      </div>
    </div>

    <graph-node-detail-drawer
      v-model="drawerOpen"
      :node="selectedNode"
      :edges="selectedEdges"
      :nodes="nodes"
      @open-in-browse="(node) => emit('open-in-browse', node)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, markRaw, ref, toRef, useTemplateRef, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { VueFlow, type Edge, type Node } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import '@vue-flow/core/dist/style.css';
import '@vue-flow/core/dist/theme-default.css';
import type { UnifiedGraphNode as UnifiedGraphNodeData } from '../types';
import {
  layoutUnifiedMemoryGraph,
  unifiedEdgeStyle,
  UNIFIED_NODE_WIDTH,
  UNIFIED_NODE_HEIGHT,
} from './memoryGraphLayout';
import { useUnifiedMemoryGraph } from './composables/useUnifiedMemoryGraph';
import UnifiedGraphNode from './UnifiedGraphNode.vue';
import GraphFilterRail from './GraphFilterRail.vue';
import GraphNodeDetailDrawer from './GraphNodeDetailDrawer.vue';

const props = defineProps<{ agentId: string | null }>();
const emit = defineEmits<{
  (e: 'open-in-browse', node: UnifiedGraphNodeData): void;
}>();

const $q = useQuasar();
const { t } = useI18n();

const {
  graph,
  nodes,
  edges,
  focusId,
  emptyReason,
  filteredEdgeCount,
  loading,
  error,
  hops,
  minWeight,
  enabledLayers,
  selectedNodeId,
  selectedNode,
  selectedEdges,
  load,
  selectNode,
  toggleLayer,
  searchNodes,
} = useUnifiedMemoryGraph(toRef(props, 'agentId'));

const nodeTypes = { unified: markRaw(UnifiedGraphNode) };
const vueFlowRef = useTemplateRef('vueFlowRef');
const drawerOpen = ref(false);

const nodesById = computed(() => new Map(nodes.value.map((n) => [n.id, n])));
const nodeCount = computed(() => graph.value?.node_count ?? nodes.value.length);
const edgeCount = computed(() => graph.value?.edge_count ?? edges.value.length);
const focusLabel = computed(() => (focusId.value ? (nodesById.value.get(focusId.value)?.label ?? '') : ''));

const flowNodes = computed<Node[]>(() => {
  const positions = layoutUnifiedMemoryGraph(nodes.value, edges.value);
  return nodes.value.map((n) => ({
    id: n.id,
    type: 'unified',
    position: positions.get(n.id) ?? { x: 0, y: 0 },
    data: {
      label: n.label,
      layer: n.layer,
      kind: n.kind,
      weight: n.weight,
      isFocus: n.id === focusId.value,
    },
  }));
});

const flowEdges = computed<Edge[]>(() =>
  edges.value.map((e) => {
    const base = unifiedEdgeStyle(e, nodesById.value.get(e.source)?.layer ?? '');
    const connected =
      selectedNodeId.value !== null && (e.source === selectedNodeId.value || e.target === selectedNodeId.value);
    const style = connected
      ? { ...base, stroke: 'var(--q-primary)', strokeWidth: '2.5', strokeDasharray: undefined }
      : selectedNodeId.value
        ? { ...base, opacity: '0.25' }
        : base;
    return {
      id: `${e.source}->${e.target}:${e.type}`,
      source: e.source,
      target: e.target,
      style,
      label: e.type === 'entity_relation' ? e.label : undefined,
      labelStyle: { fontSize: '10px', fill: 'var(--color-text-secondary)' },
      labelBgStyle: { fill: 'transparent' },
    };
  }),
);

const emptyText = computed(() => {
  if (emptyReason.value === 'no_memory_data') return t('memory.unifiedGraph.empty.noMemoryData');
  if (emptyReason.value === 'focus_not_found') return t('memory.unifiedGraph.empty.focusNotFound');
  return t('memory.unifiedGraph.empty.generic');
});

function onNodeClick({ node }: { node: Node }) {
  selectNode(node.id);
  drawerOpen.value = true;
}

function onPaneClick() {
  selectNode(null);
}

/** 搜索定位：选中节点、打开抽屉并平滑居中（FR-R7）。 */
function onLocate(nodeId: string) {
  selectNode(nodeId);
  drawerOpen.value = true;
  const target = flowNodes.value.find((n) => n.id === nodeId);
  const vf = vueFlowRef.value;
  if (target && vf) {
    vf.setCenter(target.position.x + UNIFIED_NODE_WIDTH / 2, target.position.y + UNIFIED_NODE_HEIGHT / 2, {
      zoom: 1.2,
      duration: 400,
    });
  }
}

// 选中节点在刷新后消失时同步关闭抽屉。
watch(selectedNodeId, (id) => {
  if (!id) drawerOpen.value = false;
});
</script>

<style scoped>
.graph-wrap {
  align-items: stretch;
}

.graph-canvas-col {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.graph-canvas {
  height: 560px;
  overflow: hidden;
  position: relative;
}

.graph-empty {
  left: 50%;
  max-width: 320px;
  position: absolute;
  top: 50%;
  transform: translate(-50%, -50%);
  z-index: 5;
}

.graph-status {
  color: var(--color-text-secondary);
  justify-content: center;
  padding-top: var(--space-2);
}

.graph-status__focus {
  color: var(--q-primary);
}
</style>
