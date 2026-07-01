<template>
  <div
    class="agent-sidebar-card"
    :class="`agent-sidebar-card--${displayStatus}`"
    :data-agent-key="member.agentKey"
    :data-team-session-id="teamSessionId"
    :data-team-status="teamStatus || ''"
    :data-team-id="teamId"
    @click="$emit('locate', { agentKey: member.agentKey, teamSessionId, teamId })"
  >
    <div class="agent-sidebar-card__main">
      <span class="agent-sidebar-card__avatar">
        <ResolvedAvatarImg
          v-if="resolvedAvatar && shouldRenderAgentAvatarImage(resolvedAvatar)"
          :icon="resolvedAvatar"
          :alt="member.displayName"
        />
        <q-icon
          v-else-if="resolvedAvatar && quasarAvatarIconForAgentField(resolvedAvatar)"
          :name="quasarAvatarIconForAgentField(resolvedAvatar)"
          size="14px"
        />
        <template v-else>{{ memberInitial }}</template>
      </span>
      <div class="agent-sidebar-card__info">
        <div class="agent-sidebar-card__name">
          {{ member.displayName }}
        </div>
        <div class="agent-sidebar-card__team">{{ teamName }}</div>
      </div>
      <span class="agent-sidebar-card__status">
        <!-- 执行中：CSS 转圈动画 -->
        <span v-if="displayStatus === 'running'" class="agent-sidebar-card__spinner" />
        <!-- 阻塞：黄色 ⚠ + 阻塞原因 -->
        <span v-else-if="displayStatus === 'blocked'" class="agent-sidebar-card__blocked-badge"
          >⚠ {{ statusText }}</span
        >
        <!-- 已完成：绿色标签 -->
        <span v-else-if="displayStatus === 'completed'" class="agent-sidebar-card__completed-tag">{{
          t('chat.agentSidebar.completedLabel')
        }}</span>
        <!-- 失败：红色标签 -->
        <span v-else-if="displayStatus === 'failed'" class="agent-sidebar-card__failed-badge">{{
          t('chat.agentSidebar.failedLabel')
        }}</span>
      </span>
    </div>
    <!-- 操作按钮：设置入口始终显示；运行中/阻塞时额外显示生命周期控制 -->
    <div class="agent-sidebar-card__actions">
      <button
        v-if="displayStatus === 'running'"
        class="agent-sidebar-card__btn agent-sidebar-card__btn--pause"
        @click.stop="$emit('pause', member.agentKey)"
      >
        ⏸ {{ t('chat.agentSidebar.pause') }}
      </button>
      <button
        v-else-if="displayStatus === 'blocked'"
        class="agent-sidebar-card__btn agent-sidebar-card__btn--resume"
        @click.stop="$emit('resume', member.agentKey)"
      >
        ▶ {{ t('chat.agentSidebar.resume') }}
      </button>
      <button
        v-if="showLifecycleCancel"
        class="agent-sidebar-card__btn agent-sidebar-card__btn--cancel"
        @click.stop="$emit('cancel', member.agentKey)"
      >
        ✕ {{ t('chat.agentSidebar.cancel') }}
      </button>
      <button
        class="agent-sidebar-card__btn agent-sidebar-card__btn--settings"
        :title="t('chat.agentSidebar.settings')"
        @click.stop="$emit('settings', member.agentKey)"
      >
        ⚙
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SpiritMember, SpiritTeamStatus } from '../../features/spirit/types';
import type { Agent } from '../../features/agents/types';
import type { BlockedResult } from '../../features/chat/composables/useBlockedStatus';
import { nameInitial } from '../../features/spirit/spiritUi';
import { shouldRenderAgentAvatarImage, quasarAvatarIconForAgentField } from '../../features/avatar/iconModel';
import ResolvedAvatarImg from '../avatar/ResolvedAvatarImg.vue';

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
}>();

defineEmits<{
  locate: [payload: { agentKey: string; teamSessionId: string; teamId: string }];
  pause: [agentKey: string];
  resume: [agentKey: string];
  cancel: [agentKey: string];
  /** 点击设置按钮，由外层根据 agentKey 解析 agentId 后打开设置弹窗 */
  settings: [agentKey: string];
}>();

const memberInitial = computed(() => nameInitial(props.member.displayName));

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

/** 头像解析：优先使用成员自己的 avatarUrl；后端未下发时，从用户 Agent 库中按 agentKey 查找对应 Agent 的 icon。 */
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
      return props.blockedInfo?.message || t('chat.agentSidebar.blockedFallback');
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
  border-radius: 8px
  padding: 8px 10px
  margin-bottom: 6px
  cursor: pointer
  transition: background 0.15s
  border-left: 3px solid transparent

  &:hover
    background: rgba(255, 255, 255, 0.06)

  &--running
    border-left-color: #00E5FF
    background: rgba(0, 229, 255, 0.06)

  &--blocked
    border-left-color: #E9A23B
    background: rgba(233, 162, 59, 0.10)
    animation: stuck-pulse 2s infinite

  &--completed
    border-left-color: #4CAF7C
    opacity: 0.7

  &--failed
    border-left-color: #f44

  &--pending
    border-left-color: #444

  &__main
    display: flex
    align-items: center
    gap: 10px

  &__avatar
    width: 28px
    height: 28px
    border-radius: 50%
    background: var(--glass-elevated, var(--glass-surface))
    display: flex
    align-items: center
    justify-content: center
    font-size: 11px
    font-weight: 600
    color: var(--color-text-secondary)
    flex-shrink: 0
    overflow: hidden

  &__info
    flex: 1
    min-width: 0
    display: flex
    flex-direction: column
    gap: 2px

  &__name
    font-size: 13px
    font-weight: 600
    color: var(--color-text-primary, #fff)
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis
    line-height: 1.3

  &__team
    font-size: 10px
    color: var(--color-text-tertiary, #666)
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis
    line-height: 1.3

  &__status
    flex-shrink: 0
    display: inline-flex
    align-items: center
    justify-content: center

  &__spinner
    display: inline-block
    width: 14px
    height: 14px
    border: 2px solid #00E5FF
    border-top-color: transparent
    border-radius: 50%
    animation: spin 0.8s linear infinite

  &__blocked-badge
    display: inline-flex
    align-items: center
    gap: 2px
    padding: 2px 6px
    border-radius: 8px
    background: rgba(233, 162, 59, 0.15)
    border: 1px solid rgba(233, 162, 59, 0.5)
    color: #E9A23B
    font-size: 9px
    font-weight: 600
    line-height: 1.2
    white-space: nowrap

  &__completed-tag
    display: inline-flex
    align-items: center
    gap: 2px
    padding: 2px 6px
    border-radius: 8px
    background: rgba(76, 175, 124, 0.18)
    border: 1px solid rgba(76, 175, 124, 0.5)
    color: #4CAF7C
    font-size: 9px
    font-weight: 600
    line-height: 1.2
    white-space: nowrap

  &__failed-badge
    display: inline-flex
    align-items: center
    gap: 2px
    padding: 2px 6px
    border-radius: 8px
    background: rgba(244, 67, 54, 0.15)
    border: 1px solid rgba(244, 67, 54, 0.5)
    color: #f88
    font-size: 9px
    font-weight: 600
    line-height: 1.2
    white-space: nowrap

  &__actions
    display: flex
    gap: 6px
    margin-top: 6px
    margin-left: 38px

  &__btn
    border-radius: 4px
    padding: 2px 6px
    font-size: 9px
    cursor: pointer
    border: 1px solid
    display: inline-flex
    align-items: center
    gap: 3px

    &--pause
      background: rgba(255, 165, 0, 0.15)
      border-color: rgba(255, 165, 0, 0.5)
      color: #E9A23B

    &--resume
      background: rgba(0, 229, 255, 0.15)
      border-color: rgba(0, 229, 255, 0.5)
      color: #00E5FF

    &--cancel
      background: rgba(244, 67, 54, 0.15)
      border-color: rgba(244, 67, 54, 0.5)
      color: #f88

    &--settings
      margin-left: auto
      background: transparent
      border-color: rgba(255, 255, 255, 0.12)
      color: var(--color-text-tertiary)

      &:hover
        border-color: var(--color-accent)
        color: var(--color-accent)

@keyframes spin
  to
    transform: rotate(360deg)

@keyframes stuck-pulse
  0%, 100%
    box-shadow: 0 0 0 rgba(233, 162, 59, 0)
  50%
    box-shadow: 0 0 6px rgba(233, 162, 59, 0.3)
</style>
