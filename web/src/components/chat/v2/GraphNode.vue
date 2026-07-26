<!-- web/src/components/chat/v2/GraphNode.vue
  GraphStage 中的单个节点，对应一个 PlanStep。
  视觉：showcase mk-fnode 风格卡片（div 绝对定位）— 状态徽章 + 标题 + 状态色边框。
  节点状态由 PlanStep.Status 通过 MapPlanStepToGraphNodeStatus 映射得到。
  节点状态色严格遵循设计文档 §3.7.5：
    pending → 灰色
    running → 主题强调色（日间暖金 / 夜间霓虹青）+ 脉冲动画
    completed → 绿色 + ✓
    failed → 红色 + ✗ + 抖动
    interrupted → 黄色 + ⏸
  - CSS 变量替代硬编码 hex，符合主题（日间不用霓虹青违规）
  - 状态文本走 i18n
-->
<template>
  <div
    :class="[
      'graph-node',
      `graph-node--${node.Status}`,
      {
        'graph-node--selected': isSelected,
        'graph-node--highlighted': isHighlighted,
        'graph-node--dimmed': isDimmed,
        'graph-node--enter': entranceDelayMs !== undefined,
        'graph-node--just-completed': justCompleted,
      },
    ]"
    :style="nodeStyle"
    @click="$emit('select', node.ID)"
    @mouseenter="$emit('hover', node.ID)"
    @mouseleave="$emit('hover', null)"
  >
    <span class="graph-node__badge">{{ statusIcon }} {{ statusLabel }}</span>
    <span class="graph-node__label" :title="node.Label">{{ node.Label }}</span>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphNode } from '../../../features/chat/v2Types';
import type { NodePosition } from '../../../features/chat/composables/usePlanDAGLayout';

function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{
  node: GraphNode;
  pos: NodePosition;
  nodeWidth: number;
  nodeHeight: number;
  isSelected?: boolean;
  isHighlighted?: boolean;
  isDimmed?: boolean;
  /** 级联入场动画延迟（ms）。undefined = 不播入场动画（replay / 早已存在）。 */
  entranceDelayMs?: number;
}>();

defineEmits<{
  select: [id: string];
  hover: [id: string | null];
}>();

const { t } = useSafeI18n();

// P0 状态转换动画：仅在组件实例存续期间捕获 running → completed 跃迁
// （replay 时挂载即为 completed，prevStatus 初值已是终态，不会误触发）。
const justCompleted = ref(false);
let prevStatus = props.node.Status;
let justCompletedTimer: ReturnType<typeof setTimeout> | undefined;

watch(
  () => props.node.Status,
  (cur) => {
    if (prevStatus === 'running' && cur === 'completed') {
      justCompleted.value = true;
      clearTimeout(justCompletedTimer);
      justCompletedTimer = setTimeout(() => {
        justCompleted.value = false;
      }, 1000);
    }
    prevStatus = cur;
  },
);

const nodeStyle = computed(() => {
  const style: Record<string, string> = {
    left: `${props.pos.x}px`,
    top: `${props.pos.y}px`,
    width: `${props.nodeWidth}px`,
    height: `${props.nodeHeight}px`,
  };
  if (props.entranceDelayMs !== undefined && !justCompleted.value) {
    style.animationDelay = `${props.entranceDelayMs}ms`;
  }
  return style;
});

// 状态图标
const statusIcon = computed(
  () =>
    ({
      pending: '○',
      running: '◐',
      completed: '✓',
      failed: '✗',
      interrupted: '⏸',
    })[props.node.Status] || '○',
);

const statusLabel = computed(() => {
  const map: Record<string, string> = {
    pending: t('chat.v2.statusPending'),
    running: t('chat.v2.statusRunning'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    interrupted: t('chat.v2.statusInterrupted'),
  };
  return map[props.node.Status] || props.node.Status;
});
</script>

<style lang="sass" scoped>
.graph-node
  position: absolute
  display: flex
  flex-direction: column
  justify-content: center
  gap: 4px
  padding: 8px 10px
  border: 1.5px solid var(--node-accent, var(--glass-border))
  border-radius: 10px
  background: var(--glass-elevated)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  box-shadow: 0 2px 10px rgb(0 0 0 / 8%)
  cursor: pointer
  box-sizing: border-box
  overflow: hidden
  transition: opacity 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease

// 状态色（设计文档 §3.7.5）
.graph-node--pending
  --node-accent: var(--color-text-tertiary)

.graph-node--running
  --node-accent: var(--q-primary)
  animation: graph-node-pulse 2s ease-in-out infinite

.graph-node--completed
  --node-accent: var(--color-success)

.graph-node--failed
  --node-accent: var(--color-danger)
  animation: graph-node-shake 0.4s ease-in-out 2

.graph-node--interrupted
  --node-accent: var(--color-warning)

// 状态徽章（showcase mk-fn-tag 风格）
.graph-node__badge
  align-self: flex-start
  max-width: 100%
  padding: 1px 6px
  border: 1px solid color-mix(in srgb, var(--node-accent) 55%, transparent)
  border-radius: 5px
  background: color-mix(in srgb, var(--node-accent) 12%, transparent)
  color: var(--node-accent)
  font-size: 9px
  font-weight: 600
  line-height: 1.4
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

.graph-node__label
  color: var(--color-text-primary)
  font-size: 12px
  font-weight: 600
  line-height: 1.3
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

// 选中/hover 路径高亮：状态色光晕（showcase mk-fnode.sel）
.graph-node--selected,
.graph-node--highlighted
  box-shadow: 0 0 14px color-mix(in srgb, var(--node-accent) 40%, transparent)

.graph-node:hover
  box-shadow: 0 0 16px color-mix(in srgb, var(--node-accent) 45%, transparent)

/* P1 #6: hover 节点时高亮上下游依赖路径 — 暗化非路径节点 */
.graph-node--dimmed
  opacity: 30%

@keyframes graph-node-pulse
  0%,
  100%
    box-shadow: 0 0 6px color-mix(in srgb, var(--node-accent) 25%, transparent)

  50%
    box-shadow: 0 0 16px color-mix(in srgb, var(--node-accent) 55%, transparent)

/* P0 级联入场：scale + fade + 轻微上浮，fill-mode both 保证 delay 期间保持隐藏。
   animation-delay 由父组件按 DAG 层级 stagger 注入（nodeStyle.animationDelay）。 */
.graph-node--enter
  animation: graph-node-enter 0.45s cubic-bezier(0.22, 1, 0.36, 1) both

/* 入场与状态动画共存：enter 受 inline animation-delay 控制先播，
   pulse/shake 同时启动（节点隐藏期间不可见，显现后自然衔接）。 */
.graph-node--enter.graph-node--running
  animation: graph-node-enter 0.45s cubic-bezier(0.22, 1, 0.36, 1) both, graph-node-pulse 2s ease-in-out infinite

.graph-node--enter.graph-node--failed
  animation: graph-node-enter 0.45s cubic-bezier(0.22, 1, 0.36, 1) both, graph-node-shake 0.4s ease-in-out 2

/* P0 状态转换：running → completed 瞬间的绿色呼吸 + 徽章回弹。
   class 仅存活 1s（组件内 setTimeout 移除），不持续占用 animation 属性。 */
.graph-node--just-completed
  animation: graph-node-complete-flash 0.9s ease-out

.graph-node--just-completed .graph-node__badge
  animation: graph-badge-pop 0.5s cubic-bezier(0.34, 1.56, 0.64, 1)

@keyframes graph-node-complete-flash
  0%
    box-shadow: 0 0 24px color-mix(in srgb, var(--color-success) 65%, transparent)

  100%
    box-shadow: 0 2px 10px rgb(0 0 0 / 8%)

@keyframes graph-badge-pop
  0%
    transform: scale(0.3)

  100%
    transform: scale(1)

@keyframes graph-node-enter
  from
    opacity: 0
    transform: scale(0.6) translateY(8px)

  to
    opacity: 1
    transform: scale(1) translateY(0)

/* failed 抖动 */
@keyframes graph-node-shake
  0%,
  100%
    transform: translateX(0)

  25%
    transform: translateX(-2px)

  75%
    transform: translateX(2px)

@media (prefers-reduced-motion: reduce)
  .graph-node--running,
  .graph-node--failed,
  .graph-node--enter,
  .graph-node--just-completed,
  .graph-node--just-completed .graph-node__badge
    animation: none
</style>
