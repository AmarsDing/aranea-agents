<!-- web/src/components/chat/v2/TurnContainer.vue
  设计稿 §3.6.3 组件层级：TurnContainer 内渲染 steps + TeamStagePanel。
  TeamStagePanel 通过 TeamStage.TurnID 关联到触发它的 turn。
-->
<template>
  <div class="turn-container" :data-turn-id="turn.ID">
    <template v-for="step in visibleSteps" :key="step.ID">
      <ThinkingBlock v-if="step.Kind === 'thinking'" :step="step" />
      <ActionBlock v-else-if="step.Kind === 'action'" :step="step" />
      <ReplyBlock v-else-if="step.Kind === 'reply'" :step="step" />
      <NoticeBlock v-else-if="step.Kind === 'notice'" :step="step" />
      <ConfirmBlock v-else-if="step.Kind === 'confirm'" :step="step" />
      <ErrorBlock v-else-if="step.Kind === 'error'" :step="step" />
    </template>
    <TeamStagePanel
      v-for="ts in turnTeamStages"
      :key="ts.ID"
      :team-stage="ts"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @retry-team="(teamId) => $emit('retry-team', teamId)"
      @expand="(ids) => $emit('expand', ids)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import type { Turn } from '../../../features/chat/v2Types';
import { isSystemInternalNotice } from '../../../features/chat/noticeFilter';
import ThinkingBlock from '../ThinkingBlock.vue';
import ActionBlock from '../ActionBlock.vue';
import ReplyBlock from '../ReplyBlock.vue';
import NoticeBlock from '../NoticeBlock.vue';
import ConfirmBlock from '../ConfirmBlock.vue';
import ErrorBlock from '../ErrorBlock.vue';
import TeamStagePanel from './TeamStagePanel.vue';

const props = defineProps<{ turn: Turn }>();
defineEmits<{
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  'retry-team': [teamId: string];
  expand: [sessionIds: string[]];
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

// 当前 turn 触发的 team stages（设计稿 §3.6.2）
const turnTeamStages = computed(() => store.getTurnTeamStages(props.turn.ID));
</script>
