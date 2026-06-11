<template>
  <!-- Collapsed summary view -->
  <div v-if="collapsed && block.isCompleted" class="turn-block turn-block--collapsed" @click="emit('toggle-collapse')">
    <div class="row items-center no-wrap q-gutter-xs">
      <q-avatar v-if="agentInitials" :style="{ backgroundColor: agentAvatarColor, color: 'var(--color-text-on-accent)' }" size="22px" class="turn-block__avatar">
        <span style="font-size:10px;font-weight:600;">{{ agentInitials }}</span>
      </q-avatar>
      <q-icon v-else :name="collapsedIcon" size="16px" :style="{ color: collapsedIconColor }" />
      <span v-if="agentName" class="text-caption text-weight-medium" :style="{ color: agentAvatarColor }">{{ agentName }}</span>
      <span class="text-caption ellipsis">{{ collapsedSummary }}</span>
      <q-space />
      <span v-if="collapsedDuration" class="text-caption turn-block__text-tertiary">{{ collapsedDuration }}</span>
      <q-icon name="expand_more" size="14px" class="turn-block__expand-icon" />
    </div>
  </div>
  <!-- Full content view -->
  <article v-else class="turn-block" :class="{ 'turn-block--focused': focused, 'turn-block--completed': block.isCompleted }" :data-turn-id="block.turnId">
    <!-- Agent Block Header -->
    <div class="turn-block__agent-header row items-center no-wrap q-gutter-xs" @click="block.isCompleted && emit('toggle-collapse')">
      <q-avatar v-if="agentInitials" :style="{ backgroundColor: agentAvatarColor, color: 'var(--color-on-accent)' }" size="28px" class="turn-block__avatar">
        <span style="font-size:13px;font-weight:600;">{{ agentInitials }}</span>
      </q-avatar>
      <span v-if="agentName" class="text-weight-medium" :style="{ color: agentAvatarColor, fontSize: 'var(--text-base)' }">{{ agentName }}</span>
      <span v-if="blockStatusText" class="turn-block__status-badge" :class="blockStatusClass">{{ blockStatusText }}</span>
      <span v-if="blockDuration" class="text-caption turn-block__text-tertiary">{{ blockDuration }}</span>
      <span v-if="block.members.length" class="text-caption turn-block__text-tertiary">· {{ block.members.length }} {{ t('chat.turn.block.memberCount', '个子任务') }}</span>
      <q-space />
      <q-icon v-if="block.isCompleted" name="expand_less" size="14px" class="turn-block__expand-icon turn-block__toggle" />
    </div>
    <div v-if="turnSourceLabel" class="turn-block__channel-bar text-caption" :aria-label="turnSourceLabel">
      {{ turnSourceLabel }}
    </div>
    <!-- User message -->
    <ChatMessageRow
      v-if="block.user"
      :message="block.user"
      :index="0"
      :messages="allMessages"
      :is-dark="isDark"
      :is-team-session="isTeamSession"
      :planner-kind="plannerKind"
      :react-tool-link-index="reactToolLinkIndex"
      :reasoning-sidebar-open="reasoningSidebarOpen"
      @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
      @retry="(id) => emit('retry', id)"
      @dismiss-failed="(id) => emit('dismiss-failed', id)"
      @pin-reasoning="(id) => emit('pin-reasoning', id)"
    />

    <!-- ═══ Interleaved Timeline: thinking → tool → thinking → tool → reply ═══ -->
    <template v-if="reactSteps.length">
      <!-- ReAct mode: render each step as a separate section -->
      <div v-for="(step, idx) in reactSteps" :key="`step-${idx}`">
        <!-- Thinking step (planning / reasoning / replanning) -->
        <div v-if="isThinkingStep(step)" class="turn-block__section turn-block__section--thinking">
          <div class="turn-block__section-label">
            <q-icon :name="stepIcon(step.kind)" size="14px" :style="{ color: 'var(--color-accent)' }" />
            <span class="text-caption text-weight-medium" :style="{ color: 'var(--color-accent)' }">{{ stepTitle(step) }}</span>
          </div>
          <ChatReasoningPeek
            :message-id="block.assistant?.id ?? ''"
            :reasoning="step.body"
            :is-dark="isDark"
            :streaming="isAssistantStreaming && idx === reactSteps.length - 1"
            :thinking-only="false"
          />
        </div>
        <!-- Action step: render linked tools inline -->
        <div v-if="step.kind === 'action'" class="turn-block__section turn-block__section--action">
          <div class="turn-block__section-label">
            <q-icon name="build" size="14px" :style="{ color: 'var(--color-warning)' }" />
            <span class="text-caption text-weight-medium turn-block__action-label">{{ t('chat.turn.block.actionLabel', '动作') }}</span>
          </div>
          <div v-if="step.body" class="text-body2 q-mb-xs" v-html="renderStepBody(step.body)" />
          <template v-if="toolDisplay.showToolCalls">
            <ToolCallTimeline v-if="step.linkedTools.length >= 2" :events="step.linkedTools" />
            <ChatExecutionCard
              v-else
              v-for="tool in step.linkedTools"
              :key="tool.id"
              :event="tool"
              :initial-collapsed="isToolEventCompleted(tool)"
              class="q-mb-xs"
              @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
            />
          </template>
        </div>
      </div>
      <!-- Unlinked tools (not matched to any ReAct step) -->
      <ToolStrip
        v-if="unlinkedTools.length"
        :tools="unlinkedTools"
        :is-dark="isDark"
        :is-team-session="isTeamSession"
        :planner-kind="plannerKind"
        :react-tool-link-index="reactToolLinkIndex"
        @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
      />
      <!-- Final answer from ReAct parse: split into reply rounds -->
      <template v-if="reactReplyRounds.length">
        <div v-for="(round, rIdx) in reactReplyRounds" :key="`react-reply-${rIdx}`" class="turn-block__section turn-block__section--reply">
          <div class="turn-block__section-label">
            <q-icon name="article" size="14px" :style="{ color: 'var(--color-success)' }" />
            <span class="text-caption text-weight-medium turn-block__reply-label">
              {{ reactReplyRounds.length > 1 ? t('chat.turn.block.resultLabelN', '回复') + ' ' + (rIdx + 1) : t('chat.turn.block.resultLabel', '回复') }}
            </span>
          </div>
          <div
            class="chat-message-content chat-message-prose"
            :class="{ 'chat-message-content--dark': isDark }"
            v-html="renderReactReplyRound(round, rIdx)"
          />
        </div>
      </template>
    </template>

    <template v-else>
      <!-- Non-ReAct mode: compact timeline (thinking → tools → reply) -->
      <CompactTimeline
        v-if="compactNodes.length"
        :nodes="compactNodes"
        :is-dark="isDark"
        :message-id="block.assistant?.id ?? ''"
        :is-streaming="isAssistantStreaming"
        @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
      />
      <!-- Tools without any reasoning/reply -->
      <template v-if="!compactNodes.length && visibleTools.length && toolDisplay.showToolCalls">
        <ToolCallTimeline v-if="toolEvents.length >= 2" :events="toolEvents" />
        <ToolStrip
          v-else
          :tools="visibleTools"
          :is-dark="isDark"
          :is-team-session="isTeamSession"
          :planner-kind="plannerKind"
          :react-tool-link-index="reactToolLinkIndex"
          @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
        />
      </template>
    </template>

    <!-- Sub-Agent nested section -->
    <div v-if="block.members.length" class="turn-block__sub-agents">
      <ChatMessageRow
        v-for="(member, mIdx) in block.members"
        :key="member.id"
        :message="member"
        :index="mIdx + 2"
        :messages="allMessages"
        :is-dark="isDark"
        :is-team-session="isTeamSession"
        :planner-kind="plannerKind"
        :react-tool-link-index="reactToolLinkIndex"
        :reasoning-sidebar-open="reasoningSidebarOpen"
        @a2ui-user-action="(p) => emit('a2ui-user-action', p)"
        @retry="(id) => emit('retry', id)"
        @dismiss-failed="(id) => emit('dismiss-failed', id)"
        @regenerate="(msg) => emit('regenerate', msg)"
        @pin-reasoning="(id) => emit('pin-reasoning', id)"
      />
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, inject } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatMessageRow from './ChatMessageRow.vue';
import ChatReasoningPeek from './ChatReasoningPeek.vue';
import ChatExecutionCard from './ChatExecutionCard.vue';
import ToolCallTimeline from './ToolCallTimeline.vue';
import CompactTimeline from './CompactTimeline.vue';
import ToolStrip from './ToolStrip.vue';
import type { TurnBlockGroup } from '../../features/chat/groupMessagesByTurn';
import { filterToolsForToolStrip, toolStripSummary } from '../../features/chat/groupMessagesByTurn';
import { toolEventFromMessage } from '../../features/chat/envelopeToolCall';
import {
  messageSourceChipFallback,
  messageSourceChipKey,
  messageSourceFromMessage,
} from '../../features/chat/messageSourceMeta';
import { resolveAssistantPresentation } from '../../features/chat/messagePlannerPresentation';
import { buildCompactNodes } from '../../features/chat/compactTimeline';
import { parseReactPlannerContent, shouldUseReactPlannerView } from '../../features/chat/reactPlannerParse';
import { enrichReactStepsWithToolEvents } from '../../features/chat/reactPlannerToolLink';
import { renderChatMarkdownForMessage, renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';
import type { ReactStepKind } from '../../features/chat/reactPlannerTypes';
import type { A2UIUserActionPayload } from '../../features/chat/a2uiUserAction';
import type { Message, ReactStepWithTools, ReactToolLinkIndex, ToolUseEvent } from '../../features/chat/types';
import { TOOL_DISPLAY_KEY } from '../../features/chat/types';

const props = withDefaults(
  defineProps<{
    block: TurnBlockGroup;
    allMessages: Message[];
    isDark: boolean;
    isTeamSession?: boolean;
    plannerKind?: string;
    reactToolLinkIndex: ReactToolLinkIndex;
    focused?: boolean;
    reasoningSidebarOpen?: boolean;
    collapsed?: boolean;
  }>(),
  { collapsed: false },
);

const emit = defineEmits<{
  'a2ui-user-action': [payload: A2UIUserActionPayload];
  feedback: [payload: { messageId: string; rating: 'positive' | 'negative' }];
  regenerate: [message: Message];
  retry: [messageId: string];
  'dismiss-failed': [messageId: string];
  'pin-reasoning': [messageId: string];
  'toggle-collapse': [];
}>();

const { t } = useI18n();

const toolDisplay = inject(TOOL_DISPLAY_KEY, computed(() => ({ showToolCalls: true })));

// Extract ToolUseEvent[] from the block's tool messages
const toolEvents = computed((): ToolUseEvent[] => {
  return props.block.tools
    .map((msg) => toolEventFromMessage(msg))
    .filter((ev): ev is ToolUseEvent => ev != null);
});

// ── Agent identity from first tool event or assistant message ──
const agentMeta = computed(() => {
  const firstTool = props.block.tools[0];
  if (firstTool) {
    const ev = toolEventFromMessage(firstTool);
    if (ev) return { name: ev.agent_name, key: ev.agent_key };
  }
  const assistant = props.block.assistant;
  if (assistant?.agent_ref) {
    return { name: assistant.agent_ref.name, key: assistant.agent_ref.agent_key };
  }
  return null;
});

const agentName = computed(() => agentMeta.value?.name?.trim() || '');
const agentInitials = computed(() => {
  const name = agentName.value;
  if (!name) return '';
  return name.charAt(0);
});
const agentAvatarColor = computed(() => {
  const key = agentMeta.value?.key || agentName.value || '';
  const colors = ['var(--avatar-color-1)', 'var(--avatar-color-2)', 'var(--avatar-color-3)', 'var(--avatar-color-4)', 'var(--avatar-color-5)', 'var(--avatar-color-6)'];
  let hash = 0;
  for (let i = 0; i < key.length; i++) hash = key.charCodeAt(i) + ((hash << 5) - hash);
  return colors[Math.abs(hash) % colors.length];
});

// ── Block status badge ──
const blockStatusText = computed(() => {
  if (!props.block.isCompleted) return t('chat.turn.block.running', '运行中');
  const summary = toolStripSummary(props.block.tools);
  if (summary.failed > 0) return t('chat.turn.block.failed', '失败');
  if (summary.cancelled > 0) return t('chat.turn.block.cancelled', '已中断');
  return t('chat.turn.block.completed', '已完成');
});

const blockStatusClass = computed(() => {
  if (!props.block.isCompleted) return 'turn-block__status-badge--running';
  const summary = toolStripSummary(props.block.tools);
  if (summary.failed > 0) return 'turn-block__status-badge--failed';
  if (summary.cancelled > 0) return 'turn-block__status-badge--cancelled';
  return 'turn-block__status-badge--completed';
});

const blockDuration = computed(() => {
  const summary = toolStripSummary(props.block.tools);
  if (summary.totalMs > 0) {
    const sec = Math.round(summary.totalMs / 1000);
    return props.block.isCompleted ? `${sec}s` : `${sec}s...`;
  }
  return '';
});

const turnSourceLabel = computed(() => {
  const meta = messageSourceFromMessage(props.block.user ?? null);
  if (!meta) return '';
  const key = messageSourceChipKey(meta);
  return key ? t(key, messageSourceChipFallback(meta)) : messageSourceChipFallback(meta);
});

const isAssistantStreaming = computed(() =>
  props.block.assistant?.status === 'streaming' || props.block.assistant?.status === 'tool_running',
);

// ═══════════════════════════════════════════════════════════════
// ReAct Mode: Parse steps and link tools
// ═══════════════════════════════════════════════════════════════
const reactParsed = computed(() => {
  if (!props.block.assistant) return null;
  const content = props.block.assistant.content_markdown ?? '';
  if (!shouldUseReactPlannerView(props.plannerKind ?? '', content)) return null;
  return parseReactPlannerContent(content);
});

const reactSteps = computed((): ReactStepWithTools[] => {
  if (!reactParsed.value?.steps.length || !props.block.assistant) return [];
  // Find the assistant message index in allMessages for tool linking
  const assistantIdx = props.allMessages.findIndex((m) => m.id === props.block.assistant!.id);
  if (assistantIdx < 0) return [];
  return enrichReactStepsWithToolEvents(reactParsed.value.steps, assistantIdx, props.allMessages);
});

const reactHasExplicitFinalAnswer = computed(() => reactParsed.value?.hasExplicitFinalAnswer ?? false);

const reactFinalAnswer = computed(() => {
  // Only show separate reply section when there's an explicit /*FINAL_ANSWER*/ tag.
  // Without it, the last step's body is a fallback that would duplicate content.
  if (!reactHasExplicitFinalAnswer.value) return '';
  return reactParsed.value?.finalAnswer?.trim() || '';
});

// Split ReAct final answer into reply rounds by paragraph breaks.
const reactReplyRounds = computed((): string[] => {
  const raw = reactFinalAnswer.value;
  if (!raw) return [];
  return raw.split(/\n{2,}/).map((s) => s.trim()).filter((s) => s.length > 0);
});

function renderReactReplyRound(round: string, rIdx: number): string {
  const msgId = props.block.assistant?.id ?? '';
  const isLast = rIdx === reactReplyRounds.value.length - 1;
  return renderChatMarkdownForMessage(`${msgId}-final-${rIdx}`, round, isAssistantStreaming.value && isLast);
}

// Tools NOT linked to any ReAct step (shown in ToolStrip fallback)
const unlinkedTools = computed(() => {
  if (!reactSteps.value.length) return [];
  const linkedIds = new Set<string>();
  for (const step of reactSteps.value) {
    for (const tool of step.linkedTools) {
      if (tool.id) linkedIds.add(tool.id);
    }
  }
  return props.block.tools.filter((msg) => {
    const ev = toolEventFromMessage(msg);
    return ev && !linkedIds.has(ev.id);
  });
});

// ═══════════════════════════════════════════════════════════════
// Non-ReAct Mode: Simple reasoning → tools → body
// ═══════════════════════════════════════════════════════════════
const assistantPresentation = computed(() => {
  if (!props.block.assistant) return null;
  return resolveAssistantPresentation(props.plannerKind ?? '', props.block.assistant);
});

const assistantReasoning = computed(() => assistantPresentation.value?.reasoning?.trim() || '');
const assistantBody = computed(() => assistantPresentation.value?.bodyMarkdown?.trim() || '');

const visibleTools = computed(() => filterToolsForToolStrip(props.block.tools, props.reactToolLinkIndex));

// Compact timeline: thinking (整段) → tools (按顺序) → reply (整段)
// 不切分段落，不配对，不均分工具。
// 替换原 `interleavedRounds` 1:1 配对渲染。
// @see docs/reports/2026-06-10-proposal-chat-compact-timeline.md
const compactNodes = computed(() => {
  if (!props.block.assistant) return [];
  const allToolEvents = visibleTools.value
    .map((msg) => toolEventFromMessage(msg))
    .filter((ev): ev is ToolUseEvent => ev != null);
  return buildCompactNodes({
    reasoning: assistantReasoning.value,
    bodyMarkdown: assistantBody.value,
    toolEvents: allToolEvents,
    messageId: props.block.assistant.id,
    isStreaming: isAssistantStreaming.value,
    messageStatus: props.block.assistant.status,
  });
});

// ═══════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════
function isThinkingStep(step: ReactStepWithTools): boolean {
  return step.kind === 'planning' || step.kind === 'reasoning' || step.kind === 'replanning';
}

function stepIcon(kind: ReactStepKind): string {
  switch (kind) {
    case 'planning': return 'map';
    case 'reasoning': return 'psychology';
    case 'replanning': return 'refresh';
    default: return 'psychology_alt';
  }
}

function stepTitle(step: ReactStepWithTools): string {
  return step.title || t(`chat.react.${step.kind}`, step.kind);
}

function renderStepBody(body: string): string {
  return renderChatMarkdown(body.trim());
}

function isToolEventCompleted(event: ToolUseEvent): boolean {
  const s = event.status;
  return s === 'success' || s === 'failed' || s === 'cancelled';
}

// ── Collapsed state ──
const collapsedSummary = computed(() => {
  const tools = props.block.tools;
  const members = props.block.members;
  const summary = toolStripSummary(tools);
  if (tools.length === 0 && members.length > 0) {
    return `${members.length} ${t('chat.turn.block.memberCount', '个子任务')} · ${t('chat.turn.block.completed', '已完成')}`;
  }
  if (tools.length === 0) {
    return props.block.assistant?.content_markdown?.slice(0, 60) || t('chat.turn.block.completed', '已完成');
  }
  if (tools.length === 1) {
    const ev = toolEventFromMessage(tools[0]!);
    const name = ev?.display_label || ev?.tool_name || t('chat.turn.block.tool', '工具');
    if (summary.failed > 0) return `${name} · ${t('chat.turn.block.failed', '失败')}`;
    if (summary.cancelled > 0) return `${name} · ${t('chat.turn.block.cancelled', '已中断')}`;
    return `${name} · ${t('chat.turn.block.completed', '已完成')}`;
  }
  if (summary.failed > 0) return `${t('chat.turn.block.toolsCount', { count: tools.length })} · ${summary.failed} ${t('chat.turn.block.failed', '失败')}`;
  if (summary.cancelled > 0) return `${t('chat.turn.block.toolsCount', { count: tools.length })} · ${summary.cancelled} ${t('chat.turn.block.cancelled', '已中断')}`;
  return `${t('chat.turn.block.toolsCount', { count: tools.length })} · ${t('chat.turn.block.completed', '已完成')}`;
});

const collapsedIcon = computed(() => {
  const summary = toolStripSummary(props.block.tools);
  if (summary.failed > 0) return 'error';
  if (summary.cancelled > 0) return 'pause_circle';
  return 'check_circle';
});

const collapsedIconColor = computed(() => {
  const summary = toolStripSummary(props.block.tools);
  if (summary.failed > 0) return 'var(--color-danger)';
  if (summary.cancelled > 0) return 'var(--color-warning)';
  return 'var(--color-success)';
});

const collapsedDuration = computed(() => {
  const summary = toolStripSummary(props.block.tools);
  if (summary.totalMs > 0) {
    const sec = Math.round(summary.totalMs / 1000);
    return `${sec}s`;
  }
  return '';
});
</script>

<style scoped lang="sass">
.turn-block
  margin-bottom: var(--space-2)
  padding: var(--space-2) var(--space-2) var(--space-1)
  border-radius: 14px
  background: color-mix(in srgb, var(--glass-surface) 35%, transparent)
  border: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)
  transition: box-shadow 0.25s ease, border-color 0.25s ease, background 0.2s ease

.turn-block--focused
  border-color: color-mix(in srgb, var(--color-accent) 55%, var(--glass-border))
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-accent) 25%, transparent)
  background: color-mix(in srgb, var(--glass-surface) 50%, transparent)

.turn-block__agent-header
  padding: 6px 10px
  border-radius: 8px
  cursor: default
  transition: background 0.15s ease
  &:hover
    background: var(--glass-surface-hover)

.turn-block__avatar
  flex-shrink: 0

.turn-block__status-badge
  font-size: 11px
  padding: 2px 8px
  border-radius: 10px
  font-weight: 500

.turn-block__status-badge--running
  background: color-mix(in srgb, var(--color-accent) 15%, transparent)
  color: var(--color-accent)
  animation: status-pulse 1.5s infinite

.turn-block__status-badge--completed
  background: color-mix(in srgb, var(--color-success) 15%, transparent)
  color: var(--color-success)

.turn-block__status-badge--failed
  background: color-mix(in srgb, var(--color-danger) 15%, transparent)
  color: var(--color-danger)

.turn-block__status-badge--cancelled
  background: color-mix(in srgb, var(--color-warning) 15%, transparent)
  color: var(--color-warning)

.turn-block__expand-icon
  color: var(--color-text-tertiary)

.turn-block__text-tertiary
  color: var(--color-text-tertiary)

.turn-block__action-label
  color: var(--color-warning)

.turn-block__reply-label
  color: var(--color-success)

.turn-block__toggle
  transition: transform 0.2s ease

.turn-block__channel-bar
  margin: calc(-1 * var(--space-1)) 0 var(--space-2)
  padding: var(--space-1) var(--space-2)
  border-radius: 8px
  background: color-mix(in srgb, var(--color-accent) 10%, transparent)
  color: var(--color-text-secondary)
  font-size: 11.5px
  font-weight: 600
  letter-spacing: 0.02em

.turn-block__section-label
  display: flex
  align-items: center
  gap: 5px
  margin-bottom: 4px

.turn-block__section
  padding: var(--space-1) var(--space-2)
  margin-top: var(--space-1)
  border-radius: 8px

.turn-block__section--thinking
  border-left: 3px solid color-mix(in srgb, var(--color-accent) 50%, var(--glass-border))
  background: color-mix(in srgb, var(--color-accent) 4%, transparent)

.turn-block__section--action
  border-left: 3px solid color-mix(in srgb, var(--color-warning) 50%, var(--glass-border))
  background: color-mix(in srgb, var(--color-warning) 4%, transparent)

.turn-block__section--reply
  border-left: 3px solid color-mix(in srgb, var(--color-success) 50%, var(--glass-border))

.turn-block__sub-agents
  margin-left: 12px
  border-left: 2px solid var(--glass-border)
  padding-left: 14px
  margin-top: var(--space-2)

.turn-block--collapsed
  cursor: pointer
  padding: var(--space-1) var(--space-2)
  margin-bottom: var(--space-1)
  border-radius: 8px
  background: color-mix(in srgb, var(--glass-surface) 25%, transparent)
  border: 1px solid color-mix(in srgb, var(--glass-border) 30%, transparent)
  transition: background 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--glass-surface) 45%, transparent)

@keyframes status-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.7
</style>
