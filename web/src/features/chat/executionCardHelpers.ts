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

/**
 * Generate a fallback summary from tool_name + arguments when the backend
 * doesn't provide `event.summary`.
 */
export function generateSummaryFallback(event: ToolUseEvent): string {
  const args = event.arguments ?? {};
  const toolName = event.tool_name;

  switch (toolName) {
    case 'file_edit':
    case 'file_write': {
      const path = (args.path as string) || (args.file_name as string) || '';
      const filename = path.split('/').pop() || path;
      return filename ? `Modify ${filename}` : '';
    }
    case 'file_read': {
      const path = (args.path as string) || (args.file_name as string) || '';
      const filename = path.split('/').pop() || path;
      return filename ? `Read ${filename}` : '';
    }
    case 'grep':
    case 'search_files': {
      const pattern = (args.pattern as string) || (args.query as string) || '';
      return pattern ? `Search "${truncate(pattern, 30)}"` : '';
    }
    case 'bash': {
      const command = (args.command as string) || '';
      return command ? `> ${truncate(command, 40)}` : '';
    }
    default:
      return '';
  }
}
