<template>
  <div class="agent-block" :class="{ 'agent-block--root': isRoot, 'agent-block--collapsed': localCollapsed }">
    <!-- Header (always visible) -->
    <div class="agent-header" @click="toggleCollapse">
      <div class="agent-header__avatar" :style="avatarStyle">{{ avatarText }}</div>
      <span class="agent-header__name" :style="{ color: block.agentColor }">{{ displayName }}</span>
      <span class="agent-header__status" :class="statusClass">{{ statusLabel }}</span>
      <span v-if="formattedDuration" class="agent-header__duration">{{ formattedDuration }}</span>
      <!-- Sub-task count for root agent -->
      <span v-if="isRoot && subAgentCount > 0" class="agent-header__meta"> · {{ subAgentCount }} {{ t('chat.turn.block.memberCount') }} </span>
      <!-- Team status summary for root agent -->
      <span v-if="isRoot && block.teamStatus" class="agent-header__team-status">
        <span v-if="block.teamStatus.running > 0" class="team-status-running">
          {{ block.teamStatus.running }} {{ t('chat.turn.block.running') }}
        </span>
        <span v-if="block.teamStatus.completed > 0" class="team-status-completed">
          {{ block.teamStatus.completed }} {{ t('chat.turn.block.completed') }}
        </span>
        <span v-if="block.teamStatus.failed > 0" class="team-status-failed"> {{ block.teamStatus.failed }} {{ t('chat.turn.block.failed') }} </span>
      </span>
      <q-icon
        class="agent-header__toggle"
        :class="{ 'agent-header__toggle--expanded': !localCollapsed }"
        name="expand_more"
        size="16px"
      />
    </div>

    <!-- Content (collapsible) -->
    <transition name="agent-collapse">
      <div v-show="!localCollapsed" class="agent-content">
        <!-- Task section -->
        <div v-if="block.task" class="section section--task">
          <div class="section__label">
            <q-icon name="assignment" size="14px" style="color: var(--color-warning)" />
            <span class="section__label-text">{{ t('chat.agentBlock.receiveTask') }}</span>
          </div>
          <div class="section__body">
            <div class="section__body-inner">
              {{ block.task }}
            </div>
          </div>
        </div>

        <!-- Orchestration Plan Card (root agent only) -->
        <PlanCard v-if="isRoot && block.plan" :plan="block.plan" />

        <!-- Timeline entries in chronological order -->
        <div class="agent-timeline">
          <template v-for="(entry, idx) in block.timeline" :key="entryKey(entry)">
            <!-- Thinking entry -->
            <AgentThinkingSection
              v-if="entry.kind === 'thinking'"
              :section="entry.section"
              @streaming-finished="onThinkingFinished(entry.section.id)"
            />

            <!-- Tool entry -->
            <AgentToolSection
              v-else-if="entry.kind === 'tool'"
              :section="entry.section"
              :agent-color="block.agentColor"
            />

            <!-- Reply section rendered as a chronological timeline entry.
                 P1 (reply-chronological): useAgentBlocks now emits a
                 `kind: 'reply'` entry at the assistant message's chronological
                 position for every round, so each round of reply gets its
                 own UI component (no more 1:1 pairing against thinking). -->
            <div v-else-if="entry.kind === 'reply'" class="section section--reply" :data-round="replyRoundIndex(idx)">
              <div class="section__label">
                <q-icon name="article" size="14px" style="color: var(--color-success)" />
                <span class="section__label-text">
                  {{ totalReplyCount > 1 ? t('chat.turn.block.resultLabelN') + ' ' + replyRoundIndex(idx) : t('chat.turn.block.resultLabel') }}
                </span>
                <span v-if="entry.section.durationMs != null" class="section__label-duration">
                  {{ formatDuration(entry.section.durationMs) }}
                </span>
                <span v-if="entry.section.streaming" class="pulse-dot" />
              </div>
              <div class="section__body">
                <div class="section__body-inner">
                  <div class="chat-message-prose" v-html="renderReplyContent(entry.section.content)" />
                  <span v-if="entry.section.streaming" class="cursor-blink"></span>
                </div>
              </div>
            </div>

            <!-- Execution progress step (orchestration / team / tool / thinking) -->
            <div v-else-if="entry.kind === 'progress'" class="section section--progress" :class="progressClass(entry.section)">
              <div class="section__label">
                <span class="section__label-icon">{{ progressIcon(entry.section.category) }}</span>
                <span class="section__label-text">{{ progressCategoryLabel(entry.section.category) }}</span>
                <span class="section__label-message">{{ entry.section.message }}</span>
                <span v-if="entry.section.durationMs != null" class="section__label-duration">
                  {{ formatDuration(entry.section.durationMs) }}
                </span>
                <span v-else-if="entry.section.status === 'running'" class="pulse-dot" />
                <span v-if="entry.section.status === 'failed'" class="section__label-icon" :title="t('chat.turn.block.failed')">{{ progressStatusGlyph('failed') }}</span>
                <span v-else-if="entry.section.status === 'timeout'" class="section__label-icon" :title="t('chat.agentBlock.timeout')">{{ progressStatusGlyph('timeout') }}</span>
              </div>
            </div>

            <!-- Sub-agent entry with tree connector -->
            <div v-else-if="entry.kind === 'subagent'" class="sub-agent-timeline-entry">
              <!-- Tree connector: vertical line + node dot -->
              <div class="tree-connector">
                <div class="tree-connector__line" />
                <div
                  class="tree-connector__node"
                  :class="`tree-connector__node--${entry.block.status}`"
                  :style="{ borderColor: entry.block.agentColor }"
                >
                  <div class="tree-connector__dot" :style="{ background: entry.block.agentColor }" />
                </div>
                <div v-if="idx < lastSubAgentIndex" class="tree-connector__line tree-connector__line--continue" />
              </div>
              <!-- Sub-agent block -->
              <div class="sub-agent-block-wrapper">
                <AgentBlock :block="entry.block" :is-root="false" />
              </div>
            </div>
          </template>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  PROGRESS_GLYPHS,
  PROGRESS_LABELS,
  PROGRESS_STATUS_GLYPHS,
  type AgentBlock as AgentBlockType,
  type TimelineEntry,
} from '../../features/chat/agentTreeTypes';
import AgentThinkingSection from './AgentThinkingSection.vue';
import AgentToolSection from './AgentToolSection.vue';
import PlanCard from './PlanCard.vue';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';
import { formatDuration } from '../../features/chat/agentTreeUtils';

const { t } = useI18n();

const props = defineProps<{
  block: AgentBlockType;
  isRoot: boolean;
}>();

const emit = defineEmits<{
  'streaming-finished': [sectionId: string];
}>();

/**
 * Local collapse state — initialized from props.block.collapsed.
 * Watches for status changes to auto-collapse when agent completes.
 */
const localCollapsed = ref<boolean>(props.block.collapsed);

// Auto-collapse when status transitions to completed
watch(
  () => props.block.status,
  (newStatus, oldStatus) => {
    if (newStatus === 'completed' && oldStatus === 'running') {
      // Delay auto-collapse to let user see the result briefly
      setTimeout(() => {
        localCollapsed.value = true;
      }, 800);
    }
  },
);

const avatarStyle = computed(() => ({
  background: props.block.agentColor,
  width: props.isRoot ? '28px' : '24px',
  height: props.isRoot ? '28px' : '24px',
  fontSize: props.isRoot ? '13px' : '11px',
  color: 'var(--color-surface-solid)',
}));

const statusClass = computed(() => ({
  'agent-header__status--running': props.block.status === 'running',
  'agent-header__status--completed': props.block.status === 'completed',
  'agent-header__status--failed': props.block.status === 'failed',
}));

const statusLabel = computed(() => {
  switch (props.block.status) {
    case 'running':
      return t('chat.turn.block.running');
    case 'completed':
      return t('chat.turn.block.completed');
    case 'failed':
      return t('chat.turn.block.failed');
    default:
      return '';
  }
});

const formattedDuration = computed(() => {
  const ms = props.block.durationMs;
  return ms != null ? formatDuration(ms) : '';
});

const displayName = computed(() => {
  const name = props.block.agentName || '';
  if (name.length <= 16) return name;
  return name.slice(0, 14) + '…';
});

const avatarText = computed(() => {
  const name = props.block.agentName || props.block.agentKey || 'A';
  if (/[\u4e00-\u9fff]/.test(name)) {
    return name.charAt(0);
  }
  return name.charAt(0).toUpperCase();
});

/**
 * P1 (reply-chronological): total number of reply entries in the timeline.
 * Used by the template to decide between "回复" and "回复 N" labels.
 */
const totalReplyCount = computed(() => props.block.timeline.filter((e) => e.kind === 'reply').length);

/**
 * P1 (reply-chronological): compute the 1-based round index for a reply
 * entry based on its position among all reply entries in the timeline.
 * Returns the round number (1, 2, 3, ...) the user sees in the UI label.
 */
function replyRoundIndex(timelineIdx: number): number {
  let count = 0;
  for (let i = 0; i <= timelineIdx && i < props.block.timeline.length; i++) {
    if (props.block.timeline[i].kind === 'reply') count++;
  }
  return count;
}

/** Count of sub-agent entries in timeline */
const subAgentCount = computed(() => props.block.timeline.filter((e) => e.kind === 'subagent').length);

/** Index of the last sub-agent entry (for tree connector continuation) */
const lastSubAgentIndex = computed(() => {
  let lastIdx = -1;
  for (let i = 0; i < props.block.timeline.length; i++) {
    if (props.block.timeline[i].kind === 'subagent') lastIdx = i;
  }
  return lastIdx;
});

function entryKey(entry: TimelineEntry): string {
  switch (entry.kind) {
    case 'thinking':
      return `thinking-${entry.section.id}`;
    case 'tool':
      return `tool-${entry.section.id}`;
    case 'reply':
      return `reply-${entry.section.id}`;
    case 'progress':
      return `progress-${entry.section.id}`;
    case 'subagent':
      return `subagent-${entry.block.id}`;
  }
}

function renderReplyContent(content: string): string {
  return renderChatMarkdown(content || '');
}

/** Map a ProgressCategory to a single-character / emoji glyph */
function progressIcon(category: string): string {
  return PROGRESS_GLYPHS[category as keyof typeof PROGRESS_GLYPHS] ?? '•';
}

/** Map a ProgressCategory to a display label via i18n */
function progressCategoryLabel(category: string): string {
  const key = PROGRESS_LABELS[category as keyof typeof PROGRESS_LABELS];
  return key ? t(key) : t('chat.agentBlock.progress');
}

/** Map ProgressSection.status to a CSS modifier class for color cues */
function progressClass(section: { status: string }): Record<string, boolean> {
  return {
    'section--progress-running': section.status === 'running',
    'section--progress-done': section.status === 'done',
    'section--progress-failed': section.status === 'failed',
    'section--progress-timeout': section.status === 'timeout',
  };
}

/** Map ProgressSection.status to a status glyph (with accessible title). */
function progressStatusGlyph(status: string): string {
  return PROGRESS_STATUS_GLYPHS[status as keyof typeof PROGRESS_STATUS_GLYPHS] ?? '';
}

function toggleCollapse() {
  localCollapsed.value = !localCollapsed.value;
}

function onThinkingFinished(sectionId: string) {
  emit('streaming-finished', sectionId);
}

// Listen for global expand/collapse events from AgentTreeTimeline
function onExpandAll() {
  localCollapsed.value = false;
}

function onCollapseAll() {
  localCollapsed.value = true;
}

onMounted(() => {
  window.addEventListener('agent-tree-expand-all', onExpandAll);
  window.addEventListener('agent-tree-collapse-all', onCollapseAll);
});

onUnmounted(() => {
  window.removeEventListener('agent-tree-expand-all', onExpandAll);
  window.removeEventListener('agent-tree-collapse-all', onCollapseAll);
});
</script>

<style scoped lang="sass">
.agent-block
  border: 1px solid var(--glass-border)
  border-radius: 12px
  overflow: hidden
  margin-bottom: var(--space-4, 16px)
  transition: border-color 0.2s
  background: var(--glass-surface)
  animation: fade-in 0.3s ease

  &:hover
    border-color: var(--glass-border-hover)

.agent-block--root
  border-color: color-mix(in srgb, var(--color-accent) 30%, transparent)

.agent-header
  display: flex
  align-items: center
  gap: 8px
  padding: 10px 14px
  cursor: pointer
  user-select: none
  transition: background 0.15s

  &:hover
    background: var(--glass-surface-hover)

.agent-header__avatar
  width: 28px
  height: 28px
  border-radius: 50%
  display: inline-flex
  align-items: center
  justify-content: center
  font-size: 13px
  font-weight: 600
  flex-shrink: 0
  color: var(--color-surface-solid)

.agent-header__name
  font-weight: 600
  font-size: 14px

.agent-header__status
  font-size: 11px
  padding: 2px 8px
  border-radius: 10px
  font-weight: 500

.agent-header__status--running
  background: color-mix(in srgb, var(--color-accent) 15%, transparent)
  color: var(--color-accent)
  animation: status-pulse 1.5s infinite

.agent-header__status--completed
  background: color-mix(in srgb, var(--color-success) 15%, transparent)
  color: var(--color-success)

.agent-header__status--failed
  background: color-mix(in srgb, var(--color-danger) 15%, transparent)
  color: var(--color-danger)

.agent-header__duration
  color: var(--color-text-tertiary)
  font-size: 11px

.agent-header__meta
  color: var(--color-text-tertiary)
  font-size: 11px

.agent-header__team-status
  display: flex
  gap: 6px
  font-size: 11px

.team-status-running
  color: var(--color-accent)

.team-status-completed
  color: var(--color-success)

.team-status-failed
  color: var(--color-danger)

.agent-header__toggle
  margin-left: auto
  color: var(--color-text-tertiary)
  font-size: 12px
  transition: transform 0.2s

.agent-header__toggle--expanded
  transform: rotate(180deg)

.agent-content
  padding: 4px 14px 14px
  overflow: hidden

.agent-timeline
  position: relative

// ── Tree connector for sub-agents ──
.sub-agent-timeline-entry
  display: flex
  gap: 0
  margin-bottom: 12px
  position: relative

.tree-connector
  display: flex
  flex-direction: column
  align-items: center
  width: 20px
  flex-shrink: 0
  position: relative

.tree-connector__line
  width: 2px
  flex: 1
  background: var(--glass-border)
  min-height: 8px

.tree-connector__line--continue
  background: var(--glass-border)

.tree-connector__node
  width: 14px
  height: 14px
  border-radius: 50%
  border: 2px solid var(--glass-border)
  background: var(--color-surface-solid)
  display: flex
  align-items: center
  justify-content: center
  flex-shrink: 0
  z-index: 1

.tree-connector__dot
  width: 6px
  height: 6px
  border-radius: 50%

.tree-connector__node--running
  border-color: var(--color-accent)
  .tree-connector__dot
    animation: node-pulse 1.5s infinite

.tree-connector__node--completed
  border-color: var(--color-success)
  background: color-mix(in srgb, var(--color-success) 15%, var(--color-surface-solid))

.tree-connector__node--failed
  border-color: var(--color-danger)
  background: color-mix(in srgb, var(--color-danger) 15%, var(--color-surface-solid))

.sub-agent-block-wrapper
  flex: 1
  min-width: 0

  .agent-block
    margin-bottom: 0

// ── Sections ──
.section
  margin-bottom: 12px

.section__label
  display: flex
  align-items: center
  gap: 5px
  font-size: 12px
  font-weight: 500
  margin-bottom: 4px
  cursor: pointer
  user-select: none
  padding: 2px 0
  border-radius: 4px
  transition: background 0.12s

  &:hover
    background: color-mix(in srgb, var(--color-text-primary) 4%, transparent)

.section__label-text
  text-transform: uppercase
  letter-spacing: 0.3px

.section__body-inner
  padding: 6px 10px
  border-radius: 6px
  font-size: 13px
  line-height: 1.5

.section--task
  .section__label
    color: var(--color-warning)

  .section__body-inner
    background: color-mix(in srgb, var(--color-warning) 6%, transparent)
    border-left: 2px solid var(--color-warning)
    border-radius: 0 6px 6px 0
    color: var(--color-text-secondary)

.section--result
  .section__label
    color: var(--color-success)

  .section__body-inner
    background: color-mix(in srgb, var(--color-success) 6%, transparent)
    border-left: 2px solid var(--color-success)
    border-radius: 0 6px 6px 0
    color: var(--color-text-primary)
    line-height: 1.6

.section--reply
  margin-top: 4px
  margin-bottom: 12px

  .section__label
    color: var(--color-success)

  .section__body-inner
    background: color-mix(in srgb, var(--color-success) 6%, transparent)
    border-left: 2px solid var(--color-success)
    border-radius: 0 6px 6px 0
    color: var(--color-text-primary)
    line-height: 1.6

// ── Progress section (orchestration / team / tool / thinking step) ──
.section--progress
  margin-bottom: 6px
  padding: 4px 8px
  border-radius: 6px
  border-left: 2px solid var(--color-text-tertiary)
  background: color-mix(in srgb, var(--color-text-primary) 3%, transparent)
  transition: background 0.2s, border-color 0.2s

  .section__label
    color: var(--color-text-secondary)
    font-size: 12px
    text-transform: none
    letter-spacing: 0
    margin-bottom: 0
    gap: 6px

  .section__label-text
    text-transform: uppercase
    letter-spacing: 0.3px
    color: var(--color-text-tertiary)
    font-size: 10px
    font-weight: 600

  .section__label-message
    flex: 1
    color: var(--color-text-secondary)
    font-size: 12px
    font-weight: 500
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  .section__label-duration
    color: var(--color-text-tertiary)
    font-size: 10px

.section--progress-running
  border-left-color: var(--color-accent)
  background: color-mix(in srgb, var(--color-accent) 5%, transparent)

  .section__label-message
    color: var(--color-text-primary)

.section--progress-done
  border-left-color: var(--color-success)
  background: color-mix(in srgb, var(--color-success) 5%, transparent)
  opacity: 0.85

  .section__label-message
    color: var(--color-text-tertiary)

.section--progress-failed
  border-left-color: var(--color-danger)
  background: color-mix(in srgb, var(--color-danger) 6%, transparent)

.section--progress-timeout
  border-left-color: var(--color-warning)
  background: color-mix(in srgb, var(--color-warning) 6%, transparent)

// ── Collapse transition ──
.agent-collapse-enter-active,
.agent-collapse-leave-active
  transition: max-height 0.3s ease, opacity 0.2s ease, padding 0.3s ease
  overflow: hidden

.agent-collapse-enter-from,
.agent-collapse-leave-to
  max-height: 0
  opacity: 0
  padding-top: 0
  padding-bottom: 0

// ── Animations ──
@keyframes fade-in
  from
    opacity: 0
    transform: translateY(4px)
  to
    opacity: 1
    transform: translateY(0)

@keyframes status-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.7

@keyframes node-pulse
  0%, 100%
    opacity: 1
    transform: scale(1)
  50%
    opacity: 0.5
    transform: scale(0.8)
</style>
