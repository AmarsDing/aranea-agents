<!-- web/src/components/chat/v2/GraphNode.vue
  2026-07-04 补齐：GraphStage 中的单个节点，对应一个 PlanStep。
  节点状态由 PlanStep.Status 通过 MapPlanStepToGraphNodeStatus 映射得到。
  节点状态色严格遵循设计文档 §3.7.5：
    pending → 灰色
    running → 主题强调色（日间暖金 / 夜间霓虹青）+ 脉冲动画
    completed → 绿色 + ✓
    failed → 红色 + ✗ + 抖动
    interrupted → 黄色 + ⏸
  2026-07-04 修复：
  - CSS 变量替代硬编码 hex，符合主题（日间不用霓虹青违规）
  - 状态文本走 i18n
-->
<template>
  <g
    :transform="`translate(${pos.x}, ${pos.y})`"
    :class="['graph-node', { 'graph-node--failed': node.Status === 'failed' }]"
    @click="$emit('select', node.ID)"
  >
    <rect
      :width="nodeWidth"
      :height="nodeHeight"
      rx="8"
      :class="['graph-node__rect', `graph-node__rect--${node.Status}`]"
      :stroke="nodeStrokeColor"
      :stroke-width="isSelected ? 2.5 : 1.5"
    />
    <text :x="nodeWidth / 2" :y="24" text-anchor="middle" class="graph-node__label">
      {{ node.Label }}
    </text>
    <text :x="nodeWidth / 2" :y="44" text-anchor="middle" class="graph-node__status">
      {{ statusIcon }} {{ statusLabel }}
    </text>
  </g>
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
}>();

defineEmits<{ select: [id: string] }>();

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

// 2026-07-04 修复：状态文本走 i18n
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

// 2026-07-04 问题 4 修复：节点边框色按状态区分，提高可读性
const nodeStrokeColor = computed(
  () =>
    ({
      pending: 'var(--color-text-secondary, #9e9e9e)',
      running: 'var(--q-primary, #00bcd4)',
      completed: 'var(--color-success, #4caf50)',
      failed: 'var(--color-danger, #f44336)',
      interrupted: 'var(--color-warning, #ffc107)',
    })[props.node.Status] || 'var(--glass-border)',
);
</script>

<style scoped>
.graph-node {
  cursor: pointer;
}
.graph-node__rect {
  transition: fill 0.2s ease, stroke 0.2s ease;
}
/* 2026-07-04 问题 4 修复：节点填充用半透明色，让文字在背景上可读。
   之前用纯色填充，文字和背景对比度不足。现在用 rgba 半透明覆盖，
   文字用高对比度的 primary 色，确保日间/夜间都可读。 */
.graph-node__rect--pending {
  fill: rgba(158, 158, 158, 0.15);
}
.graph-node__rect--running {
  fill: rgba(0, 188, 212, 0.18);
  animation: graph-node-pulse 2s ease-in-out infinite;
}
.graph-node__rect--completed {
  fill: rgba(76, 175, 80, 0.18);
}
.graph-node__rect--failed {
  fill: rgba(244, 67, 54, 0.18);
}
.graph-node__rect--interrupted {
  fill: rgba(255, 193, 7, 0.18);
}
@keyframes graph-node-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.75;
  }
}
/* failed 抖动 */
.graph-node--failed {
  animation: graph-node-shake 0.4s ease-in-out 2;
}
@keyframes graph-node-shake {
  0%,
  100% {
    transform: translateX(0);
  }
  25% {
    transform: translateX(-2px);
  }
  75% {
    transform: translateX(2px);
  }
}
/* 2026-07-04 问题 4 修复：文字用高对比度颜色，确保在半透明背景上可读 */
.graph-node__label {
  fill: var(--color-text-primary, #fff);
  font-size: 12px;
  font-weight: 600;
  /* 文字描边，在任意背景上都可读 */
  paint-order: stroke;
  stroke: rgba(0, 0, 0, 0.4);
  stroke-width: 2px;
  stroke-linejoin: round;
}
.graph-node__status {
  fill: var(--color-text-primary, #fff);
  font-size: 11px;
  font-weight: 500;
  paint-order: stroke;
  stroke: rgba(0, 0, 0, 0.4);
  stroke-width: 2px;
  stroke-linejoin: round;
}
</style>
