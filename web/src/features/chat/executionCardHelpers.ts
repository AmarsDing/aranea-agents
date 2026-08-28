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
  /** Live orchestration progress text (decomposing / allocating / …) for running tool cards. */
  orchestrationProgressText?: Readonly<Ref<string>>;
}

export const EXECUTION_COLLAPSE_CONTROL_KEY: InjectionKey<ExecutionCollapseControl> =
  Symbol('ExecutionCollapseControl');

export function isPlanAndExecuteTool(toolName: string | undefined | null): boolean {
  const name = toolName?.trim() ?? '';
  return name === 'plan_and_execute' || name.endsWith('_plan_and_execute');
}

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
  // search_content（运行时真实工具名）的搜索词字段。
  content_pattern?: string;
  file_pattern?: string;
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
    // 运行时真实工具名（trpc file 工具集）：diff_edit/replace_content/patch_file/save_file；
    // file_edit/file_write 为历史别名，保留兼容旧会话数据。
    case 'diff_edit':
    case 'replace_content':
    case 'patch_file':
    case 'save_file':
    case 'write_file':
    case 'file_edit':
    case 'file_write': {
      const payload = args as FileToolPayload | undefined;
      const path = payload?.path || payload?.file_name || '';
      const filename = path.split('/').pop() || path;
      return filename ? `修改 ${filename}` : '';
    }
    case 'read_file':
    case 'read_multiple_files':
    case 'file_read': {
      const payload = args as FileToolPayload | undefined;
      const path = payload?.path || payload?.file_name || '';
      const filename = path.split('/').pop() || path;
      return filename ? `读取 ${filename}` : '';
    }
    case 'search_content':
    case 'search_file':
    case 'grep':
    case 'search_files': {
      const payload = args as SearchToolPayload | undefined;
      const pattern =
        payload?.content_pattern || payload?.pattern || payload?.query || payload?.file_pattern || '';
      return pattern ? `搜索 "${truncate(pattern, 30)}"` : '';
    }
    case 'exec_command':
    case 'shell_exec':
    case 'bash': {
      const payload = args as CommandToolPayload | undefined;
      const command = payload?.command || '';
      return command ? `> ${truncate(command, 40)}` : '';
    }
    case 'plan_and_execute':
      return '正在规划并执行…';
    default:
      return '';
  }
}
