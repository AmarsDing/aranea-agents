<!-- web/src/components/chat/v2/PlanStepNode.vue
  PlanDAG 中的单个 SVG 节点。
  节点状态色与图标严格遵循设计文档 §3.7.3：
    pending → 灰色
    running → 主题强调色（日间暖金 / 夜间霓虹青）+ 脉冲动画
    completed → 绿色 + ✓
    failed → 红色 + ✗ + 抖动
    skipped → 暗灰 + —
    partial_failure → 橙色 + ⚠
  2026-07-04 修复：CSS 变量替代硬编码 hex，符合主题
-->
<template>
  <g
    :transform="`translate(${pos.x}, ${pos.y})`"
    :class="['plan-step-node', { 'plan-step-node--failed': step.Status === 'failed' }]"
    @click="$emit('select', step.ID)"
  >
    <rect
      :width="nodeWidth"
      :height="nodeHeight"
      rx="8"
      :class="['plan-step-node__rect', `plan-step-node__rect--${step.Status}`]"
      :stroke="isSelected ? 'var(--q-primary)' : 'var(--glass-border)'"
      :stroke-width="isSelected ? 2 : 1"
    />
    <text :x="nodeWidth / 2" :y="22" text-anchor="middle" class="plan-step-node__label">
      {{ step.Label }}
    </text>
    <text :x="nodeWidth / 2" :y="42" text-anchor="middle" class="plan-step-node__status">
      {{ statusIcon }} {{ statusLabel }}
    </text>
  </g>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PlanStep } from '../../../features/chat/v2Types';
import type { NodePosition } from '../../../features/chat/composables/usePlanDAGLayout';

function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{
  step: PlanStep;
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
      skipped: '—',
      partial_failure: '⚠',
    })[props.step.Status] || '○',
);

// 2026-07-04 修复：状态文本走 i18n
const statusLabel = computed(() => {
  const map: Record<string, string> = {
    pending: t('chat.v2.statusPending'),
    running: t('chat.v2.statusRunning'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    skipped: t('chat.v2.statusSkipped'),
    partial_failure: t('chat.v2.statusPartialFailure'),
  };
  return map[props.step.Status] || props.step.Status;
});
</script>

<style scoped>
.plan-step-node {
  cursor: pointer;
}
.plan-step-node__rect {
  transition: fill 0.2s ease, stroke 0.2s ease;
}
/* 节点状态色用 CSS 变量，日间/夜间自动适配 */
.plan-step-node__rect--pending {
  fill: var(--color-icon-muted, #9e9e9e);
}
.plan-step-node__rect--running {
  fill: var(--color-accent, #00bcd4);
  animation: plan-step-pulse 2s ease-in-out infinite;
}
.plan-step-node__rect--completed {
  fill: var(--color-success, #4caf50);
}
.plan-step-node__rect--failed {
  fill: var(--color-danger, #f44336);
}
.plan-step-node__rect--skipped {
  fill: var(--color-text-secondary, #616161);
}
.plan-step-node__rect--partial_failure {
  fill: var(--color-warning, #ff9800);
}
@keyframes plan-step-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.75;
  }
}
/* failed 抖动 */
.plan-step-node--failed {
  animation: plan-step-shake 0.4s ease-in-out 2;
}
@keyframes plan-step-shake {
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
.plan-step-node__label {
  fill: var(--color-text-primary);
  font-size: 12px;
  font-weight: 600;
}
.plan-step-node__status {
  fill: var(--color-text-secondary);
  font-size: 11px;
}
</style>
