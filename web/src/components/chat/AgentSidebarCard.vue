<template>
  <div
    class="agent-sidebar-card"
    :class="[`agent-sidebar-card--${displayStatus}`, { 'agent-sidebar-card--active': active }]"
    :data-agent-key="member.agentKey"
    :data-team-session-id="teamSessionId"
    :data-team-status="teamStatus || ''"
    :data-team-id="teamId"
    role="button"
    tabindex="0"
    :aria-label="member.displayName"
    @click="$emit('locate', { agentKey: member.agentKey, teamSessionId, teamId })"
    @keydown.enter="$emit('locate', { agentKey: member.agentKey, teamSessionId, teamId })"
  >
    <div class="agent-sidebar-card__main">
      <AgentAvatarQ
        v-if="resolvedAvatar"
        :icon="resolvedAvatar"
        size="32px"
        avatar-class="agent-sidebar-card__avatar-img"
        :alt="member.displayName"
      />
      <div v-else class="agent-sidebar-card__avatar agent-sidebar-card__avatar--fallback">
        <q-icon name="smart_toy" size="16px" />
      </div>
      <div class="agent-sidebar-card__info col min-width-0">
        <div class="agent-sidebar-card__name ellipsis">{{ member.displayName }}</div>
        <div class="agent-sidebar-card__status ellipsis">
          <span class="agent-sidebar-card__status-dot" :class="`agent-sidebar-card__status-dot--${displayStatus}`" />
          <span v-if="displayStatus === 'blocked'" class="agent-sidebar-card__status-text">{{
            blockedInfo?.message || t('chat.agentSidebar.blockedFallback')
          }}</span>
          <span v-else>{{ statusText }}</span>
        </div>
      </div>
    </div>
    <div class="agent-sidebar-card__actions">
      <q-btn
        v-if="displayStatus === 'running'"
        dense
        round
        flat
        size="10px"
        icon="pause"
        :title="t('chat.agentSidebar.pause')"
        :aria-label="t('chat.agentSidebar.pause')"
        class="agent-sidebar-card__action-btn"
        @click.stop="$emit('pause', member.agentKey)"
      />
      <q-btn
        v-else-if="displayStatus === 'blocked'"
        dense
        round
        flat
        size="10px"
        icon="play_arrow"
        :title="t('chat.agentSidebar.resume')"
        :aria-label="t('chat.agentSidebar.resume')"
        class="agent-sidebar-card__action-btn"
        @click.stop="$emit('resume', member.agentKey)"
      />
      <q-btn
        v-if="showLifecycleCancel"
        dense
        round
        flat
        size="10px"
        icon="close"
        :title="t('chat.agentSidebar.cancel')"
        :aria-label="t('chat.agentSidebar.cancel')"
        class="agent-sidebar-card__action-btn agent-sidebar-card__action-btn--cancel"
        @click.stop="$emit('cancel', member.agentKey)"
      />
      <q-btn
        dense
        round
        flat
        size="10px"
        icon="settings"
        :title="t('chat.agentSidebar.settings')"
        :aria-label="t('chat.agentSidebar.settings')"
        class="agent-sidebar-card__action-btn agent-sidebar-card__action-btn--settings"
        @click.stop="$emit('settings', member.agentKey)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SpiritMember, SpiritTeamStatus } from '../../features/spirit/types';
import type { Agent } from '../../features/agents/types';
import type { BlockedResult } from '../../features/chat/composables/useBlockedStatus';
import AgentAvatarQ from '../avatar/AgentAvatarQ.vue';

const { t } = useI18n();

const props = defineProps<{
  member: SpiritMember;
  teamName: string;
  teamSessionId: string;
  /** 团队 ID，用于中间面板 TeamCard 定位 */
  teamId: string;
  /** 团队整体状态，用于在成员状态滞后时同步显示团队终态 */
  teamStatus?: SpiritTeamStatus;
  /** 从 useBlockedStatus 获取的阻塞信息；命中本卡片 agentKey 时才显示阻塞态 */
  blockedInfo?: BlockedResult;
  /** 当前用户可见的 Agent 配置列表，用于补充成员头像（当后端未下发 avatar_url 时） */
  agents?: Agent[];
  /** 是否选中态（用于高亮） */
  active?: boolean;
}>();

defineEmits<{
  locate: [payload: { agentKey: string; teamSessionId: string; teamId: string }];
  pause: [agentKey: string];
  resume: [agentKey: string];
  cancel: [agentKey: string];
  /** 点击设置按钮，由外层根据 agentKey 解析 agentId 后打开设置弹窗 */
  settings: [agentKey: string];
}>();

/** 将 SpiritMember.status + teamStatus + 阻塞信息映射为显示状态。
 * 团队进入终态（completed/failed/cancelled）时，成员卡片应同步显示对应终态。
 */
const displayStatus = computed<'running' | 'blocked' | 'completed' | 'failed' | 'pending'>(() => {
  if (props.blockedInfo?.blocked) return 'blocked';

  // 团队终态优先于成员空/滞后状态
  const ts = props.teamStatus;
  if (ts === 'completed' || ts === 'archived') return 'completed';
  if (ts === 'failed') return 'failed';
  if (ts === 'cancelled' || ts === 'interrupted') return 'failed';

  const s = props.member.status;
  if (s === 'running' || s === 'executing') return 'running';
  if (s === 'blocked' || s === 'stuck') return 'blocked';
  if (s === 'completed' || s === 'ok' || s === 'done') return 'completed';
  if (s === 'failed' || s === 'error') return 'failed';
  return 'pending';
});

const showLifecycleCancel = computed(() => displayStatus.value === 'running' || displayStatus.value === 'blocked');

/** 头像解析：
 *  1. 优先使用成员自己的 avatarUrl
 *  2. 后端未下发时，从用户 Agent 库中按 agentKey 查找对应 Agent 的 icon
 *  3. 都没有时返回空字符串，模板渲染 fallback（smart_toy 临时头像）
 */
const resolvedAvatar = computed(() => {
  if (props.member.avatarUrl) return props.member.avatarUrl;
  const agent = props.agents?.find((a) => a.agent_key === props.member.agentKey);
  return agent?.icon ?? '';
});

const statusText = computed(() => {
  switch (displayStatus.value) {
    case 'running':
      return t('chat.agentSidebar.statusRunning');
    case 'blocked':
      return t('chat.agentSidebar.blockedFallback');
    case 'completed':
      return t('chat.agentSidebar.statusCompleted');
    case 'failed':
      return t('chat.agentSidebar.statusFailed');
    default:
      return t('chat.agentSidebar.statusPending');
  }
});
</script>

<style lang="sass" scoped>
.agent-sidebar-card
  display: flex
  align-items: center
  gap: var(--space-2, 8px)
  padding: 6px 8px
  border-radius: 10px
  cursor: pointer
  transition: background 0.15s ease, border-color 0.15s ease
  border: 1px solid transparent
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  border-left: 3px solid transparent

  &:hover
    background: color-mix(in srgb, var(--glass-surface) 65%, transparent)
    border-color: var(--glass-border)

  &--active
    background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface))
    border-color: color-mix(in srgb, var(--color-accent) 30%, var(--glass-border))

  &--running
    border-left-color: var(--color-accent)

  &--blocked
    border-left-color: var(--color-warning)
    animation: agent-card-stuck-pulse 2s infinite

  &--completed
    border-left-color: var(--color-success)
    opacity: 0.85

  &--failed
    border-left-color: var(--color-danger)

  &--pending
    border-left-color: var(--color-icon-muted, var(--color-text-tertiary))

  &__main
    display: flex
    align-items: center
    gap: var(--space-2, 8px)
    flex: 1
    min-width: 0

  &__avatar
    display: flex
    align-items: center
    justify-content: center
    width: 32px
    height: 32px
    border-radius: 50%
    flex-shrink: 0
    color: var(--color-text-secondary)

    &--fallback
      background: color-mix(in srgb, var(--color-accent) 12%, var(--glass-surface))
      border: 1px solid color-mix(in srgb, var(--color-accent) 25%, var(--glass-border))

  &__avatar-img
    flex-shrink: 0

  &__info
    display: flex
    flex-direction: column
    gap: 2px
    min-width: 0

  &__name
    font-size: 13px
    font-weight: 600
    color: var(--color-text-primary)
    line-height: 1.3
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis

  &__status
    display: flex
    align-items: center
    gap: 4px
    font-size: 10px
    color: var(--color-text-secondary)
    line-height: 1.3
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis

  &__status-text
    overflow: hidden
    text-overflow: ellipsis

  &__status-dot
    width: 6px
    height: 6px
    border-radius: 50%
    flex-shrink: 0

    &--running
      background: var(--color-accent)
      animation: agent-card-spin 1s linear infinite

    &--blocked
      background: var(--color-warning)

    &--completed
      background: var(--color-success)

    &--failed
      background: var(--color-danger)

    &--pending
      background: var(--color-icon-muted, var(--color-text-tertiary))

  &__actions
    display: flex
    align-items: center
    gap: 2px
    flex-shrink: 0
    opacity: 0
    transition: opacity 0.2s ease

  &:hover &__actions,
  &--active &__actions,
  &--running &__actions,
  &--blocked &__actions
    opacity: 1

  &__action-btn
    width: 24px
    height: 24px
    min-height: 24px
    border-radius: 8px
    color: var(--color-text-secondary)

    &:hover
      background: color-mix(in srgb, var(--color-accent) 15%, transparent)
      color: var(--color-accent)

    &--cancel:hover
      background: color-mix(in srgb, var(--color-danger) 15%, transparent)
      color: var(--color-danger)

    &--settings
      color: var(--color-text-tertiary)

:global(.body--dark) .agent-sidebar-card__action-btn
  color: var(--color-text-primary)

@keyframes agent-card-spin
  to
    transform: rotate(360deg)

@keyframes agent-card-stuck-pulse
  0%, 100%
    box-shadow: 0 0 0 rgba(240, 155, 84, 0)
  50%
    box-shadow: 0 0 6px rgba(240, 155, 84, 0.3)
</style>
