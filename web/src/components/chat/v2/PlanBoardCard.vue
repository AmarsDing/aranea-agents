<!-- web/src/components/chat/v2/PlanBoardCard.vue
  2026-07-04 完善：折叠/摘要逻辑（需求 §A.4.1）
  - 进行中默认展开，初始渲染时若全部完成自动折叠为摘要
  - 用户手动展开/折叠后状态由用户掌控（不被状态变化自动覆盖）
  - 计划变更直接更新 plan 内容，不显示 diff
-->
<template>
  <div class="plan-board-card" :data-plan-board-id="planBoard.ID">
    <div class="plan-header" @click="toggleCollapse">
      <div class="plan-header__left">
        <q-icon :name="collapsed ? 'expand_more' : 'expand_less'" size="20px" class="plan-header__icon" />
        <span class="plan-header__title">{{ t('chat.v2.planBoardTitle') }}</span>
        <q-badge :color="statusColor" class="plan-header__status">{{ planBoard.Status }}</q-badge>
      </div>
      <div class="plan-header__summary">
        <span class="plan-summary-text">{{ summaryText }}</span>
      </div>
    </div>
    <div v-show="!collapsed" class="plan-body">
      <PlanDAG :steps="planBoard.Steps" />
      <PlanStepDetailPanel :step="selectedStep" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { PlanBoard } from '../../../features/chat/v2Types';
import PlanDAG from './PlanDAG.vue';
import PlanStepDetailPanel from './PlanStepDetailPanel.vue';

// Safe i18n wrapper — falls back to the key when the i18n plugin isn't
// installed (e.g., during unit tests without app.use(i18n)).
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ planBoard: PlanBoard }>();
const { t } = useSafeI18n();
const selectedStepId = ref<string | null>(null);
const selectedStep = computed(() => props.planBoard.Steps.find((s) => s.ID === selectedStepId.value) || null);

// 折叠状态：默认展开。用户手动操作后 userToggled=true，不再自动变更。
const collapsed = ref(false);
const userToggled = ref(false);

// 初始渲染时若全部完成自动折叠为摘要（需求 §A.4.1）
watch(
  () => props.planBoard.ID,
  () => {
    if (userToggled.value) return;
    collapsed.value = isAllCompleted.value;
  },
  { immediate: true },
);

const isAllCompleted = computed(() => props.planBoard.Steps.length > 0 && props.planBoard.Steps.every((s) => s.Status === 'completed'));

function toggleCollapse() {
  userToggled.value = true;
  collapsed.value = !collapsed.value;
}

const summaryText = computed(() => {
  const total = props.planBoard.Steps.length;
  const done = props.planBoard.Steps.filter((s) => s.Status === 'completed').length;
  const failed = props.planBoard.Steps.filter((s) => s.Status === 'failed' || s.Status === 'partial_failure').length;
  if (failed > 0) return `${done}/${total} · ${failed} ${t('chat.v2.failedLabel')}`;
  if (done === total) return `✅ ${done}/${total}`;
  return `${done}/${total}`;
});

const statusColor = computed(
  () =>
    ({
      planning: 'orange',
      executing: 'blue',
      completed: 'green',
      failed: 'red',
      partial_failure: 'orange-8',
    })[props.planBoard.Status] || 'grey',
);
</script>

<style lang="sass" scoped>
.plan-board-card
  border: 1px solid var(--color-border, #e0e0e0)
  border-radius: 8px
  margin: 8px 0
  background: var(--color-surface, #fafafa)

.plan-header
  display: flex
  align-items: center
  justify-content: space-between
  padding: 8px 12px
  cursor: pointer
  user-select: none
  border-bottom: 1px solid var(--color-border, #e0e0e0)

  &:hover
    background: var(--color-hover, #f0f0f0)

  &__left
    display: flex
    align-items: center
    gap: 6px

  &__icon
    color: var(--color-text-secondary)

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
  padding: 12px
</style>
