<template>
  <div class="act-activity">
    <!-- Card variant -->
    <template>
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
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue';
import type { ActionEvent } from '../../features/chat/streamEventTypes';
import type { ToolUseEvent } from '../../features/chat/types';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { isTodoWriteTool } from '../../features/chat/activityPresentation';
import { useCollapseState } from '../../features/chat/composables/useCollapseState';
import TodoInlineList from './TodoInlineList.vue';

/** T8.4: Tool result/arguments longer than this threshold auto-collapse. */
const RESULT_COLLAPSE_THRESHOLD = 500;

const props = defineProps<{
  activity: ActionEvent;
  agentColor?: string;
}>();

const isTodo = computed(() => isTodoWriteTool(props.activity.tool.toolName));

/** T8.4: Compute total content length to decide auto-collapse. */
const contentLength = computed(() => {
  const result = props.activity.tool.result ?? '';
  const args = props.activity.tool.arguments ?? '';
  return result.length + args.length;
});

/** T8.4: Default collapsed when content exceeds threshold. */
const defaultCollapsed = computed(() => contentLength.value > RESULT_COLLAPSE_THRESHOLD);

// T8.4: Persisted collapse state (remembered across re-renders/refreshes).
// Note: useCollapseState reads the initial defaultCollapsed at setup time.
// For activity blocks, the content is typically available immediately, so
// this is fine. If content arrives later, the watch below syncs the default.
const { collapsed, toggle, setCollapsed } = useCollapseState(
  `action:${props.activity.id}`,
  defaultCollapsed.value,
);

// If content grows beyond threshold after initial render (e.g., streaming result),
// auto-collapse unless the user has explicitly expanded.
watch(contentLength, (len, prevLen) => {
  if (len > RESULT_COLLAPSE_THRESHOLD && prevLen <= RESULT_COLLAPSE_THRESHOLD) {
    setCollapsed(true);
  }
});

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

const expanded = computed(() => !collapsed.value);

function toggleExpand() {
  toggle();
}

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
</style>
