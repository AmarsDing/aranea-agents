<template>
  <div class="event-stream">
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
      />
      <ActionBlock
        v-else-if="event.kind === 'action'"
        :activity="event"
        :agent-color="agentColor"
      />
      <ReplyBlock
        v-else-if="event.kind === 'reply'"
        :activity="event"
      />
      <ErrorBlock
        v-else-if="event.kind === 'error'"
        :event="event"
      />
      <PlanBlock
        v-else-if="event.kind === 'plan'"
        :activity="event"
        :agent-color="agentColor"
      />
      <ConfirmBlock
        v-else-if="event.kind === 'confirm'"
        :activity="event"
        @confirm="(id, approved) => $emit('confirm', id, approved)"
      />
      <NoticeBlock
        v-else-if="event.kind === 'notice'"
        :activity="event"
      />
      <DelegateActivity v-else-if="event.kind === 'delegate'" :activity="event" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { Activity } from '../../features/chat/activityTimelineTypes';
import ThinkingBlock from './ThinkingBlock.vue';
import ActionBlock from './ActionBlock.vue';
import ReplyBlock from './ReplyBlock.vue';
import ErrorBlock from './ErrorBlock.vue';
import PlanBlock from './PlanBlock.vue';
import ConfirmBlock from './ConfirmBlock.vue';
import NoticeBlock from './NoticeBlock.vue';
import DelegateActivity from './DelegateActivity.vue';

const { t } = useI18n();

defineProps<{
  events: Activity[];
  agentColor?: string;
}>();

defineEmits<{
  confirm: [activityId: string, approved: boolean];
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
