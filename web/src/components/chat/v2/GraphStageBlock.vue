<!-- web/src/components/chat/v2/GraphStageBlock.vue
  GraphStage 流程图可视化（v2 实体，与 PlanBoard 一对一关联）。
  设计：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.7.5
  产品交互：docs/development/1-chat.design.md B.4.4（已对齐 v2：节点=PlanStep，非 team 快照）
  视觉：横向 DAG（对齐 docs/showcase/index.html mk-gcanvas）— 点阵画布 + 贝塞尔曲线边 + 卡片节点
  - store.getGraphStageNodes：独立 Map，反映最新节点状态
  - 容器 Status：终态优先用 props.graphStage.Status；运行中由子节点聚合
  - 单节点（无 DAG 价值）不展示，直接依赖 TeamStagePanel
  - Header 显示 completed/total 进度
-->
<template>
  <div v-if="shouldShow" class="graph-stage-block" :data-graph-stage-id="graphStage.ID">
    <div class="graph-stage-header">
      <span class="header-label">
        <q-icon name="account_tree" size="16px" class="header-icon" />
        {{ t('chat.v2.graphStageTitle') }}
      </span>
      <span class="header-progress">{{ completedCount }}/{{ nodes.length }}</span>
      <q-badge :color="stageStatusColor">{{ stageStatusLabel }}</q-badge>
    </div>
    <div class="graph-stage-canvas">
      <div class="graph-stage-canvas__inner" :style="{ width: `${width}px`, height: `${height}px` }">
        <!-- Dependency edges (curved bezier, showcase DAG style) -->
        <svg class="graph-stage-edges" :width="width" :height="height" :viewBox="`0 0 ${width} ${height}`">
          <path
            v-for="edge in edges"
            :key="`${edge.from}-${edge.to}`"
            :d="edge.d"
            :class="[
              'graph-edge',
              {
                'graph-edge--flowing': derivedStatus === 'running',
                'graph-edge--highlighted': highlightedEdgeKeys.has(`${edge.from}-${edge.to}`),
                'graph-edge--dimmed': hoveredNodeId !== null && !highlightedEdgeKeys.has(`${edge.from}-${edge.to}`),
              },
            ]"
          />
        </svg>
        <!-- Nodes -->
        <GraphNode
          v-for="node in nodes"
          :key="node.ID"
          :node="node"
          :pos="positions.get(node.ID) || { x: 0, y: 0 }"
          :node-width="nodeWidth"
          :node-height="nodeHeight"
          :is-selected="selectedId === node.ID"
          :is-highlighted="highlightedNodeIds.has(node.ID)"
          :is-dimmed="hoveredNodeId !== null && !highlightedNodeIds.has(node.ID)"
          @select="onSelectNode"
          @hover="onHoverNode"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import type { GraphStage, GraphStageStatus, GraphNode as GraphNodeType } from '../../../features/chat/v2Types';
import { usePlanDAGLayout } from '../../../features/chat/composables/usePlanDAGLayout';
import { useLocateTeamStage } from '../../../features/chat/composables/useLocateTeamStage';
import GraphNode from './GraphNode.vue';

// Safe i18n wrapper — falls back to the key when the i18n plugin isn't
// installed (e.g., during unit tests without app.use(i18n)).
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ graphStage: GraphStage }>();
const { t } = useSafeI18n();
const store = useActivityQueries();

const nodeWidth = 156;
const nodeHeight = 56;
const gapX = 64;
const gapY = 16;
const padX = 20;
const padY = 12;

// 不再使用 props.graphStage.Nodes 嵌入数组（创建后永不更新）。
const nodes = computed(() => store.getGraphStageNodes(props.graphStage.ID));

// B.4.4：单节点无 DAG 可视化价值，直接展示 TeamStagePanel 即可。
const shouldShow = computed(() => nodes.value.length > 1);

const completedCount = computed(() => nodes.value.filter((n) => n.Status === 'completed').length);

const { layoutDAG } = usePlanDAGLayout();
const layoutResult = computed(() =>
  layoutDAG(nodes.value, { width: 0, nodeWidth, nodeHeight, gapX, gapY, padX, padY, orientation: 'horizontal' }),
);
const positions = computed(() => layoutResult.value.positions);
const width = computed(() => layoutResult.value.computedWidth);
const height = computed(() => layoutResult.value.computedHeight);

const selectedId = ref<string | null>(null);

// P1 #6: hover 节点 → 高亮所有上下游依赖路径
const hoveredNodeId = ref<string | null>(null);

function onHoverNode(nodeId: string | null) {
  hoveredNodeId.value = nodeId;
}

// 计算上下游依赖路径节点集合（ hoveredNodeId 的所有上游 + 下游 + 自身）
const highlightedNodeIds = computed<Set<string>>(() => {
  const id = hoveredNodeId.value;
  if (!id) return new Set();
  const result = new Set<string>([id]);
  const nodeMap = new Map(nodes.value.map((n) => [n.ID, n]));
  // 上游：递归遍历 DependsOn
  function addUpstream(currentId: string) {
    const node = nodeMap.get(currentId);
    if (!node?.DependsOn) return;
    for (const depId of node.DependsOn) {
      if (!result.has(depId)) {
        result.add(depId);
        addUpstream(depId);
      }
    }
  }
  // 下游：找到所有 DependsOn 包含 currentId 的节点
  function addDownstream(currentId: string) {
    for (const n of nodes.value) {
      if (n.DependsOn?.includes(currentId) && !result.has(n.ID)) {
        result.add(n.ID);
        addDownstream(n.ID);
      }
    }
  }
  addUpstream(id);
  addDownstream(id);
  return result;
});

// 高亮路径上的边（两端节点都在 highlightedNodeIds 中）
const highlightedEdgeKeys = computed<Set<string>>(() => {
  const nodeSet = highlightedNodeIds.value;
  const keys = new Set<string>();
  for (const edge of edges.value) {
    if (nodeSet.has(edge.from) && nodeSet.has(edge.to)) {
      keys.add(`${edge.from}-${edge.to}`);
    }
  }
  return keys;
});

// P1 #5: 点击 GraphNode → 选中高亮 + 跳转到对应 TeamStagePanel。
// 优先使用 node.TeamStageID（后端回填后直接跳转）。
// Fallback：后端未回填 TeamStageID 时，通过 DagNodeID 匹配 TeamStage
// （GraphNode.DagNodeID === TeamStage.DagNodeID === PlanStep.ID）。
const { locate } = useLocateTeamStage();
function onSelectNode(nodeId: string) {
  selectedId.value = nodeId;
  const node: GraphNodeType | undefined = nodes.value.find((n) => n.ID === nodeId);
  if (!node) return;
  let teamStageId = node.TeamStageID;
  if (!teamStageId && node.DagNodeID) {
    for (const ts of store.teamStages().values()) {
      if (ts.DagNodeID === node.DagNodeID) {
        teamStageId = ts.ID;
        break;
      }
    }
  }
  if (teamStageId) {
    locate(teamStageId);
  }
}

interface Edge {
  from: string;
  to: string;
  d: string;
}
// 横向 DAG 贝塞尔曲线边（对齐 showcase mk-edge）：
// 从源节点右缘中点 → 目标节点左缘中点，三次贝塞尔平滑过渡。
const edges = computed<Edge[]>(() => {
  const out: Edge[] = [];
  for (const node of nodes.value) {
    const toPos = positions.value.get(node.ID);
    if (!toPos) continue;
    if (!node.DependsOn) continue;
    for (const depId of node.DependsOn) {
      const fromPos = positions.value.get(depId);
      if (!fromPos) continue;
      const x1 = fromPos.x + nodeWidth;
      const y1 = fromPos.y + nodeHeight / 2;
      const x2 = toPos.x;
      const y2 = toPos.y + nodeHeight / 2;
      const cx = Math.max(32, (x2 - x1) / 2);
      out.push({
        from: depId,
        to: node.ID,
        d: `M ${x1} ${y1} C ${x1 + cx} ${y1}, ${x2 - cx} ${y2}, ${x2} ${y2}`,
      });
    }
  }
  return out;
});

function isTerminalStatus(s: GraphStageStatus | string | undefined): boolean {
  return s === 'completed' || s === 'failed' || s === 'interrupted';
}

// 终态优先用后端 GraphStage.Status（terminal 事件已持久化）；
// 运行中由子节点聚合，避免仅依赖 created 时的过期 Status。
const derivedStatus = computed<GraphStageStatus>(() => {
  const backend = props.graphStage.Status;
  if (isTerminalStatus(backend)) return backend;
  if (nodes.value.length === 0) return backend || 'running';
  const hasFailed = nodes.value.some((n) => n.Status === 'failed');
  const hasInterrupted = nodes.value.some((n) => n.Status === 'interrupted');
  const allCompleted = nodes.value.every((n) => n.Status === 'completed');
  const hasRunning = nodes.value.some((n) => n.Status === 'running');
  if (allCompleted) return 'completed';
  if (hasFailed) return 'failed';
  if (hasInterrupted) return 'interrupted';
  if (hasRunning) return 'running';
  return 'running';
});

const stageStatusColor = computed(
  () =>
    ({
      running: 'blue',
      completed: 'green',
      failed: 'red',
      interrupted: 'yellow-8',
    })[derivedStatus.value] || 'grey',
);

const stageStatusLabel = computed(() => {
  const map: Record<string, string> = {
    running: t('chat.v2.statusRunning'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    interrupted: t('chat.v2.statusInterrupted'),
  };
  return map[derivedStatus.value] || derivedStatus.value;
});
</script>

<style lang="sass" scoped>
.graph-stage-block
  padding: 8px 0
  margin: 8px 0

.graph-stage-header
  display: flex
  align-items: center
  gap: 8px
  margin-bottom: 8px
  font-size: 13px
  font-weight: 600
  color: var(--color-text-primary)

.header-icon
  margin-right: 4px
  color: var(--q-primary)

.header-label
  flex: 1

.header-progress
  font-size: 12px
  font-weight: 500
  color: var(--color-text-secondary)

// 横向 DAG 画布：宽图可横向滚动（层数深时宽度是结构性的）
.graph-stage-canvas
  overflow-x: auto
  max-width: 100%
  border: 1px solid var(--glass-border)
  border-radius: 14px
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)

.graph-stage-canvas__inner
  position: relative
  // 点阵背景（对齐 showcase mk-gcanvas）
  background-image: radial-gradient(circle, color-mix(in srgb, var(--color-text-tertiary) 30%, transparent) 1px, transparent 1px)
  background-size: 22px 22px
  border-radius: 14px

.graph-stage-edges
  position: absolute
  inset: 0

.graph-edge
  fill: none
  stroke: var(--color-text-tertiary, rgba(150, 150, 150, 0.6))
  stroke-width: 1.8
  transition: opacity 0.2s ease, stroke 0.2s ease, stroke-width 0.2s ease

// 运行中：虚线流动动画，暗示执行方向（对齐 showcase edgeDash）
.graph-edge--flowing
  stroke-dasharray: 6 5
  animation: graph-edge-dash 1.4s linear infinite

@keyframes graph-edge-dash
  to
    stroke-dashoffset: -11

/* P1 #6: hover 节点时高亮上下游依赖路径 — 高亮路径上的边 */
.graph-edge--highlighted
  stroke: var(--q-primary, #00bcd4)
  stroke-width: 2.4

/* P1 #6: 非路径上的边暗化 */
.graph-edge--dimmed
  opacity: 0.2

@media (prefers-reduced-motion: reduce)
  .graph-edge--flowing
    animation: none
</style>
