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
      :default-edge-options="defaultEdgeOptions"
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
      :delete-key-code="readOnly ? null : 'Delete'"
      @node-click="onNodeClick"
      @pane-click="onPaneClick"
      @connect="onConnect"
      @nodes-change="onNodesChange"
      @edges-change="onEdgesChange"
      @node-drag-stop="onNodeDragStop"
    >
      <Background :gap="16" />
      <Controls />
      <MiniMap />
    </VueFlow>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, computed } from "vue";
import { VueFlow, useVueFlow, type Connection, type Edge, type Node, type NodeChange, type EdgeChange, Position } from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import { Controls } from "@vue-flow/controls";
import { MiniMap } from "@vue-flow/minimap";
import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
import "@vue-flow/controls/dist/style.css";
import "@vue-flow/minimap/dist/style.css";
import GraphFlowNode from "./GraphFlowNode.vue";
import GraphFlowDiamond from "./GraphFlowDiamond.vue";
import type { NodeDef, EdgeDef, ConditionalEdgeDef, NodeType, GraphDefinition } from "../../features/graph/types";
import { NODE_TYPE_STYLES } from "../../features/graph/types";
import { defaultNodePosition, readGraphLayout, writeGraphNodePosition } from "../../features/graph/editor/graphLayout";
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
}>();

const emptyExecNodeStates = new Map<string, {
  status: string;
  fineStatus?: string;
  inputPreview?: string;
  outputPreview?: string;
  currentActivity?: string;
}>();
const resolvedExecNodeStates = computed(() => props.execNodeStates ?? emptyExecNodeStates);

const emit = defineEmits<{
  selectNode: [nodeId: string | null];
  updateGraph: [];
}>();

const { project, fitView } = useVueFlow();

const nodeTypes: Record<string, any> = {
  function: GraphFlowNode,
  llm: GraphFlowNode,
  tool: GraphFlowNode,
  agent: GraphFlowNode,
  hitl: GraphFlowNode,
  router: GraphFlowDiamond,
  join: GraphFlowDiamond,
};

const readOnly = computed(() => props.readOnly ?? false);

const defaultEdgeOptions = {
  type: "smoothstep",
  animated: false,
  style: { stroke: "var(--color-icon-muted)", strokeWidth: 2 },
};

const edgeLabelStyle = {
  fill: "var(--color-text-secondary)",
  fontSize: 10,
  fontWeight: 600,
};

const edgeLabelBgStyle = {
  fill: "var(--glass-elevated)",
  fillOpacity: 0.92,
};

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

const internalNodes = ref<Node[]>([]);
const internalEdges = ref<Edge[]>([]);

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
    edges.push({
      id: `e-${e.from}-${e.to}`,
      source: e.from,
      target: e.to,
      type: "smoothstep",
      animated: isTransfer,
      style: isTransfer
        ? { stroke: "var(--color-icon-muted)", strokeWidth: 1.5, strokeDasharray: "6 4" }
        : { stroke: "var(--color-icon-muted)", strokeWidth: 2 },
      label: edgeKindLabel(e.kind) ?? (isTransfer ? "移交" : isDispatch ? "分派" : undefined),
      labelStyle: edgeLabelStyle,
      labelBgStyle: edgeLabelBgStyle,
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
        type: "smoothstep",
        label,
        labelStyle: edgeLabelStyle,
        labelBgStyle: edgeLabelBgStyle,
        labelBgPadding: [6, 4],
        labelBgBorderRadius: 6,
        style: { strokeDasharray: "5 5" },
      });
    }
  }
  return edges;
}

watch(
  () => [props.graphDef.nodes, props.graphDef.edges, props.graphDef.conditionalEdges, resolvedExecNodeStates.value, props.selectedNodeId],
  () => {
    syncingFromProp = true;
    internalNodes.value = buildNodes();
    internalEdges.value = buildEdges();
    nextTick(() => {
      syncingFromProp = false;
    });
  },
  { immediate: true, deep: true }
);

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
}

function onConnect(connection: Connection) {
  if (readOnly.value) return;
  if (connection.source && connection.target) {
    const existing = props.graphDef.edges.find((e) => e.from === connection.source && e.to === connection.target);
    if (!existing) {
      props.graphDef.edges.push({ from: connection.source, to: connection.target });
      emit("updateGraph");
    }
  }
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
        const newPathMap = { ...ce.pathMap };
        delete newPathMap[resolved.label];
        if (Object.keys(newPathMap).length === 0) {
          props.graphDef.conditionalEdges.splice(resolved.ceIdx, 1);
        } else {
          ce.pathMap = newPathMap;
        }
        emit("updateGraph");
      } else {
        const edgeIdx = props.graphDef.edges.findIndex((_, i) => `e-${props.graphDef.edges[i].from}-${props.graphDef.edges[i].to}` === edgeId);
        if (edgeIdx >= 0) {
          props.graphDef.edges.splice(edgeIdx, 1);
          emit("updateGraph");
        }
      }
    }
  }
}

function onNodesChange(changes: NodeChange[]) {
  if (syncingFromProp || readOnly.value) return;
  for (const change of changes) {
    if (change.type === "remove" && change.id) {
      const nodeId = change.id;
      const nodeIdx = props.graphDef.nodes.findIndex((n) => n.id === nodeId);
      if (nodeIdx >= 0) {
        props.graphDef.nodes.splice(nodeIdx, 1);
        props.graphDef.edges = props.graphDef.edges.filter((e) => e.from !== nodeId && e.to !== nodeId);
        props.graphDef.conditionalEdges = props.graphDef.conditionalEdges.filter((e) => e.from !== nodeId && !Object.values(e.pathMap ?? {}).includes(nodeId));
        emit("updateGraph");
      }
    }
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
  if (!type) return;

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

  props.graphDef.nodes.push(newNode);

  internalNodes.value.push({
    id,
    type,
    position: dropPosition,
    data: {
      nodeId: id,
      nodeType: type as NodeType,
      label: id,
      instruction: "",
      execStatus: undefined,
    },
    style: (type === "router" || type === "join")
      ? {}
      : {
          background: NODE_TYPE_STYLES[type].fillColor,
          borderColor: NODE_TYPE_STYLES[type].borderColor,
        },
  });

  emit("updateGraph");
  emit("selectNode", id);
}

function onNodeDragStop({ node }: { node: Node }) {
  if (readOnly.value || !node.id) return;
  writeGraphNodePosition(props.graphDef, node.id, node.position);
  emit("updateGraph");
}
</script>

<style scoped>
.graph-editor-canvas {
  flex: 1;
  height: 100%;
  min-width: 0;
}
</style>
