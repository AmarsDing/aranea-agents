<!-- web/src/components/chat/v2/TurnContainer.vue
  设计稿 §3.6.3 组件层级：TurnContainer 内渲染 steps。
  2026-07-26 方案A：TeamStagePanel 移除 — 团队执行过程由 GraphStageBlock 富节点统一展示。
-->
<template>
  <div class="turn-container" :data-turn-id="turn.ID" tabindex="-1">
    <!-- 79 R6 Fork-from-Turn：hover 显示「从此分支」。根会话与 fork 会话的
         spirit turn（SessionID === SpiritSessionID 且非 team stage）且非运行中
         可分叉——多代 fork 已放开（2026-08-28 T5，后端前缀剥离/归一保证）；
         team/member 子会话历史不闭合，隐藏按钮且后端门禁兜底。 -->
    <q-btn
      v-if="forkable"
      flat
      dense
      round
      size="sm"
      icon="call_split"
      class="turn-container__fork-btn"
      :aria-label="t('chat.forkFromHere')"
      @click.stop="$emit('fork-turn', turn)"
    >
      <q-tooltip>{{ t('chat.forkFromHere') }}</q-tooltip>
    </q-btn>
    <!-- R4 召回透明度：本轮注入的记忆条目 chips（数据源 memory_recalled notice）。
         渲染在 steps 之前——召回发生在 BeforeModel（turn 最开始），UI 顺序必须与
         实际执行顺序一致：召回 → 思考 → 行动 → 回复。 -->
    <MemoryRecallChips :turn-id="turn.ID" :session-id="turn.SessionID" :agent-key="turn.AgentKey" />
    <KnowledgeRecallChips :turn-id="turn.ID" />
    <template v-for="step in visibleSteps" :key="step.ID">
      <ThinkingBlock v-if="step.Kind === 'thinking'" :step="step" :data-step-id="step.ID" />
      <ActionBlock v-else-if="step.Kind === 'action'" :step="step" :data-step-id="step.ID" />
      <ReplyBlock v-else-if="step.Kind === 'reply'" :step="step" :data-step-id="step.ID" />
      <div v-else-if="step.Kind === 'notice'" :data-step-id="step.ID" tabindex="-1"><NoticeBlock :step="step" /></div>
      <ConfirmBlock v-else-if="step.Kind === 'confirm'" :step="step" @confirm="(p) => $emit('confirm-step', p)" />
      <ErrorBlock v-else-if="step.Kind === 'error'" :step="step" :data-step-id="step.ID" />
    </template>
    <!-- 75：含 computer_use 会话时内嵌步骤流。运行中可急停；历史 turn 只读回放。 -->
    <CuStepStream v-if="cuSessionId" :session-id="cuSessionId" :readonly="!isLiveTurn" />
  </div>
</template>

<script setup lang="ts">
import { toRef } from 'vue';
import { useI18n } from 'vue-i18n';
import { useTurnPresentation } from '../../../features/chat/composables/useTurnPresentation';
import type { Turn } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload } from '../../../features/chat/types';
import ThinkingBlock from '../ThinkingBlock.vue';
import ActionBlock from '../ActionBlock.vue';
import ReplyBlock from '../ReplyBlock.vue';
import NoticeBlock from '../NoticeBlock.vue';
import ConfirmBlock from '../ConfirmBlock.vue';
import ErrorBlock from '../ErrorBlock.vue';
import MemoryRecallChips from '../MemoryRecallChips.vue';
import KnowledgeRecallChips from '../KnowledgeRecallChips.vue';
import CuStepStream from '../../../features/computeruse/CuStepStream.vue';

const props = defineProps<{ turn: Turn }>();
defineEmits<{
  'confirm-step': [payload: ConfirmStepPayload];
  'fork-turn': [turn: Turn];
}>();

// Safe i18n wrapper — 单测环境未装 i18n 插件时回退为 key。
const { t } = (() => {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
})();
const { forkable, visibleSteps, cuSessionId, isLiveTurn } = useTurnPresentation(toRef(props, 'turn'));
</script>

<style lang="sass" scoped>
.turn-container
  position: relative

  /* 79 R6「从此分支」：hover 才显现，避免常驻视觉噪音 */
  &__fork-btn
    position: absolute
    top: 0
    right: 0
    z-index: 1
    color: var(--color-text-tertiary)
    opacity: 0
    transition: opacity 0.2s, color 0.2s

  &:hover &__fork-btn, &:focus-within &__fork-btn
    opacity: 1

  &__fork-btn:hover
    color: var(--color-accent)
</style>
