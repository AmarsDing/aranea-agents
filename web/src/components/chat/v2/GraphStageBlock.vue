<!-- web/src/components/chat/v2/GraphStageBlock.vue
  GraphStage 流程图可视化（v2 实体，与 PlanBoard 一对一关联）。
  替代 v1 GraphStageBlock.vue（通过 activity.bridge 桥接到 v2）。
  设计：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.7.5
  - 使用 store.getGraphStageNodes 查询辅助（从独立 Map，反映最新状态）
  - GraphStage.Status 派生自子 nodes 状态（后端只发 created，不发 updated/completed/failed）
  - CSS 改用 glass tokens 符合主题
-->
<template>
  <div v-if="graphStage" class="graph-stage-block" :data-graph-stage-id="graphStage.ID">
    <div class="graph-stage-header">
      <span class="header-label">
        <q-icon name="account_tree" size="16px" class="header-icon" />
        {{ t('chat.v2.graphStageTitle') }}
      </span>
      <q-badge :color="stageStatusColor">{{ stageStatusLabel }}</q-badge>
    </div>
    <svg v-if="nodes.length > 0" :width="width" :height="height" class="graph-stage-svg">
      <!-- Dependency edges -->
      <line
        v-for="edge in edges"
        :key="`${edge.from}-${edge.to}`"
        :x1="edge.x1"
        :y1="edge.y1"
        :x2="edge.x2"
        :y2="edge.y2"
        :class="[
          'graph-edge',
          {
            'graph-edge--highlighted': highlightedEdgeKeys.has(`${edge.from}-${edge.to}`),
            'graph-edge--dimmed': hoveredNodeId !== null && !highlightedEdgeKeys.has(`${edge.from}-${edge.to}`),
          },
        ]"
        marker-end="url(#graph-arrowhead)"
      />
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
      <defs>
        <marker id="graph-arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
          <polygon points="0 0, 10 3.5, 0 7" class="graph-arrowhead" />
        </marker>
      </defs>
    </svg>
    <div v-else class="graph-stage-empty">{{ t('chat.v2.graphStageEmpty') }}</div>
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

const nodeWidth = 130;
const nodeHeight = 60;
const gapX = 40;
const gapY = 30;
const maxWidth = 600;

// 不再使用 props.graphStage.Nodes 嵌入数组（创建后永不更新）。
const nodes = computed(() => store.getGraphStageNodes(props.graphStage.ID));

const { layoutDAG } = usePlanDAGLayout();
const layoutResult = computed(() => layoutDAG(nodes.value, { width: maxWidth, nodeWidth, nodeHeight, gapX, gapY }));
const positions = computed(() => layoutResult.value.positions);
const width = computed(() => layoutResult.value.computedWidth);
const height = computed(() => {
  const max = Math.max(0, ...Array.from(positions.value.values()).map((p) => p.y));
  return max + nodeHeight + 20;
});

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
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}
const edges = computed<Edge[]>(() => {
  const out: Edge[] = [];
  for (const node of nodes.value) {
    const toPos = positions.value.get(node.ID);
    if (!toPos) continue;
    if (!node.DependsOn) continue;
    for (const depId of node.DependsOn) {
      const fromPos = positions.value.get(depId);
      if (!fromPos) continue;
      out.push({
        from: depId,
        to: node.ID,
        x1: fromPos.x + nodeWidth / 2,
        y1: fromPos.y + nodeHeight,
        x2: toPos.x + nodeWidth / 2,
        y2: toPos.y,
      });
    }
  }
  return out;
});

// 后端只发 graph_stage.created（Status=running），不发 updated/completed/failed，
// 所以容器状态必须从子 node 状态聚合得到。
const derivedStatus = computed<GraphStageStatus>(() => {
  if (nodes.value.length === 0) return props.graphStage.Status || 'running';
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
  border: 1px solid var(--glass-border)
  border-radius: 8px
  padding: 12px
  margin: 8px 0
  background: var(--glass-surface)

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

.graph-stage-svg
  display: block
  max-width: 100%
  background: var(--glass-elevated, rgba(255, 255, 255, 0.08))
  border-radius: 6px
  border: 1px solid var(--glass-border)

.graph-stage-empty
  padding: 16px
  text-align: center
  color: var(--color-text-secondary)
  font-size: 12px

.graph-edge
  stroke: var(--color-text-secondary, rgba(150, 150, 150, 0.6))
  stroke-width: 2
  transition: opacity 0.2s ease, stroke-width 0.2s ease

/* P1 #6: hover 节点时高亮上下游依赖路径 — 高亮路径上的边 */
.graph-edge--highlighted
  stroke: var(--q-primary, #00bcd4)
  stroke-width: 3

/* P1 #6: 非路径上的边暗化 */
.graph-edge--dimmed
  opacity: 0.2

.graph-arrowhead
  fill: var(--color-text-secondary, rgba(150, 150, 150, 0.6))
</style>
