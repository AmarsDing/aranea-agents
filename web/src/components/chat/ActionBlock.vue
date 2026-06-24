<template>
  <div class="act-activity">
    <!-- Card variant -->
    <!-- Todo write: render structured plan list instead of raw JSON -->
    <TodoInlineList v-if="isTodo" :event="todoEvent" />
    <template v-else>
      <div class="act-activity__header" @click="toggleExpand">
        <span class="act-activity__icon">🔧</span>
        <span class="act-activity__tool-label">{{ activity.tool.toolLabel }}</span>
        <span v-if="headerHint" class="act-activity__hint" :title="headerHint">{{ headerHint }}</span>
        <span class="act-activity__status" :class="statusClass">{{ statusIcon }}</span>
        <span v-if="activity.tool.durationMs != null" class="act-activity__duration">{{ formattedDuration }}</span>
      </div>
      <div v-if="expanded" class="act-activity__detail">
        <div v-if="formattedResult" class="act-activity__result-summary">
          <div class="act-activity__detail-label">{{ t('chat.toolResultSummary') }}</div>
          <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
          <div class="act-activity__markdown chat-message-prose" v-html="renderedResultSummary" />
        </div>
        <div v-if="activity.tool.arguments" class="act-activity__args">
          <div class="act-activity__detail-label">{{ t('chat.toolArgs') }}</div>
          <pre class="act-activity__code">{{ activity.tool.arguments }}</pre>
        </div>
        <div v-if="activity.tool.result" class="act-activity__result">
          <div class="act-activity__detail-label">{{ t('chat.toolRawResult') }}</div>
          <pre class="act-activity__code">{{ activity.tool.result }}</pre>
        </div>
        <div v-if="activity.tool.error" class="act-activity__error">
          <div class="act-activity__detail-label">{{ t('chat.toolError') }}</div>
          <pre class="act-activity__code">{{ activity.tool.error }}</pre>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue';
import type { ActionEvent } from '../../features/chat/streamEventTypes';
import type { ToolUseEvent } from '../../features/chat/types';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { isTodoWriteTool } from '../../features/chat/activityPresentation';
import { formatToolResultSummary } from '../../features/chat/toolEventMarkdown';
import { renderChatMarkdownForMessage } from '../../features/chat/chatMessageMarkdown';
import { useCollapseState } from '../../features/chat/composables/useCollapseState';
import TodoInlineList from './TodoInlineList.vue';

/** T8.4: Tool result/arguments longer than this threshold auto-collapse. */
const RESULT_COLLAPSE_THRESHOLD = 500;

const props = defineProps<{
  activity: ActionEvent;
  agentColor?: string;
}>();

const isTodo = computed(() => isTodoWriteTool(props.activity.tool.toolName));

function tryParseJson(s: string | null): unknown {
  if (!s) return undefined;
  try {
    return JSON.parse(s);
  } catch {
    return undefined;
  }
}

const parsedToolArgs = computed<Record<string, unknown> | undefined>(
  () => tryParseJson(props.activity.tool.arguments) as Record<string, unknown> | undefined,
);

const parsedToolResult = computed<Record<string, unknown> | undefined>(
  () => tryParseJson(props.activity.tool.result) as Record<string, unknown> | undefined,
);

/** Format a readable result summary for the expanded tool card. */
const formattedResult = computed(() => {
  const event: Pick<ToolUseEvent, 'tool_name' | 'result'> = {
    tool_name: props.activity.tool.toolName,
    result: parsedToolResult.value,
  };
  return formatToolResultSummary(event);
});

const renderedResultSummary = computed(() =>
  renderChatMarkdownForMessage(props.activity.id, formattedResult.value, false),
);

/**
 * Compact execution-result hint shown in the tool header.
 * For read_file this is the file path; for exec_command the command; etc.
 * It surfaces the "what happened" instead of a generic tool label.
 */
const headerHint = computed(() => {
  const name = props.activity.tool.toolName;
  const args = parsedToolArgs.value;
  const result = parsedToolResult.value;

  const fileOps = new Set([
    'read_file',
    'file_read_file',
    'save_file',
    'write_file',
    'file_write',
    'file_edit',
    'list_file',
  ]);
  if (fileOps.has(name)) {
    let path = '';
    if (typeof args?.path === 'string') {
      path = args.path;
    } else if (typeof result?.path === 'string') {
      path = result.path;
    }
    if (path) return path;
  }

  const shellOps = new Set(['exec_command', 'bash', 'shell', 'shell_exec', 'workspace_exec']);
  if (shellOps.has(name)) {
    const cmd = typeof args?.command === 'string' ? args.command : '';
    if (cmd) return cmd.length > 80 ? `${cmd.slice(0, 80)}…` : cmd;
  }

  const searchOps = new Set(['search_content', 'search_files', 'grep']);
  if (searchOps.has(name)) {
    let q = '';
    if (typeof args?.query === 'string') {
      q = args.query;
    } else if (typeof args?.pattern === 'string') {
      q = args.pattern;
    } else if (typeof args?.path === 'string') {
      q = args.path;
    }
    if (q) return q.length > 80 ? `${q.slice(0, 80)}…` : q;
  }

  // Safe fallback: show the first short string argument that is not sensitive.
  if (args) {
    for (const [key, value] of Object.entries(args)) {
      if (typeof value === 'string' && value) {
        const lower = key.toLowerCase();
        if (
          lower.includes('key') ||
          lower.includes('token') ||
          lower.includes('secret') ||
          lower.includes('password') ||
          lower.includes('auth') ||
          lower.includes('cookie')
        ) {
          continue;
        }
        return value.length > 60 ? `${value.slice(0, 60)}…` : value;
      }
    }
  }
  return '';
});

/** T8.4: Compute total content length to decide auto-collapse. */
const contentLength = computed(() => {
  const result = props.activity.tool.result ?? '';
  const args = props.activity.tool.arguments ?? '';
  return result.length + args.length;
});

// T8.4: Persisted collapse state (remembered across re-renders/refreshes).
// Tool cards default expanded so users can see execution results immediately.
const { collapsed, toggle, setCollapsed } = useCollapseState(`action:${props.activity.id}`, false);

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
  try {
    parsedArgs = t.arguments ? JSON.parse(t.arguments) : undefined;
  } catch {
    parsedArgs = t.arguments;
  }
  let parsedResult: unknown;
  try {
    parsedResult = t.result ? JSON.parse(t.result) : undefined;
  } catch {
    parsedResult = t.result;
  }
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
    case 'running':
      return '⏳';
    case 'success':
      return '✓';
    case 'failed':
      return '✗';
    case 'blocked':
      return '🔒';
    case 'cancelled':
      return '⊘';
    default:
      return '🔧';
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

  &__hint
    font-size: 12px
    color: var(--color-text-secondary)
    margin-left: 4px
    max-width: 280px
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

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

  &__result-summary
    margin-bottom: 8px

  &__markdown
    font-size: 13px
    margin-left: 20px
    :deep(pre)
      max-height: 240px
      overflow-y: auto
</style>
