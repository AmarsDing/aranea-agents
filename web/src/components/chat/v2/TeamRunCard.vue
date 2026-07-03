<!-- web/src/components/chat/v2/TeamRunCard.vue
  2026-07-04 完善：三段式布局 + 操作按钮 + 注入对话框（需求 §A.4.3）
  - 头部 20%：团队名/任务名/创建时间 + 状态 badge
  - 中部 60%：成员头像+名称/进度条:状态:耗时（成员列表由 MemberSessionPanel 提供）
  - 尾部 20%：注入对话框 + 操作按钮（暂停/恢复/取消/重试）
  - 始终默认折叠，用户手动展开后状态由用户掌控
  - 操作按钮按状态切换显示
-->
<template>
  <div class="team-run-card" :data-team-run-id="teamRun.ID">
    <!-- 头部：团队名 + 状态 + 折叠按钮 -->
    <div class="team-run-header" @click="toggleCollapse">
      <div class="team-run-header__left">
        <q-icon :name="collapsed ? 'expand_more' : 'expand_less'" size="18px" class="team-run-header__icon" />
        <span class="team-run-header__title">{{ teamRun.DagNodeID || teamRun.ID }}</span>
        <q-badge :color="statusColor" class="team-run-header__status">{{ statusLabel }}</q-badge>
      </div>
      <div class="team-run-header__right">
        <span class="team-run-header__time">{{ formattedTime }}</span>
      </div>
    </div>

    <div v-show="!collapsed" class="team-run-body">
      <!-- 中部：成员列表（由 MemberSessionPanel 渲染） -->
      <div class="team-run-members">
        <div v-if="memberSessions.length === 0" class="team-run-empty">
          {{ t('chat.v2.noMembers') }}
        </div>
        <MemberSessionPanel v-for="ms in memberSessions" :key="ms.ID" :member-session="ms" />
      </div>

      <!-- 尾部：注入对话框 + 操作按钮 -->
      <div class="team-run-actions">
        <q-input
          v-model="injectText"
          dense
          outlined
          :placeholder="t('chat.v2.injectPlaceholder')"
          class="team-run-inject-input"
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
          @click="$emit('pause', teamRun.ID)"
        />
        <q-btn
          v-if="canResume"
          flat
          dense
          size="sm"
          :label="t('chat.v2.resume')"
          icon="play_arrow"
          color="positive"
          @click="$emit('resume', teamRun.ID)"
        />
        <q-btn
          v-if="canCancel"
          flat
          dense
          size="sm"
          :label="t('chat.v2.cancel')"
          icon="stop"
          color="negative"
          @click="$emit('cancel', teamRun.ID)"
        />
        <q-btn
          v-if="canRetry"
          flat
          dense
          size="sm"
          :label="t('chat.v2.retry')"
          icon="refresh"
          color="primary"
          @click="$emit('retry', teamRun.ID)"
        />
      </div>

      <div v-if="teamRun.Error" class="team-run-error">{{ teamRun.Error }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
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
  pause: [id: string];
  resume: [id: string];
  cancel: [id: string];
  retry: [id: string];
  inject: [id: string, text: string];
}>();

const { t } = useSafeI18n();
const store = useChatActivityStore();
const memberSessions = computed(() => store.getTeamRunMemberSessions(props.teamRun.ID));

// 折叠状态：默认折叠（需求 §A.4.3）
const collapsed = ref(true);
const userToggled = ref(false);

// 当 teamRun.Status 变为 running 时，若用户未操作过，保持折叠
function toggleCollapse() {
  userToggled.value = true;
  collapsed.value = !collapsed.value;
}

// 注入对话框
const injectText = ref('');
function submitInject() {
  const text = injectText.value.trim();
  if (!text || !canInject.value) return;
  emit('inject', props.teamRun.ID, text);
  injectText.value = '';
}

const statusColor = computed(
  () =>
    ({
      running: 'blue',
      completed: 'green',
      failed: 'red',
      cancelled: 'grey',
    })[props.teamRun.Status] || 'grey',
);

const statusLabel = computed(() => {
  const map: Record<string, string> = {
    running: t('chat.v2.statusRunning'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
    cancelled: t('chat.v2.statusCancelled'),
  };
  return map[props.teamRun.Status] || props.teamRun.Status;
});

// 按钮可见性（需求 §A.4.3）
const canPause = computed(() => props.teamRun.Status === 'running');
const canResume = computed(() => props.teamRun.Status === 'running'); // 暂停态待后端支持
const canCancel = computed(() => props.teamRun.Status === 'running');
const canRetry = computed(() => props.teamRun.Status === 'failed' || props.teamRun.Status === 'cancelled');
const canInject = computed(() => props.teamRun.Status === 'running');

const formattedTime = computed(() => {
  const raw = props.teamRun.StartedAt;
  if (!raw) return '';
  const d = new Date(raw);
  if (isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
});
</script>

<style lang="sass" scoped>
.team-run-card
  border: 1px solid var(--color-border, #e0e0e0)
  border-radius: 6px
  margin: 4px 0
  background: var(--color-surface, #fff)

.team-run-header
  display: flex
  align-items: center
  justify-content: space-between
  padding: 6px 10px
  cursor: pointer
  user-select: none

  &:hover
    background: var(--color-hover, #f5f5f5)

  &__left
    display: flex
    align-items: center
    gap: 4px

  &__icon
    color: var(--color-text-secondary)

  &__title
    font-size: 12px
    font-weight: 600
    color: var(--color-text-primary)
    max-width: 200px
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__status
    margin-left: 4px

  &__right
    display: flex
    align-items: center

  &__time
    font-size: 11px
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums

.team-run-body
  padding: 8px 10px

.team-run-members
  margin-bottom: 8px

.team-run-empty
  font-size: 12px
  color: var(--color-text-tertiary)
  text-align: center
  padding: 8px

.team-run-actions
  display: flex
  align-items: center
  gap: 4px
  padding-top: 6px
  border-top: 1px solid var(--color-border, #f0f0f0)

.team-run-inject-input
  flex: 1
  min-width: 0

.team-run-error
  margin-top: 6px
  font-size: 12px
  color: var(--color-negative, #f44336)
</style>
