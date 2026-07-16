<!-- web/src/components/chat/v2/TeamRunCard.vue
  2026-07-05 重构：三段式横向布局 2:3:1（设计稿 §4.2 + §B.10.10 修订）
  - 头部 ~33%：groups 图标 + 团队名 / 任务名 / 创建时间（垂直 1:1:1）
  - 中部 ~50%：成员 chips / 进度条（垂直 1:2）
  - 尾部 ~17%：状态徽章（大、居中）+ 耗时（右下角）
  - team 面板无任何操作按钮（纯展示）；影响 agent 的操作由 MemberSessionPanel 输入栏承担
  - 展开后：成员列表（MemberSessionPanel 各自带输入栏）
  - 展开折叠：整条 team-run-bar 可点击切换；running 默认展开，终态默认折叠
-->
<template>
  <div class="team-run-card" :data-team-run-id="teamRun.ID">
    <!-- 顶部长条：三段式横向布局 2:3:1 -->
    <div class="team-run-bar" @click="toggleCollapse">
      <!-- 头部 ~33%：groups 图标 + 垂直 1:1:1（团队名 / 任务名 / 创建时间） -->
      <div class="team-run-bar__header">
        <q-icon name="groups" size="18px" class="team-run-bar__icon" />
        <div class="header-rows">
          <div class="header-row header-row--team" :title="displayTitle">
            <span class="header-row__label">{{ t('chat.v2.teamLabel') }}</span>
            <span class="header-row__value">{{ displayTitle }}</span>
          </div>
          <div class="header-row header-row--task" :title="taskName">
            <span class="header-row__label">{{ t('chat.v2.taskLabel') }}</span>
            <span class="header-row__value">{{ taskName }}</span>
          </div>
          <div class="header-row header-row--time">
            <span class="header-row__label">{{ t('chat.v2.createdTimeLabel') }}</span>
            <span class="header-row__value">{{ formattedTime }}</span>
          </div>
        </div>
      </div>

      <!-- 中部 ~50%：垂直 1:2（成员 chips / 进度条） -->
      <div class="team-run-bar__middle">
        <div class="middle-members">
          <template v-if="memberChips.length > 0">
            <div
              v-for="chip in memberChips"
              :key="chip.agentKey"
              class="member-chip"
              :class="`member-chip--${chip.status}`"
              :title="`${chip.agentName} · ${chip.status}`"
            >
              <q-avatar v-if="chip.avatarURL" :src="chip.avatarURL" size="20px" />
              <q-icon v-else name="person" size="16px" />
              <span class="member-chip__name">{{ chip.agentName }}</span>
            </div>
          </template>
          <span v-else class="middle-members__empty">{{ t('chat.v2.noMembers') }}</span>
        </div>
        <div class="middle-progress">
          <div class="progress-bar">
            <div class="progress-bar__fill" :style="{ width: progress.pct + '%' }" />
            <span class="progress-bar__text">{{ progress.completed }}/{{ progress.total }}</span>
          </div>
        </div>
      </div>

      <!-- 尾部 ~17%：状态徽章（大、居中）+ 耗时（右下角） -->
      <div class="team-run-bar__tail">
        <div class="tail-status">
          <q-badge :color="statusColor" class="tail-status__badge">{{ statusLabel }}</q-badge>
        </div>
        <div class="tail-duration" :title="t('chat.v2.durationLabel')">
          <q-icon name="schedule" size="11px" class="tail-duration__icon" />
          <span class="tail-duration__text">{{ duration }}</span>
        </div>
      </div>
      <!-- 折叠箭头 -->
      <q-icon :name="collapsed ? 'expand_more' : 'expand_less'" size="18px" class="team-run-bar__toggle" />
    </div>

    <!-- 展开后：成员列表（每个 MemberSessionPanel 自带底部输入栏） -->
    <div v-show="!collapsed" class="team-run-expand">
      <div v-if="teamRun.Error" class="team-run-error">
        <span>{{ teamRun.Error }}</span>
        <q-btn
          v-if="teamRun.Status === 'failed' && retryTeamId"
          flat
          dense
          size="sm"
          color="negative"
          icon="refresh"
          :label="t('chat.v2.retry')"
          class="team-run-error__retry"
          @click.stop="onRetry"
        />
      </div>
      <div v-if="memberSessions.length === 0" class="team-run-empty">
        {{ t('chat.v2.noMembers') }}
      </div>
      <MemberSessionPanel
        v-for="ms in memberSessions"
        :key="ms.ID"
        :member-session="ms"
        @pause-agent="(sid) => $emit('pause-agent', sid)"
        @inject-agent="(p) => $emit('inject-agent', p)"
        @expand="(ids) => $emit('expand', ids)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, inject, watch, onMounted, onUnmounted, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import type { TeamRun } from '../../../features/chat/v2Types';
import MemberSessionPanel from './MemberSessionPanel.vue';

// Safe i18n wrapper
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ teamRun: TeamRun }>();
const emit = defineEmits<{
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  'retry-team': [teamId: string];
  expand: [sessionIds: string[]];
}>();

const { t } = useSafeI18n();
const store = useActivityQueries();
const memberSessions = computed(() => store.getTeamRunMemberSessions(props.teamRun.ID));

const retryTeamId = computed(() => {
  const ts = store.teamStages().get(props.teamRun.TeamStageID);
  return ts?.TeamID || '';
});

function onRetry() {
  if (retryTeamId.value) emit('retry-team', retryTeamId.value);
}
// 显示标题：优先 TeamStage.TeamName → PlanStep.Label → DagNodeID → ID
const displayTitle = computed(() => {
  const ts = store.teamStages().get(props.teamRun.TeamStageID);
  if (ts?.TeamName) return ts.TeamName;
  if (props.teamRun.DagNodeID) {
    const ps = store.planSteps().get(props.teamRun.DagNodeID);
    if (ps?.Label) return ps.Label;
  }
  return props.teamRun.DagNodeID || props.teamRun.ID;
});

// 任务名称：优先 PlanStep.Description（更详细），fallback PlanStep.Label，再 fallback DagNodeID
const taskName = computed(() => {
  if (props.teamRun.DagNodeID) {
    const ps = store.planSteps().get(props.teamRun.DagNodeID);
    if (ps?.Description) return ps.Description;
    if (ps?.Label) return ps.Label;
  }
  const ts = store.teamStages().get(props.teamRun.TeamStageID);
  if (ts?.TeamID) return ts.TeamID;
  return props.teamRun.DagNodeID || '-';
});

// 成员 chips
interface MemberChip {
  agentKey: string;
  agentName: string;
  avatarURL: string;
  status: string;
}
const memberChips = computed<MemberChip[]>(() => {
  return memberSessions.value.map((ms) => ({
    agentKey: ms.AgentKey,
    agentName: ms.AgentName || ms.AgentKey,
    avatarURL: ms.AvatarURL || '',
    status: ms.Status,
  }));
});

// 进度计算
const progress = computed(() => {
  const total = memberSessions.value.length;
  const completed = memberSessions.value.filter((ms) => ms.Status === 'completed').length;
  const pct = total > 0 ? Math.round((completed / total) * 100) : 0;
  return { completed, total, pct };
});

// 耗时计算
const now = ref(Date.now());
let timer: ReturnType<typeof setInterval> | null = null;
const duration = computed(() => {
  const start = props.teamRun.StartedAt;
  if (!start) return '-';
  const startMs = new Date(start).getTime();
  if (isNaN(startMs)) return '-';
  const endMs = props.teamRun.CompletedAt ? new Date(props.teamRun.CompletedAt).getTime() : now.value;
  if (isNaN(endMs)) return '-';
  const diffMs = Math.max(0, endMs - startMs);
  return formatDuration(diffMs);
});

function formatDuration(ms: number): string {
  const totalSec = Math.floor(ms / 1000);
  if (totalSec < 60) return `${totalSec}s`;
  const min = Math.floor(totalSec / 60);
  const sec = totalSec % 60;
  if (min < 60) return `${min}m${sec}s`;
  const hr = Math.floor(min / 60);
  const minRem = min % 60;
  return `${hr}h${minRem}m`;
}

// 折叠状态：running/paused 默认展开，终态默认折叠
const collapsed = ref(props.teamRun.Status !== 'running' && props.teamRun.Status !== 'paused');
const userToggled = ref(false);

onMounted(() => {
  if (props.teamRun.Status === 'running') {
    timer = setInterval(() => {
      now.value = Date.now();
    }, 1000);
  }
  // Default-expanded: lazy-load member steps on mount (A.4.7)
  if (!collapsed.value) {
    const ids = memberSessions.value.map((ms) => ms.SessionID).filter(Boolean);
    if (ids.length) emit('expand', ids);
  }
});

onUnmounted(() => {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
});

// 响应 ChatMessageList 的 autoExpandFor
const autoExpandFor = inject<Ref<{ agentKey: string; teamId: string } | null>>('chat:autoExpandFor', ref(null));
watch(
  autoExpandFor,
  (cmd) => {
    if (!cmd || userToggled.value) return;
    const matched = memberSessions.value.some((ms) => ms.AgentKey === cmd.agentKey);
    if (matched) collapsed.value = false;
  },
  { immediate: true },
);

function toggleCollapse() {
  userToggled.value = true;
  const next = !collapsed.value;
  collapsed.value = next;
  if (!next) {
    // Expanding: lazy-load member session steps (A.4.7)
    const ids = memberSessions.value.map((ms) => ms.SessionID).filter(Boolean);
    if (ids.length) emit('expand', ids);
  }
}

// 状态色映射
const statusColor = computed(
  () =>
    ({
      running: 'blue',
      paused: 'warning',
      completed: 'green',
      failed: 'red',
      cancelled: 'grey',
    })[props.teamRun.Status] || 'grey',
);

const statusLabel = computed(() => {
  const map: Record<string, string> = {
    running: t('chat.v2.statusRunning'),
    paused: t('chat.v2.statusPaused'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    cancelled: t('chat.v2.statusCancelled'),
  };
  return map[props.teamRun.Status] || props.teamRun.Status;
});

const formattedTime = computed(() => {
  const raw = props.teamRun.StartedAt;
  if (!raw) return '-';
  const d = new Date(raw);
  if (isNaN(d.getTime())) return '-';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
});
</script>

<style lang="sass" scoped>
// 2026-07-05 重构：三段式 2:3:1 横向布局
.team-run-card
  border: 1px solid var(--glass-border)
  border-radius: 6px
  margin: 4px 0
  background: var(--glass-surface)
  overflow: hidden

// 顶部长条：三段式横向 flex（2:3:1）
.team-run-bar
  display: flex
  flex-direction: row
  align-items: stretch
  cursor: pointer
  user-select: none
  position: relative
  min-height: 64px

  &:hover
    background: var(--glass-surface-hover)

// 头部 ~33%：groups 图标 + 垂直 1:1:1
.team-run-bar__header
  flex: 0 0 33%
  display: flex
  flex-direction: row
  align-items: center
  gap: 8px
  padding: 6px 10px
  border-right: 1px solid var(--glass-border)
  min-width: 0

.team-run-bar__icon
  color: var(--color-text-secondary)
  flex-shrink: 0

.header-rows
  flex: 1
  display: flex
  flex-direction: column
  justify-content: space-between
  min-width: 0

.header-row
  display: flex
  align-items: center
  gap: 4px
  min-width: 0
  font-size: 11px
  line-height: 1.3

  &__label
    color: var(--color-text-tertiary)
    flex-shrink: 0

  &__value
    color: var(--color-text-primary)
    font-weight: 500
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
    min-width: 0

  &--team &__value
    font-weight: 600
    font-size: 12px

  &--task &__value
    color: var(--color-text-secondary)
    font-size: 11px

  &--time &__value
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums
    font-size: 11px

// 中部 ~50%：垂直 1:2
.team-run-bar__middle
  flex: 0 0 50%
  display: flex
  flex-direction: column
  padding: 6px 10px
  border-right: 1px solid var(--glass-border)
  min-width: 0

.middle-members
  flex: 1
  display: flex
  align-items: center
  gap: 6px
  flex-wrap: wrap
  min-height: 22px
  overflow: hidden

.middle-members__empty
  font-size: 11px
  color: var(--color-text-tertiary)

.member-chip
  display: inline-flex
  align-items: center
  gap: 3px
  padding: 1px 6px
  border-radius: 10px
  background: var(--glass-surface-hover)
  border: 1px solid var(--glass-border)
  font-size: 11px
  color: var(--color-text-secondary)
  max-width: 100px

  &__name
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &--completed
    background: rgba(76, 175, 80, 0.12)
    border-color: rgba(76, 175, 80, 0.3)
    color: var(--color-positive, #4caf50)

  &--running
    background: rgba(33, 150, 243, 0.12)
    border-color: rgba(33, 150, 243, 0.3)
    color: var(--color-info, #2196f3)

  &--failed
    background: rgba(244, 67, 54, 0.12)
    border-color: rgba(244, 67, 54, 0.3)
    color: var(--color-danger, #f44336)

  &--pending
    background: rgba(158, 158, 158, 0.12)
    border-color: rgba(158, 158, 158, 0.3)
    color: var(--color-text-tertiary)

// 进度条（占满中部下 2/3）
.middle-progress
  flex: 2
  display: flex
  align-items: center
  padding-top: 4px
  min-height: 22px

.progress-bar
  flex: 1
  position: relative
  height: 14px
  background: var(--glass-surface-hover)
  border-radius: 7px
  overflow: hidden
  border: 1px solid var(--glass-border)

  &__fill
    height: 100%
    background: linear-gradient(90deg, var(--color-info, #2196f3), var(--color-positive, #4caf50))
    transition: width 0.3s ease

  &__text
    position: absolute
    inset: 0
    display: flex
    align-items: center
    justify-content: center
    font-size: 10px
    font-weight: 600
    color: var(--color-text-primary)
    font-variant-numeric: tabular-nums

// 尾部 ~17%：状态徽章（大、居中）+ 耗时（右下角）
.team-run-bar__tail
  flex: 0 0 17%
  display: flex
  flex-direction: column
  align-items: center
  justify-content: center
  padding: 6px 8px
  position: relative
  min-width: 0

.tail-status
  flex: 1
  display: flex
  align-items: center
  justify-content: center
  width: 100%

  &__badge
    font-size: 12px
    font-weight: 600
    padding: 4px 10px
    min-width: 60px
    justify-content: center

.tail-duration
  display: flex
  align-items: center
  gap: 2px
  font-size: 11px
  color: var(--color-text-secondary)
  font-variant-numeric: tabular-nums
  align-self: flex-end
  margin-top: 2px

  &__icon
    color: var(--color-icon-muted)

  &__text
    white-space: nowrap

// 折叠箭头
.team-run-bar__toggle
  position: absolute
  top: 4px
  right: 4px
  color: var(--color-icon-muted)
  pointer-events: none

// 展开后成员列表 + 底部输入栏
.team-run-expand
  padding: 6px 10px
  border-top: 1px solid var(--glass-border)

.team-run-empty
  font-size: 12px
  color: var(--color-text-secondary)
  text-align: center
  padding: 8px

.team-run-error
  margin-bottom: 6px
  font-size: 12px
  color: var(--color-danger)
  padding: 4px 6px
  background: rgba(229, 92, 92, 0.08)
  border-radius: 3px
  display: flex
  align-items: center
  justify-content: space-between
  gap: 8px

  &__retry
    flex-shrink: 0
</style>
