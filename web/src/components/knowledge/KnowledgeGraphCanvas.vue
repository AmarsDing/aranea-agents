<template>
  <!-- G4 3D 知识图谱画布（V12.7）：3d-force-graph 力导向图。
       左键旋转 / 右键平移 / 滚轮缩放；hover 高亮一跳邻居淡化其余；点击选中联动操作台。 -->
  <div ref="containerEl" class="knowledge-graph-canvas" />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import ForceGraph3D, { type ForceGraph3DInstance } from '3d-force-graph';
import type { NodeObject } from 'three-forcegraph';
import { graphContainmentForce, graphDocTypeColor, graphLinkColor, graphNodeVal, oneHopNeighborIds } from '../../features/knowledge/graphUi';
import type { CollectionGraphEdge, CollectionGraphNode } from '../../features/knowledge/types';

interface GraphNode extends NodeObject {
  id: string;
  name: string;
  docType: string;
  relPath: string;
  degree: number;
}

const props = defineProps<{
  /** 渲染节点（已经过孤立裁剪）。 */
  nodes: CollectionGraphNode[];
  edges: CollectionGraphEdge[];
  /** 选中节点 doc_id（高亮描边色）。 */
  selectedNodeId: string;
  /** 聚焦信号：+1 时相机飞往选中节点。 */
  focusSignal: number;
  /** 数据代际：变化时重置视野（zoomToFit）。 */
  generation: number;
}>();

const emit = defineEmits<{
  'node-click': [docId: string];
  'background-click': [];
}>();

/** hover 高亮集（null = 无 hover，全量正常着色）。 */
let highlightIds: Set<string> | null = null;
/** hover 临时淡化色（非邻居节点/边）。 */
const DIM_COLOR = '#3a4152';
const SELECT_COLOR = '#ffd166';

const containerEl = ref<HTMLElement | null>(null);
let graph: ForceGraph3DInstance<GraphNode> | null = null;
let resizeObserver: ResizeObserver | null = null;

function toGraphData() {
  return {
    nodes: props.nodes.map((n) => ({
      id: n.doc_id,
      name: n.name,
      docType: n.doc_type,
      relPath: n.rel_path,
      degree: n.degree,
    })),
    links: props.edges.map((e) => ({ source: e.source, target: e.target, type: e.type })),
  };
}

function nodeColor(n: GraphNode): string {
  if (n.id === props.selectedNodeId) return SELECT_COLOR;
  if (highlightIds && !highlightIds.has(n.id)) return DIM_COLOR;
  return graphDocTypeColor(n.docType);
}

function linkColor(l: { source: unknown; target: unknown; type?: string }): string {
  if (highlightIds) {
    const s = typeof l.source === 'object' ? (l.source as GraphNode).id : String(l.source);
    const t = typeof l.target === 'object' ? (l.target as GraphNode).id : String(l.target);
    if (!highlightIds.has(s) || !highlightIds.has(t)) return DIM_COLOR;
  }
  return graphLinkColor(l.type ?? '');
}

/** 颜色集变更后触发 three.js 重评估（accessor 重挂 = 官方高亮模式）。 */
function refreshColors() {
  if (!graph) return;
  graph.nodeColor(graph.nodeColor());
  graph.linkColor(graph.linkColor());
}

onMounted(() => {
  const el = containerEl.value;
  if (!el) return;
  graph = new ForceGraph3D(el, { rendererConfig: { alpha: true, antialias: true } })
    .backgroundColor('rgba(0,0,0,0)')
    .showNavInfo(false)
    .nodeId('id')
    .nodeLabel((n: GraphNode) => `${n.name}${n.docType ? ` · ${n.docType}` : ''}`)
    .nodeVal((n: GraphNode) => graphNodeVal(n.degree))
    .nodeColor(nodeColor)
    .linkColor(linkColor)
    .linkDirectionalArrowLength(3.5)
    .linkDirectionalArrowRelPos(1)
    .linkDirectionalArrowColor(linkColor)
    .onNodeHover((node: GraphNode | null) => {
      highlightIds = node ? oneHopNeighborIds(node.id, props.edges) : null;
      el.style.cursor = node ? 'pointer' : '';
      refreshColors();
    })
    .onNodeClick((node: GraphNode) => emit('node-click', node.id))
    .onBackgroundClick(() => emit('background-click'))
    .graphData(toGraphData());

  // 径向 containment：断链/孤立节点不再被电荷斥力无界推散（否则 zoomToFit 框住巨大
  // bbox，相机拉飞 → 画布空白/伪影）。d3Force 挂载即注册到 d3 simulation。
  graph.d3Force('contain', graphContainmentForce() as never);

  resizeObserver = new ResizeObserver(() => {
    if (graph && el.clientWidth > 0 && el.clientHeight > 0) {
      graph.width(el.clientWidth).height(el.clientHeight);
    }
  });
  resizeObserver.observe(el);
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  resizeObserver = null;
  graph?._destructor();
  graph = null;
});

// 数据变化：重灌图数据（力导向布局增量更新）。
watch(
  () => [props.nodes, props.edges],
  () => graph?.graphData(toGraphData()),
);

// 数据代际变化（重新加载）：布局稳定后重置视野。
watch(
  () => props.generation,
  () => {
    window.setTimeout(() => graph?.zoomToFit(800, 40), 600);
  },
);

// 选中变化：仅刷新配色（选中描边色）。
watch(
  () => props.selectedNodeId,
  () => refreshColors(),
);

// 聚焦信号：相机飞往选中节点（节点未渲染/坐标未就绪时跳过）。
watch(
  () => props.focusSignal,
  () => {
    if (!graph || !props.selectedNodeId) return;
    const node = graph.graphData().nodes.find((n) => n.id === props.selectedNodeId);
    if (!node || node.x === undefined || node.y === undefined || node.z === undefined) return;
    const distance = 120;
    const hyp = Math.hypot(node.x, node.y, node.z) || 1;
    const ratio = 1 + distance / hyp;
    graph.cameraPosition(
      { x: node.x * ratio, y: node.y * ratio, z: node.z * ratio },
      { x: node.x, y: node.y, z: node.z },
      1200,
    );
  },
);
</script>

<style lang="scss" scoped>
.knowledge-graph-canvas {
  width: 100%;
  height: 100%;
  min-height: 420px;
  overflow: hidden;
  border-radius: 10px;
}
</style>
