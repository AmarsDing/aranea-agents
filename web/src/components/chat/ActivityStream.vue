<template>
  <div class="event-stream">
    <template v-for="(item, idx) in renderItems" :key="item.activity.id">
      <!-- Phase transition indicator between thinking and reply -->
      <div
        v-if="idx > 0 && item.event?.kind === 'reply' && renderItems[idx - 1].event?.kind === 'thinking'"
        class="event-stream__transition"
      >
        <span class="event-stream__transition-line" />
        <span class="event-stream__transition-label">{{ t('chat.transition.thinkingToReply') }}</span>
        <span class="event-stream__transition-line" />
      </div>

      <!-- task (non-failed) → UserMessageBubble -->
      <UserMessageBubble
        v-if="item.event === null"
        :message="taskToMessage(item.activity)"
        @confirm="(id, approved) => $emit('confirm', id, approved)"
        @error-retry="(e) => $emit('error-retry', e)"
        @error-switch-model="(e) => $emit('error-switch-model', e)"
        @error-rephrase="(e) => $emit('error-rephrase', e)"
        @error-check-config="(e) => $emit('error-check-config', e)"
        @error-remove-attachment="(e) => $emit('error-remove-attachment', e)"
        @error-relogin="(e) => $emit('error-relogin', e)"
      />
      <!-- All other activities (incl. task.failed → ErrorBlock): dispatch by StreamEvent kind -->
      <template v-else-if="item.event.kind === 'thinking'">
        <template v-if="(item.event as ThinkingEvent).subSteps?.length">
          <ThinkingBlock
            v-for="step in (item.event as ThinkingEvent).subSteps"
            :key="step.id"
            :message-id="step.id"
            :reasoning="step.content"
            :streaming="step.streaming"
            :duration-ms="step.durationMs"
            :default-collapsed="step.collapsed"
          />
        </template>
        <ThinkingBlock
          v-else
          :message-id="item.event.id"
          :reasoning="(item.event as ThinkingEvent).content"
          :streaming="(item.event as ThinkingEvent).streaming"
          :duration-ms="(item.event as ThinkingEvent).durationMs"
          :default-collapsed="(item.event as ThinkingEvent).collapsed"
        />
      </template>
      <ActionBlock v-else-if="item.event.kind === 'action'" :activity="item.event as ActionEvent" :agent-color="agentColor" />
      <ReplyBlock v-else-if="item.event.kind === 'reply'" :activity="item.event as ReplyEvent" />
      <ErrorBlock
        v-else-if="item.event.kind === 'error'"
        :event="item.event as ErrorEvent"
        @retry="(e) => $emit('error-retry', e)"
        @switch-model="(e) => $emit('error-switch-model', e)"
        @rephrase="(e) => $emit('error-rephrase', e)"
        @check-config="(e) => $emit('error-check-config', e)"
        @remove-attachment="(e) => $emit('error-remove-attachment', e)"
        @relogin="(e) => $emit('error-relogin', e)"
      />
      <PlanBlock v-else-if="item.event.kind === 'plan'" :activity="item.event as PlanEvent" :agent-color="agentColor" />
      <ConfirmBlock
        v-else-if="item.event.kind === 'confirm'"
        :activity="item.event as ConfirmEvent"
        @confirm="(id, approved) => $emit('confirm', id, approved)"
      />
      <NoticeBlock v-else-if="item.event.kind === 'notice'" :activity="item.event as NoticeEvent" />
      <!-- Phase 3: Stage kinds for unified Team/Graph/Session rendering -->
      <TeamStageBlock v-else-if="item.event.kind === 'team_stage'" :activity="item.event as TeamStageEvent" />
      <GraphStageBlock v-else-if="item.event.kind === 'graph_stage'" :activity="item.event as GraphStageEvent" />
      <SessionStageBlock v-else-if="item.event.kind === 'session'" :activity="item.event as SessionStageEvent" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Activity } from '../../features/chat/activityTypes';
import type {
  ActionEvent,
  ConfirmEvent,
  ErrorEvent,
  GraphStageEvent,
  NoticeEvent,
  PlanEvent,
  ReplyEvent,
  SessionStageEvent,
  StreamEvent,
  TeamStageEvent,
  ThinkingEvent,
} from '../../features/chat/streamEventTypes';
import type { Message } from '../../domain/types';
import { activityToStreamEvent } from '../../features/chat/composables/useActivityTimeline';
import ThinkingBlock from './ThinkingBlock.vue';
import ActionBlock from './ActionBlock.vue';
import ReplyBlock from './ReplyBlock.vue';
import ErrorBlock from './ErrorBlock.vue';
import PlanBlock from './PlanBlock.vue';
import ConfirmBlock from './ConfirmBlock.vue';
import NoticeBlock from './NoticeBlock.vue';
import TeamStageBlock from './TeamStageBlock.vue';
import GraphStageBlock from './GraphStageBlock.vue';
import SessionStageBlock from './SessionStageBlock.vue';
import UserMessageBubble from './UserMessageBubble.vue';

const props = defineProps<{
  activities: Activity[];
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

const { t } = useI18n();

interface RenderItem {
  activity: Activity;
  /** StreamEvent for dispatch; null when task (non-failed) renders as UserMessageBubble. */
  event: StreamEvent | null;
}

// Map flat Activity[] → { activity, event } pairs. Non-failed task activities
// are flagged with event=null so they render as UserMessageBubble; everything
// else (incl. task.failed → ErrorEvent) goes through the existing kind dispatch.
const renderItems = computed<RenderItem[]>(() =>
  props.activities.map((activity) => ({
    activity,
    event:
      activity.kind === 'task' && activity.status !== 'failed'
        ? null
        : activityToStreamEvent({ ...activity, children: [] }),
  })),
);

/** Adapt a task Activity into the Message shape expected by UserMessageBubble. */
function taskToMessage(activity: Activity): Message {
  return {
    id: activity.id,
    session_id: activity.sessionId || '',
    parent_message_id: '',
    turn_id: activity.turnId || '',
    turn_number: 0,
    seq_in_turn: 0,
    role: 'user',
    content_markdown: activity.content || '',
    model_name: '',
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: activity.status === 'failed' ? 'failed' : 'ok',
    attachments_count: 0,
    options_json: '',
    error_message: '',
    created_at: activity.timestamp,
  };
}
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
