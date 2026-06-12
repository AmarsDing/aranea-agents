<template>
  <div class="agent-work-panel">
    <!-- Agent header -->
    <div class="agent-work-panel__header">
      <div
        class="agent-work-panel__avatar"
        :style="{ background: agentWork.agentColor }"
      >
        {{ agentWork.agentIcon }}
      </div>
      <span class="agent-work-panel__name" :style="{ color: agentWork.agentColor }">
        {{ agentWork.agentName }}
      </span>
      <span class="agent-work-panel__status" :class="statusClass">
        {{ statusLabel }}
      </span>
      <span v-if="formattedDuration" class="agent-work-panel__duration">
        {{ formattedDuration }}
      </span>
    </div>

    <!-- Branch: ActivityTimeline or TeamPanel -->
    <div class="agent-work-panel__body">
      <ActivityTimeline
        v-if="!agentWork.panel"
        :activities="agentWork.activities"
        :agent-color="agentWork.agentColor"
        variant="card"
      />
      <!-- Team panel (v7 style) -->
      <template v-else>
        <TeamPanel :panel="agentWork.panel" />
        <ActivityTimeline
          :activities="agentWork.activities"
          :agent-color="agentWork.agentColor"
          variant="card"
        />
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { AgentWorkProcess } from '../../features/chat/activityTimelineTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import ActivityTimeline from './ActivityTimeline.vue';
import TeamPanel from './TeamPanel.vue';

const { t } = useI18n();

const props = defineProps<{
  agentWork: AgentWorkProcess;
}>();

const statusClass = computed(() => ({
  'agent-work-panel__status--running': props.agentWork.status === 'running',
  'agent-work-panel__status--completed': props.agentWork.status === 'completed',
  'agent-work-panel__status--failed': props.agentWork.status === 'failed',
}));

const statusLabel = computed(() => {
  switch (props.agentWork.status) {
    case 'running': return t('chat.turn.block.running');
    case 'completed': return t('chat.turn.block.completed');
    case 'failed': return t('chat.turn.block.failed');
    default: return '';
  }
});

const formattedDuration = computed(() => formatDuration(props.agentWork.durationMs));
</script>

<style lang="sass" scoped>
.agent-work-panel
  margin-left: 38px

  &__header
    display: flex
    align-items: center
    gap: 8px
    margin-bottom: 8px

  &__avatar
    width: 24px
    height: 24px
    border-radius: 50%
    display: flex
    align-items: center
    justify-content: center
    font-size: 12px
    color: var(--color-text-on-accent, #fff)
    flex-shrink: 0

  &__name
    font-weight: 600
    font-size: 14px

  &__status
    font-size: 12px
    padding: 1px 6px
    border-radius: 8px
    background: var(--glass-surface)
    color: var(--color-text-secondary)

    &--running
      color: var(--color-accent)

    &--completed
      color: var(--color-success)

    &--failed
      color: var(--color-danger)

  &__duration
    font-size: 12px
    color: var(--color-text-secondary)

  &__body
    margin-left: 12px
</style>
