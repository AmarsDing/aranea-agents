<!-- web/src/components/chat/v2/TaskCard.vue -->
<template>
  <div
    class="task-card"
    :class="{
      'task-card--clickable': !hydrated || collapsed,
    }"
    :data-task-id="task.ID"
    @click="onCardClick"
  >
    <!-- 用户消息统一面板：时间 + 头像 + 名称 + 消息内容 + 操作按钮（原样，按钮加 .stop） -->
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
          @click.stop="copyMessage"
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
          @click.stop="emit('regenerate', task)"
        >
          <q-tooltip>{{ t('chat.regenerate') }}</q-tooltip>
        </q-btn>
      </div>
    </div>

    <!-- 水合中：用户面板 + 3 条 shimmer 骨架 -->
    <div v-if="!hydrated && hydrationState === 'loading'" class="task-card__skeleton" aria-hidden="true">
      <div class="task-card__skeleton-bar" style="width: 62%" />
      <div class="task-card__skeleton-bar" style="width: 38%" />
      <div class="task-card__skeleton-bar" style="width: 81%" />
    </div>

    <!-- 折叠态（未水合 / 水合后手动收起）：slim meta-bar -->
    <div
      v-else-if="!hydrated || collapsed"
      class="task-meta-bar"
      :class="[`task-meta-bar--${statusTone}`, { 'task-meta-bar--error': hydrationState === 'error' }]"
    >
      <span class="task-meta-bar__badge">{{ statusLabel }}</span>
      <span v-if="durationText" class="task-meta-bar__duration">⏱ {{ durationText }}</span>
      <span v-if="hydrationState === 'error'" class="task-meta-bar__error-text">
        {{ t('chat.v2.loadFailedRetry') }}
      </span>
    </div>

    <!-- 水合态：现状完整渲染 + 底部收起按钮 -->
    <template v-else>
      <div v-if="task.Status === 'running'" class="task-status">{{ t('chat.v2.taskProcessing') }}</div>
      <!-- 澄清门卡片：orphan step（TurnID 空，澄清在 Run/Turn 创建前发布） -->
      <ClarifyBlock
        v-for="s in orphanClarifySteps"
        :key="s.ID"
        :step="s"
        @submit-clarification="(p) => emit('submit-clarification', p)"
      />
      <!-- L3: 中断任务入口 — 服务重启导致的中断，点击「继续执行」触发 WS resume_task -->
      <div v-if="task.Status === 'interrupted'" class="task-interrupted">
        <q-icon name="pause_circle_outline" size="16px" class="task-interrupted__icon" />
        <span class="task-interrupted__label">{{ t('chat.v2.taskInterrupted') }}</span>
        <q-btn
          unelevated
          dense
          no-caps
          size="sm"
          color="accent"
          class="task-interrupted__btn"
          :label="t('chat.v2.resumeTask')"
          @click.stop="emit('resume-task', task)"
        />
      </div>
      <TurnList
        v-if="prePlanTurns.length"
        :turns="prePlanTurns"
        @pause-agent="(sid) => emit('pause-agent', sid)"
        @inject-agent="(p) => emit('inject-agent', p)"
        @retry-team="(teamId) => emit('retry-team', teamId)"
        @expand="(ids) => emit('expand', ids)"
        @confirm-step="(p) => emit('confirm-step', p)"
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
        @pause-agent="(sid) => emit('pause-agent', sid)"
        @inject-agent="(p) => emit('inject-agent', p)"
        @retry-team="(teamId) => emit('retry-team', teamId)"
        @expand="(ids) => emit('expand', ids)"
        @confirm-step="(p) => emit('confirm-step', p)"
      />
      <!-- Mode B: orphan member sessions not under any TeamRun (sub-agent without team shell) -->
      <MemberSessionPanel
        v-for="ms in orphanMemberSessions"
        :key="ms.ID"
        :member-session="ms"
        @pause-agent="(sid) => emit('pause-agent', sid)"
        @inject-agent="(p) => emit('inject-agent', p)"
        @expand="(ids) => emit('expand', ids)"
        @confirm-step="(p) => emit('confirm-step', p)"
      />
      <TurnList
        v-if="postPlanTurns.length"
        :turns="postPlanTurns"
        @pause-agent="(sid) => emit('pause-agent', sid)"
        @inject-agent="(p) => emit('inject-agent', p)"
        @retry-team="(teamId) => emit('retry-team', teamId)"
        @expand="(ids) => emit('expand', ids)"
        @confirm-step="(p) => emit('confirm-step', p)"
      />
      <!-- B.10.17: orphan notice steps（TurnID 空，附着到 Task）作为任务总结 footer
           渲染在任务卡末尾 — 执行总结报告（synthesis_completed → ExecutionReportCard）
           与兜底完成通知（synthesis 硬失败时的"所有团队已完成"）。系统内部通知
           （context_usage 等）沿用 TurnContainer 同款过滤。 -->
      <NoticeBlock v-for="s in orphanNoticeSteps" :key="s.ID" :step="s" />
      <button class="task-card__collapse-btn" type="button" @click.stop="emit('toggle-collapse', task)">
        {{ t('chat.v2.collapseExecution') }} ▴
      </button>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import { useSafeAuth } from '../../../features/chat/composables/useSafeAuth';
import type { Task } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload, SubmitClarificationPayload } from '../../../features/chat/types';
import TurnList from './TurnList.vue';
import TeamStagePanel from './TeamStagePanel.vue';
import PlanBoardCard from './PlanBoardCard.vue';
import GraphStageBlock from './GraphStageBlock.vue';
import MemberSessionPanel from './MemberSessionPanel.vue';
import ClarifyBlock from './ClarifyBlock.vue';
import NoticeBlock from '../NoticeBlock.vue';
import { isSystemInternalNotice } from '../../../features/chat/noticeFilter';

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
// NOTE: useQuasar() is a bare inject() — returns undefined (no throw) without
// the plugin, so an explicit null check is required.
function useSafeQuasar() {
  try {
    const $q = useQuasar();
    if ($q) return $q;
  } catch {
    // fall through to no-op
  }
  return { notify: (_: unknown) => {} } as unknown as ReturnType<typeof useQuasar>;
}

const props = withDefaults(
  defineProps<{
    task: Task;
    /** 执行子树是否已水合（false = 折叠卡）。默认 true 兼容既有用法。 */
    hydrated?: boolean;
    /** 水合进行/失败态（仅 !hydrated 时有意义）。 */
    hydrationState?: 'loading' | 'error';
    /** 水合后手动收起（UI 态，数据保留 store，再展开零请求）。 */
    collapsed?: boolean;
  }>(),
  { hydrated: true, hydrationState: undefined, collapsed: false },
);
const emit = defineEmits<{
  regenerate: [task: Task];
  'resume-task': [task: Task];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  'retry-team': [teamId: string];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
  'submit-clarification': [payload: SubmitClarificationPayload];
  hydrate: [task: Task];
  'toggle-collapse': [task: Task];
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
const orphanMemberSessions = computed(() => store.getTaskOrphanMemberSessions(props.task.ID));
// 澄清门 orphan steps（kind=clarify；TurnID 空，澄清在 Run/Turn 创建前发布）
const orphanClarifySteps = computed(() =>
  store.getTaskOrphanSteps(props.task.ID).filter((s) => s.Kind === 'clarify'),
);
// B.10.17: orphan notice steps（kind=notice；TurnID 空，附着到 Task）— 执行总结
// 报告（synthesis_completed → ExecutionReportCard）与兜底完成通知。过滤系统内部
// 通知（context_usage 等），与 TurnContainer 保持一致。
const orphanNoticeSteps = computed(() =>
  store.getTaskOrphanSteps(props.task.ID).filter((s) => s.Kind === 'notice' && !isSystemInternalNotice(s.Kind, s.NoticeType)),
);
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
  const sameDay =
    d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate();
  const pad = (n: number) => String(n).padStart(2, '0');
  return sameDay
    ? `${pad(d.getHours())}:${pad(d.getMinutes())}`
    : `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
});

/** 折叠卡整卡点击：未水合 → 请求水合；水合后收起态 → 本地展开（零请求）。 */
function onCardClick() {
  if (!props.hydrated) emit('hydrate', props.task);
  else if (props.collapsed) emit('toggle-collapse', props.task);
}

/** meta-bar 状态徽章文案：复用 chat.v2.status* 既有键。 */
const statusLabel = computed(() => {
  const key = `chat.v2.status${props.task.Status.charAt(0).toUpperCase()}${props.task.Status.slice(1)}`;
  return t(key);
});

/** meta-bar 状态色调：completed=success / failed=danger / 其余=neutral。 */
const statusTone = computed(() => {
  if (props.task.Status === 'completed') return 'success';
  if (props.task.Status === 'failed') return 'danger';
  return 'neutral';
});

/** 耗时文案：CompletedAt - CreatedAt → "Ns" / "NmSSs"；未完结不显示。 */
const durationText = computed(() => {
  if (!props.task.CompletedAt) return '';
  const ms = Date.parse(props.task.CompletedAt) - Date.parse(props.task.CreatedAt);
  if (!Number.isFinite(ms) || ms < 0) return '';
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}m${String(s % 60).padStart(2, '0')}s`;
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

/* L3: 中断任务提示条 */
.task-interrupted
  display: flex
  align-items: center
  gap: 8px
  margin: 4px 0 8px
  padding: 8px 12px
  border-radius: 10px
  border: 1px solid var(--color-warning, #ed6c02)
  background: color-mix(in srgb, var(--color-warning, #ed6c02) 8%, transparent)

  &__icon
    color: var(--color-warning, #ed6c02)
    flex-shrink: 0

  &__label
    flex: 1
    font-size: 13px
    color: var(--color-text-secondary)

  &__btn
    flex-shrink: 0

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

/* 折叠卡整卡可点（复制/重新生成按钮 @click.stop 不触发） */
.task-card--clickable
  cursor: pointer

  .task-user-panel__body
    transition: border-color 0.2s, background 0.2s

  &:hover .task-user-panel__body
    border-color: var(--color-accent)
    background: color-mix(in srgb, var(--glass-surface) 80%, var(--color-accent) 8%)

/* slim meta-bar：状态徽章 + 耗时（color-mix 状态色，日夜 token） */
.task-meta-bar
  display: flex
  align-items: center
  gap: 8px
  margin: -2px 0 8px auto
  width: fit-content
  padding: 4px 10px
  border-radius: 999px
  font-size: 12px
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)

  &__badge
    font-weight: 500

  &__duration
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums

  &--success &__badge
    color: var(--color-success, #2e7d32)

  &--danger &__badge
    color: var(--color-danger, #d32f2f)

  &--neutral &__badge
    color: var(--color-text-secondary)

  &--error
    border-color: var(--color-danger, #d32f2f)

  &__error-text
    color: var(--color-danger, #d32f2f)

/* 水合中 shimmer 骨架（thinking/action/reply 三条） */
.task-card__skeleton
  display: flex
  flex-direction: column
  gap: 10px
  margin: 4px 0 8px

  &-bar
    height: 14px
    border-radius: 7px
    background: linear-gradient(90deg, var(--glass-surface) 25%, var(--glass-border) 50%, var(--glass-surface) 75%)
    background-size: 200% 100%
    animation: task-card-shimmer 1.4s infinite

@keyframes task-card-shimmer
  0%
    background-position: 200% 0
  100%
    background-position: -200% 0

/* 收起执行过程按钮（水合态底部） */
.task-card__collapse-btn
  display: block
  margin: 4px auto 8px
  padding: 4px 12px
  border: none
  background: transparent
  color: var(--color-text-tertiary)
  font-size: 12px
  cursor: pointer
  border-radius: 6px
  transition: color 0.2s, background 0.2s

  &:hover
    color: var(--color-accent)
    background: color-mix(in srgb, var(--color-accent) 8%, transparent)
</style>
