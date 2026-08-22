<template>
  <div class="act-activity">
    <!-- Card variant -->
    <!-- Todo write: render structured plan list instead of raw JSON -->
    <TodoInlineList v-if="isTodo" :event="todoEvent" />
    <template v-else>
      <div class="act-activity__header" @click="toggleExpand">
        <span class="act-activity__icon">{{ toolIcon }}</span>
        <span class="act-activity__tool-label">{{ step.ToolName }}</span>
        <span v-if="headerHint" class="act-activity__hint" :title="headerHint">{{ headerHint }}</span>
        <span class="act-activity__status" :class="statusClass">{{ statusIcon }}</span>
        <span v-if="step.ToolDurationMs > 0" class="act-activity__duration">{{ formattedDuration }}</span>
      </div>
      <!-- Per-category dispatch: pick the specialized detail component based
           on toolCategory (inferred from toolName via classifyTool). Falls
           back to GenericToolDetail for unknown categories. -->
      <component :is="detailComponent" v-if="expanded" :step="step" class="act-activity__detail" />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, watch, type Component } from 'vue';
import type { Step } from '../../features/chat/v2Types';
import type { ToolUseEvent } from '../../features/chat/types';
import { formatDuration } from '../../features/chat/agentTreeUtils';
import { isTodoWriteTool } from '../../features/chat/activityPresentation';
import { useCollapseState } from '../../features/chat/composables/useCollapseState';
import TodoInlineList from './TodoInlineList.vue';
import GenericToolDetail from './tools/GenericToolDetail.vue';
import ShellToolDetail from './tools/ShellToolDetail.vue';
import BrowserToolDetail from './tools/BrowserToolDetail.vue';
import FileReadToolDetail from './tools/FileReadToolDetail.vue';
import FileWriteToolDetail from './tools/FileWriteToolDetail.vue';
import FileSearchToolDetail from './tools/FileSearchToolDetail.vue';
import WebSearchToolDetail from './tools/WebSearchToolDetail.vue';
import McpToolDetail from './tools/McpToolDetail.vue';
import CodeToolDetail from './tools/CodeToolDetail.vue';
import MediaToolDetail from './tools/MediaToolDetail.vue';
import { classifyTool, TOOL_CATEGORY_ICON, type ToolCategory } from './tools/classifyTool';
import { asRecord } from './tools/toolDetailShared';

/** T8.4: Tool result/arguments longer than this threshold auto-collapse. */
const RESULT_COLLAPSE_THRESHOLD = 500;

const props = defineProps<{
  step: Step;
  agentColor?: string;
}>();

const isTodo = computed(() => isTodoWriteTool(props.step.ToolName));

// Infer tool category from toolName (v2 Step has no ToolCategory field yet).
const toolCategory = computed<ToolCategory>(() => classifyTool(props.step.ToolName));

// Per-category icon — gives the user a visual hint of tool type at a glance.
const toolIcon = computed(() => TOOL_CATEGORY_ICON[toolCategory.value]);

// Pick the specialized detail component based on tool category.
const detailComponent = computed<Component>(() => {
  switch (toolCategory.value) {
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
    case 'media':
      return MediaToolDetail;
    default:
      return GenericToolDetail;
  }
});

const parsedToolArgs = computed(() => asRecord(props.step.ToolArgs));
const parsedToolResult = computed(() => asRecord(props.step.ToolResult));

/**
 * Compact execution-result hint shown in the tool header.
 * For read_file this is the file path; for exec_command the command; etc.
 * It surfaces the "what happened" instead of a generic tool label.
 *
 * v2 Step has no toolCategory, so hint selection is name-based only.
 */
const headerHint = computed(() => {
  const name = props.step.ToolName;
  const args = parsedToolArgs.value;
  const result = parsedToolResult.value;

  const fileOps = new Set([
    'read_file',
    'read_multiple_files',
    'file_read_file',
    'save_file',
    'write_file',
    'file_write',
    'file_edit',
    'diff_edit',
    'replace_content',
    'patch_file',
    'list_file',
  ]);
  if (fileOps.has(name)) {
    let path = '';
    // 运行时 file 工具集参数名为 file_name；path 为历史别名/结果字段。
    if (typeof args?.file_name === 'string') {
      path = args.file_name;
    } else if (typeof args?.path === 'string') {
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

  const searchOps = new Set(['search_content', 'search_file', 'search_files', 'grep']);
  if (searchOps.has(name)) {
    let q = '';
    // search_content 的搜索词参数为 content_pattern。
    if (typeof args?.content_pattern === 'string') {
      q = args.content_pattern;
    } else if (typeof args?.query === 'string') {
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
  const result = props.step.ToolResult != null ? JSON.stringify(props.step.ToolResult) : '';
  const args = props.step.ToolArgs != null ? JSON.stringify(props.step.ToolArgs) : '';
  return result.length + args.length;
});

// Chat UI #1: Persisted collapse state (remembered across re-renders/refreshes).
// Tool cards default COLLAPSED so the chat transcript stays scannable —
// the compact header (icon + label + status + duration) is enough to convey
// "a tool ran", and the full detail (tool name, args, result) is revealed
// on demand. Users who want to peek into a specific tool click the header.
// If content grows beyond the threshold while the user has expanded the
// card, auto-collapse re-engages to keep long outputs out of the way.
const { collapsed, toggle, setCollapsed } = useCollapseState(`action:${props.step.ID}`, true);

// If content grows beyond threshold after initial render (e.g., streaming result),
// auto-collapse unless the user has explicitly expanded.
watch(contentLength, (len, prevLen) => {
  if (len > RESULT_COLLAPSE_THRESHOLD && prevLen <= RESULT_COLLAPSE_THRESHOLD) {
    setCollapsed(true);
  }
});

/** Map v2 StepStatus to ToolUseEvent status (consumed by TodoInlineList). */
function mapStepStatusToToolUseStatus(status: Step['Status']): ToolUseEvent['status'] {
  switch (status) {
    case 'running':
    case 'tool_running':
    case 'pending':
      return 'running';
    case 'tool_blocked':
      return 'blocked';
    case 'completed':
      return 'success';
    case 'failed':
      return 'failed';
    case 'cancelled':
      return 'cancelled';
  }
}

/** Adapt v2 Step to ToolUseEvent for TodoInlineList consumption. */
const todoEvent = computed<ToolUseEvent>(() => {
  const s = props.step;
  const status = mapStepStatusToToolUseStatus(s.Status);
  return {
    id: s.ID,
    phase: status === 'running' ? 'before' : 'after',
    status,
    agent_id: '',
    agent_key: '',
    agent_name: '',
    tool_name: s.ToolName,
    tool_label: s.ToolName,
    arguments: s.ToolArgs ?? undefined,
    result: s.ToolResult ?? undefined,
    error: s.ToolErrorCode || undefined,
    occurred_at: '',
    duration_ms: s.ToolDurationMs || undefined,
  };
});

const expanded = computed(() => !collapsed.value);

function toggleExpand() {
  toggle();
}

const statusClass = computed(() => {
  const s = props.step.Status;
  return {
    'act-activity__status--running': s === 'running' || s === 'tool_running' || s === 'pending',
    'act-activity__status--success': s === 'completed',
    'act-activity__status--failed': s === 'failed',
    'act-activity__status--blocked': s === 'tool_blocked',
    // Chat UI fix: 'cancelled' previously had an icon (⊘) but no CSS modifier,
    // so cancelled tools rendered with no status color — inconsistent with
    // the design spec which requires a muted/cancelled visual state.
    'act-activity__status--cancelled': s === 'cancelled',
  };
});

const statusIcon = computed(() => {
  switch (props.step.Status) {
    case 'running':
    case 'tool_running':
    case 'pending':
      return '⏳';
    case 'completed':
      return '✓';
    case 'failed':
      return '✗';
    case 'tool_blocked':
      return '🔒';
    case 'cancelled':
      return '⊘';
    default:
      return '🔧';
  }
});

const formattedDuration = computed(() => formatDuration(props.step.ToolDurationMs));
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
