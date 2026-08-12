<!-- web/src/components/chat/v2/TurnContainer.vue
  设计稿 §3.6.3 组件层级：TurnContainer 内渲染 steps。
  2026-07-26 方案A：TeamStagePanel 移除 — 团队执行过程由 GraphStageBlock 富节点统一展示。
-->
<template>
  <div class="turn-container" :data-turn-id="turn.ID">
    <!-- R4 召回透明度：本轮注入的记忆条目 chips（数据源 memory_recalled notice）。
         渲染在 steps 之前——召回发生在 BeforeModel（turn 最开始），UI 顺序必须与
         实际执行顺序一致：召回 → 思考 → 行动 → 回复。 -->
    <MemoryRecallChips :turn-id="turn.ID" />
    <template v-for="step in visibleSteps" :key="step.ID">
      <ThinkingBlock v-if="step.Kind === 'thinking'" :step="step" />
      <ActionBlock v-else-if="step.Kind === 'action'" :step="step" />
      <ReplyBlock v-else-if="step.Kind === 'reply'" :step="step" />
      <NoticeBlock v-else-if="step.Kind === 'notice'" :step="step" />
      <ConfirmBlock v-else-if="step.Kind === 'confirm'" :step="step" @confirm="(p) => $emit('confirm-step', p)" />
      <ErrorBlock v-else-if="step.Kind === 'error'" :step="step" />
    </template>
    <!-- 75 M1.4（设计 §3.8）：运行中的 turn 含 computer_use 会话时，气泡尾部
         内嵌实时步骤流 + 急停；历史 turn 不渲染（回放走监控页审计 API）。 -->
    <CuStepStream v-if="liveCuSessionId" :session-id="liveCuSessionId" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import type { Turn } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload } from '../../../features/chat/types';
import { isSystemInternalNotice } from '../../../features/chat/noticeFilter';
import { cuSessionIdFromSteps } from '../../../features/computeruse/useCuStepStream';
import ThinkingBlock from '../ThinkingBlock.vue';
import ActionBlock from '../ActionBlock.vue';
import ReplyBlock from '../ReplyBlock.vue';
import NoticeBlock from '../NoticeBlock.vue';
import ConfirmBlock from '../ConfirmBlock.vue';
import ErrorBlock from '../ErrorBlock.vue';
import MemoryRecallChips from '../MemoryRecallChips.vue';
import CuStepStream from '../../../features/computeruse/CuStepStream.vue';

const props = defineProps<{ turn: Turn }>();
defineEmits<{
  'confirm-step': [payload: ConfirmStepPayload];
}>();
const store = useActivityQueries();
const visibleSteps = computed(() =>
  store.getTurnSteps(props.turn.ID).filter((s) => {
    // 过滤系统内部通知（context_usage 等）
    if (isSystemInternalNotice(s.Kind, s.NoticeType)) return false;
    // 过滤空 reply step：Status 非 running 且 Content 为空或纯空白。
    // 防止后端遗漏场景导致空 ReplyBlock 显示。streaming 中的 reply 仍显示。
    // Spec: docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md §4.2
    if (s.Kind === 'reply' && s.Status !== 'running' && !s.Content?.trim()) {
      return false;
    }
    return true;
  }),
);

// 仅运行中的 turn 内嵌步骤流：急停对活会话才有意义；历史 turn 的审计回放
// 走监控页（ListComputerUseSteps），避免对死会话渲染可用急停按钮。
const liveCuSessionId = computed(() =>
  props.turn.Status === 'running' ? cuSessionIdFromSteps(visibleSteps.value) : '',
);
</script>
