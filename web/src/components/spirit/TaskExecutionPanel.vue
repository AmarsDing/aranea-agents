<template>
  <div class="task-execution-panel column no-wrap">
    <!-- v7: Breadcrumb navigation -->
    <div class="task-execution-panel__nav q-px-md q-py-sm row items-center no-wrap q-gutter-sm">
      <q-breadcrumbs dense separator="chevron_right" active-color="accent">
        <q-breadcrumbs-el :label="t('spirit.breadcrumbSpirit')" icon="auto_awesome" />
        <q-breadcrumbs-el :label="team.teamName" />
      </q-breadcrumbs>
      <OrchestrationModeBadge
        v-if="topology && topology !== 'coordinator'"
        :topology="topology"
        :reason="team.topologyReason"
      />
      <q-space />
      <SessionStatusBadge :status="mappedStatus" :status-reason="undefined" :status-changed-at="undefined" />
      <ToolStuckBadge :count="stuckToolCount" />
    </div>

    <!-- v7: User message bubble -->
    <div v-if="userMessage" class="task-execution-panel__user-bubble q-mx-md q-mt-sm">
      <div class="task-execution-panel__user-bubble-label text-caption text-grey q-mb-xs">
        {{ t('spirit.userMessage') }}
      </div>
      <div class="task-execution-panel__user-bubble-content">{{ userMessage }}</div>
    </div>

    <!-- v7: ThinkingArea — reasoning/thinking display -->
    <q-separator />
    <div class="q-pa-md">
      <ThinkingArea :content="thinkingContent" :is-active="isThinkingActive" :collapsed="!isThinkingActive" />
    </div>

    <!-- Interrupted team recovery card (OBS-07) -->
    <InterruptedTeamCard
      v-if="team.status === 'interrupted'"
      :team="team"
      :can-resume="canResume"
      :interrupt-reason="interruptReason"
      @resume="(teamId) => emit('resume-team', teamId)"
      @cancel="(teamId) => emit('cancel-team', teamId)"
    />

    <!-- v7: UnifiedExecutionPanel — replaces ParallelTeamOverview + TeamProgressCard list + DAGDiagramCard -->
    <template v-if="allTeams && allTeams.length > 0">
      <q-separator />
      <div class="q-pa-md">
        <UnifiedExecutionPanel :teams="allTeams" :task-nodes="taskNodes" :plan-entries="planEntries" />
      </div>
    </template>

    <q-separator />

    <div class="task-execution-panel__timeline col q-pa-md">
      <div class="text-caption text-weight-medium text-grey q-mb-sm">{{ t('spirit.executionTimeline') }}</div>
      <template v-if="messages.length > 0">
        <ChatExecutionCard
          v-for="msg in toolMessages"
          :key="msg.id"
          :event="msg"
          :show-member-label="true"
          :initial-collapsed="isToolEventCompleted(msg)"
        />
      </template>
      <div v-else class="text-caption text-grey">{{ t('spirit.noExecutionRecords') }}</div>
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
            <div class="text-caption text-grey q-mb-xs">{{ formatTime(msg.created_at) }}</div>
            <div class="chat-message-prose" v-html="props.renderMarkdown(msg.content_markdown)" />
          </div>
        </template>
        <div v-else class="text-caption text-grey">{{ t('spirit.noDialogOutput') }}</div>
      </div>
    </q-expansion-item>

    <!-- D3: Synthesis result card -->
    <template v-if="synthesisResult">
      <q-separator />
      <div class="q-pa-md">
        <SynthesisResultCard
          :result="synthesisResult"
          :rendered-content="props.renderMarkdown(synthesisResult.content)"
        />
      </div>
    </template>

    <!-- v7: Spirit reply area -->
    <template v-if="spiritReply">
      <q-separator />
      <div class="q-pa-md">
        <div class="text-caption text-weight-medium text-grey q-mb-xs">{{ t('spirit.spiritReply') }}</div>
        <div class="task-execution-panel__spirit-reply chat-message-prose" v-html="props.renderMarkdown(spiritReply)" />
      </div>
    </template>

    <!-- v7: SpiritStatusBar at bottom -->
    <SpiritStatusBar
      v-if="statusBarData"
      v-bind="statusBarData"
      @click-running="emit('click-running')"
      @click-interrupted="emit('click-interrupted')"
      @click-last-event="emit('click-last-event')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import SessionStatusBadge from '../sessions/SessionStatusBadge.vue';
import ChatExecutionCard from '../chat/ChatExecutionCard.vue';
import UnifiedExecutionPanel from './UnifiedExecutionPanel.vue';
import ThinkingArea from './ThinkingArea.vue';
import ToolStuckBadge from './ToolStuckBadge.vue';
import SynthesisResultCard from './SynthesisResultCard.vue';
import InterruptedTeamCard from './InterruptedTeamCard.vue';
import SpiritStatusBar from './SpiritStatusBar.vue';
import { mapSpiritStatusToSession, modeToTopology } from '../../features/spirit/spiritUi';
import { isStuckTool } from '../../features/chat/lib/isStuckTool';
import type {
  SpiritTeam,
  SynthesisOutput,
  CompletionStats,
  TaskNode,
  SpiritStatusBarData,
} from '../../features/spirit/types';
import type { PlanEntry } from '../../features/chat/agentTreeTypes';
import OrchestrationModeBadge from './OrchestrationModeBadge.vue';
import type { Message, ToolUseEvent } from '../../features/chat/types';
import type { TopologyType } from '../../features/spirit/types';

const { t } = useI18n();

const props = defineProps<{
  team: SpiritTeam;
  messages: Message[];
  /** All teams in the current spirit session (for parallel overview). */
  allTeams?: SpiritTeam[];
  /** Max parallel teams config from store (resolved by parent). */
  maxParallel?: number;
  /** Whether all teams have completed. */
  allTeamsCompleted?: boolean;
  /** Synthesis result for all teams. */
  synthesisResult?: SynthesisOutput | null;
  /** Team completion breakdown from spirit_teams_all_completed event. */
  completionStats?: CompletionStats | null;
  /** DAG task nodes for dependency visualization. */
  taskNodes?: TaskNode[];
  /** Plan entries from agent blocks. */
  planEntries?: PlanEntry[];
  /** Markdown render function injected by parent (avoids cross-domain import). */
  renderMarkdown: (text: string) => string;
  /** The user's message to display as a chat bubble above the execution panel. */
  userMessage?: string;
  /** The spirit's reply/summary to display below the execution panel. */
  spiritReply?: string;
  /** Status bar data for SpiritStatusBar integration. */
  statusBarData?: SpiritStatusBarData | null;
}>();

const emit = defineEmits<{
  'return-to-spirit': [];
  'select-team': [teamId: string];
  'cancel-team': [teamId: string];
  'resume-team': [teamId: string];
  'retry-team': [teamId: string];
  'select-member': [memberId: string];
  'archive-team': [teamId: string];
  'click-running': [];
  'click-interrupted': [];
  'click-last-event': [];
}>();

const toolMessages = computed<ToolUseEvent[]>(() => {
  return props.messages.filter((m) => m.role === 'tool' && m.tool_event).map((m) => m.tool_event as ToolUseEvent);
});

const assistantMessages = computed(() =>
  props.messages.filter((m) => m.role === 'assistant' && m.content_markdown.trim()),
);

const outputLabel = computed(() => t('spirit.dialogOutputCount', { count: assistantMessages.value.length }));

const mappedStatus = computed(() => mapSpiritStatusToSession(props.team.status));

const canResume = computed(() => Boolean(props.team.graphExecutionId || props.team.dagNodeId));

const interruptReason = computed(() => {
  return props.team.interruptReason || t('spirit.executionInterrupted');
});

const topology = computed<TopologyType>(() => modeToTopology(props.team.mode));

// ── v7: ThinkingArea computed ──
const thinkingContent = computed(() => {
  const reasoningMsgs = props.messages.filter((m) => m.role === 'assistant' && m.reasoning_markdown?.trim());
  if (reasoningMsgs.length === 0) return '';
  return reasoningMsgs[reasoningMsgs.length - 1].reasoning_markdown ?? '';
});

const isThinkingActive = computed(() => {
  return props.team.status === 'running' && !!thinkingContent.value;
});

// ── v7: ToolStuckBadge computed ──
const stuckToolCount = computed(() => {
  return props.messages.filter(
    (m) => m.role === 'tool' && m.tool_event && isStuckTool(m.tool_event as unknown as ToolUseEvent),
  ).length;
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
  overflow-y: auto

.task-execution-panel__nav
  border-bottom: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)
  background: color-mix(in srgb, var(--glass-surface) 30%, transparent)
  flex-shrink: 0

.task-execution-panel__user-bubble
  flex-shrink: 0

.task-execution-panel__user-bubble-label
  font-size: var(--text-xs)

.task-execution-panel__user-bubble-content
  padding: var(--space-2) var(--space-3)
  border-radius: 10px
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  border: 1px solid color-mix(in srgb, var(--color-accent) 20%, var(--glass-border))
  color: var(--color-text-primary)
  font-size: var(--text-sm)
  line-height: 1.5
  word-break: break-word

.task-execution-panel__timeline
  min-height: 0

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

.task-execution-panel__spirit-reply
  padding: var(--space-3)
  border-radius: 10px
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  border: 1px solid var(--glass-border)
</style>
