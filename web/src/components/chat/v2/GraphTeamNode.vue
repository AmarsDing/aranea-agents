<!-- web/src/components/chat/v2/GraphTeamNode.vue
  Graph 富卡片节点（方案A）：一个 PlanStep = 一个团队任务卡片。
  结构：头部（状态点 + 标题 + 状态文本）/ 成员行（状态点 + 名称 + 耗时或状态）/ 底部进度条。
  - 成员解析链路：GraphNode → TeamStage（TeamStageID 优先，DagNodeID 兜底）→ TeamRun → MemberSession
  - 点击成员行 → emit select-member（父组件弹 MemberSessionDialog）；点击头部 → emit select
  - 卡片尺寸与 graphTeamNodeUi 常量保持一致（布局算法 heightOf 依赖）
-->
<template>
  <div
    :class="[
      'graph-team-node',
      `graph-team-node--${node.Status}`,
      {
        'graph-team-node--selected': isSelected,
        'graph-team-node--highlighted': isHighlighted,
        'graph-team-node--dimmed': isDimmed,
        'graph-team-node--enter': entranceDelayMs !== undefined,
        'graph-team-node--just-completed': justCompleted,
      },
    ]"
    :style="nodeStyle"
    @mouseenter="$emit('hover', node.ID)"
    @mouseleave="$emit('hover', null)"
  >
    <!-- 头部：状态点 + 标题 + 状态文本 -->
    <div class="gtn-header" @click="onHeaderClick">
      <span class="gtn-header__dot" :class="`gtn-header__dot--${nodeTone}`" />
      <span class="gtn-header__label" :title="node.Label">{{ node.Label }}</span>
      <span class="gtn-header__status" :class="`gtn-header__status--${nodeTone}`">{{ nodeStatusLabel }}</span>
    </div>

    <!-- 成员行：状态点 + 名称 + 耗时/状态（无成员时 1 行占位，保持卡片高度可预期） -->
    <div class="gtn-members">
      <template v-if="members.length > 0">
        <div
          v-for="ms in members"
          :key="ms.ID"
          class="gtn-member"
          :class="{ 'gtn-member--active': ms.Status === 'running' }"
          :data-member-id="ms.ID"
          @click.stop="onMemberClick(ms)"
        >
          <span
            class="gtn-member__dot"
            :class="[
              `gtn-member__dot--${memberToneOf(ms)}`,
              { 'gtn-member__dot--ripple': ms.Status === 'running' },
            ]"
          />
          <span class="gtn-member__name" :title="memberName(ms)">{{ memberName(ms) }}</span>
          <span class="gtn-member__meta" :class="`gtn-member__meta--${memberToneOf(ms)}`">{{ memberMeta(ms) }}</span>
        </div>
      </template>
      <div v-else class="gtn-member gtn-member--empty">
        <span class="gtn-member__dot gtn-member__dot--muted" />
        <span class="gtn-member__name">{{ t('chat.v2.noMembers') }}</span>
      </div>
    </div>

    <!-- 状态行（2026-07-27 F）：固定一行占位——错误摘要（failed 优先）/ 当前动作（running） -->
    <div class="gtn-status-row">
      <div v-if="statusRow?.kind === 'error'" class="gtn-error" :title="statusRow.title">
        <q-icon name="error" size="12px" class="gtn-status-row__icon" />
        <span class="gtn-status-row__text">{{ statusRow.text }}</span>
      </div>
      <div v-else-if="statusRow" class="gtn-action" :title="statusRow.text">
        <q-icon
          :name="statusRow.icon"
          size="12px"
          class="gtn-status-row__icon gtn-status-row__icon--active"
        />
        <span class="gtn-status-row__text">{{ statusRow.text }}</span>
      </div>
    </div>

    <!-- 底部进度条：完成成员 / 总成员 -->
    <div class="gtn-progress">
      <div class="gtn-progress__track">
        <div class="gtn-progress__fill" :style="{ width: `${progressPct}%` }" />
      </div>
      <span class="gtn-progress__text">{{ progressCompleted }}/{{ members.length }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useGraphNodeTeam } from '../../../features/chat/composables/useGraphNodeTeam';
import type { GraphNode, MemberSession } from '../../../features/chat/v2Types';
import type { NodePosition } from '../../../features/chat/composables/usePlanDAGLayout';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import {
  GTN_WIDTH,
  graphTeamNodeHeight,
  graphTeamNodeTone,
  graphTeamNodeStatusText,
  formatMemberDuration,
  type GraphTeamNodeTone,
} from './graphTeamNodeUi';

function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{
  node: GraphNode;
  pos: NodePosition;
  isSelected?: boolean;
  isHighlighted?: boolean;
  isDimmed?: boolean;
  /** 级联入场动画延迟（ms）。undefined = 不播入场动画（replay / 早已存在）。 */
  entranceDelayMs?: number;
}>();

const emit = defineEmits<{
  select: [id: string];
  hover: [id: string | null];
  /** 点击成员行：父组件打开 MemberSessionDialog 展示该成员对话内容。 */
  'select-member': [member: MemberSession];
}>();

const { t } = useSafeI18n();
const { membersOf } = useGraphNodeTeam();

// ── 成员解析：GraphNode → TeamStage → TeamRun → MemberSession（共享 composable） ──
const members = computed<MemberSession[]>(() => membersOf(props.node));

const queries = useActivityQueries();

// ── 状态行（2026-07-27 F）：错误摘要（failed 成员优先）/ 当前动作（running/paused 成员最新 step） ──
const statusRow = computed(() =>
  graphTeamNodeStatusText(
    members.value,
    (ms) => {
      const steps = queries.getMemberSessionSteps(ms);
      const last = steps.length > 0 ? steps[steps.length - 1] : null;
      return last ? { Kind: last.Kind, Status: last.Status, ToolName: last.ToolName } : null;
    },
    {
      running: t('chat.v2.statusRunning'),
      paused: t('chat.v2.statusPaused'),
      failed: t('chat.v2.statusFailed'),
    },
  ),
);

// ── 布局：高度与 graphTeamNodeHeight 保持一致（DAG heightOf 的单一事实源） ──
const nodeStyle = computed(() => {
  const style: Record<string, string> = {
    left: `${props.pos.x}px`,
    top: `${props.pos.y}px`,
    width: `${GTN_WIDTH}px`,
    height: `${graphTeamNodeHeight(members.value.length)}px`,
  };
  if (props.entranceDelayMs !== undefined && !justCompleted.value) {
    style.animationDelay = `${props.entranceDelayMs}ms`;
  }
  return style;
});

// ── 状态转换动画：running → completed 瞬间绿色呼吸（与旧 GraphNode 一致） ──
const justCompleted = ref(false);
let prevStatus = props.node.Status;
let justCompletedTimer: ReturnType<typeof setTimeout> | undefined;

watch(
  () => props.node.Status,
  (cur) => {
    if (prevStatus === 'running' && cur === 'completed') {
      justCompleted.value = true;
      clearTimeout(justCompletedTimer);
      justCompletedTimer = setTimeout(() => {
        justCompleted.value = false;
      }, 1000);
    }
    prevStatus = cur;
  },
);

// ── 运行中成员耗时实时刷新（1s 滴答，节点终态后停止） ──
const now = ref(Date.now());
let tickTimer: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  if (props.node.Status === 'running') {
    tickTimer = setInterval(() => {
      now.value = Date.now();
    }, 1000);
  }
});

watch(
  () => props.node.Status,
  (cur) => {
    if (cur === 'running' && !tickTimer) {
      tickTimer = setInterval(() => {
        now.value = Date.now();
      }, 1000);
    } else if (cur !== 'running' && tickTimer) {
      clearInterval(tickTimer);
      tickTimer = null;
    }
  },
);

onUnmounted(() => {
  if (tickTimer) {
    clearInterval(tickTimer);
    tickTimer = null;
  }
  clearTimeout(justCompletedTimer);
});

// ── 展示派生 ──
const nodeTone = computed<GraphTeamNodeTone>(() => graphTeamNodeTone(props.node.Status));

const nodeStatusLabel = computed(() => {
  const map: Record<string, string> = {
    pending: t('chat.v2.statusPending'),
    running: t('chat.v2.statusRunning'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    interrupted: t('chat.v2.statusInterrupted'),
  };
  return map[props.node.Status] || props.node.Status;
});

function memberToneOf(ms: MemberSession): GraphTeamNodeTone {
  return graphTeamNodeTone(ms.Status);
}

function memberName(ms: MemberSession): string {
  return ms.AgentName || ms.AgentKey;
}

function memberMeta(ms: MemberSession): string {
  const startMs = Date.parse(ms.StartedAt);
  const durationFrom = Number.isNaN(startMs) ? null : startMs;
  switch (ms.Status) {
    case 'completed': {
      const endMs = ms.FinishedAt ? Date.parse(ms.FinishedAt) : NaN;
      if (durationFrom !== null && !Number.isNaN(endMs)) return formatMemberDuration(endMs - durationFrom);
      return t('chat.v2.statusCompleted');
    }
    case 'running':
    case 'paused': {
      if (durationFrom !== null) return formatMemberDuration(now.value - durationFrom);
      return t(ms.Status === 'paused' ? 'chat.v2.statusPaused' : 'chat.v2.statusRunning');
    }
    case 'failed':
      return t('chat.v2.statusFailed');
    case 'skipped':
      return t('chat.v2.statusSkipped');
    default:
      return t('chat.v2.statusPending');
  }
}

// ── 进度：完成成员 / 总成员；无成员时由节点状态推导 ──
const progressCompleted = computed(() => members.value.filter((ms) => ms.Status === 'completed').length);
const progressPct = computed(() => {
  const total = members.value.length;
  if (total > 0) return Math.round((progressCompleted.value / total) * 100);
  return props.node.Status === 'completed' ? 100 : 0;
});

// ── 交互 ──
function onHeaderClick() {
  emit('select', props.node.ID);
}

function onMemberClick(ms: MemberSession) {
  emit('select-member', ms);
}
</script>

<style lang="sass" scoped>
.graph-team-node
  position: absolute
  display: flex
  flex-direction: column
  padding: 10px 12px
  border: 2px solid var(--node-accent, var(--glass-border))
  border-radius: 10px
  background: var(--glass-elevated)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  box-shadow: 0 2px 10px rgb(0 0 0 / 8%)
  box-sizing: border-box
  overflow: hidden
  transition: opacity 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease

  &:hover
    transform: translateY(-2px)
    box-shadow: 0 6px 20px color-mix(in srgb, var(--node-accent) 35%, rgb(0 0 0 / 10%))

// 状态色（设计文档 §3.7.5）
.graph-team-node--pending
  --node-accent: var(--color-text-tertiary)

.graph-team-node--running
  --node-accent: var(--q-primary)
  background: linear-gradient(160deg, color-mix(in srgb, var(--q-primary) 7%, transparent), transparent 60%), var(--glass-elevated)
  animation: gtn-pulse 2s ease-in-out infinite

.graph-team-node--completed
  --node-accent: var(--color-success)
  background: linear-gradient(160deg, color-mix(in srgb, var(--color-success) 8%, transparent), transparent 60%), var(--glass-elevated)

.graph-team-node--failed
  --node-accent: var(--color-danger)
  background: linear-gradient(160deg, color-mix(in srgb, var(--color-danger) 9%, transparent), transparent 60%), var(--glass-elevated)
  animation: gtn-shake 0.4s ease-in-out 2

.graph-team-node--interrupted
  --node-accent: var(--color-warning)

.graph-team-node--selected,
.graph-team-node--highlighted
  box-shadow: 0 0 14px color-mix(in srgb, var(--node-accent) 40%, transparent)

.graph-team-node--dimmed
  opacity: 30%

// ── 头部 ──
.gtn-header
  display: flex
  align-items: center
  gap: 6px
  height: 20px
  margin-bottom: 8px
  cursor: pointer
  user-select: none

  &__dot
    flex: 0 0 auto
    width: 8px
    height: 8px
    border-radius: 50%

  &__label
    flex: 1 1 auto
    min-width: 0
    color: var(--color-text-primary)
    font-size: 12px
    font-weight: 600
    line-height: 1.3
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis

  &__status
    flex: 0 0 auto
    padding: 2px 7px
    border-radius: 999px
    font-size: 9px
    font-weight: 600
    line-height: 1.2

// ── 成员行 ──
.gtn-members
  display: flex
  flex-direction: column
  gap: 6px
  margin-bottom: 10px

.gtn-member
  display: flex
  align-items: center
  gap: 6px
  height: 24px
  padding: 0 4px
  border-radius: 6px
  cursor: pointer
  user-select: none
  transition: background 0.15s ease, transform 0.15s ease

  &:hover
    background: var(--glass-surface-hover)
    transform: translateX(2px)

  &--active
    background: color-mix(in srgb, var(--q-primary) 7%, transparent)

  &--empty
    cursor: default
    color: var(--color-text-tertiary)

    &:hover
      background: transparent

  &__dot
    flex: 0 0 auto
    width: 8px
    height: 8px
    border-radius: 50%

  &__name
    flex: 1 1 auto
    min-width: 0
    color: var(--color-text-primary)
    font-size: 11px
    font-weight: 500
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis

  &--empty &__name
    color: var(--color-text-tertiary)

  &__meta
    flex: 0 0 auto
    font-size: 10px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums

// ── 底部进度条 ──
.gtn-progress
  display: flex
  align-items: center
  gap: 8px
  height: 6px

  &__track
    flex: 1 1 auto
    height: 6px
    border-radius: 3px
    background: color-mix(in srgb, var(--node-accent) 14%, transparent)
    overflow: hidden

  &__fill
    position: relative
    height: 100%
    border-radius: 3px
    background: linear-gradient(90deg, color-mix(in srgb, var(--node-accent) 72%, white), var(--node-accent))
    overflow: hidden
    transition: width 0.3s ease

  .graph-team-node--running &__fill
    animation: gtn-progress-breathe 1.6s ease-in-out infinite

  // running 时光泽扫过（动效增强）
  .graph-team-node--running &__fill::after
    content: ''
    position: absolute
    inset: 0
    background: linear-gradient(90deg, transparent, rgb(255 255 255 / 45%), transparent)
    animation: gtn-progress-shimmer 1.8s linear infinite

  &__text
    flex: 0 0 auto
    font-size: 9px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums

// ── 色调（dot / 状态文本共用） ──
.gtn-header__dot--accent,
.gtn-member__dot--accent
  background: var(--q-primary)

.gtn-header__dot--success,
.gtn-member__dot--success
  background: var(--color-success)

.gtn-header__dot--danger,
.gtn-member__dot--danger
  background: var(--color-danger)

.gtn-header__dot--warning,
.gtn-member__dot--warning
  background: var(--color-warning)

.gtn-header__dot--muted,
.gtn-member__dot--muted
  background: var(--color-text-tertiary)

.gtn-header__status--accent
  color: var(--q-primary)
  background: color-mix(in srgb, var(--q-primary) 13%, transparent)

.gtn-header__status--success
  color: var(--color-success)
  background: color-mix(in srgb, var(--color-success) 14%, transparent)

.gtn-header__status--danger
  color: var(--color-danger)
  background: color-mix(in srgb, var(--color-danger) 13%, transparent)

.gtn-header__status--warning
  color: var(--color-warning)
  background: color-mix(in srgb, var(--color-warning) 15%, transparent)

.gtn-header__status--muted
  color: var(--color-text-tertiary)
  background: color-mix(in srgb, var(--color-text-tertiary) 12%, transparent)

// 状态点光晕（同色 glow，增强状态点辨识度）
.gtn-header__dot--accent,
.gtn-member__dot--accent
  box-shadow: 0 0 6px color-mix(in srgb, var(--q-primary) 65%, transparent)

.gtn-header__dot--success,
.gtn-member__dot--success
  box-shadow: 0 0 6px color-mix(in srgb, var(--color-success) 60%, transparent)

.gtn-header__dot--danger,
.gtn-member__dot--danger
  box-shadow: 0 0 6px color-mix(in srgb, var(--color-danger) 60%, transparent)

.gtn-header__dot--warning,
.gtn-member__dot--warning
  box-shadow: 0 0 6px color-mix(in srgb, var(--color-warning) 60%, transparent)

// running 成员状态点波纹扩散（动效增强）
.gtn-member__dot--ripple
  position: relative

  &::after
    content: ''
    position: absolute
    inset: -3px
    border-radius: 50%
    border: 1.5px solid var(--q-primary)
    animation: gtn-dot-ripple 1.6s ease-out infinite

@keyframes gtn-dot-ripple
  from
    transform: scale(0.6)
    opacity: 0.9
  to
    transform: scale(1.9)
    opacity: 0

.gtn-member__meta--danger
  color: var(--color-danger)

.gtn-member__meta--warning
  color: var(--color-warning)

// ── 状态行（2026-07-27 F）：当前动作 / 错误摘要，固定一行占位 ──
.gtn-status-row
  display: flex
  align-items: center
  height: 24px
  margin-bottom: 6px
  font-size: 10px

.gtn-action,
.gtn-error
  display: flex
  align-items: center
  gap: 4px
  min-width: 0
  max-width: 100%
  padding: 2px 6px
  border-radius: 6px

.gtn-action
  color: var(--q-primary)
  background: color-mix(in srgb, var(--q-primary) 10%, transparent)

.gtn-error
  color: var(--color-danger)
  background: color-mix(in srgb, var(--color-danger) 10%, transparent)

.gtn-status-row__icon
  flex: 0 0 auto

.gtn-status-row__icon--active
  animation: gtn-action-pulse 1.5s ease-in-out infinite

.gtn-status-row__text
  min-width: 0
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

@keyframes gtn-action-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.45

@keyframes gtn-progress-shimmer
  from
    transform: translateX(-100%)
  to
    transform: translateX(100%)

// ── 动画 ──
@keyframes gtn-pulse
  0%, 100%
    box-shadow: 0 0 6px color-mix(in srgb, var(--node-accent) 25%, transparent)
  50%
    box-shadow: 0 0 16px color-mix(in srgb, var(--node-accent) 55%, transparent)

@keyframes gtn-shake
  0%, 100%
    transform: translateX(0)
  25%
    transform: translateX(-2px)
  75%
    transform: translateX(2px)

@keyframes gtn-progress-breathe
  0%, 100%
    opacity: 1
  50%
    opacity: 0.55

.graph-team-node--enter
  animation: gtn-enter 0.45s cubic-bezier(0.22, 1, 0.36, 1) both

.graph-team-node--enter.graph-team-node--running
  animation: gtn-enter 0.45s cubic-bezier(0.22, 1, 0.36, 1) both, gtn-pulse 2s ease-in-out infinite

.graph-team-node--enter.graph-team-node--failed
  animation: gtn-enter 0.45s cubic-bezier(0.22, 1, 0.36, 1) both, gtn-shake 0.4s ease-in-out 2

@keyframes gtn-enter
  from
    opacity: 0
    transform: scale(0.6) translateY(8px)
  to
    opacity: 1
    transform: scale(1) translateY(0)

.graph-team-node--just-completed
  animation: gtn-complete-flash 0.9s ease-out

@keyframes gtn-complete-flash
  0%
    box-shadow: 0 0 24px color-mix(in srgb, var(--color-success) 65%, transparent)
  100%
    box-shadow: 0 2px 10px rgb(0 0 0 / 8%)

@media (prefers-reduced-motion: reduce)
  .graph-team-node--running,
  .graph-team-node--failed,
  .graph-team-node--enter,
  .graph-team-node--just-completed,
  .graph-team-node--running .gtn-progress__fill,
  .graph-team-node--running .gtn-progress__fill::after,
  .gtn-member__dot--ripple::after,
  .gtn-status-row__icon--active
    animation: none

  .graph-team-node:hover,
  .gtn-member:hover
    transform: none
</style>
