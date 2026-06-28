<template>
  <div class="event-stream">
    <template v-for="(item, idx) in renderItems" :key="item.activity.id + (item.keySuffix ? '-' + item.keySuffix : '')">
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
            :label="step.label"
          />
        </template>
        <ThinkingBlock
          v-else
          :message-id="item.event.id"
          :reasoning="(item.event as ThinkingEvent).content"
          :streaming="(item.event as ThinkingEvent).streaming"
          :duration-ms="(item.event as ThinkingEvent).durationMs"
          :default-collapsed="(item.event as ThinkingEvent).collapsed"
          :label="(item.event as ThinkingEvent).label"
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
      <PlanBlock v-else-if="item.event.kind === 'plan'" :activity="item.event as PlanEvent" />
      <ConfirmBlock
        v-else-if="item.event.kind === 'confirm'"
        :activity="item.event as ConfirmEvent"
        @confirm="(id, approved) => $emit('confirm', id, approved)"
      />
      <NoticeBlock v-else-if="item.event.kind === 'notice'" :activity="item.event as NoticeEvent" />
      <!-- Phase 3: Stage kinds for unified Team/Graph/Session rendering -->
      <GraphStageBlock v-else-if="item.event.kind === 'graph_stage'" :activity="item.event as GraphStageEvent" />
      <!-- B.4.1 team-card: replaces TeamStageBlock. Children rendered via slot
           so TeamCard's `expanded` state controls their visibility (B.4.5 fold rule). -->
      <TeamCard
        v-else-if="item.event.kind === 'team_stage'"
        :activity="item.event as TeamStageEvent"
        @expand-member="(p) => $emit('expand-member', p)"
        @cancel-team="(teamId: string) => $emit('cancel-team', teamId)"
        @retry-team="(teamId: string) => $emit('retry-team', teamId)"
      >
        <template v-if="item.activity.children?.length">
          <ActivityStream
            :activity-tree="item.activity.children"
            :agent-color="agentColor"
            @confirm="(id: string, approved: boolean) => $emit('confirm', id, approved)"
            @error-retry="(e: ErrorEvent) => $emit('error-retry', e)"
            @error-switch-model="(e: ErrorEvent) => $emit('error-switch-model', e)"
            @error-rephrase="(e: ErrorEvent) => $emit('error-rephrase', e)"
            @error-check-config="(e: ErrorEvent) => $emit('error-check-config', e)"
            @error-remove-attachment="(e: ErrorEvent) => $emit('error-remove-attachment', e)"
            @error-relogin="(e: ErrorEvent) => $emit('error-relogin', e)"
            @expand-member="(p) => $emit('expand-member', p)"
            @enter-session="(sid) => $emit('enter-session', sid)"
            @cancel-team="(teamId: string) => $emit('cancel-team', teamId)"
            @retry-team="(teamId: string) => $emit('retry-team', teamId)"
            @cancel-agent="(sessionId: string) => $emit('cancel-agent', sessionId)"
            @retry-agent="(sessionId: string) => $emit('retry-agent', sessionId)"
          />
        </template>
      </TeamCard>
      <!-- B.4.2 agent-card: replaces SessionStageBlock. Children rendered via slot. -->
      <AgentCard
        v-else-if="item.event.kind === 'session'"
        :activity="item.event as SessionStageEvent"
        @enter-session="(sid) => $emit('enter-session', sid)"
        @cancel-agent="(sessionId: string) => $emit('cancel-agent', sessionId)"
        @retry-agent="(sessionId: string) => $emit('retry-agent', sessionId)"
      >
        <template v-if="item.activity.children?.length">
          <ActivityStream
            :activity-tree="item.activity.children"
            :agent-color="agentColor"
            @confirm="(id: string, approved: boolean) => $emit('confirm', id, approved)"
            @error-retry="(e: ErrorEvent) => $emit('error-retry', e)"
            @error-switch-model="(e: ErrorEvent) => $emit('error-switch-model', e)"
            @error-rephrase="(e: ErrorEvent) => $emit('error-rephrase', e)"
            @error-check-config="(e: ErrorEvent) => $emit('error-check-config', e)"
            @error-remove-attachment="(e: ErrorEvent) => $emit('error-remove-attachment', e)"
            @error-relogin="(e: ErrorEvent) => $emit('error-relogin', e)"
            @expand-member="(p) => $emit('expand-member', p)"
            @enter-session="(sid) => $emit('enter-session', sid)"
            @cancel-team="(teamId: string) => $emit('cancel-team', teamId)"
            @retry-team="(teamId: string) => $emit('retry-team', teamId)"
            @cancel-agent="(sessionId: string) => $emit('cancel-agent', sessionId)"
            @retry-agent="(sessionId: string) => $emit('retry-agent', sessionId)"
          />
        </template>
      </AgentCard>

      <!-- B-04 / Phase A: Recursive nested rendering for activities with children.
           Renders child activities (sub-thinking, sub-action, sub-reply, etc.)
           in an indented container. This is the unified renderer's nested-rendering
           layer — replaces the previous flat sortedActivities pipeline that
           stripped children via `activityToStreamEvent({ ...activity, children: [] })`.
           For failed tasks (which expand to two render items: msg + err), only
           render children on the 'msg' item so the order becomes
           UserMessageBubble → Children → ErrorBlock and children are not duplicated.
           Note: team_stage / session render their children via TeamCard/AgentCard
           slots (above), so they are skipped here to avoid duplication. -->
      <div
        v-if="item.activity.children?.length && item.keySuffix !== 'err' && item.activity.kind !== 'team_stage' && item.activity.kind !== 'session'"
        class="event-stream__children"
      >
        <ActivityStream
          :activity-tree="item.activity.children"
          :agent-color="agentColor"
          @confirm="(id: string, approved: boolean) => $emit('confirm', id, approved)"
          @error-retry="(e: ErrorEvent) => $emit('error-retry', e)"
          @error-switch-model="(e: ErrorEvent) => $emit('error-switch-model', e)"
          @error-rephrase="(e: ErrorEvent) => $emit('error-rephrase', e)"
          @error-check-config="(e: ErrorEvent) => $emit('error-check-config', e)"
          @error-remove-attachment="(e: ErrorEvent) => $emit('error-remove-attachment', e)"
          @error-relogin="(e: ErrorEvent) => $emit('error-relogin', e)"
          @expand-member="(p) => $emit('expand-member', p)"
          @enter-session="(sid) => $emit('enter-session', sid)"
          @cancel-team="(teamId: string) => $emit('cancel-team', teamId)"
          @retry-team="(teamId: string) => $emit('retry-team', teamId)"
          @cancel-agent="(sessionId: string) => $emit('cancel-agent', sessionId)"
          @retry-agent="(sessionId: string) => $emit('retry-agent', sessionId)"
        />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Activity, ActivityTreeNode } from '../../features/chat/activityTypes';
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
import GraphStageBlock from './GraphStageBlock.vue';
import TeamCard from './TeamCard.vue';
import AgentCard from './AgentCard.vue';
import UserMessageBubble from './UserMessageBubble.vue';

// B-04 / Phase A: ActivityStream is now recursive. The component name is
// required for Vue's recursive self-reference in the template above.
defineOptions({ name: 'ActivityStream' });

const props = defineProps<{
  /** B-04 / Phase A: Activity tree (roots) for nested rendering. Each node
   * carries its own children; ActivityStream recurses over them to render
   * parent-child Activities with visual indentation. Replaces the previous
   * flat `activities: Activity[]` prop which stripped children via
   * `activityToStreamEvent({ ...activity, children: [] })`. */
  activityTree: ActivityTreeNode[];
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
  'expand-member': [payload: { agentKey: string; agentName?: string; teamId?: string }];
  'enter-session': [sessionId: string];
  'cancel-team': [teamId: string];
  'retry-team': [teamId: string];
  'cancel-agent': [sessionId: string];
  'retry-agent': [sessionId: string];
}>();

const { t } = useI18n();

interface RenderItem {
  activity: ActivityTreeNode;
  /** StreamEvent for dispatch; null when task (non-failed) renders as UserMessageBubble. */
  event: StreamEvent | null;
  /** Suffix appended to the v-for key to keep it unique when a single activity
   * expands to multiple render items (e.g. failed task → bubble + error block). */
  keySuffix: string;
}

// Map tree nodes → { activity, event } pairs. Non-failed task activities
// are flagged with event=null so they render as UserMessageBubble; everything
// else goes through the existing kind dispatch.
// A failed task expands to TWO render items so the user's input is preserved:
//   1. { event: null }           → UserMessageBubble (shows activity.content)
//   2. { event: ErrorEvent }     → ErrorBlock (shows meta.error_message)
// Without this, a failed turn would replace the user's message bubble with a
// red error box that echoed the user's own text.
// B-04: Pass the tree node directly (with children intact) — do NOT strip
// children via `{ ...activity, children: [] }`. activityToStreamEvent's plan
// case still maps children into PlanEvent.children for any non-rendering
// consumers, but the actual nested rendering is handled by the recursive
// <ActivityStream :activity-tree="item.activity.children" /> in the template.
const renderItems = computed<RenderItem[]>(() => {
  const items: RenderItem[] = [];
  for (const activity of props.activityTree) {
    if (activity.kind === 'task' && activity.status === 'failed') {
      items.push({ activity, event: null, keySuffix: 'msg' });
      items.push({ activity, event: activityToStreamEvent(activity), keySuffix: 'err' });
    } else if (activity.kind === 'task') {
      items.push({ activity, event: null, keySuffix: 'msg' });
    } else {
      items.push({ activity, event: activityToStreamEvent(activity), keySuffix: '' });
    }
  }
  return items;
});

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

  &__children
    margin-left: 14px
    padding-left: 8px
    border-left: 2px solid var(--glass-border)
    display: flex
    flex-direction: column
    gap: 4px
    margin-top: 2px
</style>
