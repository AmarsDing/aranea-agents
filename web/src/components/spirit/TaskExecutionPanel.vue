<template>
  <div class="task-execution-panel column no-wrap">
    <div class="task-execution-panel__overview">
      <div class="row items-center q-gutter-sm">
        <q-icon name="groups" size="20px" color="accent" />
        <div class="col min-width-0">
          <div class="task-execution-panel__title ellipsis">{{ team.teamName }}</div>
          <div class="task-execution-panel__summary ellipsis">{{ team.taskSummary }}</div>
        </div>
        <SessionStatusBadge :status="mappedStatus" :status-reason="undefined" :status-changed-at="undefined" />
      </div>
      <div v-if="team.totalSteps > 0" class="q-mt-sm">
        <q-linear-progress :value="team.completedSteps / team.totalSteps" size="6px" rounded color="accent" />
        <div class="text-caption text-grey-6 q-mt-xs">{{ team.completedSteps }} / {{ team.totalSteps }} 步骤完成</div>
      </div>
      <div v-if="team.members.length > 0" class="q-mt-sm">
        <div class="text-caption text-weight-medium text-grey-7 q-mb-xs">成员状态</div>
        <div class="row items-center q-gutter-xs">
          <div
            v-for="member in team.members"
            :key="member.agentKey"
            class="task-execution-panel__member task-execution-panel__member--clickable row items-center q-gutter-xs"
            @click="emit('select-member', member.agentId)"
          >
            <q-avatar size="20px">
              <img v-if="member.avatarUrl" :src="member.avatarUrl" alt="" />
              <q-icon v-else name="person" size="14px" color="grey-6" />
            </q-avatar>
            <span class="text-caption ellipsis" style="max-width: 80px">{{ member.displayName }}</span>
            <AgentStatusLabel :label="spiritMemberStatusToLabel(member.status)" />
          </div>
        </div>
      </div>
      <q-btn
        flat
        dense
        no-caps
        icon="arrow_back"
        label="返回精灵"
        color="accent"
        class="q-mt-sm"
        @click="emit('return-to-spirit')"
      />
    </div>

    <!-- Parallel Team Overview (when multiple teams exist) -->
    <template v-if="allTeams && allTeams.length > 1">
      <q-separator />
      <div class="q-pa-md">
        <ParallelTeamOverview
          :teams="allTeams"
          :max-parallel="maxParallel ?? props.maxConcurrentTeams ?? DEFAULT_MAX_PARALLEL_TEAMS"
          :all-completed="allTeamsCompleted ?? false"
          :completion-stats="completionStats"
          :synthesis-result="synthesisResult"
          @select-team="(teamId) => emit('select-team', teamId)"
          @cancel-team="(teamId) => emit('cancel-team', teamId)"
          @retry-team="(teamId) => emit('retry-team', teamId)"
          @archive-team="(teamId) => emit('archive-team', teamId)"
        />
      </div>
    </template>

    <!-- Interrupted team recovery card (OBS-07) -->
    <InterruptedTeamCard
      v-if="team.status === 'interrupted'"
      :team="team"
      :can-resume="canResume"
      :interrupt-reason="interruptReason"
      @resume="(teamId) => emit('resume-team', teamId)"
      @cancel="(teamId) => emit('cancel-team', teamId)"
    />

    <q-separator />

    <div class="task-execution-panel__timeline col q-pa-md">
      <div class="text-caption text-weight-medium text-grey-7 q-mb-sm">执行时间线</div>
      <template v-if="messages.length > 0">
        <ChatExecutionCard
          v-for="msg in toolMessages"
          :key="msg.id"
          :event="msg"
          :show-member-label="true"
          :initial-collapsed="isToolEventCompleted(msg)"
        />
      </template>
      <div v-else class="text-caption text-grey-6">暂无执行记录</div>
    </div>

    <q-separator />

    <q-expansion-item
      dense
      :label="outputLabel"
      header-class="task-execution-panel__output-header"
      :default-opened="false"
    >
      <div class="task-execution-panel__output q-pa-md">
        <template v-if="assistantMessages.length > 0">
          <div v-for="msg in assistantMessages" :key="msg.id" class="task-execution-panel__output-item">
            <div class="text-caption text-grey-7 q-mb-xs">{{ formatTime(msg.created_at) }}</div>
            <div class="chat-message-prose" v-html="renderChatMarkdown(msg.content_markdown)" />
          </div>
        </template>
        <div v-else class="text-caption text-grey-6">暂无对话输出</div>
      </div>
    </q-expansion-item>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import SessionStatusBadge from '../sessions/SessionStatusBadge.vue';
import ChatExecutionCard from '../chat/ChatExecutionCard.vue';
import ParallelTeamOverview from './ParallelTeamOverview.vue';
import { mapSpiritStatusToSession, spiritMemberStatusToLabel } from '../../features/spirit/spiritUi';
import { DEFAULT_MAX_PARALLEL_TEAMS } from '../../features/spirit/observabilityConstants';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';
import type { SpiritTeam, SynthesisOutput, CompletionStats } from '../../features/spirit/types';
import AgentStatusLabel from './AgentStatusLabel.vue';
import type { Message, ToolUseEvent } from '../../features/chat/types';

const props = defineProps<{
  team: SpiritTeam;
  messages: Message[];
  /** All teams in the current spirit session (for parallel overview). */
  allTeams?: SpiritTeam[];
  /** Max parallel teams config. */
  maxParallel?: number;
  /** Whether all teams have completed. */
  allTeamsCompleted?: boolean;
  /** Synthesis result for all teams. */
  synthesisResult?: SynthesisOutput | null;
  /** Team completion breakdown from spirit_teams_all_completed event. */
  completionStats?: CompletionStats | null;
  /** Max concurrent teams from store (for ParallelTeamOverview). */
  maxConcurrentTeams?: number;
}>();

const emit = defineEmits<{
  'return-to-spirit': [];
  'select-team': [teamId: string];
  'cancel-team': [teamId: string];
  'resume-team': [teamId: string];
  'retry-team': [teamId: string];
  'select-member': [memberId: string];
  'archive-team': [teamId: string];
}>();

const toolMessages = computed<ToolUseEvent[]>(() => {
  return props.messages.filter((m) => m.role === 'tool' && m.tool_event).map((m) => m.tool_event as ToolUseEvent);
});

const assistantMessages = computed(() =>
  props.messages.filter((m) => m.role === 'assistant' && m.content_markdown.trim()),
);

const outputLabel = computed(() => `对话输出 (${assistantMessages.value.length})`);

const mappedStatus = computed(() => mapSpiritStatusToSession(props.team.status));

const canResume = computed(() => Boolean(props.team.graphExecutionId || props.team.dagNodeId));

const interruptReason = computed(() => {
  return props.team.interruptReason || '执行中断';
});

function isToolEventCompleted(event: ToolUseEvent): boolean {
  const s = event.status;
  return s === 'success' || s === 'failed' || s === 'cancelled';
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString();
}
</script>

<style scoped lang="sass">
.task-execution-panel
  height: 100%
  overflow: hidden

.task-execution-panel__overview
  padding: var(--space-4)
  flex-shrink: 0

.task-execution-panel__title
  font-size: var(--text-base)
  font-weight: 700
  color: var(--color-text-primary)

.task-execution-panel__summary
  font-size: var(--text-sm)
  color: var(--color-text-tertiary)

.task-execution-panel__timeline
  overflow-y: auto
  min-height: 0

.task-execution-panel__member
  padding: 2px 6px
  border-radius: 6px
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)

  &--clickable
    cursor: pointer
    transition: background 0.15s

    &:hover
      background: color-mix(in srgb, var(--color-accent) 15%, transparent)

.task-execution-panel__output-header
  font-size: var(--text-sm)
  font-weight: 600

.task-execution-panel__output
  max-height: 240px
  overflow-y: auto

.task-execution-panel__output-item
  margin-bottom: var(--space-3)
  padding: var(--space-3)
  border-radius: 10px
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  border: 1px solid var(--glass-border)
</style>
