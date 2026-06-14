<template>
  <div class="act-activity" :class="`act-activity--${variant}`">
    <!-- Card variant -->
    <template v-if="variant === 'card'">
      <!-- Todo write: render structured plan list instead of raw JSON -->
      <TodoInlineList v-if="isTodo" :event="todoEvent" />
      <template v-else>
        <div class="act-activity__header" @click="toggleExpand">
          <span class="act-activity__icon">🔧</span>
          <span class="act-activity__tool-label">{{ activity.tool.toolLabel }}</span>
          <span class="act-activity__status" :class="statusClass">{{ statusIcon }}</span>
          <span v-if="activity.tool.durationMs != null" class="act-activity__duration">{{ formattedDuration }}</span>
        </div>
        <div v-if="expanded" class="act-activity__detail">
          <div v-if="activity.tool.arguments" class="act-activity__args">
            <div class="act-activity__detail-label">参数</div>
            <pre class="act-activity__code">{{ activity.tool.arguments }}</pre>
          </div>
          <div v-if="activity.tool.result" class="act-activity__result">
            <div class="act-activity__detail-label">结果</div>
            <pre class="act-activity__code">{{ activity.tool.result }}</pre>
          </div>
          <div v-if="activity.tool.error" class="act-activity__error">
            <div class="act-activity__detail-label">错误</div>
            <pre class="act-activity__code">{{ activity.tool.error }}</pre>
          </div>
        </div>
      </template>
    </template>

    <!-- Compact variant -->
    <template v-else>
      <div class="act-activity__compact">
        <span class="act-activity__compact-icon">{{ statusIcon }}</span>
        <span class="act-activity__compact-tool">{{ activity.tool.toolLabel }}</span>
        <span v-if="activity.tool.durationMs != null" class="act-activity__compact-duration">{{ formattedDuration }}</span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import type { ActActivity as ActActivityType, ActivityVariant } from '../../features/chat/activityTimelineTypes';
import type { ToolUseEvent } from '../../features/chat/types';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { isTodoWriteTool } from '../../features/chat/activityPresentation';
import TodoInlineList from './TodoInlineList.vue';

const props = defineProps<{
  activity: ActActivityType;
  variant?: ActivityVariant;
  agentColor?: string;
}>();

const expanded = ref(false);

function toggleExpand() {
  expanded.value = !expanded.value;
}

const isTodo = computed(() => isTodoWriteTool(props.activity.tool.toolName));

/** Adapt ToolActivity (AF type) to ToolUseEvent for TodoInlineList consumption. */
const todoEvent = computed<ToolUseEvent>(() => {
  const t = props.activity.tool;
  let parsedArgs: unknown;
  try { parsedArgs = t.arguments ? JSON.parse(t.arguments) : undefined; } catch { parsedArgs = t.arguments; }
  let parsedResult: unknown;
  try { parsedResult = t.result ? JSON.parse(t.result) : undefined; } catch { parsedResult = t.result; }
  return {
    id: props.activity.id,
    phase: t.status === 'running' ? 'before' : 'after',
    status: t.status,
    agent_id: '',
    agent_key: '',
    agent_name: '',
    tool_name: t.toolName,
    tool_label: t.toolLabel,
    arguments: parsedArgs,
    result: parsedResult,
    error: t.error ?? undefined,
    occurred_at: '',
    duration_ms: t.durationMs ?? undefined,
  };
});

const statusClass = computed(() => ({
  'act-activity__status--running': props.activity.tool.status === 'running',
  'act-activity__status--success': props.activity.tool.status === 'success',
  'act-activity__status--failed': props.activity.tool.status === 'failed',
  'act-activity__status--blocked': props.activity.tool.status === 'blocked',
}));

const statusIcon = computed(() => {
  switch (props.activity.tool.status) {
    case 'running': return '⏳';
    case 'success': return '✓';
    case 'failed': return '✗';
    case 'blocked': return '🔒';
    case 'cancelled': return '⊘';
    default: return '🔧';
  }
});

const formattedDuration = computed(() => formatDuration(props.activity.tool.durationMs));
</script>

<style lang="sass" scoped>
.act-activity
  &__header
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 0
    cursor: pointer
    user-select: none

  &__icon
    font-size: 14px

  &__tool-label
    font-size: 13px
    color: var(--color-text-primary)
    font-weight: 500

  &__status
    font-size: 12px
    &--success
      color: var(--color-success)
    &--failed
      color: var(--color-danger)
    &--running
      color: var(--color-accent)
    &--blocked
      color: var(--color-warning)

  &__duration
    font-size: 11px
    color: var(--color-text-secondary)

  &__detail
    margin-left: 20px
    margin-bottom: 4px

  &__detail-label
    font-size: 11px
    color: var(--color-text-secondary)
    margin-bottom: 2px

  &__code
    font-size: 12px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 6px
    padding: 6px 8px
    overflow-x: auto
    max-height: 200px
    overflow-y: auto
    margin: 0

  &__error
    .act-activity__code
      border-color: var(--color-danger)

  &__compact
    display: flex
    align-items: center
    gap: 6px
    padding: 2px 0
    font-size: 12px

  &__compact-icon
    font-size: 11px

  &__compact-tool
    color: var(--color-text-primary)

  &__compact-duration
    color: var(--color-text-secondary)
    font-size: 11px
</style>
