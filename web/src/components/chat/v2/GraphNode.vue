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
      },
    ]"
    :style="{ left: `${pos.x}px`, top: `${pos.y}px`, width: `${nodeWidth}px`, height: `${nodeHeight}px` }"
    @click="$emit('select', node.ID)"
    @mouseenter="$emit('hover', node.ID)"
    @mouseleave="$emit('hover', null)"
  >
    <span class="graph-node__badge">{{ statusIcon }} {{ statusLabel }}</span>
    <span class="graph-node__label" :title="node.Label">{{ node.Label }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
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
}>();

defineEmits<{
  select: [id: string];
  hover: [id: string | null];
}>();

const { t } = useSafeI18n();

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
  .graph-node--failed
    animation: none
</style>
