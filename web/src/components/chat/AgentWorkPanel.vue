<template>
  <div class="agent-work-panel">
    <!-- Agent header -->
    <div class="agent-work-panel__header">
      <agent-avatar-q
        :icon="agentWork.agentIcon"
        size="24px"
        avatar-class="agent-work-panel__avatar"
      />
      <span class="agent-work-panel__name" :style="{ color: agentWork.agentColor }">
        {{ agentWork.agentName }}
      </span>
      <span class="agent-work-panel__status" :class="statusClass">
        {{ statusLabel }}
      </span>
      <span v-if="formattedDuration" class="agent-work-panel__duration">
        {{ formattedDuration }}
      </span>
    </div>

    <!-- Branch: TeamPanel, EventStream -->
    <div class="agent-work-panel__body">
      <!-- Running indicator when no activities yet (waiting for LLM first byte) -->
      <div v-if="agentWork.status === 'running' && !agentWork.activities.length && !agentWork.progressSections?.length" class="agent-work-panel__waiting">
        <span class="pulse-dot"></span>
        <span class="agent-work-panel__waiting-text">{{ t('chat.thinking.thinking', '正在思考…') }}</span>
      </div>
      <!-- Progress sections (orchestration / thinking / tool steps from execution_progress envelopes) -->
      <template v-if="agentWork.progressSections?.length">
        <div v-for="(ps, psi) in agentWork.progressSections" :key="'ps-' + psi" class="agent-work-panel__progress" :class="progressClass(ps)">
          <span class="agent-work-panel__progress-icon">{{ progressIcon(ps.category) }}</span>
          <span class="agent-work-panel__progress-message">{{ ps.message }}</span>
          <span v-if="ps.durationMs != null" class="agent-work-panel__progress-duration">{{ formatDuration(ps.durationMs) }}</span>
          <span v-else-if="ps.status === 'running'" class="pulse-dot" />
          <span v-if="ps.status === 'failed'" class="agent-work-panel__progress-status" :title="t('chat.turn.block.failed')">{{ progressStatusGlyph('failed') }}</span>
          <span v-else-if="ps.status === 'timeout'" class="agent-work-panel__progress-status" :title="t('chat.agentBlock.timeout')">{{ progressStatusGlyph('timeout') }}</span>
        </div>
      </template>
      <!-- Team panel (v7 style) — displayed above EventStream when panel data exists -->
      <TeamPanel v-if="agentWork.panel" :panel="agentWork.panel" />
      <!-- Unified event stream rendering (replaces TaskBoard + ActivityTimeline) -->
      <EventStream
        v-if="agentWork.activities.length"
        :events="agentWork.activities"
        :agent-color="agentWork.agentColor"
        :activity-tree="agentWork.activityTree"
        @confirm="(id, approved) => $emit('confirm', id, approved)"
        @error-retry="(e) => $emit('error-retry', e)"
        @error-switch-model="(e) => $emit('error-switch-model', e)"
        @error-rephrase="(e) => $emit('error-rephrase', e)"
        @error-check-config="(e) => $emit('error-check-config', e)"
        @error-remove-attachment="(e) => $emit('error-remove-attachment', e)"
        @error-relogin="(e) => $emit('error-relogin', e)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { AgentWorkProcess } from '../../features/chat/activityTimelineTypes';
import type { ProgressCategory, ProgressSection } from '../../features/chat/agentTreeTypes';
import { PROGRESS_GLYPHS, PROGRESS_STATUS_GLYPHS } from '../../features/chat/agentTreeTypes';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import type { ErrorEvent } from '../../features/chat/streamEventTypes';
import EventStream from './EventStream.vue';
import TeamPanel from './TeamPanel.vue';
import AgentAvatarQ from '../avatar/AgentAvatarQ.vue';

const { t } = useI18n();

const props = defineProps<{
  agentWork: AgentWorkProcess;
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

const statusClass = computed(() => ({
  'agent-work-panel__status--running': props.agentWork.status === 'running',
  'agent-work-panel__status--completed': props.agentWork.status === 'completed',
  'agent-work-panel__status--failed': props.agentWork.status === 'failed',
}));

const statusLabel = computed(() => {
  switch (props.agentWork.status) {
    case 'running': return t('chat.turn.block.running');
    case 'completed': return t('chat.turn.block.completed');
    case 'failed': return t('chat.turn.block.failed');
    default: return '';
  }
});

const formattedDuration = computed(() => props.agentWork.durationMs != null ? formatDuration(props.agentWork.durationMs) : '');

function progressIcon(category: ProgressCategory): string {
  return PROGRESS_GLYPHS[category] ?? '•';
}

function progressStatusGlyph(status: 'running' | 'done' | 'failed' | 'timeout'): string {
  return PROGRESS_STATUS_GLYPHS[status] ?? '';
}

function progressClass(ps: ProgressSection): Record<string, boolean> {
  return {
    'agent-work-panel__progress--running': ps.status === 'running',
    'agent-work-panel__progress--done': ps.status === 'done',
    'agent-work-panel__progress--failed': ps.status === 'failed',
    'agent-work-panel__progress--timeout': ps.status === 'timeout',
  };
}
</script>

<style lang="sass" scoped>
.agent-work-panel
  margin-left: 38px

  &__header
    display: flex
    align-items: center
    gap: 8px
    margin-bottom: 8px

  &__avatar
    flex-shrink: 0

  &__name
    font-weight: 600
    font-size: 14px

  &__status
    font-size: 12px
    padding: 1px 6px
    border-radius: 8px
    background: var(--glass-surface)
    color: var(--color-text-secondary)

    &--running
      color: var(--color-accent)

    &--completed
      color: var(--color-success)

    &--failed
      color: var(--color-danger)

  &__duration
    font-size: 12px
    color: var(--color-text-secondary)

  &__body
    margin-left: 12px

  &__waiting
    display: flex
    align-items: center
    gap: 8px
    padding: 8px 0
    color: var(--color-text-secondary)
    font-style: italic

  &__waiting-text
    font-size: 13px

  &__progress
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 0
    font-size: 13px

    &--running
      color: var(--color-accent)

    &--done
      color: var(--color-success)

    &--failed
      color: var(--color-danger)

    &--timeout
      color: var(--color-warning)

  &__progress-icon
    flex-shrink: 0

  &__progress-message
    flex: 1

  &__progress-duration
    font-size: 12px
    color: var(--color-text-secondary)

  &__progress-status
    flex-shrink: 0
</style>
