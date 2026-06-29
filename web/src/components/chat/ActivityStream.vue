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
        @confirm="(id: string, approved: boolean) => $emit('confirm', id, approved)"
        @error-retry="(e: ErrorEvent) => $emit('error-retry', e)"
        @error-switch-model="(e: ErrorEvent) => $emit('error-switch-model', e)"
        @error-rephrase="(e: ErrorEvent) => $emit('error-rephrase', e)"
        @error-check-config="(e: ErrorEvent) => $emit('error-check-config', e)"
        @error-remove-attachment="(e: ErrorEvent) => $emit('error-remove-attachment', e)"
        @error-relogin="(e: ErrorEvent) => $emit('error-relogin', e)"
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
            :thinking-only="!step.content.trim()"
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
          :thinking-only="!(item.event as ThinkingEvent).content.trim()"
        />
      </template>
      <ActionBlock
        v-else-if="item.event.kind === 'action'"
        :activity="item.event as ActionEvent"
        :agent-color="agentColor"
      />
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
        :agent-map="props.agentMap"
        :run-status="props.runStatus"
        @expand-member="(p) => $emit('expand-member', p)"
        @cancel-team="(teamId: string) => $emit('cancel-team', teamId)"
        @retry-team="(teamId: string) => $emit('retry-team', teamId)"
        @pause-team="(teamId: string) => $emit('pause-team', teamId)"
        @unpause-team="(teamId: string) => $emit('unpause-team', teamId)"
        @inject-team="(p: { teamId: string; message: string }) => $emit('inject-team', p)"
        @expand="(ids: string[]) => $emit('expand', ids)"
      >
        <template v-if="item.activity.children?.length">
          <ActivityStream
            :activity-tree="item.activity.children"
            :agent-color="agentColor"
            :agent-map="props.agentMap"
            :run-status="props.runStatus"
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
            @pause-team="(teamId: string) => $emit('pause-team', teamId)"
            @unpause-team="(teamId: string) => $emit('unpause-team', teamId)"
            @inject-team="(p: { teamId: string; message: string }) => $emit('inject-team', p)"
            @cancel-agent="(sessionId: string) => $emit('cancel-agent', sessionId)"
            @retry-agent="(sessionId: string) => $emit('retry-agent', sessionId)"
            @pause-agent="(sessionId: string) => $emit('pause-agent', sessionId)"
            @resume-agent="(sessionId: string) => $emit('resume-agent', sessionId)"
            @inject-agent="(p: { sessionId: string; message: string }) => $emit('inject-agent', p)"
            @expand="(ids: string[]) => $emit('expand', ids)"
          />
        </template>
      </TeamCard>
      <!-- B.4.2 agent-card: replaces SessionStageBlock. Children rendered via slot. -->
      <AgentCard
        v-else-if="item.event.kind === 'session'"
        :activity="item.event as SessionStageEvent"
        :agent-map="props.agentMap"
        :run-status="props.runStatus"
        @enter-session="(sid) => $emit('enter-session', sid)"
        @cancel-agent="(sessionId: string) => $emit('cancel-agent', sessionId)"
        @retry-agent="(sessionId: string) => $emit('retry-agent', sessionId)"
        @pause-agent="(sessionId: string) => $emit('pause-agent', sessionId)"
        @resume-agent="(sessionId: string) => $emit('resume-agent', sessionId)"
        @inject-agent="(p: { sessionId: string; message: string }) => $emit('inject-agent', p)"
        @expand="(ids: string[]) => $emit('expand', ids)"
      >
        <template v-if="item.activity.children?.length">
          <ActivityStream
            :activity-tree="item.activity.children"
            :agent-color="agentColor"
            :agent-map="props.agentMap"
            :run-status="props.runStatus"
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
            @pause-team="(teamId: string) => $emit('pause-team', teamId)"
            @unpause-team="(teamId: string) => $emit('unpause-team', teamId)"
            @inject-team="(p: { teamId: string; message: string }) => $emit('inject-team', p)"
            @cancel-agent="(sessionId: string) => $emit('cancel-agent', sessionId)"
            @retry-agent="(sessionId: string) => $emit('retry-agent', sessionId)"
            @pause-agent="(sessionId: string) => $emit('pause-agent', sessionId)"
            @resume-agent="(sessionId: string) => $emit('resume-agent', sessionId)"
            @inject-agent="(p: { sessionId: string; message: string }) => $emit('inject-agent', p)"
            @expand="(ids: string[]) => $emit('expand', ids)"
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
        v-if="
          item.activity.children?.length &&
          item.keySuffix !== 'err' &&
          item.activity.kind !== 'team_stage' &&
          item.activity.kind !== 'session'
        "
        class="event-stream__children"
      >
        <ActivityStream
          :activity-tree="item.activity.children"
          :agent-color="agentColor"
          :agent-map="props.agentMap"
          :run-status="props.runStatus"
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
          @pause-team="(teamId: string) => $emit('pause-team', teamId)"
          @unpause-team="(teamId: string) => $emit('unpause-team', teamId)"
          @inject-team="(p: { teamId: string; message: string }) => $emit('inject-team', p)"
          @cancel-agent="(sessionId: string) => $emit('cancel-agent', sessionId)"
          @retry-agent="(sessionId: string) => $emit('retry-agent', sessionId)"
          @pause-agent="(sessionId: string) => $emit('pause-agent', sessionId)"
          @resume-agent="(sessionId: string) => $emit('resume-agent', sessionId)"
          @inject-agent="(p: { sessionId: string; message: string }) => $emit('inject-agent', p)"
          @expand="(ids: string[]) => $emit('expand', ids)"
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
  /** P1#1/2: agent key → display name lookup for TeamCard/AgentCard. */
  agentMap?: Map<string, { displayName: string; agentKey: string }>;
  /** P1#3: parent run status to gate cancel button visibility. */
  runStatus?: import('../../features/chat/types').RunStatusValue;
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
  // B.5.3: pause/unpause/inject for team and agent run lifecycle control.
  'pause-team': [teamId: string];
  'unpause-team': [teamId: string];
  'inject-team': [payload: { teamId: string; message: string }];
  'cancel-agent': [sessionId: string];
  'retry-agent': [sessionId: string];
  'pause-agent': [sessionId: string];
  'resume-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  // T5.2/T5.3 / §B.7.2: Bubble up team-card / agent-card expand events so the
  // page can lazy-load member/child session activities (cache-aware via
  // ensureActivitiesLoaded).
  expand: [sessionIds: string[]];
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
//
// Problem 1 fix (方案B): Orphan notices (kind=notice, no parentActivityId)
// are NOT rendered as standalone items. Instead, they are attached to the
// nearest subsequent task activity and rendered as NoticeBlocks right after
// the UserMessageBubble. This fixes "指令显示滞后" — the pre-planning gate
// notice no longer appears BEFORE the user's instruction; it appears after it
// as part of the task's visual unit.
const renderItems = computed<RenderItem[]>(() => {
  const items: RenderItem[] = [];

  // Separate orphan notices from other root activities.
  const orphanNotices: ActivityTreeNode[] = [];
  const otherRoots: ActivityTreeNode[] = [];
  for (const activity of props.activityTree) {
    if (activity.kind === 'notice' && !activity.parentActivityId) {
      orphanNotices.push(activity);
    } else {
      otherRoots.push(activity);
    }
  }

  // Match each orphan notice to the nearest subsequent task (same session,
  // timestamp >= notice timestamp). This attaches pre-planning / intent-pass
  // notices to the task they were assessing.
  const noticesByTaskId = new Map<string, ActivityTreeNode[]>();
  const unmatchedNotices: ActivityTreeNode[] = [];
  for (const notice of orphanNotices) {
    const noticeTime = new Date(notice.timestamp).getTime();
    let nearestTask: ActivityTreeNode | null = null;
    let nearestTaskTime = Infinity;
    for (const a of otherRoots) {
      if (a.kind !== 'task') continue;
      const taskTime = new Date(a.timestamp).getTime();
      if (taskTime >= noticeTime && taskTime < nearestTaskTime) {
        nearestTask = a;
        nearestTaskTime = taskTime;
      }
    }
    if (nearestTask) {
      const arr = noticesByTaskId.get(nearestTask.id) ?? [];
      arr.push(notice);
      noticesByTaskId.set(nearestTask.id, arr);
    } else {
      // No subsequent task found (e.g. notice arrived but task not yet
      // created). Render at the end as fallback to avoid losing the info.
      unmatchedNotices.push(notice);
    }
  }

  for (const activity of otherRoots) {
    if (activity.kind === 'task' && activity.status === 'failed') {
      items.push({ activity, event: null, keySuffix: 'msg' });
      items.push({ activity, event: activityToStreamEvent(activity), keySuffix: 'err' });
    } else if (activity.kind === 'task') {
      items.push({ activity, event: null, keySuffix: 'msg' });
      // Render attached orphan notices right after the task bubble.
      const attached = noticesByTaskId.get(activity.id);
      if (attached) {
        for (const n of attached) {
          items.push({ activity: n, event: activityToStreamEvent(n), keySuffix: 'notice' });
        }
      }
    } else {
      // Skip terminal thinking activities with no visible content. Empty
      // thinking blocks appear when the backend emits thinking events that
      // never receive content, leaving blank cards in the UI.
      if (activity.kind === 'thinking' && !isThinkingRenderable(activity)) {
        continue;
      }
      // P1#6: suppress noisy progress-check tool calls. These are internal
      // heartbeat probes that produce no user-facing value.
      if (activity.kind === 'action' && isSilentTool(activity)) {
        continue;
      }
      items.push({ activity, event: activityToStreamEvent(activity), keySuffix: '' });
    }
  }

  // Fallback: render unmatched orphan notices at the end.
  for (const n of unmatchedNotices) {
    items.push({ activity: n, event: activityToStreamEvent(n), keySuffix: '' });
  }

  return items;
});

/** Determine whether a thinking activity should be rendered.
 *  Streaming thinkings are always shown (they may receive content soon).
 *  Terminal thinkings are only shown if they have non-empty content/reasoning. */
function isThinkingRenderable(activity: ActivityTreeNode): boolean {
  if (activity.status === 'running' || activity.status === 'tool_running') return true;
  return hasVisibleThinkingContent(activity);
}

/** True if the thinking activity has any visible textual content. */
function hasVisibleThinkingContent(activity: ActivityTreeNode): boolean {
  return (activity.reasoning || activity.content || '').trim().length > 0;
}

/** True for internal tools that should not surface in the chat UI. */
function isSilentTool(activity: ActivityTreeNode): boolean {
  return activity.toolName === 'check_progress' || activity.toolName === 'progress_check';
}

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
