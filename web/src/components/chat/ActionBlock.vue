<template>
  <div class="act-activity">
    <!-- Card variant -->
    <!-- Todo write: render structured plan list instead of raw JSON -->
    <TodoInlineList v-if="isTodo" :event="todoEvent" />
    <template v-else>
      <div class="act-activity__header" @click="toggleExpand">
        <span class="act-activity__icon">{{ toolIcon }}</span>
        <span class="act-activity__tool-label">{{ activity.tool.toolLabel }}</span>
        <span v-if="headerHint" class="act-activity__hint" :title="headerHint">{{ headerHint }}</span>
        <span class="act-activity__status" :class="statusClass">{{ statusIcon }}</span>
        <span v-if="activity.tool.durationMs != null" class="act-activity__duration">{{ formattedDuration }}</span>
      </div>
      <!-- Phase 3: dispatch to per-category detail component (§8.4 of analysis doc) -->
      <component :is="detailComponent" v-if="expanded" :activity="activity" class="act-activity__detail" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, watch, type Component } from 'vue';
import type { ActionEvent } from '../../features/chat/streamEventTypes';
import type { ToolUseEvent } from '../../features/chat/types';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { isTodoWriteTool } from '../../features/chat/activityPresentation';
import { useCollapseState } from '../../features/chat/composables/useCollapseState';
import TodoInlineList from './TodoInlineList.vue';
import ShellToolDetail from './tools/ShellToolDetail.vue';
import BrowserToolDetail from './tools/BrowserToolDetail.vue';
import FileReadToolDetail from './tools/FileReadToolDetail.vue';
import FileWriteToolDetail from './tools/FileWriteToolDetail.vue';
import FileSearchToolDetail from './tools/FileSearchToolDetail.vue';
import WebSearchToolDetail from './tools/WebSearchToolDetail.vue';
import McpToolDetail from './tools/McpToolDetail.vue';
import CodeToolDetail from './tools/CodeToolDetail.vue';
import TodoToolDetail from './tools/TodoToolDetail.vue';
import GenericToolDetail from './tools/GenericToolDetail.vue';

/** T8.4: Tool result/arguments longer than this threshold auto-collapse. */
const RESULT_COLLAPSE_THRESHOLD = 500;

const props = defineProps<{
  activity: ActionEvent;
  agentColor?: string;
}>();

const isTodo = computed(() => isTodoWriteTool(props.activity.tool.toolName));

/**
 * Phase 3: select a detail component by tool_category (AF).
 * Falls back to GenericToolDetail when category is missing or unknown.
 * todo_write is intercepted earlier by `isTodo` -> TodoInlineList, but
 * TodoToolDetail is mapped for completeness/consistency.
 */
const detailComponent = computed<Component>(() => {
  const cat = props.activity.tool.toolCategory ?? 'other';
  switch (cat) {
    case 'shell':
      return ShellToolDetail;
    case 'browser':
      return BrowserToolDetail;
    case 'file_read':
      return FileReadToolDetail;
    case 'file_write':
      return FileWriteToolDetail;
    case 'file_search':
      return FileSearchToolDetail;
    case 'web_search':
      return WebSearchToolDetail;
    case 'mcp':
      return McpToolDetail;
    case 'code':
      return CodeToolDetail;
    case 'todo':
      return TodoToolDetail;
    default:
      return GenericToolDetail;
  }
});

/**
 * Tool icon based on tool_category (AF).
 * Falls back to a generic wrench when category is unknown.
 */
const toolIcon = computed(() => {
  const cat = props.activity.tool.toolCategory;
  if (!cat || cat === 'other') return '🔧';
  const iconMap: Record<string, string> = {
    shell: '$',
    browser: '🌐',
    file_read: '📖',
    file_write: '✏️',
    file_search: '🔍',
    web_search: '🔎',
    mcp: '🔌',
    code: '💻',
    todo: '✅',
  };
  return iconMap[cat] ?? '🔧';
});

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

/**
 * Compact execution-result hint shown in the tool header.
 * For read_file this is the file path; for exec_command the command; etc.
 * It surfaces the "what happened" instead of a generic tool label.
 *
 * AF: When tool_category is available, use it to pick the hint strategy
 * directly (no need to match tool_name strings). Falls back to name-based
 * matching for legacy events without tool_category.
 */
const headerHint = computed(() => {
  const name = props.activity.tool.toolName;
  const args = parsedToolArgs.value;
  const result = parsedToolResult.value;
  const cat = props.activity.tool.toolCategory;

  // AF path: use tool_category to pick hint strategy.
  if (cat) {
    const hint = hintByCategory(cat, name, args, result);
    if (hint) return hint;
  }

  // Legacy fallback: name-based matching (for events without tool_category).
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

/** Extract a compact hint from tool arguments/result based on tool_category. */
function hintByCategory(
  cat: string,
  name: string,
  args: Record<string, unknown> | undefined,
  result: Record<string, unknown> | undefined,
): string {
  const truncate = (s: string, max = 80) => (s.length > max ? `${s.slice(0, max)}…` : s);
  switch (cat) {
    case 'shell':
      if (typeof args?.command === 'string' && args.command) return truncate(args.command);
      if (typeof args?.cmd === 'string' && args.cmd) return truncate(args.cmd);
      break;
    case 'browser':
      if (typeof args?.url === 'string' && args.url) return truncate(args.url, 60);
      if (typeof args?.action === 'string' && args.action) return truncate(args.action, 60);
      break;
    case 'file_read':
    case 'file_write':
    case 'file_search':
      if (typeof args?.path === 'string' && args.path) return truncate(args.path, 60);
      if (typeof args?.file_path === 'string' && args.file_path) return truncate(args.file_path, 60);
      if (typeof args?.pattern === 'string' && args.pattern) return truncate(args.pattern, 60);
      if (typeof result?.path === 'string' && result.path) return truncate(result.path, 60);
      break;
    case 'web_search':
      if (typeof args?.query === 'string' && args.query) return truncate(args.query);
      break;
    case 'mcp':
      if (typeof args?.method === 'string' && args.method) return truncate(args.method, 60);
      if (typeof args?.server === 'string' && args.server) return truncate(args.server, 60);
      break;
    case 'code':
      if (typeof args?.language === 'string' && args.language) return truncate(args.language, 30);
      break;
  }
  return '';
}

/** T8.4: Compute total content length to decide auto-collapse. */
const contentLength = computed(() => {
  const result = props.activity.tool.result ?? '';
  const args = props.activity.tool.arguments ?? '';
  return result.length + args.length;
});

// Chat UI #1: Persisted collapse state (remembered across re-renders/refreshes).
// Tool cards default COLLAPSED so the chat transcript stays scannable —
// the compact header (icon + label + status + duration) is enough to convey
// "a tool ran", and the full detail (tool name, args, result) is revealed
// on demand. Users who want to peek into a specific tool click the header.
// If content grows beyond the threshold while the user has expanded the
// card, auto-collapse re-engages to keep long outputs out of the way.
const { collapsed, toggle, setCollapsed } = useCollapseState(`action:${props.activity.id}`, true);

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
  // Chat UI fix: 'cancelled' previously had an icon (⊘) but no CSS modifier,
  // so cancelled tools rendered with no status color — inconsistent with
  // the design spec which requires a muted/cancelled visual state.
  'act-activity__status--cancelled': props.activity.tool.status === 'cancelled',
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
    // Chat UI fix: cancelled tools use the muted text color to signal
    // "aborted/no longer active" without raising visual alarm.
    &--cancelled
      color: var(--color-text-secondary)

  &__duration
    font-size: 11px
    color: var(--color-text-secondary)

  &__detail
    margin-left: 20px
    margin-bottom: 4px
</style>
