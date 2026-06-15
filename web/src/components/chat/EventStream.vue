<template>
  <div class="event-stream" :class="`event-stream--${variant}`">
    <template v-for="(event, idx) in events" :key="event.id">
      <!-- Phase transition indicator between thinking and reply -->
      <div
        v-if="idx > 0 && event.kind === 'reply' && events[idx - 1].kind === 'thinking'"
        class="event-stream__transition"
      >
        <span class="event-stream__transition-line" />
        <span class="event-stream__transition-label">{{ t('chat.transition.thinkingToReply', '生成回复') }}</span>
        <span class="event-stream__transition-line" />
      </div>

      <ThinkingBlock
        v-if="event.kind === 'thinking'"
        :message-id="event.id"
        :reasoning="event.content"
        :streaming="event.streaming"
        :duration-ms="event.durationMs"
        :default-collapsed="event.collapsed"
        :variant="variant"
      />
      <ActionBlock
        v-else-if="event.kind === 'action'"
        :activity="event"
        :variant="variant"
        :agent-color="agentColor"
      />
      <ReplyBlock
        v-else-if="event.kind === 'reply'"
        :activity="event"
        :variant="variant"
      />
      <ErrorBlock
        v-else-if="event.kind === 'error'"
        :event="event"
      />
      <!-- Plan: placeholder until PlanBlock is implemented -->
      <div v-else-if="event.kind === 'plan'" class="event-stream__plan-placeholder">
        {{ t('chat.plan.label', '计划') }} ({{ event.steps.length }} {{ t('chat.plan.steps', '步骤') }})
      </div>
      <DelegateActivity v-else-if="event.kind === 'delegate'" :activity="event" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { Activity, ActivityVariant } from '../../features/chat/activityTimelineTypes';
import ThinkingBlock from './ThinkingBlock.vue';
import ActionBlock from './ActionBlock.vue';
import ReplyBlock from './ReplyBlock.vue';
import ErrorBlock from './ErrorBlock.vue';
import DelegateActivity from './DelegateActivity.vue';

const { t } = useI18n();

defineProps<{
  events: Activity[];
  agentColor?: string;
  variant?: ActivityVariant;
}>();
</script>

<style lang="sass" scoped>
.event-stream
  display: flex
  flex-direction: column
  gap: 4px

  &--compact
    gap: 2px

  &__transition
    display: flex
    align-items: center
    gap: 8px
    padding: 2px 0

  &__transition-line
    flex: 1
    height: 1px
    background: var(--glass-border)

  &__transition-label
    font-size: 11px
    color: var(--color-text-tertiary)
    white-space: nowrap

  &__plan-placeholder
    font-size: 13px
    color: var(--color-text-secondary)
    padding: 4px 0
</style>
