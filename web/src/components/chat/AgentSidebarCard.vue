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
    <!-- 单行布局：名称 → 状态标签 → 暂停/重试按钮 → 设置按钮 -->
    <span class="agent-sidebar-card__name ellipsis">{{ member.displayName }}</span>
    <span class="agent-sidebar-card__status" :class="`agent-sidebar-card__status--${displayStatus}`">
      <span class="agent-sidebar-card__status-dot" :class="`agent-sidebar-card__status-dot--${displayStatus}`" />
      <span class="agent-sidebar-card__status-text">{{ statusText }}</span>
    </span>
    <q-btn
      v-if="displayStatus === 'running'"
      dense
      round
      flat
      size="9px"
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
      size="9px"
      icon="play_arrow"
      :title="t('chat.agentSidebar.resume')"
      :aria-label="t('chat.agentSidebar.resume')"
      class="agent-sidebar-card__action-btn"
      @click.stop="$emit('resume', member.agentKey)"
    />
    <q-btn
      v-else-if="displayStatus === 'failed'"
      dense
      round
      flat
      size="9px"
      icon="refresh"
      :title="t('chat.agentSidebar.resume')"
      :aria-label="t('chat.agentSidebar.resume')"
      class="agent-sidebar-card__action-btn"
      @click.stop="$emit('resume', member.agentKey)"
    />
    <q-btn
      dense
      round
      flat
      size="9px"
      icon="settings"
      :title="t('chat.agentSidebar.settings')"
      :aria-label="t('chat.agentSidebar.settings')"
      class="agent-sidebar-card__action-btn agent-sidebar-card__action-btn--settings"
      @click.stop="$emit('settings', member.agentKey)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SpiritMember, SpiritTeamStatus } from '../../features/spirit/types';
import type { BlockedResult } from '../../features/chat/composables/useBlockedStatus';

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
  /** 是否选中态（用于高亮） */
  active?: boolean;
}>();

defineEmits<{
  locate: [payload: { agentKey: string; teamSessionId: string; teamId: string }];
  pause: [agentKey: string];
  resume: [agentKey: string];
  /** 点击设置按钮，由外层根据 agentKey 解析 agentId 后打开设置弹窗 */
  settings: [agentKey: string];
}>();

/** 将 SpiritMember.status + teamStatus + 阻塞信息映射为显示状态。
 * 团队进入终态（completed/failed/cancelled）时，成员卡片应同步显示对应终态。
 * 团队运行中（running/pending）时，成员未明确终态则继承运行态。
 */
const displayStatus = computed<'running' | 'blocked' | 'completed' | 'failed' | 'pending'>(() => {
  if (props.blockedInfo?.blocked) return 'blocked';

  const ts = props.teamStatus;
  if (ts === 'completed' || ts === 'archived') return 'completed';
  if (ts === 'failed') return 'failed';
  if (ts === 'cancelled' || ts === 'interrupted') return 'failed';

  const s = props.member.status;
  if (s === 'running' || s === 'executing') return 'running';
  if (s === 'blocked' || s === 'stuck') return 'blocked';
  if (s === 'completed' || s === 'ok' || s === 'done') return 'completed';
  if (s === 'failed' || s === 'error') return 'failed';
  // 团队运行中时，成员未明确终态则显示为 running（触发暂停按钮）
  if (ts === 'running' || ts === 'pending') return 'running';
  return 'pending';
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
  gap: 6px
  padding: 6px 8px
  border-radius: 8px
  cursor: pointer
  transition: background 0.15s ease, border-color 0.15s ease
  border: 1px solid transparent
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  border-left: 3px solid transparent
  // 单行布局：name flex-1 min-width-0，status 和 actions 不收缩
  min-width: 0

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

  &__name
    font-size: 12px
    font-weight: 600
    color: var(--color-text-primary)
    line-height: 1.3
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis
    flex: 1 1 auto
    min-width: 0

  &__status
    display: inline-flex
    align-items: center
    gap: 3px
    font-size: 10px
    line-height: 1.2
    padding: 1px 5px
    border-radius: 8px
    white-space: nowrap
    flex-shrink: 0
    background: color-mix(in srgb, var(--color-text-tertiary) 12%, transparent)
    color: var(--color-text-secondary)

    &--running
      background: color-mix(in srgb, var(--color-accent) 15%, transparent)
      color: var(--color-accent)

    &--blocked
      background: color-mix(in srgb, var(--color-warning) 18%, transparent)
      color: var(--color-warning)

    &--completed
      background: color-mix(in srgb, var(--color-success) 15%, transparent)
      color: var(--color-success)

    &--failed
      background: color-mix(in srgb, var(--color-danger) 15%, transparent)
      color: var(--color-danger)

    &--pending
      background: color-mix(in srgb, var(--color-text-tertiary) 12%, transparent)
      color: var(--color-text-secondary)

  &__status-text
    overflow: hidden
    text-overflow: ellipsis
    max-width: 60px

  &__status-dot
    width: 5px
    height: 5px
    border-radius: 50%
    flex-shrink: 0

    &--running
      background: currentColor
      animation: agent-card-spin 1s linear infinite

    &--blocked
      background: currentColor

    &--completed
      background: currentColor

    &--failed
      background: currentColor

    &--pending
      background: currentColor

  &__action-btn
    width: 22px
    height: 22px
    min-height: 22px
    border-radius: 6px
    color: var(--color-text-secondary)
    flex-shrink: 0

    &:hover
      background: color-mix(in srgb, var(--color-accent) 15%, transparent)
      color: var(--color-accent)

    &--settings
      color: var(--color-text-tertiary)

      &:hover
        background: color-mix(in srgb, var(--color-text-secondary) 15%, transparent)
        color: var(--color-text-primary)

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
