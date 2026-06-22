/**
 * Shared helpers and types for ChatExecutionCard collapse/expand control.
 *
 * SP-FE-28: generateSummaryFallback — produces a summary when the backend
 * doesn't provide one, based on tool_name + arguments.
 * SP-FE-30: ExecutionCollapseControl + InjectionKey — Provide/Inject signal
 * pattern for global expand/collapse control.
 */
import type { InjectionKey, Ref } from 'vue';
import type { ToolUseEvent } from './types';

// ── SP-FE-30: Provide/Inject global control ──

export interface ExecutionCollapseControl {
  /** Incremented when "Expand All" is triggered. */
  expandAllSignal: Readonly<Ref<number>>;
  /** Incremented when "Collapse All" is triggered. */
  collapseAllSignal: Readonly<Ref<number>>;
}

export const EXECUTION_COLLAPSE_CONTROL_KEY: InjectionKey<ExecutionCollapseControl> =
  Symbol('execution-collapse-control');

// ── SP-FE-28: Summary fallback ──

function truncate(str: string, max: number): string {
  if (str.length <= max) return str;
  return str.slice(0, max) + '...';
}

interface FileToolPayload {
  path?: string;
  file_name?: string;
}

interface SearchToolPayload {
  pattern?: string;
  query?: string;
}

interface CommandToolPayload {
  command?: string;
}

/**
 * Generate a fallback summary from tool_name + arguments when the backend
 * doesn't provide `event.summary`.
 */
export function generateSummaryFallback(event: ToolUseEvent): string {
  const args = (event.arguments ?? {}) as Record<string, unknown>;
  const toolName = event.tool_name;

  switch (toolName) {
    case 'file_edit':
    case 'file_write': {
      const payload = args as FileToolPayload | undefined;
      const path = payload?.path || payload?.file_name || '';
      const filename = path.split('/').pop() || path;
      return filename ? `修改 ${filename}` : '';
    }
    case 'file_read': {
      const payload = args as FileToolPayload | undefined;
      const path = payload?.path || payload?.file_name || '';
      const filename = path.split('/').pop() || path;
      return filename ? `读取 ${filename}` : '';
    }
    case 'grep':
    case 'search_files': {
      const payload = args as SearchToolPayload | undefined;
      const pattern = payload?.pattern || payload?.query || '';
      return pattern ? `搜索 "${truncate(pattern, 30)}"` : '';
    }
    case 'bash': {
      const payload = args as CommandToolPayload | undefined;
      const command = payload?.command || '';
      return command ? `> ${truncate(command, 40)}` : '';
    }
    default:
      return '';
  }
}
