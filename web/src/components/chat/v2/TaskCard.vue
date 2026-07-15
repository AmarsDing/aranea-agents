<!-- web/src/components/chat/v2/TaskCard.vue -->
<template>
  <div class="task-card" :data-task-id="task.ID">
    <!-- 用户消息统一面板：时间 + 头像 + 名称 + 消息内容 + 操作按钮 -->
    <div class="task-user-panel">
      <div class="task-user-panel__header">
        <span class="task-user-panel__time">{{ formattedTime }}</span>
        <div class="task-user-panel__avatar" :aria-label="userLabel">{{ avatarLetter }}</div>
        <span class="task-user-panel__name">{{ userLabel }}</span>
      </div>
      <div class="task-user-panel__body" :data-chat-user-prompt="task.UserMessage">
        <div class="task-user-panel__text">{{ task.UserMessage }}</div>
      </div>
      <div class="task-user-panel__actions">
        <q-btn
          flat
          dense
          round
          size="sm"
          :aria-label="t('chat.copy')"
          icon="content_copy"
          class="task-user-panel__action-btn"
          @click="copyMessage"
        >
          <q-tooltip>{{ t('chat.copy') }}</q-tooltip>
        </q-btn>
        <q-btn
          flat
          dense
          round
          size="sm"
          :aria-label="t('chat.regenerate')"
          icon="refresh"
          class="task-user-panel__action-btn"
          @click="$emit('regenerate', task)"
        >
          <q-tooltip>{{ t('chat.regenerate') }}</q-tooltip>
        </q-btn>
      </div>
    </div>
    <div v-if="task.Status === 'running'" class="task-status">{{ t('chat.v2.taskProcessing') }}</div>
    <TurnList
      v-if="prePlanTurns.length"
      :turns="prePlanTurns"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
    />
    <template v-for="pb in planBoards" :key="pb.ID">
      <PlanBoardCard :plan-board="pb" />
      <GraphStageBlock v-if="graphStageByPlanBoard(pb.ID)" :graph-stage="graphStageByPlanBoard(pb.ID)!" />
    </template>
    <!-- Fallback: TurnID 为空的 team stages（后端未正确关联 TurnID 时兜底）。
         后端修复 PlanBoard.TurnID 后这些会自动移入 TurnContainer。 -->
    <TeamStagePanel
      v-for="ts in orphanTeamStages"
      :key="ts.ID"
      :team-stage="ts"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
    />
    <TurnList
      v-if="postPlanTurns.length"
      :turns="postPlanTurns"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import { useSafeAuth } from '../../../features/chat/composables/useSafeAuth';
import type { Task } from '../../../features/chat/v2Types';
import TurnList from './TurnList.vue';
import TeamStagePanel from './TeamStagePanel.vue';
import PlanBoardCard from './PlanBoardCard.vue';
import GraphStageBlock from './GraphStageBlock.vue';

// Safe i18n wrapper — falls back to the key when the i18n plugin isn't
// installed (e.g., during unit tests without app.use(i18n)).
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

// Safe Quasar wrapper — returns a no-op notify when Quasar isn't installed.
function useSafeQuasar() {
  try {
    return useQuasar();
  } catch {
    return { notify: (_: unknown) => {} } as ReturnType<typeof useQuasar>;
  }
}

const props = defineProps<{ task: Task }>();
defineEmits<{
  regenerate: [task: Task];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
}>();

const { t } = useSafeI18n();
const $q = useSafeQuasar();
const auth = useSafeAuth();
const store = useActivityQueries();
const turns = computed(() => store.getTaskTurns(props.task.ID));
const planBoards = computed(() => store.getTaskPlanBoards(props.task.ID));
// Fallback: TurnID 为空或未匹配到任何 turn 的 team stages（后端 PlanBoard.TurnID 未正确填充时兜底）
const orphanTeamStages = computed(() => {
  const all = store.getTaskTeamStages(props.task.ID);
  const turnIds = new Set(turns.value.map((t) => t.ID));
  return all.filter((ts) => !ts.TurnID || !turnIds.has(ts.TurnID));
});
const spiritTurns = computed(() => turns.value.filter((t) => !t.TeamStageID));
const prePlanTurns = computed(() => {
  const pbs = planBoards.value;
  if (pbs.length === 0) return spiritTurns.value;
  const firstPlanTime = pbs[0].StartedAt;
  return spiritTurns.value.filter((t) => !firstPlanTime || t.StartedAt < firstPlanTime);
});
const postPlanTurns = computed(() => {
  const pbs = planBoards.value;
  if (pbs.length === 0) return [];
  const firstPlanTime = pbs[0].StartedAt;
  return spiritTurns.value.filter((t) => firstPlanTime && t.StartedAt >= firstPlanTime);
});
function graphStageByPlanBoard(planBoardId: string) {
  return store.getGraphStageByPlanBoard(planBoardId);
}

const userLabel = computed(() => auth.displayLabel || '你');
const avatarLetter = computed(() => auth.avatarLetter || 'U');

/** 格式化任务创建时间为 HH:MM（同天）或 MM-DD HH:MM（跨天）。 */
const formattedTime = computed(() => {
  const raw = props.task.CreatedAt;
  if (!raw) return '';
  const d = new Date(raw);
  if (isNaN(d.getTime())) return '';
  const now = new Date();
  const sameDay = d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate();
  const pad = (n: number) => String(n).padStart(2, '0');
  return sameDay ? `${pad(d.getHours())}:${pad(d.getMinutes())}` : `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
});

function copyMessage() {
  navigator.clipboard?.writeText(props.task.UserMessage).then(() => {
    $q.notify({ type: 'positive', message: t('chat.copied', '已复制'), timeout: 1500 });
  });
}
</script>

<style lang="sass" scoped>
.task-card
  padding: 0

/* 用户消息统一面板：时间 + 头像 + 名称 + 消息内容 + 操作按钮 */
.task-user-panel
  display: flex
  flex-direction: column
  align-items: flex-end
  gap: 6px
  margin-bottom: 8px

  &__header
    display: flex
    align-items: center
    gap: 8px
    padding-right: 4px

  &__time
    font-size: 12px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums

  &__avatar
    flex-shrink: 0
    width: 24px
    height: 24px
    border-radius: 50%
    display: flex
    align-items: center
    justify-content: center
    font-size: 12px
    font-weight: 600
    color: var(--color-on-accent, #fff)
    background: var(--color-accent)
    user-select: none

  &__name
    font-size: 13px
    color: var(--color-text-secondary)
    font-weight: 500
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__body
    max-width: 70%
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 14px 14px 4px 14px
    padding: 10px 14px
    word-break: break-word

  &__text
    font-size: 14px
    line-height: 1.5
    white-space: pre-wrap

  &__actions
    display: flex
    gap: 2px
    opacity: 0
    transition: opacity 0.2s

  &:hover &__actions
    opacity: 1

  &__action-btn
    color: var(--color-text-secondary)
</style>
