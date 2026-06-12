<template>
  <div class="delegate-activity">
    <div class="delegate-activity__header">
      <span class="delegate-activity__icon">👥</span>
      <span class="delegate-activity__name" :style="{ color: activity.subAgent.agentColor }">
        {{ activity.subAgent.agentName }}
      </span>
      <span class="delegate-activity__status" :class="statusClass">{{ statusLabel }}</span>
    </div>
    <div class="delegate-activity__body">
      <AgentWorkPanel :agent-work="activity.subAgent" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { DelegateActivity as DelegateActivityType } from '../../features/chat/activityTimelineTypes';
import AgentWorkPanel from './AgentWorkPanel.vue';

const { t } = useI18n();

const props = defineProps<{
  activity: DelegateActivityType;
}>();

const statusClass = computed(() => ({
  'delegate-activity__status--running': props.activity.subAgent.status === 'running',
  'delegate-activity__status--completed': props.activity.subAgent.status === 'completed',
  'delegate-activity__status--failed': props.activity.subAgent.status === 'failed',
}));

const statusLabel = computed(() => {
  switch (props.activity.subAgent.status) {
    case 'running': return t('chat.turn.block.running');
    case 'completed': return t('chat.turn.block.completed');
    case 'failed': return t('chat.turn.block.failed');
    default: return '';
  }
});
</script>

<style lang="sass" scoped>
.delegate-activity
  margin-left: 12px
  padding-left: 12px
  border-left: 2px solid var(--glass-border)

  &__header
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 0

  &__icon
    font-size: 14px

  &__name
    font-size: 13px
    font-weight: 600

  &__status
    font-size: 12px
    &--running
      color: var(--color-accent)
    &--completed
      color: var(--color-success)
    &--failed
      color: var(--color-danger)

  &__body
    margin-top: 4px
</style>
