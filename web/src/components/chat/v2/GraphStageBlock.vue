<!-- web/src/components/chat/v2/GraphStageBlock.vue
  2026-07-04 补齐：GraphStage 流程图可视化（v2 实体，与 PlanBoard 一对一关联）。
  替代 v1 GraphStageBlock.vue（通过 activity.bridge 桥接到 v2）。
  设计：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.7.5
-->
<template>
  <div v-if="graphStage" class="graph-stage-block">
    <div class="graph-stage-header">
      <span class="header-label">
        <q-icon name="account_tree" size="16px" class="header-icon" />
        {{ t('chat.v2.graphStageTitle') }}
      </span>
      <q-badge :color="stageStatusColor">{{ graphStage.Status }}</q-badge>
    </div>
    <svg v-if="graphStage.Nodes.length > 0" :width="width" :height="height" class="graph-stage-svg">
      <!-- Dependency edges -->
      <line
        v-for="edge in edges"
        :key="`${edge.from}-${edge.to}`"
        :x1="edge.x1"
        :y1="edge.y1"
        :x2="edge.x2"
        :y2="edge.y2"
        stroke="#bbb"
        stroke-width="1.5"
        marker-end="url(#graph-arrowhead)"
      />
      <!-- Nodes -->
      <GraphNode
        v-for="node in graphStage.Nodes"
        :key="node.ID"
        :node="node"
        :pos="positions.get(node.ID) || { x: 0, y: 0 }"
        :node-width="nodeWidth"
        :node-height="nodeHeight"
        :is-selected="selectedId === node.ID"
        @select="selectedId = $event"
      />
      <defs>
        <marker id="graph-arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
          <polygon points="0 0, 10 3.5, 0 7" fill="#bbb" />
        </marker>
      </defs>
    </svg>
    <div v-else class="graph-stage-empty">{{ t('chat.v2.graphStageEmpty') }}</div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphStage } from '../../../features/chat/v2Types';
import { usePlanDAGLayout } from '../../../features/chat/composables/usePlanDAGLayout';
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

const nodeWidth = 130;
const nodeHeight = 60;
const gapX = 40;
const gapY = 30;
const width = 600;

const { layoutDAG } = usePlanDAGLayout();
const positions = computed(() => layoutDAG(props.graphStage.Nodes, { width, nodeWidth, nodeHeight, gapX, gapY }));
const height = computed(() => {
  const max = Math.max(0, ...Array.from(positions.value.values()).map((p) => p.y));
  return max + nodeHeight + 20;
});

const selectedId = ref<string | null>(null);

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
  for (const node of props.graphStage.Nodes) {
    const toPos = positions.value.get(node.ID);
    if (!toPos) continue;
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

// GraphStage 状态色（与 PlanBoard 状态色保持一致）
const stageStatusColor = computed(
  () =>
    ({
      running: 'blue',
      completed: 'green',
      failed: 'red',
      interrupted: 'orange-8',
    })[props.graphStage.Status] || 'grey',
);
</script>

<style scoped>
.graph-stage-block {
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 12px;
  margin: 8px 0;
  background: #fafafa;
}
.graph-stage-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 600;
  color: #555;
}
.header-icon {
  margin-right: 4px;
}
.header-label {
  flex: 1;
}
.graph-stage-svg {
  display: block;
  max-width: 100%;
  background: white;
  border-radius: 4px;
}
.graph-stage-empty {
  padding: 16px;
  text-align: center;
  color: #999;
  font-size: 12px;
}
</style>
