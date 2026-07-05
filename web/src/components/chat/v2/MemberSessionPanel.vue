<!-- web/src/components/chat/v2/MemberSessionPanel.vue
  2026-07-04 完善：简化布局 + 操作按钮 + 注入对话框（需求 §A.4.4）
  - 头部 80%：avatar + 名称 + status badge + 创建时间
  - 尾部 20%：注入对话框 + 操作按钮（暂停/恢复/取消/重试）
  - 状态映射：running/paused/completed/failed/cancelled
  - 系统 agent 排除：__spirit__ 及 __ 前缀 agent 不显示操作按钮和对话框
  - 始终默认折叠，与 team-card 一致
  2026-07-04 修复：
  - 渲染 agent 内部活动（thinking/action/reply 等 steps）
  - CSS 改用 glass tokens 符合主题
-->
<template>
  <div class="member-session-panel" :data-agent-key="memberSession.AgentKey">
    <!-- 头部 80%：avatar + 名称 + status badge + 创建时间 -->
    <div class="member-header" @click="toggleCollapse">
      <div class="member-header__left">
        <q-icon :name="collapsed ? 'expand_more' : 'expand_less'" size="16px" class="member-header__icon" />
        <q-avatar v-if="memberSession.AvatarURL" :src="memberSession.AvatarURL" size="24px" />
        <q-icon v-else name="person" size="20px" class="member-header__avatar-fallback" />
        <span class="member-header__name">{{ memberSession.AgentName }}</span>
        <q-badge :color="statusColor" class="member-header__status">{{ statusLabel }}</q-badge>
      </div>
      <div class="member-header__right">
        <span class="member-header__time">{{ formattedTime }}</span>
      </div>
    </div>

    <div v-show="!collapsed" class="member-body">
      <!-- 错误提示 -->
      <div v-if="memberSession.Error" class="member-error">{{ memberSession.Error }}</div>

      <!-- 2026-07-04 修复：渲染 agent 内部活动（thinking/action/reply 等 steps） -->
      <div v-if="memberSteps.length > 0" class="member-activities">
        <template v-for="step in memberSteps" :key="step.ID">
          <ThinkingBlock v-if="step.Kind === 'thinking'" :step="step" />
          <ActionBlock v-else-if="step.Kind === 'action'" :step="step" />
          <ReplyBlock v-else-if="step.Kind === 'reply'" :step="step" />
          <NoticeBlock v-else-if="step.Kind === 'notice'" :step="step" />
          <ConfirmBlock v-else-if="step.Kind === 'confirm'" :step="step" />
          <ErrorBlock v-else-if="step.Kind === 'error'" :step="step" />
        </template>
      </div>
      <div v-else class="member-activities-empty">
        {{ t('chat.v2.noActivities') }}
      </div>

      <!-- 尾部 20%：注入对话框 + 操作按钮（系统 agent 排除） -->
      <div v-if="!isSystemAgent" class="member-actions">
        <q-input
          v-model="injectText"
          dense
          outlined
          :placeholder="t('chat.v2.injectPlaceholder')"
          class="member-inject-input"
          @keyup.enter="submitInject"
        />
        <q-btn
          v-if="canInject"
          flat
          dense
          size="sm"
          :label="t('chat.v2.inject')"
          color="primary"
          :disable="!injectText.trim()"
          @click="submitInject"
        />
        <q-btn
          v-if="canPause"
          flat
          dense
          size="sm"
          :label="t('chat.v2.pause')"
          icon="pause"
          color="warning"
          @click="$emit('pause', memberSession.ID)"
        />
        <q-btn
          v-if="canResume"
          flat
          dense
          size="sm"
          :label="t('chat.v2.resume')"
          icon="play_arrow"
          color="positive"
          @click="$emit('resume', memberSession.ID)"
        />
        <q-btn
          v-if="canCancel"
          flat
          dense
          size="sm"
          :label="t('chat.v2.cancel')"
          icon="stop"
          color="negative"
          @click="$emit('cancel', memberSession.ID)"
        />
        <q-btn
          v-if="canRetry"
          flat
          dense
          size="sm"
          :label="t('chat.v2.retry')"
          icon="refresh"
          color="primary"
          @click="$emit('retry', memberSession.ID)"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, inject, watch, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { isSystemInternalNotice } from '../../../features/chat/noticeFilter';
import type { MemberSession } from '../../../features/chat/v2Types';
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
  pause: [id: string];
  resume: [id: string];
  cancel: [id: string];
  retry: [id: string];
  inject: [id: string, text: string];
}>();

const { t } = useSafeI18n();
const store = useChatActivityStore();

// 2026-07-04 修复：查询 member session 对应的 agent 内部活动 steps。
// 通过 AuthorAgentKey + TaskID 间接匹配（Step 无 MemberSessionID 直接外键）。
const memberSteps = computed(() => {
  return store
    .getMemberSessionSteps(props.memberSession)
    .filter((s) => {
      // 过滤系统内部通知（context_usage 等）
      if (isSystemInternalNotice(s.Kind, s.NoticeType)) return false;
      // 过滤空 reply step
      if (s.Kind === 'reply' && s.Status !== 'running' && !s.Content?.trim()) {
        return false;
      }
      return true;
    });
});

// 折叠状态：默认折叠（与 team-card 一致，需求 §A.4.4）
const collapsed = ref(true);

const autoExpandFor = inject<Ref<{ agentKey: string; teamId: string } | null>>('chat:autoExpandFor', ref(null));
watch(
  autoExpandFor,
  (cmd) => {
    if (!cmd) return;
    if (props.memberSession.AgentKey === cmd.agentKey) collapsed.value = false;
  },
  { immediate: true },
);

function toggleCollapse() {
  collapsed.value = !collapsed.value;
}

// 注入对话框
const injectText = ref('');
function submitInject() {
  const text = injectText.value.trim();
  if (!text || !canInject.value) return;
  emit('inject', props.memberSession.ID, text);
  injectText.value = '';
}

// 系统 agent 排除：__spirit__ 及 __ 前缀不显示操作按钮和对话框（需求 §A.4.4）
const isSystemAgent = computed(() => {
  const key = props.memberSession.AgentKey || '';
  return key === '__spirit__' || key.startsWith('__');
});

// 状态映射：running/paused/completed/failed/cancelled（需求 §A.4.4）
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

// 按钮可见性（按状态切换，需求 §A.4.4）
const canPause = computed(() => props.memberSession.Status === 'running');
const canResume = computed(() => props.memberSession.Status === 'paused');
const canCancel = computed(() => props.memberSession.Status === 'running' || props.memberSession.Status === 'paused');
const canRetry = computed(() => props.memberSession.Status === 'failed' || props.memberSession.Status === 'cancelled');
const canInject = computed(() => props.memberSession.Status === 'running' || props.memberSession.Status === 'paused');

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
.member-session-panel
  border: 1px solid var(--glass-border)
  border-radius: 4px
  margin: 4px 0
  background: var(--glass-surface)

.member-header
  display: flex
  align-items: center
  justify-content: space-between
  padding: 4px 8px
  cursor: pointer
  user-select: none

  &:hover
    background: var(--glass-surface-hover)

  &__left
    display: flex
    align-items: center
    gap: 6px
    flex: 1
    min-width: 0

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

  &__time
    font-size: 11px
    color: var(--color-icon-muted)
    font-variant-numeric: tabular-nums

.member-body
  padding: 6px 8px

.member-activities
  margin-bottom: 8px

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

.member-actions
  display: flex
  align-items: center
  gap: 4px
  padding-top: 6px
  border-top: 1px solid var(--glass-border)

.member-inject-input
  flex: 1
  min-width: 0
</style>
