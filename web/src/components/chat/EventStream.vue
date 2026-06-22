<template>
  <div class="event-stream">
    <template v-for="(event, idx) in events" :key="event.id">
      <!-- Phase transition indicator between thinking and reply -->
      <div
        v-if="idx > 0 && event.kind === 'reply' && events[idx - 1].kind === 'thinking'"
        class="event-stream__transition"
      >
        <span class="event-stream__transition-line" />
        <span class="event-stream__transition-label">{{ t('chat.transition.thinkingToReply') }}</span>
        <span class="event-stream__transition-line" />
      </div>

      <ThinkingBlock
        v-if="event.kind === 'thinking'"
        :message-id="event.id"
        :reasoning="event.content"
        :streaming="event.streaming"
        :duration-ms="event.durationMs"
        :default-collapsed="event.collapsed"
      />
      <ActionBlock v-else-if="event.kind === 'action'" :activity="event" :agent-color="agentColor" />
      <ReplyBlock v-else-if="event.kind === 'reply'" :activity="event" />
      <ErrorBlock
        v-else-if="event.kind === 'error'"
        :event="event"
        @retry="(e) => $emit('error-retry', e)"
        @switch-model="(e) => $emit('error-switch-model', e)"
        @rephrase="(e) => $emit('error-rephrase', e)"
        @check-config="(e) => $emit('error-check-config', e)"
        @remove-attachment="(e) => $emit('error-remove-attachment', e)"
        @relogin="(e) => $emit('error-relogin', e)"
      />
      <PlanBlock v-else-if="event.kind === 'plan'" :activity="event" :agent-color="agentColor" />
      <ConfirmBlock
        v-else-if="event.kind === 'confirm'"
        :activity="event"
        @confirm="(id, approved) => $emit('confirm', id, approved)"
      />
      <NoticeBlock v-else-if="event.kind === 'notice'" :activity="event" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { Activity as TimelineActivity } from '../../features/chat/activityTimelineTypes';
import type { ErrorEvent } from '../../features/chat/streamEventTypes';
import ThinkingBlock from './ThinkingBlock.vue';
import ActionBlock from './ActionBlock.vue';
import ReplyBlock from './ReplyBlock.vue';
import ErrorBlock from './ErrorBlock.vue';
import PlanBlock from './PlanBlock.vue';
import ConfirmBlock from './ConfirmBlock.vue';
import NoticeBlock from './NoticeBlock.vue';

const { t } = useI18n();

defineProps<{
  events: TimelineActivity[];
  agentColor?: string;
}>();

defineEmits<{
  confirm: [activityId: string, approved: boolean];
  'error-retry': [event: ErrorEvent];
  'error-switch-model': [event: ErrorEvent];
  'error-rephrase': [event: ErrorEvent];
  'error-check-config': [event: ErrorEvent];
  'error-remove-attachment': [event: ErrorEvent];
  'error-relogin': [event: ErrorEvent];
}>();
</script>

<style lang="sass" scoped>
.event-stream
  display: flex
  flex-direction: column
  gap: 4px

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
</style>
