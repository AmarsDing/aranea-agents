<!-- web/src/components/chat/v2/PlanBoardCard.vue
  2026-07-04 重写：按设计文档 §A.4.1 / §B.4.3 规范
  - 执行计划 = 编号列表（1/2/3/4）+ 状态圆点/打勾，文字为主
  - 不再渲染 DAG 图（DAG 由 GraphStageBlock 负责）
  - PlanBoard.Status 派生自子 steps 状态（后端只发 created，不发 updated）
  - 折叠/展开：进行中默认展开，全部完成自动折叠为摘要
  - CSS 使用 glass tokens 符合主题
-->
<template>
  <div class="plan-board-card" :data-plan-board-id="planBoard.ID">
    <div class="plan-header" @click="toggleCollapse">
      <div class="plan-header__left">
        <q-icon :name="collapsed ? 'expand_more' : 'expand_less'" size="20px" class="plan-header__icon" />
        <span class="plan-header__title">{{ t('chat.v2.planBoardTitle') }}</span>
        <q-badge :color="statusColor" class="plan-header__status">{{ statusLabel }}</q-badge>
      </div>
      <div class="plan-header__summary">
        <span class="plan-summary-text">{{ summaryText }}</span>
      </div>
    </div>
    <div v-show="!collapsed" class="plan-body">
      <ol v-if="steps.length > 0" class="plan-step-list">
        <li
          v-for="(step, idx) in steps"
          :key="step.ID"
          :class="['plan-step-item', `plan-step-item--${step.Status}`]"
        >
          <div class="plan-step-item__index">{{ idx + 1 }}</div>
          <div class="plan-step-item__main">
            <div class="plan-step-item__label">{{ step.Label }}</div>
            <div v-if="step.Description" class="plan-step-item__desc">{{ step.Description }}</div>
          </div>
          <div class="plan-step-item__status">
            <span :class="['plan-step-item__icon', `plan-step-item__icon--${step.Status}`]">
              {{ statusIcon(step.Status) }}
            </span>
            <span class="plan-step-item__status-text">{{ statusText(step.Status) }}</span>
          </div>
        </li>
      </ol>
      <div v-else class="plan-empty">{{ t('chat.v2.graphStageEmpty') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import type { PlanBoard, PlanStepStatus, PlanStatus } from '../../../features/chat/v2Types';

function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ planBoard: PlanBoard }>();
const { t } = useSafeI18n();
const store = useActivityQueries();

// 从 store.getPlanBoardSteps 查询辅助获取最新 steps（独立 Map，反映最新状态）
const steps = computed(() => store.getPlanBoardSteps(props.planBoard.ID));

// 折叠状态：默认展开。用户手动操作后 userToggled=true，不再自动变更。
const collapsed = ref(false);
const userToggled = ref(false);

const isAllCompleted = computed(() => steps.value.length > 0 && steps.value.every((s) => s.Status === 'completed'));

// 初始渲染时若全部完成自动折叠为摘要（需求 §A.4.1）
watch(
  () => props.planBoard.ID,
  () => {
    if (userToggled.value) return;
    collapsed.value = isAllCompleted.value;
  },
  { immediate: true },
);

function toggleCollapse() {
  userToggled.value = true;
  collapsed.value = !collapsed.value;
}

const summaryText = computed(() => {
  const total = steps.value.length;
  const done = steps.value.filter((s) => s.Status === 'completed').length;
  const failed = steps.value.filter((s) => s.Status === 'failed' || s.Status === 'partial_failure').length;
  if (failed > 0) return `${done}/${total} · ${failed} ${t('chat.v2.failedLabel')}`;
  if (done === total) return `✅ ${done}/${total}`;
  return `${done}/${total}`;
});

// PlanBoard.Status 派生自子 steps 状态（后端只发 plan_board.created，Status=planning）
const derivedStatus = computed<PlanStatus>(() => {
  if (steps.value.length === 0) return props.planBoard.Status || 'planning';
  const hasFailed = steps.value.some((s) => s.Status === 'failed');
  const hasPartial = steps.value.some((s) => s.Status === 'partial_failure');
  const allCompleted = steps.value.every((s) => s.Status === 'completed');
  const hasRunning = steps.value.some((s) => s.Status === 'running');
  if (allCompleted) return 'completed';
  if (hasFailed) return hasPartial || hasRunning ? 'partial_failure' : 'failed';
  if (hasPartial) return 'partial_failure';
  if (hasRunning) return 'executing';
  return 'planning';
});

const statusColor = computed(
  () =>
    ({
      planning: 'orange',
      executing: 'blue',
      completed: 'green',
      failed: 'red',
      partial_failure: 'orange-8',
    })[derivedStatus.value] || 'grey',
);

const statusLabel = computed(() => {
  const map: Record<string, string> = {
    planning: t('chat.v2.statusPlanning'),
    executing: t('chat.v2.statusRunning'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    partial_failure: t('chat.v2.statusPartialFailure'),
  };
  return map[derivedStatus.value] || derivedStatus.value;
});

// 状态图标（§B.4.3 视觉表）
function statusIcon(status: PlanStepStatus): string {
  const map: Record<string, string> = {
    pending: '○',
    running: '◐',
    completed: '✓',
    failed: '✗',
    skipped: '—',
    partial_failure: '⚠',
  };
  return map[status] || '○';
}

// 状态文本（i18n）
function statusText(status: PlanStepStatus): string {
  const map: Record<string, string> = {
    pending: t('chat.v2.statusPending'),
    running: t('chat.v2.statusRunning'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    skipped: t('chat.v2.statusSkipped'),
    partial_failure: t('chat.v2.statusPartialFailure'),
  };
  return map[status] || status;
}
</script>

<style lang="sass" scoped>
.plan-board-card
  border: 1px solid var(--glass-border)
  border-radius: 8px
  margin: 8px 0
  background: var(--glass-surface)

.plan-header
  display: flex
  align-items: center
  justify-content: space-between
  padding: 8px 12px
  cursor: pointer
  user-select: none
  border-bottom: 1px solid var(--glass-border)

  &:hover
    background: var(--glass-surface-hover)

  &__left
    display: flex
    align-items: center
    gap: 6px

  &__icon
    color: var(--color-icon-muted)

  &__title
    font-size: 13px
    font-weight: 600
    color: var(--color-text-primary)

  &__status
    margin-left: 4px

  &__summary
    font-size: 12px
    color: var(--color-text-secondary)

.plan-summary-text
  font-variant-numeric: tabular-nums

.plan-body
  padding: 8px 12px

.plan-step-list
  list-style: none
  margin: 0
  padding: 0
  display: flex
  flex-direction: column
  gap: 4px

.plan-step-item
  display: flex
  align-items: flex-start
  gap: 10px
  padding: 6px 8px
  border-radius: 4px
  transition: background 0.15s

  &:hover
    background: var(--glass-surface-hover)

  &__index
    flex-shrink: 0
    width: 22px
    height: 22px
    border-radius: 50%
    background: var(--glass-elevated)
    border: 1px solid var(--glass-border)
    display: flex
    align-items: center
    justify-content: center
    font-size: 11px
    font-weight: 600
    color: var(--color-text-secondary)
    font-variant-numeric: tabular-nums

  &__main
    flex: 1
    min-width: 0

  &__label
    font-size: 13px
    color: var(--color-text-primary)
    line-height: 1.4
    word-break: break-word

  &__desc
    font-size: 12px
    color: var(--color-text-secondary)
    line-height: 1.4
    margin-top: 2px

  &__status
    flex-shrink: 0
    display: flex
    align-items: center
    gap: 4px

  &__icon
    font-size: 14px
    line-height: 1

    &--pending
      color: var(--color-icon-muted)
    &--running
      color: var(--color-accent)
      animation: plan-step-pulse 2s ease-in-out infinite
    &--completed
      color: var(--color-success)
    &--failed
      color: var(--color-danger)
    &--skipped
      color: var(--color-icon-muted)
    &--partial_failure
      color: var(--color-warning)

  &__status-text
    font-size: 11px
    color: var(--color-text-secondary)

  // 已完成的步骤加删除线
  &--completed &__label
    text-decoration: line-through
    color: var(--color-text-secondary)

  // 失败的步骤加红色左边框
  &--failed
    border-left: 2px solid var(--color-danger)
    padding-left: 6px

@keyframes plan-step-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.6

.plan-empty
  font-size: 12px
  color: var(--color-text-secondary)
  text-align: center
  padding: 12px
</style>
