<!-- web/src/components/chat/v2/GraphStageBlock.vue
  GraphStage 流程图可视化（方案A 重写 2026-07-26）：
  - GraphTeamNode 富卡片节点（标题+状态 / 成员行 / 进度条），单节点也始终渲染
  - 视口缩放/平移（useGraphViewport）：按钮/滚轮缩放、左键拖拽平移、初始自适应
  - 成员行点击 → MemberSessionDialog 弹框展示对话内容（替代原 TeamStagePanel 折叠展开）
  - 节点状态由 PlanStep.Status 映射；容器 Status 终态优先后端、运行中由子节点聚合
-->
<template>
  <div class="graph-stage-block" :data-graph-stage-id="graphStage.ID">
    <div class="graph-stage-header">
      <span class="header-label">
        <q-icon name="account_tree" size="16px" class="header-icon" />
        {{ t('chat.v2.graphStageTitle') }}
      </span>
      <span class="header-progress">{{ completedCount }}/{{ nodes.length }}</span>
      <q-badge :color="stageStatusColor">{{ stageStatusLabel }}</q-badge>
      <div class="graph-viewport-controls">
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="remove"
          data-testid="zoom-out"
          :aria-label="t('chat.v2.zoomOut')"
          @click="zoomOutCenter"
        />
        <span class="graph-viewport-controls__scale">{{ scalePct }}%</span>
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="add"
          data-testid="zoom-in"
          :aria-label="t('chat.v2.zoomIn')"
          @click="zoomInCenter"
        />
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="fit_screen"
          data-testid="zoom-fit"
          :aria-label="t('chat.v2.zoomFit')"
          @click="fitView"
        />
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="restart_alt"
          data-testid="zoom-reset"
          :aria-label="t('chat.v2.zoomReset')"
          @click="reset"
        />
      </div>
    </div>
    <div
      ref="viewportRef"
      class="graph-stage-viewport"
      :class="{ 'graph-stage-viewport--panning': isPanning }"
      @wheel="onWheel"
      @pointerdown="onPanStart"
      @pointermove="handlePanMove"
      @pointerup="onPanEnd"
      @pointerleave="onPanEnd"
    >
      <div
        class="graph-stage-canvas__inner"
        :style="[transformStyle, { width: `${width}px`, height: `${height}px` }]"
      >
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
                'graph-edge--enter': entranceNodeIds.has(edge.to),
              },
            ]"
            :style="
              entranceNodeIds.has(edge.to)
                ? { animationDelay: `${entranceDelayOf(edge.to) + NODE_ENTER_MS}ms` }
                : undefined
            "
          />
        </svg>
        <!-- Rich team nodes -->
        <GraphTeamNode
          v-for="node in nodes"
          :key="node.ID"
          :node="node"
          :pos="positions.get(node.ID) || { x: 0, y: 0 }"
          :is-selected="selectedId === node.ID"
          :is-highlighted="highlightedNodeIds.has(node.ID)"
          :is-dimmed="hoveredNodeId !== null && !highlightedNodeIds.has(node.ID)"
          :entrance-delay-ms="entranceNodeIds.has(node.ID) ? entranceDelayOf(node.ID) : undefined"
          @select="onSelectNode"
          @hover="onHoverNode"
          @select-member="onSelectMember"
        />
      </div>
    </div>
    <MemberSessionDialog
      v-model:open="memberDialogOpen"
      :member-session="activeMember"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @expand="(ids) => $emit('expand', ids)"
      @confirm-step="(p) => $emit('confirm-step', p)"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import { useGraphNodeTeam } from '../../../features/chat/composables/useGraphNodeTeam';
import {
  useGraphViewport,
  GRAPH_VIEWPORT_ZOOM_STEP,
} from '../../../features/chat/composables/useGraphViewport';
import type { GraphStage, GraphStageStatus, MemberSession } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload } from '../../../features/chat/types';
import { usePlanDAGLayout } from '../../../features/chat/composables/usePlanDAGLayout';
import GraphTeamNode from './GraphTeamNode.vue';
import MemberSessionDialog from './MemberSessionDialog.vue';
import { GTN_WIDTH, graphTeamNodeHeight } from './graphTeamNodeUi';

function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ graphStage: GraphStage }>();
defineEmits<{
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
}>();

const { t } = useSafeI18n();
const store = useActivityQueries();
const { membersOf } = useGraphNodeTeam();

// ── 布局：横向 DAG + 富卡片 per-node 高度（成员数决定卡片高度） ──
const gapX = 64;
const gapY = 16;
const padX = 20;
const padY = 12;

// 不再使用 props.graphStage.Nodes 嵌入数组（创建后永不更新）。
const nodes = computed(() => store.getGraphStageNodes(props.graphStage.ID));

const completedCount = computed(() => nodes.value.filter((n) => n.Status === 'completed').length);

function heightOfNode(nodeId: string): number {
  const n = nodes.value.find((x) => x.ID === nodeId);
  return n ? graphTeamNodeHeight(membersOf(n).length) : graphTeamNodeHeight(0);
}

const { layoutDAG } = usePlanDAGLayout();
const layoutResult = computed(() =>
  layoutDAG(nodes.value, {
    width: 0,
    nodeWidth: GTN_WIDTH,
    nodeHeight: graphTeamNodeHeight(0),
    gapX,
    gapY,
    padX,
    padY,
    orientation: 'horizontal',
    heightOf: heightOfNode,
  }),
);
const positions = computed(() => layoutResult.value.positions);
const width = computed(() => layoutResult.value.computedWidth);
const height = computed(() => layoutResult.value.computedHeight);

// ── 视口：缩放/平移 ──
const viewport = useGraphViewport();
const { scale, isPanning, justPanned, transformStyle, onWheel, onPanStart, onPanMove, onPanEnd, reset } = viewport;

const viewportRef = ref<HTMLElement | null>(null);

const scalePct = computed(() => Math.round(scale.value * 100));

function viewportCenter(): { mx: number; my: number } {
  const el = viewportRef.value;
  if (!el) return { mx: 0, my: 0 };
  return { mx: el.clientWidth / 2, my: el.clientHeight / 2 };
}

function zoomInCenter() {
  const { mx, my } = viewportCenter();
  viewport.zoomAt(mx, my, scale.value * GRAPH_VIEWPORT_ZOOM_STEP);
}

function zoomOutCenter() {
  const { mx, my } = viewportCenter();
  viewport.zoomAt(mx, my, scale.value / GRAPH_VIEWPORT_ZOOM_STEP);
}

function fitView() {
  const el = viewportRef.value;
  if (!el) return;
  viewport.zoomFit(width.value, height.value, el.clientWidth, el.clientHeight);
}

// 初始自适应一次：挂载时若节点已就绪直接 fit；否则等首批节点到达后 fit。
let fitted = false;
function fitOnce() {
  if (fitted) return;
  fitted = true;
  void nextTick(() => fitView());
}
onMounted(fitOnce);
watch(
  () => nodes.value.length,
  (len) => {
    if (len > 0) fitOnce();
  },
);

// 指针捕获延迟到确认拖拽（位移超阈值）后才启用：
// pointerdown 即捕获会把后续 pointerup 重定向到视口，click 落在公共祖先上，
// 导致成员行/节点头部的 @click 在真实浏览器中永不触发（弹框打不开）。
function handlePanMove(e: PointerEvent) {
  const wasPan = justPanned.value;
  onPanMove(e);
  if (!wasPan && justPanned.value) {
    // 捕获后拖出视口边界仍能连续平移
    (e.currentTarget as HTMLElement | null)?.setPointerCapture?.(e.pointerId);
  }
}

// ── P0 级联入场动画 ──
const LIVE_WINDOW_MS = 60_000;
const LAYER_STAGGER_MS = 150;
const NODE_STAGGER_MS = 80;
// 边淡入相对目标节点入场的衔接延迟（节点先出现、边随后连接）。
const NODE_ENTER_MS = 250;

const isLiveEntrance = computed(() => {
  const started = Date.parse(props.graphStage.StartedAt);
  if (Number.isNaN(started)) return false;
  return Date.now() - started < LIVE_WINDOW_MS;
});

const seenNodeIds = new Set<string>();
const entranceNodeIds = ref<Set<string>>(new Set());
let firstSnapshot = true;

watch(
  nodes,
  (ns) => {
    const animateNew = !firstSnapshot || isLiveEntrance.value;
    for (const n of ns) {
      if (seenNodeIds.has(n.ID)) continue;
      seenNodeIds.add(n.ID);
      if (animateNew) entranceNodeIds.value.add(n.ID);
    }
    firstSnapshot = false;
  },
  { immediate: true },
);

function entranceDelayOf(nodeId: string): number {
  const layer = layoutResult.value.layers.get(nodeId) ?? 0;
  const order = layoutResult.value.orderInLayer.get(nodeId) ?? 0;
  return layer * LAYER_STAGGER_MS + order * NODE_STAGGER_MS;
}

// ── 选中 / hover 路径高亮 ──
const selectedId = ref<string | null>(null);
const hoveredNodeId = ref<string | null>(null);

function onHoverNode(nodeId: string | null) {
  hoveredNodeId.value = nodeId;
}

// 计算上下游依赖路径节点集合（hoveredNodeId 的所有上游 + 下游 + 自身）
const highlightedNodeIds = computed<Set<string>>(() => {
  const id = hoveredNodeId.value;
  if (!id) return new Set();
  const result = new Set<string>([id]);
  const nodeMap = new Map(nodes.value.map((n) => [n.ID, n]));
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

// ── 节点选中 / 成员弹框（拖拽 pan 后的一次 click 被抑制，避免误触） ──
function onSelectNode(nodeId: string) {
  if (justPanned.value) return;
  selectedId.value = nodeId;
}

const memberDialogOpen = ref(false);
const activeMemberId = ref<string | null>(null);

// 实时查询：store 以新对象替换方式更新 memberSession（activityV2Store upsert），
// 点击时存快照会导致弹框中 Status/canInject 过期（停止/输入栏显示错误、状态不流转）。
const activeMember = computed<MemberSession | null>(() => {
  const id = activeMemberId.value;
  return id ? (store.memberSessions().get(id) ?? null) : null;
});

function onSelectMember(ms: MemberSession) {
  if (justPanned.value) return;
  activeMemberId.value = ms.ID;
  memberDialogOpen.value = true;
}

// ── 依赖边：贝塞尔曲线（源右缘中点 → 目标左缘中点，per-node 高度） ──
interface Edge {
  from: string;
  to: string;
  d: string;
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
      const x1 = fromPos.x + GTN_WIDTH;
      const y1 = fromPos.y + heightOfNode(depId) / 2;
      const x2 = toPos.x;
      const y2 = toPos.y + heightOfNode(node.ID) / 2;
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

// ── 容器状态：终态优先后端，运行中由子节点聚合 ──
function isTerminalStatus(s: GraphStageStatus | string | undefined): boolean {
  return s === 'completed' || s === 'failed' || s === 'interrupted';
}

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

// 视口控制条：缩放按钮 + 当前比例
.graph-viewport-controls
  display: flex
  align-items: center
  gap: 2px
  margin-left: 8px
  padding: 0 4px
  border: 1px solid var(--glass-border)
  border-radius: 8px
  background: var(--glass-surface)

  &__scale
    min-width: 40px
    text-align: center
    font-size: 11px
    font-weight: 500
    color: var(--color-text-secondary)
    font-variant-numeric: tabular-nums

// 视口：内容超出可拖拽平移，滚轮缩放
.graph-stage-viewport
  position: relative
  overflow: hidden
  max-width: 100%
  min-height: 120px
  border: 1px solid var(--glass-border)
  border-radius: 14px
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  cursor: grab
  touch-action: none

  &--panning
    cursor: grabbing

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

/* P0 级联入场：边在目标节点出现后淡入连接 */
.graph-edge--enter
  opacity: 0
  animation: graph-edge-enter 0.5s ease-out both

@keyframes graph-edge-enter
  from
    opacity: 0
    stroke-width: 0.5

  to
    opacity: 1
    stroke-width: 1.8

/* hover 节点时高亮上下游依赖路径 */
.graph-edge--highlighted
  stroke: var(--q-primary, #00bcd4)
  stroke-width: 2.4

.graph-edge--dimmed
  opacity: 0.2

@media (prefers-reduced-motion: reduce)
  .graph-edge--flowing
    animation: none

  .graph-edge--enter
    animation: none
    opacity: 1
</style>
