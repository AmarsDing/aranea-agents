<template>
  <div class="team-progress-section" :class="`team-progress-section--${section.status}`">
    <div class="team-progress-section__header">
      <span class="team-progress-section__icon">{{ section.teamIcon }}</span>
      <span class="team-progress-section__name">{{ section.teamName }}</span>
      <span class="team-progress-section__status-badge" :class="`team-progress-section__status-badge--${section.status}`">
        {{ statusLabel }}
      </span>
      <span v-if="section.durationMs != null" class="team-progress-section__duration">
        {{ formattedDuration }}
      </span>
    </div>

    <!-- Progress bar -->
    <div class="team-progress-section__bar">
      <div
        class="team-progress-section__bar-fill"
        :style="{ width: `${section.progressPercent}%` }"
      ></div>
    </div>

    <!-- Agent details -->
    <div class="team-progress-section__agents">
      <div v-for="agent in section.agents" :key="agent.agentKey" class="agent-progress">
        <div class="agent-progress__header">
          <span class="agent-progress__icon">{{ agent.agentIcon }}</span>
          <span class="agent-progress__name">{{ agent.agentName }}</span>
          <span class="agent-progress__status" :class="`agent-progress__status--${agent.status}`">
            {{ agentStatusIcon(agent.status) }}
          </span>
        </div>
        <ActivityTimeline
          :activities="agent.activities"
          variant="compact"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TeamProgressSection as TeamProgressSectionType } from '../../features/chat/activityTimelineTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import ActivityTimeline from './ActivityTimeline.vue';

const { t } = useI18n();

const props = defineProps<{
  section: TeamProgressSectionType;
}>();

const statusLabel = computed(() => {
  switch (props.section.status) {
    case 'running': return t('chat.turn.block.running');
    case 'completed': return t('chat.turn.block.completed');
    case 'failed': return t('chat.turn.block.failed');
    case 'interrupted': return t('chat.agentBlock.interrupted', '中断');
    default: return '';
  }
});

const formattedDuration = computed(() => formatDuration(props.section.durationMs));

function agentStatusIcon(status: string): string {
  switch (status) {
    case 'running': return '⏳';
    case 'completed': return '✓';
    case 'failed': return '✗';
    case 'waiting': return '⏸';
    default: return '○';
  }
}
</script>

<style lang="sass" scoped>
.team-progress-section
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  border-radius: 12px
  padding: 12px
  margin-bottom: 8px

  &__header
    display: flex
    align-items: center
    gap: 8px
    margin-bottom: 8px

  &__icon
    font-size: 16px

  &__name
    font-size: 14px
    font-weight: 600
    color: var(--color-text-primary)

  &__status-badge
    font-size: 11px
    padding: 2px 8px
    border-radius: 8px

    &--running
      background: rgba(0, 229, 255, 0.1) /* accent-bg, dark-mode only */
      color: var(--color-accent)

    &--completed
      background: rgba(63, 224, 160, 0.1) /* success-bg */
      color: var(--color-success)

    &--failed
      background: rgba(255, 94, 122, 0.1) /* danger-bg */
      color: var(--color-danger)

    &--interrupted
      background: rgba(240, 155, 84, 0.1) /* warning-bg */
      color: var(--color-warning)

  &__duration
    font-size: 12px
    color: var(--color-text-secondary)

  &__bar
    height: 4px
    background: var(--glass-border)
    border-radius: 2px
    margin-bottom: 10px
    overflow: hidden

  &__bar-fill
    height: 100%
    background: var(--color-accent)
    border-radius: 2px
    transition: width 0.3s ease

  &__agents
    display: flex
    flex-direction: column
    gap: 6px

.agent-progress
  &__header
    display: flex
    align-items: center
    gap: 6px
    margin-bottom: 2px

  &__icon
    font-size: 13px

  &__name
    font-size: 13px
    font-weight: 500
    color: var(--color-text-primary)

  &__status
    font-size: 11px
</style>
