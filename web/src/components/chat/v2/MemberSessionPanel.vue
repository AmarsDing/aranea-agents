<!-- web/src/components/chat/v2/MemberSessionPanel.vue
  2026-07-05 重构：
  - 头部三段式：左（avatar+名称+status）| 中（最新动作 icon+text）| 右（时间）
  - 折叠规则：running 默认展开，终态默认折叠，用户意图优先（userToggled）
  - 自动滚动（状态制）：running + 展开时跟随滚底；用户滚离后永不自动滚动，滚回底部即恢复（useFollowScroll）
  - 底部输入栏：空+running → stop（暂停 agent）；有文字 → send（注入消息）
  - 系统 agent 排除：__spirit__ 及 __ 前缀 agent 不显示输入栏
-->
<template>
  <div class="member-session-panel" :class="`member-session-panel--${memberSession.Status}`" :data-agent-key="memberSession.AgentKey">
    <!-- 头部：左（avatar+名称+status+时间）| 中（最新动作） -->
    <div class="member-header" @click="toggleCollapse">
      <div class="member-header__left">
        <q-icon :name="collapsed ? 'expand_more' : 'expand_less'" size="16px" class="member-header__icon" />
        <q-avatar v-if="memberSession.AvatarURL" :src="memberSession.AvatarURL" size="24px" />
        <q-icon v-else name="person" size="20px" class="member-header__avatar-fallback" />
        <span class="member-header__name">{{ memberSession.AgentName }}</span>
        <q-badge :color="statusColor" class="member-header__status">{{ statusLabel }}</q-badge>
        <span class="member-header__time">{{ formattedTime }}</span>
      </div>
      <div v-if="latestAction" class="member-header__center">
        <q-icon
          :name="latestAction.icon"
          size="14px"
          :class="{ 'member-header__action-icon--active': latestAction.active }"
        />
        <span class="member-header__action-text ellipsis">{{ latestAction.text }}</span>
      </div>
    </div>

    <div v-show="!collapsed" class="member-body">
      <!-- 错误提示 -->
      <div v-if="memberSession.Error" class="member-error">{{ memberSession.Error }}</div>

      <!-- agent 内部活动（thinking/action/reply 等 steps），max-height 300px + 滚动条 -->
      <div v-if="memberSteps.length > 0" ref="activitiesRef" class="member-activities" @scroll.passive="onScroll">
        <template v-for="step in memberSteps" :key="step.ID">
          <ThinkingBlock v-if="step.Kind === 'thinking'" :step="step" />
          <ActionBlock v-else-if="step.Kind === 'action'" :step="step" />
          <ReplyBlock v-else-if="step.Kind === 'reply'" :step="step" />
          <NoticeBlock v-else-if="step.Kind === 'notice'" :step="step" />
          <ConfirmBlock v-else-if="step.Kind === 'confirm'" :step="step" @confirm="(p) => $emit('confirm-step', p)" />
          <ErrorBlock v-else-if="step.Kind === 'error'" :step="step" />
        </template>
      </div>
      <div v-else class="member-activities-empty">
        {{ t('chat.v2.noActivities') }}
      </div>

      <!-- 底部输入栏：发送/停止双功能按钮（系统 agent 排除；仅 running 状态显示） -->
      <div v-if="canInject" class="member-input-bar">
        <q-input
          v-model="inputText"
          dense
          outlined
          :placeholder="t('chat.v2.injectPlaceholder')"
          class="member-input-bar__input"
          @keyup.enter="submitInput"
        >
          <template #append>
            <q-btn
              v-if="!inputText.trim() && memberSession.Status === 'running'"
              flat
              dense
              round
              icon="stop"
              color="negative"
              :aria-label="t('chat.v2.pause')"
              @click.stop="$emit('pause-agent', memberSession.SessionID)"
            />
            <q-btn
              v-else-if="inputText.trim()"
              flat
              dense
              round
              icon="send"
              color="primary"
              :aria-label="t('chat.v2.inject')"
              @click.stop="submitInput"
            />
          </template>
        </q-input>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, inject, watch, onMounted, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import { isSystemInternalNotice } from '../../../features/chat/noticeFilter';
import { useFollowScroll } from '../../../features/chat/composables/useFollowScroll';
import type { MemberSession } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload } from '../../../features/chat/types';
import ThinkingBlock from '../ThinkingBlock.vue';
import ActionBlock from '../ActionBlock.vue';
import ReplyBlock from '../ReplyBlock.vue';
import NoticeBlock from '../NoticeBlock.vue';
import ConfirmBlock from '../ConfirmBlock.vue';
import ErrorBlock from '../ErrorBlock.vue';

// Safe i18n wrapper — falls back to the key when the i18n plugin isn't
// installed (e.g., during unit tests without app.use(i18n)).
function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ memberSession: MemberSession }>();
const emit = defineEmits<{
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  /** Lazy-load member session steps (A.4.7). Emits chat SessionID, not entity ID. */
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
}>();

const { t } = useSafeI18n();
const store = useActivityQueries();

function emitExpandIfNeeded() {
  const sid = props.memberSession.SessionID?.trim();
  if (sid) emit('expand', [sid]);
}

// 查询 member session 对应的 agent 内部活动 steps
const memberSteps = computed(() => {
  return store.getMemberSessionSteps(props.memberSession).filter((s) => {
    if (isSystemInternalNotice(s.Kind, s.NoticeType)) return false;
    if (s.Kind === 'reply' && s.Status !== 'running' && !s.Content?.trim()) {
      return false;
    }
    return true;
  });
});

// 标题栏中间区域：最新动作摘要（图标 + 简短文本）
// 设计：取 memberSteps 最后一步，按 Kind 映射图标，running 状态显示动态指示
const latestAction = computed(() => {
  const steps = memberSteps.value;
  if (steps.length === 0) return null;
  const last = steps[steps.length - 1];
  const isActive = last.Status === 'running' || last.Status === 'tool_running' || last.Status === 'tool_blocked';
  switch (last.Kind) {
    case 'thinking':
      return { icon: 'psychology', text: t('chat.v2.latestThinking'), active: isActive };
    case 'action':
      // 工具名本身就是简短标识（shell / file_read / web_search 等）
      return { icon: 'build', text: last.ToolName || t('chat.v2.latestThinking'), active: isActive };
    case 'reply':
      return { icon: 'chat', text: t('chat.v2.latestReplying'), active: isActive };
    case 'notice':
      return { icon: 'info', text: t('chat.v2.latestNotice'), active: isActive };
    case 'confirm':
      return { icon: 'help', text: t('chat.v2.latestWaitingConfirm'), active: isActive };
    case 'error':
      return { icon: 'error', text: t('chat.v2.latestError'), active: isActive };
    default:
      return null;
  }
});

// 折叠状态：running 默认展开，终态默认折叠，用户意图优先
// 与 TeamRunCard 保持一致的 userToggled 模式
const collapsed = ref(props.memberSession.Status !== 'running' && props.memberSession.Status !== 'paused');
const userToggled = ref(false);

// 状态变化时自动展开/折叠（仅在用户未手动操作时生效）
watch(
  () => props.memberSession.Status,
  (newStatus) => {
    if (userToggled.value) return;
    if (newStatus === 'running' || newStatus === 'paused') {
      collapsed.value = false;
    } else if (newStatus === 'completed' || newStatus === 'failed' || newStatus === 'skipped') {
      collapsed.value = true;
    }
  },
);

// 响应 ChatMessageList 的 autoExpandFor（跨组件展开信号）
const autoExpandFor = inject<Ref<{ agentKey: string; teamId: string } | null>>('chat:autoExpandFor', ref(null));
watch(
  autoExpandFor,
  (cmd) => {
    if (!cmd || userToggled.value) return;
    if (props.memberSession.AgentKey === cmd.agentKey) collapsed.value = false;
  },
  { immediate: true },
);

function toggleCollapse() {
  userToggled.value = true;
  const next = !collapsed.value;
  collapsed.value = next;
  if (!next) emitExpandIfNeeded();
}

onMounted(() => {
  // Default-expanded (running/paused): lazy-load steps on mount (A.4.7)
  if (!collapsed.value) emitExpandIfNeeded();
});

// 自动滚动（状态制）：running + 展开时跟随滚底；用户滚离后永不自动滚动，滚回底部即恢复
const activitiesRef = ref<HTMLElement | null>(null);
const autoScrollEnabled = computed(() => !collapsed.value && props.memberSession.Status === 'running');
// 内容签名：steps 数量 + 最后一步 ID + 内容长度（检测流式增长）
const contentSignature = computed(() => {
  const steps = memberSteps.value;
  if (steps.length === 0) return '0:';
  const last = steps[steps.length - 1];
  return `${steps.length}:${last.ID}:${last.Content?.length ?? 0}`;
});
const { onScroll } = useFollowScroll({
  scrollEl: activitiesRef,
  contentSignature,
  enabled: autoScrollEnabled,
});

// 底部输入栏：发送/停止双功能按钮
// - 输入为空 + running → stop 按钮 → emit('pause-agent', sessionId)
// - 输入有文字 → send 按钮 → emit('inject-agent', { sessionId, message })
const inputText = ref('');
function submitInput() {
  const text = inputText.value.trim();
  if (!text || !canInject.value) return;
  emit('inject-agent', { sessionId: props.memberSession.SessionID, message: text });
  inputText.value = '';
}

// 系统 agent 排除：__spirit__ 及 __ 前缀不显示输入栏
const isSystemAgent = computed(() => {
  const key = props.memberSession.AgentKey || '';
  return key === '__spirit__' || key.startsWith('__');
});

// 输入栏在 running / paused 状态显示（用户 spec：agent 正在执行时显示；
// paused 时保留输入栏以便注入新指令恢复执行——后端 resume 依赖新消息重触发，
// 见 internal/service/chat_pause.go）
const canInject = computed(
  () => !isSystemAgent.value && (props.memberSession.Status === 'running' || props.memberSession.Status === 'paused'),
);

// 状态映射：running/paused/completed/failed/cancelled
const statusColor = computed(
  () =>
    ({
      pending: 'grey',
      running: 'blue',
      paused: 'warning',
      completed: 'green',
      failed: 'red',
      cancelled: 'grey-6',
      skipped: 'grey-5',
    })[props.memberSession.Status] || 'grey',
);

const statusLabel = computed(() => {
  const map: Record<string, string> = {
    pending: t('chat.v2.statusPending'),
    running: t('chat.v2.statusRunning'),
    paused: t('chat.v2.statusPaused'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    cancelled: t('chat.v2.statusCancelled'),
    skipped: t('chat.v2.statusSkipped'),
  };
  return map[props.memberSession.Status] || props.memberSession.Status;
});

const formattedTime = computed(() => {
  const raw = props.memberSession.StartedAt;
  if (!raw) return '';
  const d = new Date(raw);
  if (isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
});
</script>

<style lang="sass" scoped>
// 2026-07-22 左边线体系：去边框+背景，3px 左状态线 + margin-left 14px（挂在团队线之下形成视觉树）
.member-session-panel
  margin: 4px 0 4px 14px
  padding-left: 10px
  border-left: 3px solid var(--glass-border)

  &--running
    border-left-color: var(--color-accent)
    animation: member-border-pulse 1.6s ease-in-out infinite

  &--paused
    border-left-color: var(--color-warning)

  &--completed
    border-left-color: var(--color-success)

  &--failed
    border-left-color: var(--color-danger)

  &--cancelled, &--skipped
    border-left-color: var(--color-text-tertiary)

@keyframes member-border-pulse
  0%, 100%
    border-left-color: var(--color-accent)
  50%
    border-left-color: color-mix(in srgb, var(--color-accent) 35%, transparent)

.member-header
  display: flex
  align-items: center
  padding: 4px 8px
  cursor: pointer
  user-select: none
  gap: 8px

  &:hover
    background: var(--glass-surface-hover)

  // 三段式布局：左（固定）| 中（弹性增长）| 右（固定）
  &__left
    display: flex
    align-items: center
    gap: 6px
    flex: 0 0 auto
    min-width: 0

  &__center
    display: flex
    align-items: center
    gap: 4px
    flex: 1 1 auto
    min-width: 0
    overflow: hidden
    color: var(--color-text-tertiary)
    font-size: 11px

  &__action-text
    color: var(--color-text-tertiary)

  &__action-icon--active
    animation: member-action-pulse 1.5s ease-in-out infinite
    color: var(--color-accent)

  &__icon
    color: var(--color-text-secondary)

  &__avatar-fallback
    color: var(--color-text-secondary)

  &__name
    font-size: 12px
    font-weight: 500
    color: var(--color-text-primary)
    max-width: 140px
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__status
    margin-left: 2px

  &__right
    display: flex
    align-items: center
    flex: 0 0 auto

  &__time
    font-size: 11px
    color: var(--color-icon-muted)
    font-variant-numeric: tabular-nums

@keyframes member-action-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.4

.member-body
  padding: 6px 8px

.member-activities
  margin-bottom: 8px
  // 2026-07-05: 限制活动列表最大高度 300px，超出显示滚动条
  max-height: 300px
  overflow-y: auto
  // 滚动条样式（细条，符合玻璃主题）
  &::-webkit-scrollbar
    width: 6px
  &::-webkit-scrollbar-thumb
    background: var(--glass-border)
    border-radius: 3px
  &::-webkit-scrollbar-track
    background: transparent

.member-activities-empty
  font-size: 12px
  color: var(--color-icon-muted)
  text-align: center
  padding: 8px

.member-error
  font-size: 11px
  color: var(--color-danger)
  margin-bottom: 6px
  padding: 4px 6px
  background: rgba(229, 92, 92, 0.08)
  border-radius: 3px

// 底部输入栏：发送/停止双功能按钮
.member-input-bar
  margin-top: 8px
  padding-top: 8px
  border-top: 1px dashed var(--glass-border)

  &__input
    :deep(.q-field__append)
      padding: 0 4px
</style>
