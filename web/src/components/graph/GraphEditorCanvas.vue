<template>
  <div :class="['graph-editor-canvas', { 'is-dark': isDark }]" @dragover.prevent="onDragOver" @drop="onDrop">
    <VueFlow
      v-model:nodes="internalNodes"
      v-model:edges="internalEdges"
      :node-types="nodeTypes"
      :edge-types="edgeTypes"
      :default-edge-options="defaultEdgeOptions"
      :connection-line-style="connectionLineStyle"
      :fit-view-on-init="true"
      :snap-to-grid="false"
      :min-zoom="0.2"
      :max-zoom="2"
      :nodes-draggable="!readOnly"
      :nodes-connectable="!readOnly"
      :edges-updatable="!readOnly"
      :edges-deletable="!readOnly"
      :elements-selectable="true"
      :selection-mode="SelectionMode.Partial"
      :delete-key-code="null"
      @node-click="onNodeClick"
      @pane-click="onPaneClick"
      @pane-context-menu="onPaneContextMenu"
      @node-context-menu="onNodeContextMenu"
      @connect="onConnect"
      @connect-start="onConnectStart"
      @connect-end="onConnectEnd"
      @nodes-change="onNodesChange"
      @edges-change="onEdgesChange"
      @edge-update="onEdgeUpdate"
      @edge-context-menu="onEdgeContextMenu"
      @node-drag-start="onNodeDragStart"
      @node-drag="onNodeDrag"
      @node-drag-stop="onNodeDragStop"
    >
      <Background :gap="16" />
      <template #connection-line="connectionLineProps">
        <GraphConnectionLine v-bind="connectionLineProps" />
      </template>
      <template #zoom-pane>
        <svg v-if="snapLines.length > 0" class="snap-guide-layer">
          <line
            v-for="(line, idx) in snapLines"
            :key="idx"
            :x1="line.orientation === 'vertical' ? line.position : line.from"
            :y1="line.orientation === 'horizontal' ? line.position : line.from"
            :x2="line.orientation === 'vertical' ? line.position : line.to"
            :y2="line.orientation === 'horizontal' ? line.position : line.to"
            class="snap-guide-line"
            :class="`snap-guide-line--${line.orientation}`"
          />
        </svg>
      </template>
    </VueFlow>
    <GraphContextMenu
      :visible="ctxMenuVisible"
      :x="ctxMenuX"
      :y="ctxMenuY"
      :items="ctxMenuItems"
      @select="onCtxMenuSelect"
      @close="onCtxMenuClose"
    />
    <GraphContextMenu
      :visible="paneMenuVisible"
      :x="paneMenuX"
      :y="paneMenuY"
      :items="paneMenuItems"
      @select="onPaneMenuSelect"
      @close="paneMenuVisible = false"
    />
    <GraphContextMenu
      :visible="edgeMenuVisible"
      :x="edgeMenuX"
      :y="edgeMenuY"
      :items="edgeMenuItems"
      @select="onEdgeMenuSelect"
      @close="onEdgeMenuClose"
    />
    <GraphNodeSearch
      :visible="searchVisible"
      :match-index="searchMatchIndex"
      :match-count="searchMatchCount"
      @search="onSearchInput"
      @prev="onSearchPrev"
      @next="onSearchNext"
      @close="onSearchClose"
    />
    <div class="graph-editor-canvas__zoom-indicator">
      <q-btn flat dense round icon="remove" size="xs" @click="zoomOut">
        <q-tooltip>{{ t('graphs.canvasZoomOut') }}</q-tooltip>
      </q-btn>
      <span class="graph-editor-canvas__zoom-text" @click="zoomToFit">{{ zoomLabel }}</span>
      <q-btn flat dense round icon="add" size="xs" @click="zoomIn">
        <q-tooltip>{{ t('graphs.canvasZoomIn') }}</q-tooltip>
      </q-btn>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, computed, onMounted, onUnmounted, markRaw } from 'vue';
import type { Ref, Component } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import {
  VueFlow,
  useVueFlow,
  type Connection,
  type Edge,
  type Node,
  type NodeChange,
  type EdgeChange,
  type EdgeUpdateEvent,
  SelectionMode,
} from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import '@vue-flow/core/dist/style.css';
import '@vue-flow/core/dist/theme-default.css';
import GraphFlowNode from './GraphFlowNode.vue';
import GraphFlowDiamond from './GraphFlowDiamond.vue';
import GraphFlowEdge from './GraphFlowEdge.vue';
import GraphConnectionLine from './GraphConnectionLine.vue';
import GraphContextMenu from './GraphContextMenu.vue';
import type { ContextMenuItem } from './GraphContextMenu.vue';
import GraphNodeSearch from './GraphNodeSearch.vue';
import type {
  NodeDef,
  EdgeDef,
  ConditionalEdgeDef,
  NodeType,
  GraphDefinition,
  NodeIssueInfo,
} from '../../features/graph/types';
import { NODE_TYPE_STYLES } from '../../features/graph/types';
import { isValidConnectionQuick } from '../../features/graph/portTypes';
import type { useGraphUndoRedo } from '../../features/graph/useGraphUndoRedo';
import { defaultNodePosition, readGraphLayout, writeGraphNodePosition } from '../../features/graph/editor/graphLayout';
import { useSnapGuide } from '../../features/graph/useSnapGuide';
import type { SnapGuideNode } from '../../features/graph/useSnapGuide';
import { graphNodeDisplayLabel } from '../../features/orchestration/teamNodeDisplay';

const graphDef = defineModel<GraphDefinition>('graphDef', { required: true });

const props = defineProps<{
  isDark: boolean;
  execNodeStates?: Map<
    string,
    {
      status: string;
      fineStatus?: string;
      inputPreview?: string;
      outputPreview?: string;
      currentActivity?: string;
    }
  >;
  selectedNodeId?: string | null;
  /** When true, pans/zooms to selected node (Observatory focus sync). */
  focusSelectedNode?: boolean;
  /** Run/monitor mode: disable editing gestures. */
  readOnly?: boolean;
  undoRedo?: ReturnType<typeof useGraphUndoRedo>;
  /** nodeId → 校验问题（驱动节点错误态）。 */
  nodeIssues?: Record<string, NodeIssueInfo>;
  /** 聚光灯目标节点；设置时其余节点压暗并 fitView 居中。 */
  spotlightNodeId?: string | null;
  /** 用户视角场景（如团队编排页）：隐藏 agent 编码等技术标识，仅显示可读名称与中文角色。 */
  hideTechIds?: boolean;
}>();

const EMPTY_EXEC_NODE_STATES: Map<
  string,
  {
    status: string;
    fineStatus?: string;
    inputPreview?: string;
    outputPreview?: string;
    currentActivity?: string;
  }
> = new Map();

const resolvedExecNodeStates = computed(() => props.execNodeStates ?? EMPTY_EXEC_NODE_STATES);

const emit = defineEmits<{
  selectNode: [nodeId: string | null];
  updateGraph: [];
  requestAutoLayout: [];
  focusPropertyPanel: [nodeId: string];
  clearSpotlight: [];
}>();

const { project, fitView, getSelectedNodes, onViewportChange, zoomTo, getNodes } = useVueFlow();
const internalNodes = ref<Node[]>([]);
const internalEdges = ref<Edge[]>([]);
const { snapLines, computeSnapLines, clearSnapLines } = useSnapGuide(internalNodes as Ref<SnapGuideNode[]>);
const $q = useQuasar();
const { t } = useI18n();

const ctxMenuVisible = ref(false);
const ctxMenuX = ref(0);
const ctxMenuY = ref(0);
const ctxMenuNodeId = ref<string | null>(null);
const edgeMenuVisible = ref(false);
const edgeMenuX = ref(0);
const edgeMenuY = ref(0);
const edgeMenuEdgeId = ref<string | null>(null);
const paneMenuVisible = ref(false);
const paneMenuX = ref(0);
const paneMenuY = ref(0);

const searchVisible = ref(false);
const searchQuery = ref('');
const searchMatchIndex = ref(0);
const connectingFrom = ref<string | null>(null);
const zoomLevel = ref(1);
const dragStartPositions = new Map<string, { x: number; y: number }>();

onViewportChange(({ zoom }) => {
  zoomLevel.value = zoom;
});

const zoomLabel = computed(() => `${Math.round(zoomLevel.value * 100)}%`);

function zoomIn() {
  zoomTo(Math.min(zoomLevel.value + 0.15, 2), { duration: 200 });
}

function zoomOut() {
  zoomTo(Math.max(zoomLevel.value - 0.15, 0.2), { duration: 200 });
}

function zoomToFit() {
  fitView({ padding: 0.2, duration: 300 });
}

const nodeTypes: Record<string, Component> = markRaw({
  function: markRaw(GraphFlowNode),
  llm: markRaw(GraphFlowNode),
  tool: markRaw(GraphFlowNode),
  agent: markRaw(GraphFlowNode),
  hitl: markRaw(GraphFlowNode),
  router: markRaw(GraphFlowDiamond),
  join: markRaw(GraphFlowDiamond),
});

const edgeTypes: Record<string, Component> = markRaw({
  flowEdge: markRaw(GraphFlowEdge),
});

const readOnly = computed(() => props.readOnly ?? false);

const defaultEdgeOptions = {
  type: 'flowEdge',
  animated: false,
  style: { stroke: 'var(--graph-edge-normal)', strokeWidth: 1 },
};

const connectionLineStyle = {
  stroke: 'var(--graph-edge-normal)',
  strokeWidth: 1.5,
  strokeDasharray: '6 4',
};

function edgeKindLabel(kind?: string): string | undefined {
  switch ((kind ?? '').toLowerCase()) {
    case 'transfer':
      return t('graphs.edgeKindTransfer');
    case 'dispatch':
      return t('graphs.edgeKindDispatch');
    case 'flow':
      return undefined;
    default:
      return kind?.trim() || undefined;
  }
}

let syncingFromProp = false;
let preferSavedLayout = false;

function buildNodes(): Node[] {
  const existingPositions = new Map<string, { x: number; y: number }>();
  for (const n of internalNodes.value) {
    existingPositions.set(n.id, n.position);
  }
  const savedLayout = readGraphLayout(graphDef.value);

  return graphDef.value.nodes.map((n, index) => {
    const style = NODE_TYPE_STYLES[n.type as NodeType] ?? NODE_TYPE_STYLES.function;
    const isDiamond = n.type === 'router' || n.type === 'join';
    const execState = resolvedExecNodeStates.value.get(n.id);
    const issue = props.nodeIssues?.[n.id];
    const spotlightId = props.spotlightNodeId ?? null;
    const dimmed = spotlightId !== null && spotlightId !== n.id;
    const pos = preferSavedLayout
      ? (savedLayout[n.id] ?? existingPositions.get(n.id) ?? defaultNodePosition(index))
      : (existingPositions.get(n.id) ?? savedLayout[n.id] ?? defaultNodePosition(index));
    return {
      id: n.id,
      type: n.type,
      position: pos,
      selected: n.id === props.selectedNodeId,
      class: dimmed ? 'graph-node--dimmed' : undefined,
      data: {
        nodeId: n.id,
        nodeType: n.type as NodeType,
        label: graphNodeDisplayLabel(n),
        agentName: props.hideTechIds ? '' : n.agentName,
        role: n.requiredRole,
        description: n.description,
        instruction: n.instruction || n.description,
        execStatus: execState?.status,
        fineStatus: execState?.fineStatus,
        inputPreview: execState?.inputPreview,
        outputPreview: execState?.outputPreview,
        currentActivity: execState?.currentActivity,
        toolNames: n.toolNames,
        issue,
        spotlighted: spotlightId === n.id,
      },
      style: isDiamond
        ? {
            background: style.fillColor,
            borderColor: style.borderColor,
          }
        : {},
    };
  });
}

function buildEdges(): Edge[] {
  const edges: Edge[] = [];
  for (const e of graphDef.value.edges) {
    const isTransfer = (e.kind ?? '').toLowerCase() === 'transfer';
    const isDispatch = (e.kind ?? '').toLowerCase() === 'dispatch';
    const edgeClass = isTransfer ? 'graph-edge--transfer' : isDispatch ? 'graph-edge--dispatch' : '';
    edges.push({
      id: `e-${e.from}-${e.to}`,
      source: e.from,
      target: e.to,
      type: 'flowEdge',
      animated: isTransfer,
      class: edgeClass,
      data: { edgeClass },
      style: { stroke: 'var(--graph-edge-normal)', strokeWidth: 1 },
      label:
        edgeKindLabel(e.kind) ??
        (isTransfer ? t('graphs.edgeKindTransfer') : isDispatch ? t('graphs.edgeKindDispatch') : undefined),
      labelStyle: { fill: 'var(--graph-ctx-text)', fontSize: 10, fontWeight: 600 },
      labelBgStyle: {
        fill: 'var(--graph-ctx-bg)',
        fillOpacity: 0.9,
        stroke: 'var(--graph-ctx-border)',
        strokeWidth: 0.5,
      },
      labelBgPadding: [6, 4],
      labelBgBorderRadius: 6,
    });
  }
  for (const ce of graphDef.value.conditionalEdges) {
    const pathMap = ce.pathMap ?? {};
    for (const [label, target] of Object.entries(pathMap)) {
      edges.push({
        id: `ce-${ce.from}-${target}-${label}`,
        source: ce.from,
        target,
        type: 'flowEdge',
        class: 'graph-edge--conditional',
        data: { edgeClass: 'graph-edge--conditional' },
        label,
        labelStyle: { fill: 'var(--graph-edge-conditional)', fontSize: 10, fontWeight: 600 },
        labelBgStyle: {
          fill: 'var(--graph-ctx-bg)',
          fillOpacity: 0.9,
          stroke: 'var(--graph-cond-edge-label-stroke)',
          strokeWidth: 0.5,
        },
        labelBgPadding: [6, 4],
        labelBgBorderRadius: 6,
        style: { stroke: 'var(--graph-edge-conditional)', strokeWidth: 1 },
      });
    }
  }
  return edges;
}

function execNodeStatesFingerprint(
  map: Map<
    string,
    { status: string; fineStatus?: string; inputPreview?: string; outputPreview?: string; currentActivity?: string }
  >,
): string {
  if (map.size === 0) return '';
  return [...map.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([id, st]) => `${id}:${st.status}:${st.fineStatus ?? ''}:${st.currentActivity ?? ''}`)
    .join('|');
}

let lastNodeSig = '';
let lastEdgeSig = '';
let lastCondSig = '';
let lastLayoutSig = '';
let lastExecFp = '';

watch(
  () => graphDef.value.nodes.map((n) => n.id).join('\0'),
  (sig) => {
    if (sig === lastNodeSig) return;
    lastNodeSig = sig;
    rebuildAll();
  },
  { immediate: false },
);

watch(
  () => graphDef.value.edges.map((e) => `${e.from}->${e.to}:${e.kind ?? ''}`).join('\0'),
  (sig) => {
    if (sig === lastEdgeSig) return;
    lastEdgeSig = sig;
    rebuildAll();
  },
  { immediate: false },
);

watch(
  () =>
    graphDef.value.conditionalEdges
      .map(
        (ce) =>
          `${ce.from}:${Object.keys(ce.pathMap ?? {})
            .sort()
            .join(',')}`,
      )
      .join('\0'),
  (sig) => {
    if (sig === lastCondSig) return;
    lastCondSig = sig;
    rebuildAll();
  },
  { immediate: false },
);

watch(
  () => JSON.stringify(readGraphLayout(graphDef.value)),
  (sig) => {
    if (sig === lastLayoutSig) return;
    lastLayoutSig = sig;
    preferSavedLayout = true;
    rebuildAll();
  },
  { immediate: false },
);

watch(
  () => execNodeStatesFingerprint(resolvedExecNodeStates.value),
  (fp) => {
    if (fp === lastExecFp) return;
    lastExecFp = fp;
    syncingFromProp = true;
    internalNodes.value = buildNodes();
    nextTick(() => {
      syncingFromProp = false;
    });
  },
);

watch(
  () => props.selectedNodeId,
  () => {
    syncingFromProp = true;
    internalNodes.value = buildNodes();
    nextTick(() => {
      syncingFromProp = false;
    });
  },
);

// 校验问题映射变化 → 刷新节点错误态（错误/警告边框、内联条）
watch(
  () =>
    Object.entries(props.nodeIssues ?? {})
      .map(([id, i]) => `${id}:${i.level}:${i.code}:${i.message}`)
      .sort()
      .join('|'),
  () => {
    syncingFromProp = true;
    internalNodes.value = buildNodes();
    nextTick(() => {
      syncingFromProp = false;
    });
  },
);

// 聚光灯：目标节点 fitView 居中 + 其余节点压暗；清除时恢复
watch(
  () => props.spotlightNodeId,
  (nodeId, prev) => {
    if (nodeId === prev) return;
    syncingFromProp = true;
    internalNodes.value = buildNodes();
    nextTick(() => {
      syncingFromProp = false;
    });
    if (nodeId) {
      fitView({ nodes: [nodeId], padding: 0.4, duration: 280, maxZoom: 1.2 });
    }
  },
);

function rebuildAll() {
  syncingFromProp = true;
  internalNodes.value = buildNodes();
  internalEdges.value = buildEdges();
  nextTick(() => {
    syncingFromProp = false;
    preferSavedLayout = false;
  });
}

rebuildAll();

watch(
  () => props.selectedNodeId,
  (nodeId) => {
    if (!props.focusSelectedNode || !nodeId) return;
    nextTick(() => {
      if (internalNodes.value.some((n) => n.id === nodeId)) {
        fitView({ nodes: [nodeId], padding: 0.35, duration: 280, maxZoom: 1.25 });
      }
    });
  },
);

function onNodeClick({ node }: { node: Node }) {
  emit('selectNode', node.id);
}

function onPaneClick() {
  emit('selectNode', null);
  emit('clearSpotlight');
  ctxMenuVisible.value = false;
  edgeMenuVisible.value = false;
  paneMenuVisible.value = false;
}

function onPaneContextMenu(event: MouseEvent) {
  event.preventDefault();
  ctxMenuVisible.value = false;
  paneMenuX.value = event.clientX;
  paneMenuY.value = event.clientY;
  paneMenuVisible.value = true;
}

const paneMenuItems = computed<ContextMenuItem[]>(() => {
  const count = getSelectedNodes.value.length;
  const items: ContextMenuItem[] = [];
  if (!readOnly.value) {
    items.push({ icon: '⊞', label: t('graphs.canvasPaneAutoLayout'), action: 'autoLayout' });
  }
  items.push({ icon: '▣', label: t('graphs.canvasPaneSelectAll'), shortcut: 'Ctrl+A', action: 'selectAll' });
  if (count > 1 && !readOnly.value) {
    items.push({
      icon: '✕',
      label: t('graphs.canvasPaneDeleteSelected', { count }),
      shortcut: 'Del',
      danger: true,
      action: 'deleteSelected',
    });
  }
  return items;
});

const ctxMenuItems = computed<ContextMenuItem[]>(() => {
  const items: ContextMenuItem[] = [
    { icon: '✎', label: t('graphs.canvasCtxViewProps'), shortcut: 'Enter', action: 'edit' },
  ];
  if (!readOnly.value) {
    items.push(
      { icon: '⧉', label: t('graphs.canvasCtxDuplicate'), shortcut: 'Ctrl+D', action: 'duplicate' },
      { icon: '✕', label: t('graphs.canvasCtxDelete'), shortcut: 'Del', danger: true, action: 'delete' },
      { icon: '⟂', label: t('graphs.canvasCtxDisconnect'), action: 'disconnect' },
      { icon: '▷', label: t('graphs.canvasCtxSetEntry'), success: true, action: 'setEntry' },
      { icon: '◻', label: t('graphs.canvasCtxSetFinish'), danger: true, action: 'setFinish' },
    );
  }
  return items;
});

const searchMatches = computed(() => {
  if (!searchQuery.value.trim()) return [];
  const q = searchQuery.value.trim().toLowerCase();
  return graphDef.value.nodes
    .filter(
      (n) =>
        n.id.toLowerCase().includes(q) ||
        (n.description ?? '').toLowerCase().includes(q) ||
        (n.instruction ?? '').toLowerCase().includes(q) ||
        (n.agentName ?? '').toLowerCase().includes(q),
    )
    .map((n) => n.id);
});

const searchMatchCount = computed(() => searchMatches.value.length);

function onNodeContextMenu({ event, node }: { event: MouseEvent; node: Node }) {
  event.preventDefault();
  paneMenuVisible.value = false;
  ctxMenuNodeId.value = node.id;
  ctxMenuX.value = event.clientX;
  ctxMenuY.value = event.clientY;
  ctxMenuVisible.value = true;
  emit('selectNode', node.id);
}

function onCtxMenuSelect(action: string) {
  const nodeId = ctxMenuNodeId.value;
  if (!nodeId) return;
  ctxMenuVisible.value = false;

  switch (action) {
    case 'edit':
      emit('focusPropertyPanel', nodeId);
      break;
    case 'duplicate':
      duplicateNode(nodeId);
      break;
    case 'delete':
      deleteNode(nodeId);
      break;
    case 'disconnect':
      disconnectNode(nodeId);
      break;
    case 'setEntry':
      if (props.undoRedo) {
        props.undoRedo.pushSetGraphProperty('entryPoint', graphDef.value.entryPoint, nodeId);
      } else {
        graphDef.value.entryPoint = nodeId;
        emit('updateGraph');
      }
      break;
    case 'setFinish':
      if (props.undoRedo) {
        props.undoRedo.pushSetGraphProperty('finishPoint', graphDef.value.finishPoint, nodeId);
      } else {
        graphDef.value.finishPoint = nodeId;
        emit('updateGraph');
      }
      break;
  }
}

function onCtxMenuClose() {
  ctxMenuVisible.value = false;
}

const edgeMenuItems = computed<ContextMenuItem[]>(() => {
  const items: ContextMenuItem[] = [];
  if (!readOnly.value) {
    items.push({
      icon: '✕',
      label: t('graphs.canvasEdgeMenuDelete'),
      shortcut: 'Del',
      danger: true,
      action: 'deleteEdge',
    });
  }
  return items;
});

function onEdgeContextMenu({ edge, event }: { edge: Edge; event: MouseEvent }) {
  if (readOnly.value) return;
  event.preventDefault();
  ctxMenuVisible.value = false;
  paneMenuVisible.value = false;
  edgeMenuEdgeId.value = edge.id;
  edgeMenuX.value = event.clientX;
  edgeMenuY.value = event.clientY;
  edgeMenuVisible.value = true;
}

function deleteEdgeById(edgeId: string) {
  const resolved = resolveConditionalEdgeRemoval(edgeId);
  if (resolved) {
    const ce = graphDef.value.conditionalEdges[resolved.ceIdx];
    if (props.undoRedo) {
      // execute() 会通过 redo() 完成首次删除，此处禁止预改 graphDef
      props.undoRedo.pushDeleteConditionalEdge(ce, resolved.ceIdx, resolved.label);
    } else {
      const newPathMap = { ...ce.pathMap };
      delete newPathMap[resolved.label];
      if (Object.keys(newPathMap).length === 0) {
        graphDef.value.conditionalEdges.splice(resolved.ceIdx, 1);
      } else {
        ce.pathMap = newPathMap;
      }
      emit('updateGraph');
    }
  } else {
    const edgeIdx = graphDef.value.edges.findIndex(
      (_, i) => `e-${graphDef.value.edges[i].from}-${graphDef.value.edges[i].to}` === edgeId,
    );
    if (edgeIdx >= 0) {
      const edge = { ...graphDef.value.edges[edgeIdx] };
      graphDef.value.edges.splice(edgeIdx, 1);
      if (props.undoRedo) {
        props.undoRedo.pushDeleteEdge(edge, edgeIdx);
      } else {
        emit('updateGraph');
      }
    }
  }
}

function onEdgeMenuSelect(action: string) {
  edgeMenuVisible.value = false;
  const edgeId = edgeMenuEdgeId.value;
  if (!edgeId) return;
  if (action === 'deleteEdge') {
    deleteEdgeById(edgeId);
  }
}

function onEdgeMenuClose() {
  edgeMenuVisible.value = false;
}

function onPaneMenuSelect(action: string) {
  paneMenuVisible.value = false;
  switch (action) {
    case 'deleteSelected':
      deleteSelectedNodes();
      break;
    case 'selectAll':
      for (const node of getNodes.value) {
        node.selected = true;
      }
      break;
    case 'autoLayout':
      emit('requestAutoLayout');
      break;
  }
}

function onSearchInput(q: string) {
  searchQuery.value = q;
  searchMatchIndex.value = 0;
  if (searchMatches.value.length > 0) {
    emit('selectNode', searchMatches.value[0]);
  }
}

function onSearchPrev() {
  if (searchMatches.value.length === 0) return;
  searchMatchIndex.value = (searchMatchIndex.value - 1 + searchMatches.value.length) % searchMatches.value.length;
  emit('selectNode', searchMatches.value[searchMatchIndex.value]);
}

function onSearchNext() {
  if (searchMatches.value.length === 0) return;
  searchMatchIndex.value = (searchMatchIndex.value + 1) % searchMatches.value.length;
  emit('selectNode', searchMatches.value[searchMatchIndex.value]);
}

function onSearchClose() {
  searchVisible.value = false;
  searchQuery.value = '';
}

function duplicateNode(nodeId: string) {
  if (readOnly.value) return;
  const src = graphDef.value.nodes.find((n) => n.id === nodeId);
  if (!src) return;
  const newId = `${src.type}_${Date.now()}`;
  const baseDesc = src.description || src.id;
  const dup: NodeDef = { ...src, id: newId, description: `${baseDesc}${t('graphs.canvasDuplicateNodeSuffix')}` };
  const index = graphDef.value.nodes.length;

  const srcNode = (internalNodes.value as SnapGuideNode[]).find((n) => n.id === nodeId);
  const pos = srcNode ? { x: srcNode.position.x + 40, y: srcNode.position.y + 40 } : { x: 100, y: 100 };
  writeGraphNodePosition(graphDef.value, newId, pos);

  if (props.undoRedo) {
    // execute() 会通过 redo() 完成首次插入，此处禁止预改 graphDef
    props.undoRedo.pushDuplicateNode(nodeId, dup, index);
  } else {
    graphDef.value.nodes.push(dup);
    emit('updateGraph');
  }
  emit('selectNode', newId);
}

function deleteNode(nodeId: string) {
  if (readOnly.value) return;
  const nodeIdx = graphDef.value.nodes.findIndex((n) => n.id === nodeId);
  if (nodeIdx < 0) return;
  const nodeLabel = graphDef.value.nodes[nodeIdx].description || nodeId;
  const connectedEdges = graphDef.value.edges.filter((e) => e.from === nodeId || e.to === nodeId);
  const connectedCondEdges = graphDef.value.conditionalEdges.filter(
    (e) => e.from === nodeId || Object.values(e.pathMap ?? {}).includes(nodeId),
  );
  const totalEdges = connectedEdges.length + connectedCondEdges.length;
  const edgeHint = totalEdges > 0 ? t('graphs.canvasDeleteNodeEdgeHint', { count: totalEdges }) : '';
  $q.dialog({
    title: t('graphs.canvasDeleteNodeTitle'),
    message: t('graphs.canvasDeleteNodeConfirm', { name: nodeLabel, edgeHint }),
    cancel: true,
    persistent: true,
  }).onOk(() => {
    const idx = graphDef.value.nodes.findIndex((n) => n.id === nodeId);
    if (idx < 0) return;
    const node = { ...graphDef.value.nodes[idx] };
    const edges = graphDef.value.edges.filter((e) => e.from === nodeId || e.to === nodeId);
    const condEdges = graphDef.value.conditionalEdges.filter(
      (e) => e.from === nodeId || Object.values(e.pathMap ?? {}).includes(nodeId),
    );
    graphDef.value.nodes.splice(idx, 1);
    graphDef.value.edges = graphDef.value.edges.filter((e) => e.from !== nodeId && e.to !== nodeId);
    graphDef.value.conditionalEdges = graphDef.value.conditionalEdges.filter(
      (e) => e.from !== nodeId && !Object.values(e.pathMap ?? {}).includes(nodeId),
    );
    if (props.undoRedo) {
      props.undoRedo.pushDeleteNode(node, idx, edges, condEdges);
    } else {
      emit('updateGraph');
    }
  });
}

function disconnectNode(nodeId: string) {
  if (readOnly.value) return;
  const edges = graphDef.value.edges.filter((e) => e.from === nodeId || e.to === nodeId);
  const condEdges = graphDef.value.conditionalEdges.filter(
    (e) => e.from === nodeId || Object.values(e.pathMap ?? {}).includes(nodeId),
  );
  graphDef.value.edges = graphDef.value.edges.filter((e) => e.from !== nodeId && e.to !== nodeId);
  graphDef.value.conditionalEdges = graphDef.value.conditionalEdges.filter(
    (e) => e.from !== nodeId && !Object.values(e.pathMap ?? {}).includes(nodeId),
  );
  if (props.undoRedo) {
    props.undoRedo.pushDisconnectNode(nodeId, edges, condEdges);
  } else {
    emit('updateGraph');
  }
}

function deleteSelectedNodes() {
  if (readOnly.value) return;
  const selected = getSelectedNodes.value;
  if (selected.length === 0) return;
  if (selected.length === 1) {
    deleteNode(selected[0].id);
    return;
  }
  const ids = selected.map((n) => n.id);
  $q.dialog({
    title: t('graphs.canvasBatchDeleteTitle'),
    message: t('graphs.canvasBatchDeleteConfirm', { count: ids.length }),
    cancel: true,
    persistent: true,
  }).onOk(() => {
    const deleted: { node: NodeDef; index: number; edges: EdgeDef[]; condEdges: ConditionalEdgeDef[] }[] = [];
    for (const id of ids) {
      const idx = graphDef.value.nodes.findIndex((n) => n.id === id);
      if (idx >= 0) {
        deleted.push({
          node: { ...graphDef.value.nodes[idx] },
          index: idx,
          edges: graphDef.value.edges.filter((e) => e.from === id || e.to === id),
          condEdges: graphDef.value.conditionalEdges.filter(
            (e) => e.from === id || Object.values(e.pathMap ?? {}).includes(id),
          ),
        });
      }
    }
    const idSet = new Set(ids);
    graphDef.value.nodes = graphDef.value.nodes.filter((n) => !idSet.has(n.id));
    graphDef.value.edges = graphDef.value.edges.filter((e) => !idSet.has(e.from) && !idSet.has(e.to));
    graphDef.value.conditionalEdges = graphDef.value.conditionalEdges.filter(
      (e) => !idSet.has(e.from) && !Object.values(e.pathMap ?? {}).some((v) => idSet.has(v)),
    );
    if (props.undoRedo) {
      props.undoRedo.pushDeleteNodes(deleted);
    } else {
      emit('updateGraph');
    }
    emit('selectNode', null);
  });
}

function onConnect(connection: Connection) {
  if (readOnly.value) return;
  if (connection.source && connection.target) {
    // P0-3：连接验证 — 自连接/重复边/字段不匹配
    const validation = isValidConnectionQuick(
      connection.source,
      connection.sourceHandle ?? null,
      connection.target,
      connection.targetHandle ?? null,
      graphDef.value.edges,
    );
    if (!validation.valid) {
      $q.notify({ type: 'negative', message: validation.reason ?? t('graphs.canvasInvalidConnection'), timeout: 3000 });
      return;
    }
    if (validation.warning) {
      $q.notify({ type: 'warning', message: validation.warning, timeout: 4000 });
    }

    const edge: EdgeDef = { from: connection.source, to: connection.target, kind: '' };
    if (props.undoRedo) {
      // execute() 会通过 redo() 完成首次插入，此处禁止预改 graphDef
      props.undoRedo.pushAddEdge(edge);
    } else {
      graphDef.value.edges.push(edge);
      emit('updateGraph');
    }
  }
}

function onConnectStart(params: { nodeId?: string }) {
  connectingFrom.value = params.nodeId ?? null;
}

function onConnectEnd() {
  connectingFrom.value = null;
}

function resolveConditionalEdgeRemoval(edgeId: string): { ceIdx: number; label: string } | null {
  if (!edgeId.startsWith('ce-')) return null;
  for (let ceIdx = 0; ceIdx < graphDef.value.conditionalEdges.length; ceIdx++) {
    const ce = graphDef.value.conditionalEdges[ceIdx];
    for (const [label, target] of Object.entries(ce.pathMap ?? {})) {
      if (`ce-${ce.from}-${target}-${label}` === edgeId) {
        return { ceIdx, label };
      }
    }
  }
  return null;
}

function onEdgesChange(changes: EdgeChange[]) {
  if (syncingFromProp || readOnly.value) return;
  for (const change of changes) {
    if (change.type === 'remove') {
      deleteEdgeById(change.id);
    }
  }
}

function onEdgeUpdate({ edge, connection }: EdgeUpdateEvent) {
  if (readOnly.value || !edge) return;
  const edgeIdx = graphDef.value.edges.findIndex((e) => e.from === edge.source && e.to === edge.target);
  if (edgeIdx < 0) return;
  const oldFrom = graphDef.value.edges[edgeIdx].from;
  const oldTo = graphDef.value.edges[edgeIdx].to;
  const newFrom = connection.source;
  const newTo = connection.target;
  if (oldFrom === newFrom && oldTo === newTo) return;
  graphDef.value.edges[edgeIdx].from = newFrom;
  graphDef.value.edges[edgeIdx].to = newTo;
  if (props.undoRedo) {
    props.undoRedo.pushReconnectEdge(edgeIdx, oldFrom, oldTo, newFrom, newTo);
  } else {
    emit('updateGraph');
  }
}

function onNodesChange(changes: NodeChange[]) {
  if (syncingFromProp || readOnly.value) return;
  const removeIds = changes
    .filter((c) => c.type === 'remove')
    .map((c) => c.id)
    .filter(Boolean) as string[];
  if (removeIds.length === 0) return;
  if (removeIds.length === 1) {
    deleteNode(removeIds[0]);
  } else {
    deleteSelectedNodes();
  }
}

function onDragOver(event: DragEvent) {
  event.preventDefault();
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'move';
  }
}

function onDrop(event: DragEvent) {
  if (readOnly.value) return;
  const type = event.dataTransfer?.getData('application/graph-node-type') as NodeType | undefined;
  if (!type || !NODE_TYPE_STYLES[type]) return;

  const id = `${type}_${Date.now()}`;
  const newNode: NodeDef = {
    id,
    funcRef: '',
    interruptBefore: false,
    interruptAfter: false,
    type,
    description: '',
    instruction: '',
    modelName: '',
    toolNames: [],
    agentName: '',
    destinations: [],
    requiredRole: '',
    assignmentMode: 'static',
    assignmentStrategy: '',
    reviewerAgent: '',
    reviewRules: '',
    timeoutSeconds: 0,
    heartbeatIntervalSeconds: 0,
    enableLeaseExtension: false,
    retryMaxAttempts: 0,
    failureAction: '',
    fallbackAgent: '',
    inputMapperJson: '',
    outputMapperJson: '',
    isolatedMessages: false,
    inputFromLastResponse: false,
    cacheEnabled: false,
    cacheTtlSeconds: 0,
  };

  const dropPosition = project({ x: event.clientX, y: event.clientY });

  const index = graphDef.value.nodes.length;

  writeGraphNodePosition(graphDef.value, id, dropPosition);

  if (props.undoRedo) {
    // execute() 会通过 redo() 完成首次插入，此处禁止预改 graphDef
    props.undoRedo.pushAddNode(newNode, index);
  } else {
    graphDef.value.nodes.push(newNode);
    emit('updateGraph');
  }
  emit('selectNode', id);
}

function onNodeDragStart({ node }: { node: Node }) {
  if (readOnly.value) return;
  dragStartPositions.clear();
  const selected = getSelectedNodes.value;
  const tracked = selected.length > 1 ? selected : [node];
  for (const n of tracked) {
    dragStartPositions.set(n.id, { ...n.position });
  }
}

function onNodeDrag() {
  if (readOnly.value) return;
  const ids = new Set(dragStartPositions.keys());
  if (ids.size > 0) {
    computeSnapLines(ids);
  }
}

function onNodeDragStop({ node }: { node: Node }) {
  if (readOnly.value || !node.id) return;
  const ids = new Set(dragStartPositions.keys());
  // 拖拽结束时计算最终吸附修正
  let snapDelta = { x: 0, y: 0 };
  if (ids.size > 0) {
    const result = computeSnapLines(ids);
    snapDelta = result.delta;
  }
  const moves: { nodeId: string; oldPos: { x: number; y: number }; newPos: { x: number; y: number } }[] = [];
  for (const [id, oldPos] of dragStartPositions) {
    const vfNode = (internalNodes.value as SnapGuideNode[]).find((n) => n.id === id);
    if (!vfNode) continue;
    // 应用吸附修正
    const newPos = {
      x: vfNode.position.x + snapDelta.x,
      y: vfNode.position.y + snapDelta.y,
    };
    vfNode.position.x = newPos.x;
    vfNode.position.y = newPos.y;
    if (oldPos.x !== newPos.x || oldPos.y !== newPos.y) {
      moves.push({ nodeId: id, oldPos, newPos });
    }
  }
  dragStartPositions.clear();
  clearSnapLines();
  if (moves.length === 0) return;
  if (props.undoRedo) {
    props.undoRedo.pushMoveNodes(moves);
  } else {
    for (const move of moves) {
      writeGraphNodePosition(graphDef.value, move.nodeId, move.newPos);
    }
    emit('updateGraph');
  }
}

function isEditableTarget(el: EventTarget | null): boolean {
  if (!el || !(el instanceof HTMLElement)) return false;
  const tag = el.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
  if (el.isContentEditable) return true;
  return false;
}

function onCanvasKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    ctxMenuVisible.value = false;
    return;
  }
  if (isEditableTarget(e.target)) return;
  if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
    e.preventDefault();
    searchVisible.value = !searchVisible.value;
    return;
  }
  if (!readOnly.value && e.key === 'Delete') {
    const selectedNodes = getSelectedNodes.value;
    if (selectedNodes.length > 0) {
      if (selectedNodes.length === 1) {
        deleteNode(selectedNodes[0].id);
      } else {
        deleteSelectedNodes();
      }
      return;
    }
    // 删除选中的边
    const selectedEdges = (internalEdges.value as Array<Edge & { selected?: boolean }>).filter((edge) => edge.selected);
    if (selectedEdges.length > 0) {
      for (const edge of selectedEdges) {
        deleteEdgeById(edge.id);
      }
      return;
    }
    return;
  }
  if (readOnly.value) return;
  if (e.key === 'Enter') {
    const selected = getSelectedNodes.value;
    if (selected.length === 1) {
      emit('selectNode', selected[0].id);
    }
    return;
  }
  if (e.key === 'd' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault();
    const selected = getSelectedNodes.value;
    if (selected.length === 1) {
      duplicateNode(selected[0].id);
    }
    return;
  }
  if (e.key === 'a' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault();
    for (const node of getNodes.value) {
      node.selected = true;
    }
    return;
  }
}

onMounted(() => {
  document.addEventListener('keydown', onCanvasKeydown, true);
});

onUnmounted(() => {
  document.removeEventListener('keydown', onCanvasKeydown, true);
});
</script>

<style lang="sass" scoped>
.graph-editor-canvas
  position: relative
  width: 100%
  height: 100%

  &__zoom-indicator
    position: absolute
    bottom: 20px
    left: 50%
    transform: translateX(-50%)
    display: flex
    align-items: center
    gap: 8px
    padding: 6px 12px
    border-radius: 20px
    background: var(--glass-surface)
    backdrop-filter: blur(var(--glass-blur-soft))
    border: 1px solid var(--glass-border)
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2)
    z-index: 10

  &__zoom-text
    font-size: 12px
    font-weight: 500
    color: var(--color-text-primary)
    cursor: pointer
    user-select: none
    min-width: 40px
    text-align: center

    &:hover
      color: var(--color-accent)
</style>
