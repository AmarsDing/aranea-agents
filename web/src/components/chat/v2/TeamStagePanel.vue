<!-- web/src/components/chat/v2/TeamStagePanel.vue
  - 添加完整样式（glass tokens 符合主题）
  - 状态色映射 + i18n
  - 问题 3 修复：展示 TeamName 而非 TeamID（fallback 到 TeamID 兼容旧数据）
-->
<template>
  <div
    class="team-stage-panel"
    :data-team-stage-id="teamStage.ID"
    :data-team-id="teamStage.TeamID"
  >
    <div class="team-stage-header">
      <div class="team-stage-header__left">
        <q-icon name="groups" size="16px" class="team-stage-header__icon" />
        <span class="team-stage-header__title">{{ displayTeamName }}</span>
        <q-badge :color="stageColor" class="team-stage-header__status">{{ stageLabel }}</q-badge>
      </div>
      <div v-if="teamStage.Members.length > 0" class="team-stage-members">
        <span v-for="m in teamStage.Members" :key="m.AgentKey" class="member-chip">
          <q-avatar v-if="m.AvatarURL" :src="m.AvatarURL" size="18px" />
          <q-icon v-else name="person" size="14px" class="member-chip__icon" />
          <span class="member-chip__name">{{ m.AgentName }}</span>
        </span>
      </div>
    </div>
    <TeamRunCard v-for="tr in teamRuns" :key="tr.ID" :team-run="tr" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { TeamStage } from '../../../features/chat/v2Types';
import TeamRunCard from './TeamRunCard.vue';

function useSafeI18n() {
  try {
    return useI18n();
  } catch {
    return { t: (key: string) => key };
  }
}

const props = defineProps<{ teamStage: TeamStage }>();
const store = useChatActivityStore();
const { t } = useSafeI18n();
const teamRuns = computed(() => store.getTeamStageTeamRuns(props.teamStage.ID));

// （兼容旧数据，旧数据可能因 DDL 未升级而没有 team_name 字段）
const displayTeamName = computed(() => props.teamStage.TeamName || props.teamStage.TeamID);

const stageColor = computed(
  () =>
    ({
      assembled: 'grey',
      planning: 'orange',
      executing: 'blue',
      completed: 'green',
      failed: 'red',
    })[props.teamStage.Stage] || 'grey',
);

const stageLabel = computed(() => {
  const map: Record<string, string> = {
    assembled: t('chat.v2.stageAssembled'),
    planning: t('chat.v2.statusPlanning'),
    executing: t('chat.v2.statusRunning'),
    completed: t('chat.v2.statusCompleted'),
    failed: t('chat.v2.statusFailed'),
  };
  return map[props.teamStage.Stage] || props.teamStage.Stage;
});
</script>

<style lang="sass" scoped>
.team-stage-panel
  border: 1px solid var(--glass-border)
  border-radius: 6px
  margin: 8px 0
  background: var(--glass-surface)

.team-stage-header
  display: flex
  align-items: center
  justify-content: space-between
  padding: 6px 10px
  border-bottom: 1px solid var(--glass-border)

  &__left
    display: flex
    align-items: center
    gap: 6px

  &__icon
    color: var(--color-text-secondary)

  &__title
    font-size: 13px
    font-weight: 600
    color: var(--color-text-primary)
    max-width: 200px
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__status
    margin-left: 4px

.team-stage-members
  display: flex
  align-items: center
  gap: 4px
  flex-wrap: wrap
  max-width: 60%

.member-chip
  display: inline-flex
  align-items: center
  gap: 2px
  padding: 2px 6px
  border-radius: 10px
  background: var(--glass-elevated)
  font-size: 11px
  color: var(--color-text-secondary)

  &__icon
    color: var(--color-icon-muted)

  &__name
    max-width: 80px
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
</style>
