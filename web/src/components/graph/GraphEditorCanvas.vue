<template>
  <div
    :class="['graph-editor-canvas', { 'is-dark': isDark }]"
    @dragover.prevent="onDragOver"
    @drop="onDrop"
  >
    <VueFlow
      v-model:nodes="internalNodes"
      v-model:edges="internalEdges"
      :node-types="nodeTypes"
      :edge-types="edgeTypes"
      :default-edge-options="defaultEdgeOptions"
      :connection-line-style="connectionLineStyle"
      :fit-view-on-init="true"
      :snap-to-grid="true"
      :snap-grid="[16, 16]"
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
      @pane-contextmenu="onPaneContextMenu"
      @node-contextmenu="onNodeContextMenu"
      @connect="onConnect"
      @connect-start="onConnectStart"
      @connect-end="onConnectEnd"
      @nodes-change="onNodesChange"
      @edges-change="onEdgesChange"
      @edge-update="onEdgeUpdate"
      @node-drag-start="onNodeDragStart"
      @node-drag="onNodeDrag"
      @node-drag-stop="onNodeDragStop"
    >
      <Background :gap="16" />
      <Controls />
      <MiniMap :node-color="miniMapNodeColor" />
      <template #connection-line="{ }" />
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
        <q-tooltip>缩小</q-tooltip>
      </q-btn>
      <span class="graph-editor-canvas__zoom-text" @click="zoomToFit">{{ zoomLabel }}</span>
      <q-btn flat dense round icon="add" size="xs" @click="zoomIn">
        <q-tooltip>放大</q-tooltip>
      </q-btn>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, computed, onMounted, onUnmounted } from "vue";
import { useQuasar } from "quasar";
import { VueFlow, useVueFlow, type Connection, type Edge, type Node, type NodeChange, type EdgeChange, type EdgeUpdateEvent, Position, SelectionMode } from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import { Controls } from "@vue-flow/controls";
import { MiniMap } from "@vue-flow/minimap";
import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
import "@vue-flow/controls/dist/style.css";
import "@vue-flow/minimap/dist/style.css";
import GraphFlowNode from "./GraphFlowNode.vue";
import GraphFlowDiamond from "./GraphFlowDiamond.vue";
import GraphFlowEdge from "./GraphFlowEdge.vue";
import GraphContextMenu from "./GraphContextMenu.vue";
import type { ContextMenuItem } from "./GraphContextMenu.vue";
import GraphNodeSearch from "./GraphNodeSearch.vue";
import type { NodeDef, EdgeDef, ConditionalEdgeDef, NodeType, GraphDefinition } from "../../features/graph/types";
import { NODE_TYPE_STYLES, NODE_DEFAULT_WIDTH, NODE_DEFAULT_HEIGHT } from "../../features/graph/types";
import type { useGraphUndoRedo } from "../../features/graph/useGraphUndoRedo";
import { defaultNodePosition, readGraphLayout, writeGraphNodePosition } from "../../features/graph/editor/graphLayout";
import { useSnapGuide } from "../../features/graph/useSnapGuide";
import type { SnapLine } from "../../features/graph/useSnapGuide";
import { graphNodeDisplayLabel } from "../../features/orchestration/teamNodeDisplay";

const props = defineProps<{
  graphDef: GraphDefinition;
  isDark: boolean;
  execNodeStates?: Map<string, {
    status: string;
    fineStatus?: string;
    inputPreview?: string;
    outputPreview?: string;
    currentActivity?: string;
  }>;
  selectedNodeId?: string | null;
  /** When true, pans/zooms to selected node (Observatory focus sync). */
  focusSelectedNode?: boolean;
  /** Run/monitor mode: disable editing gestures. */
  readOnly?: boolean;
  undoRedo?: ReturnType<typeof useGraphUndoRedo>;
}>();

const EMPTY_EXEC_NODE_STATES: Map<string, {
  status: string;
  fineStatus?: string;
  inputPreview?: string;
  outputPreview?: string;
  currentActivity?: string;
}> = new Map();

const resolvedExecNodeStates = computed(() => props.execNodeStates ?? EMPTY_EXEC_NODE_STATES);

const emit = defineEmits<{
  selectNode: [nodeId: string | null];
  updateGraph: [];
  requestAutoLayout: [];
  focusPropertyPanel: [nodeId: string];
}>();

const { project, fitView, getSelectedNodes, onViewportChange, zoomTo, getNodes } = useVueFlow();
const internalNodes = ref<Node[]>([]);
const internalEdges = ref<Edge[]>([]);
const { snapLines, computeSnapLines, clearSnapLines } = useSnapGuide(internalNodes);
const $q = useQuasar();

const ctxMenuVisible = ref(false);
const ctxMenuX = ref(0);
const ctxMenuY = ref(0);
const ctxMenuNodeId = ref<string | null>(null);
const paneMenuVisible = ref(false);
const paneMenuX = ref(0);
const paneMenuY = ref(0);

const searchVisible = ref(false);
const searchQuery = ref("");
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

const nodeTypes: Record<string, any> = {
  function: GraphFlowNode,
  llm: GraphFlowNode,
  tool: GraphFlowNode,
  agent: GraphFlowNode,
  hitl: GraphFlowNode,
  router: GraphFlowDiamond,
  join: GraphFlowDiamond,
};

const edgeTypes: Record<string, any> = {
  flowEdge: GraphFlowEdge,
};

const readOnly = computed(() => props.readOnly ?? false);

const defaultEdgeOptions = {
  type: "flowEdge",
  animated: false,
  style: { stroke: "var(--graph-edge-normal)", strokeWidth: 1 },
};

const connectionLineStyle = {
  stroke: "var(--graph-edge-normal)",
  strokeWidth: 1.5,
  strokeDasharray: "6 4",
};

function miniMapNodeColor(node: Node): string {
  const execState = resolvedExecNodeStates.value.get(node.id);
  if (execState) {
    switch (execState.status) {
      case "running": return "var(--graph-status-running)";
      case "completed": return "var(--graph-status-completed)";
      case "failed":
      case "error": return "var(--graph-status-failed)";
      case "interrupted": return "var(--graph-status-interrupted)";
    }
  }
  const style = NODE_TYPE_STYLES[node.type as NodeType];
  return style?.borderColor ?? "var(--color-accent)";
}

function edgeKindLabel(kind?: string): string | undefined {
  switch ((kind ?? "").toLowerCase()) {
    case "transfer":
      return "移交";
    case "dispatch":
      return "分派";
    case "flow":
      return undefined;
    default:
      return kind?.trim() || undefined;
  }
}

let syncingFromProp = false;

function buildNodes(): Node[] {
  const existingPositions = new Map<string, { x: number; y: number }>();
  for (const n of internalNodes.value) {
    existingPositions.set(n.id, n.position);
  }
  const savedLayout = readGraphLayout(props.graphDef);

  return props.graphDef.nodes.map((n, index) => {
    const style = NODE_TYPE_STYLES[n.type as NodeType] ?? NODE_TYPE_STYLES.function;
    const isDiamond = n.type === "router" || n.type === "join";
    const execState = resolvedExecNodeStates.value.get(n.id);
    const pos =
      existingPositions.get(n.id) ??
      savedLayout[n.id] ??
      defaultNodePosition(index);
    return {
      id: n.id,
      type: n.type,
      position: pos,
      selected: n.id === props.selectedNodeId,
      data: {
        nodeId: n.id,
        nodeType: n.type as NodeType,
        label: graphNodeDisplayLabel(n),
        agentName: n.agentName,
        role: n.requiredRole,
        description: n.description,
        instruction: n.instruction || n.description,
        execStatus: execState?.status,
        fineStatus: execState?.fineStatus,
        inputPreview: execState?.inputPreview,
        outputPreview: execState?.outputPreview,
        currentActivity: execState?.currentActivity,
        toolNames: n.toolNames,
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
  for (const e of props.graphDef.edges) {
    const isTransfer = (e.kind ?? "").toLowerCase() === "transfer";
    const isDispatch = (e.kind ?? "").toLowerCase() === "dispatch";
    const edgeClass = isTransfer ? "graph-edge--transfer" : isDispatch ? "graph-edge--dispatch" : "";
    edges.push({
      id: `e-${e.from}-${e.to}`,
      source: e.from,
      target: e.to,
      type: "flowEdge",
      animated: isTransfer,
      class: edgeClass,
      data: { edgeClass },
      style: { stroke: "var(--graph-edge-normal)", strokeWidth: 1 },
      label: edgeKindLabel(e.kind) ?? (isTransfer ? "移交" : isDispatch ? "分派" : undefined),
      labelStyle: { fill: "var(--graph-ctx-text)", fontSize: 10, fontWeight: 600 },
      labelBgStyle: { fill: "var(--graph-ctx-bg)", fillOpacity: 0.9, stroke: "var(--graph-ctx-border)", strokeWidth: 0.5 },
      labelBgPadding: [6, 4],
      labelBgBorderRadius: 6,
    });
  }
  for (const ce of props.graphDef.conditionalEdges) {
    const pathMap = ce.pathMap ?? {};
    for (const [label, target] of Object.entries(pathMap)) {
      edges.push({
        id: `ce-${ce.from}-${target}-${label}`,
        source: ce.from,
        target,
        type: "flowEdge",
        class: "graph-edge--conditional",
        data: { edgeClass: "graph-edge--conditional" },
        label,
        labelStyle: { fill: "var(--graph-edge-conditional)", fontSize: 10, fontWeight: 600 },
        labelBgStyle: { fill: "var(--graph-ctx-bg)", fillOpacity: 0.9, stroke: "var(--graph-cond-edge-label-stroke)", strokeWidth: 0.5 },
        labelBgPadding: [6, 4],
        labelBgBorderRadius: 6,
        style: { stroke: "var(--graph-edge-conditional)", strokeWidth: 1 },
      });
    }
  }
  return edges;
}

function execNodeStatesFingerprint(
  map: Map<string, { status: string; fineStatus?: string; inputPreview?: string; outputPreview?: string; currentActivity?: string }>,
): string {
  if (map.size === 0) return "";
  return [...map.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([id, st]) => `${id}:${st.status}:${st.fineStatus ?? ""}:${st.currentActivity ?? ""}`)
    .join("|");
}

let lastNodeSig = "";
let lastEdgeSig = "";
let lastCondSig = "";
let lastLayoutSig = "";
let lastExecFp = "";

watch(
  () => props.graphDef.nodes.map((n) => n.id).join("\0"),
  (sig) => {
    if (sig === lastNodeSig) return;
    lastNodeSig = sig;
    rebuildAll();
  },
  { immediate: false },
);

watch(
  () => props.graphDef.edges.map((e) => `${e.from}->${e.to}:${e.kind ?? ""}`).join("\0"),
  (sig) => {
    if (sig === lastEdgeSig) return;
    lastEdgeSig = sig;
    rebuildAll();
  },
  { immediate: false },
);

watch(
  () => props.graphDef.conditionalEdges
    .map((ce) => `${ce.from}:${Object.keys(ce.pathMap ?? {}).sort().join(",")}`)
    .join("\0"),
  (sig) => {
    if (sig === lastCondSig) return;
    lastCondSig = sig;
    rebuildAll();
  },
  { immediate: false },
);

watch(
  () => JSON.stringify(readGraphLayout(props.graphDef)),
  (sig) => {
    if (sig === lastLayoutSig) return;
    lastLayoutSig = sig;
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

function rebuildAll() {
  syncingFromProp = true;
  internalNodes.value = buildNodes();
  internalEdges.value = buildEdges();
  nextTick(() => {
    syncingFromProp = false;
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
  }
);

function onNodeClick({ node }: { node: Node }) {
  emit("selectNode", node.id);
}

function onPaneClick() {
  emit("selectNode", null);
  ctxMenuVisible.value = false;
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
    items.push({ icon: "⊞", label: "自动布局", action: "autoLayout" });
  }
  items.push({ icon: "▣", label: "全选节点", shortcut: "Ctrl+A", action: "selectAll" });
  if (count > 1 && !readOnly.value) {
    items.push({ icon: "✕", label: `删除选中 ${count} 个节点`, shortcut: "Del", danger: true, action: "deleteSelected" });
  }
  return items;
});

const ctxMenuItems = computed<ContextMenuItem[]>(() => {
  const items: ContextMenuItem[] = [
    { icon: "✎", label: "查看属性", shortcut: "Enter", action: "edit" },
  ];
  if (!readOnly.value) {
    items.push(
      { icon: "⧉", label: "复制节点", shortcut: "Ctrl+D", action: "duplicate" },
      { icon: "✕", label: "删除节点", shortcut: "Del", danger: true, action: "delete" },
      { icon: "⟂", label: "断开所有连线", action: "disconnect" },
      { icon: "▷", label: "设为入口节点", success: true, action: "setEntry" },
      { icon: "◻", label: "设为结束节点", danger: true, action: "setFinish" },
    );
  }
  return items;
});

const searchMatches = computed(() => {
  if (!searchQuery.value.trim()) return [];
  const q = searchQuery.value.trim().toLowerCase();
  return props.graphDef.nodes.filter(
    (n) =>
      n.id.toLowerCase().includes(q) ||
      (n.description ?? "").toLowerCase().includes(q) ||
      (n.instruction ?? "").toLowerCase().includes(q) ||
      (n.agentName ?? "").toLowerCase().includes(q),
  ).map((n) => n.id);
});

const searchMatchCount = computed(() => searchMatches.value.length);

function onNodeContextMenu({ event, node }: { event: MouseEvent; node: Node }) {
  event.preventDefault();
  paneMenuVisible.value = false;
  ctxMenuNodeId.value = node.id;
  ctxMenuX.value = event.clientX;
  ctxMenuY.value = event.clientY;
  ctxMenuVisible.value = true;
  emit("selectNode", node.id);
}

function onCtxMenuSelect(action: string) {
  const nodeId = ctxMenuNodeId.value;
  if (!nodeId) return;
  ctxMenuVisible.value = false;

  switch (action) {
    case "edit":
      emit("focusPropertyPanel", nodeId);
      break;
    case "duplicate":
      duplicateNode(nodeId);
      break;
    case "delete":
      deleteNode(nodeId);
      break;
    case "disconnect":
      disconnectNode(nodeId);
      break;
    case "setEntry":
      if (props.undoRedo) {
        props.undoRedo.pushSetGraphProperty("entryPoint", props.graphDef.entryPoint, nodeId);
      } else {
        props.graphDef.entryPoint = nodeId;
        emit("updateGraph");
      }
      break;
    case "setFinish":
      if (props.undoRedo) {
        props.undoRedo.pushSetGraphProperty("finishPoint", props.graphDef.finishPoint, nodeId);
      } else {
        props.graphDef.finishPoint = nodeId;
        emit("updateGraph");
      }
      break;
  }
}

function onCtxMenuClose() {
  ctxMenuVisible.value = false;
}

function onPaneMenuSelect(action: string) {
  paneMenuVisible.value = false;
  switch (action) {
    case "deleteSelected":
      deleteSelectedNodes();
      break;
    case "selectAll":
      for (const node of getNodes.value) {
        node.selected = true;
      }
      break;
    case "autoLayout":
      emit("requestAutoLayout");
      break;
  }
}

function onSearchInput(q: string) {
  searchQuery.value = q;
  searchMatchIndex.value = 0;
  if (searchMatches.value.length > 0) {
    emit("selectNode", searchMatches.value[0]);
  }
}

function onSearchPrev() {
  if (searchMatches.value.length === 0) return;
  searchMatchIndex.value = (searchMatchIndex.value - 1 + searchMatches.value.length) % searchMatches.value.length;
  emit("selectNode", searchMatches.value[searchMatchIndex.value]);
}

function onSearchNext() {
  if (searchMatches.value.length === 0) return;
  searchMatchIndex.value = (searchMatchIndex.value + 1) % searchMatches.value.length;
  emit("selectNode", searchMatches.value[searchMatchIndex.value]);
}

function onSearchClose() {
  searchVisible.value = false;
  searchQuery.value = "";
}

function duplicateNode(nodeId: string) {
  if (readOnly.value) return;
  const src = props.graphDef.nodes.find((n) => n.id === nodeId);
  if (!src) return;
  const newId = `${src.type}_${Date.now()}`;
  const dup: NodeDef = { ...src, id: newId, description: `${src.description || src.id} (副本)` };
  const index = props.graphDef.nodes.length;
  props.graphDef.nodes.push(dup);

  const srcNode = internalNodes.value.find((n) => n.id === nodeId);
  const pos = srcNode ? { x: srcNode.position.x + 40, y: srcNode.position.y + 40 } : { x: 100, y: 100 };
  writeGraphNodePosition(props.graphDef, newId, pos);

  if (props.undoRedo) {
    props.undoRedo.pushDuplicateNode(nodeId, dup, index);
  } else {
    emit("updateGraph");
  }
  emit("selectNode", newId);
}

function deleteNode(nodeId: string) {
  if (readOnly.value) return;
  const nodeIdx = props.graphDef.nodes.findIndex((n) => n.id === nodeId);
  if (nodeIdx < 0) return;
  const nodeLabel = props.graphDef.nodes[nodeIdx].description || nodeId;
  const connectedEdges = props.graphDef.edges.filter((e) => e.from === nodeId || e.to === nodeId);
  const connectedCondEdges = props.graphDef.conditionalEdges.filter((e) => e.from === nodeId || Object.values(e.pathMap ?? {}).includes(nodeId));
  const totalEdges = connectedEdges.length + connectedCondEdges.length;
  const edgeHint = totalEdges > 0 ? `，同时移除 ${totalEdges} 条连线` : "";
  $q.dialog({
    title: "删除节点",
    message: `确定删除节点「${nodeLabel}」${edgeHint}？`,
    cancel: true,
    persistent: true,
  }).onOk(() => {
    const idx = props.graphDef.nodes.findIndex((n) => n.id === nodeId);
    if (idx < 0) return;
    const node = { ...props.graphDef.nodes[idx] };
    const edges = props.graphDef.edges.filter((e) => e.from === nodeId || e.to === nodeId);
    const condEdges = props.graphDef.conditionalEdges.filter((e) => e.from === nodeId || Object.values(e.pathMap ?? {}).includes(nodeId));
    props.graphDef.nodes.splice(idx, 1);
    props.graphDef.edges = props.graphDef.edges.filter((e) => e.from !== nodeId && e.to !== nodeId);
    props.graphDef.conditionalEdges = props.graphDef.conditionalEdges.filter((e) => e.from !== nodeId && !Object.values(e.pathMap ?? {}).includes(nodeId));
    if (props.undoRedo) {
      props.undoRedo.pushDeleteNode(node, idx, edges, condEdges);
    } else {
      emit("updateGraph");
    }
  });
}

function disconnectNode(nodeId: string) {
  if (readOnly.value) return;
  const edges = props.graphDef.edges.filter((e) => e.from === nodeId || e.to === nodeId);
  const condEdges = props.graphDef.conditionalEdges.filter((e) => e.from === nodeId || Object.values(e.pathMap ?? {}).includes(nodeId));
  props.graphDef.edges = props.graphDef.edges.filter((e) => e.from !== nodeId && e.to !== nodeId);
  props.graphDef.conditionalEdges = props.graphDef.conditionalEdges.filter((e) => e.from !== nodeId && !Object.values(e.pathMap ?? {}).includes(nodeId));
  if (props.undoRedo) {
    props.undoRedo.pushDisconnectNode(nodeId, edges, condEdges);
  } else {
    emit("updateGraph");
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
    title: "批量删除节点",
    message: `确定删除选中的 ${ids.length} 个节点？`,
    cancel: true,
    persistent: true,
  }).onOk(() => {
    const deleted: { node: NodeDef; index: number; edges: EdgeDef[]; condEdges: ConditionalEdgeDef[] }[] = [];
    for (const id of ids) {
      const idx = props.graphDef.nodes.findIndex((n) => n.id === id);
      if (idx >= 0) {
        deleted.push({
          node: { ...props.graphDef.nodes[idx] },
          index: idx,
          edges: props.graphDef.edges.filter((e) => e.from === id || e.to === id),
          condEdges: props.graphDef.conditionalEdges.filter((e) => e.from === id || Object.values(e.pathMap ?? {}).includes(id)),
        });
      }
    }
    const idSet = new Set(ids);
    props.graphDef.nodes = props.graphDef.nodes.filter((n) => !idSet.has(n.id));
    props.graphDef.edges = props.graphDef.edges.filter((e) => !idSet.has(e.from) && !idSet.has(e.to));
    props.graphDef.conditionalEdges = props.graphDef.conditionalEdges.filter(
      (e) => !idSet.has(e.from) && !Object.values(e.pathMap ?? {}).some((v) => idSet.has(v)),
    );
    if (props.undoRedo) {
      props.undoRedo.pushDeleteNodes(deleted);
    } else {
      emit("updateGraph");
    }
    emit("selectNode", null);
  });
}

function onConnect(connection: Connection) {
  if (readOnly.value) return;
  if (connection.source && connection.target) {
    const existing = props.graphDef.edges.find((e) => e.from === connection.source && e.to === connection.target);
    if (!existing) {
      const edge: EdgeDef = { from: connection.source, to: connection.target };
      props.graphDef.edges.push(edge);
      if (props.undoRedo) {
        props.undoRedo.pushAddEdge(edge);
      } else {
        emit("updateGraph");
      }
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
  if (!edgeId.startsWith("ce-")) return null;
  for (let ceIdx = 0; ceIdx < props.graphDef.conditionalEdges.length; ceIdx++) {
    const ce = props.graphDef.conditionalEdges[ceIdx];
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
    if (change.type === "remove") {
      const edgeId = change.id;
      const resolved = resolveConditionalEdgeRemoval(edgeId);
      if (resolved) {
        const ce = props.graphDef.conditionalEdges[resolved.ceIdx];
        const oldPathMap = { ...ce.pathMap };
        const newPathMap = { ...ce.pathMap };
        delete newPathMap[resolved.label];
        if (Object.keys(newPathMap).length === 0) {
          props.graphDef.conditionalEdges.splice(resolved.ceIdx, 1);
        } else {
          ce.pathMap = newPathMap;
        }
        if (props.undoRedo) {
          props.undoRedo.pushDeleteConditionalEdge(ce, resolved.ceIdx, resolved.label);
        } else {
          emit("updateGraph");
        }
      } else {
        const edgeIdx = props.graphDef.edges.findIndex((_, i) => `e-${props.graphDef.edges[i].from}-${props.graphDef.edges[i].to}` === edgeId);
        if (edgeIdx >= 0) {
          const edge = { ...props.graphDef.edges[edgeIdx] };
          props.graphDef.edges.splice(edgeIdx, 1);
          if (props.undoRedo) {
            props.undoRedo.pushDeleteEdge(edge, edgeIdx);
          } else {
            emit("updateGraph");
          }
        }
      }
    }
  }
}

function onEdgeUpdate({ edge, connection }: EdgeUpdateEvent) {
  if (readOnly.value || !edge) return;
  const edgeIdx = props.graphDef.edges.findIndex((e) => e.from === edge.source && e.to === edge.target);
  if (edgeIdx < 0) return;
  const oldFrom = props.graphDef.edges[edgeIdx].from;
  const oldTo = props.graphDef.edges[edgeIdx].to;
  const newFrom = connection.source;
  const newTo = connection.target;
  if (oldFrom === newFrom && oldTo === newTo) return;
  props.graphDef.edges[edgeIdx].from = newFrom;
  props.graphDef.edges[edgeIdx].to = newTo;
  if (props.undoRedo) {
    props.undoRedo.pushReconnectEdge(edgeIdx, oldFrom, oldTo, newFrom, newTo);
  } else {
    emit("updateGraph");
  }
}

function onNodesChange(changes: NodeChange[]) {
  if (syncingFromProp || readOnly.value) return;
  const removeIds = changes.filter((c) => c.type === "remove").map((c) => c.id).filter(Boolean) as string[];
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
    event.dataTransfer.dropEffect = "move";
  }
}

function onDrop(event: DragEvent) {
  if (readOnly.value) return;
  const type = event.dataTransfer?.getData("application/graph-node-type") as NodeType | undefined;
  if (!type || !NODE_TYPE_STYLES[type]) return;

  const id = `${type}_${Date.now()}`;
  const newNode: NodeDef = {
    id,
    funcRef: "",
    interruptBefore: false,
    interruptAfter: false,
    type,
    description: "",
    instruction: "",
    modelName: "",
    toolNames: [],
    agentName: "",
    destinations: [],
    requiredRole: "",
    assignmentMode: "static",
    assignmentStrategy: "",
    reviewerAgent: "",
    reviewRules: "",
    timeoutSeconds: 0,
    heartbeatIntervalSeconds: 0,
    enableLeaseExtension: false,
    retryMaxAttempts: 0,
    failureAction: "",
    fallbackAgent: "",
    inputMapperJson: "",
    outputMapperJson: "",
    isolatedMessages: false,
    inputFromLastResponse: false,
    cacheEnabled: false,
    cacheTtlSeconds: 0,
  };

  const dropPosition = project({ x: event.clientX, y: event.clientY });

  const index = props.graphDef.nodes.length;
  props.graphDef.nodes.push(newNode);

  writeGraphNodePosition(props.graphDef, id, dropPosition);

  if (props.undoRedo) {
    props.undoRedo.pushAddNode(newNode, index);
  } else {
    emit("updateGraph");
  }
  emit("selectNode", id);
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
  const moves: { nodeId: string; oldPos: { x: number; y: number }; newPos: { x: number; y: number } }[] = [];
  for (const [id, oldPos] of dragStartPositions) {
    const vfNode = internalNodes.value.find((n) => n.id === id);
    if (!vfNode) continue;
    const newPos = { ...vfNode.position };
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
      writeGraphNodePosition(props.graphDef, move.nodeId, move.newPos);
    }
    emit("updateGraph");
  }
}

function isEditableTarget(el: EventTarget | null): boolean {
  if (!el || !(el instanceof HTMLElement)) return false;
  const tag = el.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  if (el.isContentEditable) return true;
  return false;
}

function onCanvasKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") {
    ctxMenuVisible.value = false;
    return;
  }
  if (isEditableTarget(e.target)) return;
  if ((e.ctrlKey || e.metaKey) && e.key === "f") {
    e.preventDefault();
    searchVisible.value = !searchVisible.value;
    return;
  }
  if (!readOnly.value && e.key === "Delete") {
    const selected = getSelectedNodes.value;
    if (selected.length === 1) {
      deleteNode(selected[0].id);
    } else if (selected.length > 1) {
      deleteSelectedNodes();
    }
    return;
  }
  if (readOnly.value) return;
  if (e.key === "Enter") {
    const selected = getSelectedNodes.value;
    if (selected.length === 1) {
      emit("selectNode", selected[0].id);
    }
    return;
  }
  if (e.key === "d" && (e.ctrlKey || e.metaKey)) {
    e.preventDefault();
    const selected = getSelectedNodes.value;
    if (selected.length === 1) {
      duplicateNode(selected[0].id);
    }
    return;
  }
  if (e.key === "a" && (e.ctrlKey || e.metaKey)) {
    e.preventDefault();
    for (const node of getNodes.value) {
      node.selected = true;
    }
    return;
  }
}

onMounted(() => {
  document.addEventListener("keydown", onCanvasKeydown, true);
});

onUnmounted(() => {
  document.removeEventListener("keydown", onCanvasKeydown, true);
});
</script>
