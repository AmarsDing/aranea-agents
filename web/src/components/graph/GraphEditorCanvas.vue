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
      @node-click="onNodeClick"
      @pane-click="onPaneClick"
      @connect="onConnect"
      @nodes-change="onNodesChange"
      @edges-change="onEdgesChange"
    >
      <Background :gap="16" />
      <Controls />
      <MiniMap />
    </VueFlow>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from "vue";
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

const props = defineProps<{
  graphDef: GraphDefinition;
  isDark: boolean;
  execNodeStates: Map<string, { status: string }>;
}>();

const emit = defineEmits<{
  selectNode: [nodeId: string | null];
  updateGraph: [];
}>();

const { project, addNodes, addEdges, removeNodes, getNodes } = useVueFlow();

const nodeTypes: Record<string, any> = {
  function: GraphFlowNode,
  llm: GraphFlowNode,
  tool: GraphFlowNode,
  agent: GraphFlowNode,
  router: GraphFlowDiamond,
  join: GraphFlowDiamond,
};

const defaultEdgeOptions = {
  type: "smoothstep",
  animated: false,
  style: { stroke: "#757575", strokeWidth: 2 },
};

const internalNodes = ref<Node[]>([]);
const internalEdges = ref<Edge[]>([]);

let syncingFromProp = false;

function buildNodes(): Node[] {
  const existingPositions = new Map<string, { x: number; y: number }>();
  for (const n of internalNodes.value) {
    existingPositions.set(n.id, n.position);
  }

  return props.graphDef.nodes.map((n) => {
    const style = NODE_TYPE_STYLES[n.type as NodeType] ?? NODE_TYPE_STYLES.function;
    const isDiamond = n.type === "router" || n.type === "join";
    const execState = props.execNodeStates.get(n.id);
    const pos = existingPositions.get(n.id) ?? { x: 100 + Math.random() * 300, y: 100 + Math.random() * 300 };
    return {
      id: n.id,
      type: n.type,
      position: pos,
      data: {
        nodeId: n.id,
        nodeType: n.type as NodeType,
        label: n.id,
        instruction: n.instruction,
        execStatus: execState?.status,
      },
      style: isDiamond
        ? {}
        : {
            background: style.fillColor,
            borderColor: style.borderColor,
          },
    };
  });
}

function buildEdges(): Edge[] {
  const edges: Edge[] = [];
  for (const e of props.graphDef.edges) {
    edges.push({
      id: `e-${e.from}-${e.to}`,
      source: e.from,
      target: e.to,
      type: "smoothstep",
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
        style: { strokeDasharray: "5 5" },
      });
    }
  }
  return edges;
}

watch(
  () => [props.graphDef.nodes, props.graphDef.edges, props.graphDef.conditionalEdges, props.execNodeStates],
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

function onNodeClick({ node }: { node: Node }) {
  emit("selectNode", node.id);
}

function onPaneClick() {
  emit("selectNode", null);
}

function onConnect(connection: Connection) {
  if (connection.source && connection.target) {
    const existing = props.graphDef.edges.find((e) => e.from === connection.source && e.to === connection.target);
    if (!existing) {
      props.graphDef.edges.push({ from: connection.source, to: connection.target });
      emit("updateGraph");
    }
  }
}

function onEdgesChange(changes: EdgeChange[]) {
  if (syncingFromProp) return;
  for (const change of changes) {
    if (change.type === "remove") {
      const edgeId = change.id;
      if (edgeId.startsWith("ce-")) {
        const parts = edgeId.replace("ce-", "").split("-");
        if (parts.length >= 3) {
          const from = parts[0];
          const target = parts[1];
          const label = parts.slice(2).join("-");
          const ceIdx = props.graphDef.conditionalEdges.findIndex((ce) => {
            return ce.from === from && ce.pathMap?.[label] === target;
          });
          if (ceIdx >= 0) {
            const ce = props.graphDef.conditionalEdges[ceIdx];
            const newPathMap = { ...ce.pathMap };
            delete newPathMap[label];
            if (Object.keys(newPathMap).length === 0) {
              props.graphDef.conditionalEdges.splice(ceIdx, 1);
            } else {
              ce.pathMap = newPathMap;
            }
            emit("updateGraph");
          }
        }
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
  if (syncingFromProp) return;
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
</script>

<style scoped>
.graph-editor-canvas {
  flex: 1;
  height: 100%;
  background: var(--canvas-base, #fefbf4);
}

.graph-editor-canvas.is-dark {
  background: var(--canvas-base, #090d14);
}

.graph-editor-canvas :deep(.vue-flow) {
  width: 100%;
  height: 100%;
}

.graph-editor-canvas.is-dark :deep(.vue-flow__background) {
  background: #090d14;
}

.graph-editor-canvas.is-dark :deep(.vue-flow__minimap) {
  background: rgba(18, 24, 34, 0.8);
  border-color: rgba(255, 255, 255, 0.08);
}

.graph-editor-canvas.is-dark :deep(.vue-flow__controls) {
  background: rgba(18, 24, 34, 0.8);
  border-color: rgba(255, 255, 255, 0.08);
}

.graph-editor-canvas.is-dark :deep(.vue-flow__controls-button) {
  background: rgba(30, 41, 59, 0.8);
  border-color: rgba(255, 255, 255, 0.08);
  fill: #94a3b8;
}
</style>
