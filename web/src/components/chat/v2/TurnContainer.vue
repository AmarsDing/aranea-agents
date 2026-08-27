<!-- web/src/components/chat/v2/TurnContainer.vue
  设计稿 §3.6.3 组件层级：TurnContainer 内渲染 steps。
  2026-07-26 方案A：TeamStagePanel 移除 — 团队执行过程由 GraphStageBlock 富节点统一展示。
-->
<template>
  <div class="turn-container" :data-turn-id="turn.ID">
    <!-- 79 R6 Fork-from-Turn：hover 显示「从此分支」。仅根会话的 spirit turn
         （SessionID === SpiritSessionID 且非 team stage）且非运行中可分叉——
         member 子会话事件不在根会话框架事件流内，后端会拒绝；已分叉会话
         （链式 fork）同样隐藏，后端对 child session 硬拒绝。 -->
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
      <ThinkingBlock v-if="step.Kind === 'thinking'" :step="step" />
      <ActionBlock v-else-if="step.Kind === 'action'" :step="step" />
      <ReplyBlock v-else-if="step.Kind === 'reply'" :step="step" />
      <NoticeBlock v-else-if="step.Kind === 'notice'" :step="step" />
      <ConfirmBlock v-else-if="step.Kind === 'confirm'" :step="step" @confirm="(p) => $emit('confirm-step', p)" />
      <ErrorBlock v-else-if="step.Kind === 'error'" :step="step" />
    </template>
    <!-- 75：含 computer_use 会话时内嵌步骤流。运行中可急停；历史 turn 只读回放。 -->
    <CuStepStream v-if="cuSessionId" :session-id="cuSessionId" :readonly="!isLiveTurn" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import { useUiConfigStore } from '../../../stores/uiConfig';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
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

// 79 R6：可分叉 = 根会话 spirit turn（SessionID === SpiritSessionID、非 team
// stage）且非运行中（running turn 事件仍在追加，分叉边界不稳定）。
// 链式 fork 后端硬拒绝（继承 id 前缀与复制事件 invocationId 无法命中，见
// biz/session/fork.go）——已分叉会话（parent_session_id 非空）直接隐藏按钮，
// 不让用户点击后吃 400。findSessionById 覆盖 agent 列表与 team 列表（team 场景
// selectedSession 为空、仅有 teamSelectedSessionId），未加载到时 fail-open 由
// 后端门禁兜底。
const chatSessionStore = useChatSessionStore();
const isChildSession = computed(() => {
  const cur = chatSessionStore.findSessionById(props.turn.SessionID);
  return !!cur?.parent_session_id?.trim();
});
const forkable = computed(
  () =>
    !isChildSession.value &&
    !props.turn.TeamStageID &&
    props.turn.SessionID === props.turn.SpiritSessionID &&
    props.turn.Status !== 'running',
);
const store = useActivityQueries();
// 2026-08-21 全链路审查 R2：showToolCalls 开关此前只管 TodoKanban，action
// steps（工具调用块）无条件渲染，开关名不副实。uiConfig 是全局 UI 偏好
// store（localStorage 持久化），叶子组件直读与 useChatMessagePanelBindings
// 同款，避免 Page→…→TaskCard→TurnList 五层 prop 钻透。
const uiConfig = useUiConfigStore();
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
    // showToolCalls=false 时隐藏工具调用块（与 TodoKanban 开关语义对齐）。
    if (s.Kind === 'action' && !uiConfig.showToolCalls) {
      return false;
    }
    return true;
  }),
);

// 含 computer_use 会话即内嵌步骤流；仅运行中的 turn 显示急停。
const cuSessionId = computed(() => cuSessionIdFromSteps(visibleSteps.value));
const isLiveTurn = computed(() => props.turn.Status === 'running');
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

  &:hover &__fork-btn
    opacity: 1

  &__fork-btn:hover
    color: var(--color-accent)
</style>
