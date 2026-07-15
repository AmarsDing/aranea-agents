/**
 * Classify a tool by its name into a functional category for UI rendering.
 *
 * Mirrors backend `biz.ToolCategory` constants (internal/biz/activity.go).
 * The v2 Step struct does not carry ToolCategory from the backend, so the
 * frontend infers it from `step.ToolName`. When/if the backend starts
 * emitting ToolCategory on Step, this function can be replaced by a direct
 * field read (see TODO below).
 *
 * Categories follow the spec in docs/development/1-chat.md §1.5:
 * shell / browser / file_read / file_write / file_search / web_search /
 * mcp / code / todo / other.
 */

export type ToolCategory =
  | 'shell'
  | 'browser'
  | 'file_read'
  | 'file_write'
  | 'file_search'
  | 'web_search'
  | 'mcp'
  | 'code'
  | 'todo'
  | 'other';

/** Shell command execution tools. */
const SHELL_TOOLS = new Set([
  'exec_command',
  'bash',
  'shell',
  'shell_exec',
  'workspace_exec',
  'run_command',
  'execute_command',
  'terminal',
]);

/** Browser automation tools. */
const BROWSER_TOOLS = new Set([
  'browser',
  'browser_navigate',
  'browser_click',
  'browser_fill',
  'browser_evaluate',
  'browser_take_screenshot',
  'browser_snapshot',
  'web_fetch_page',
  'playwright',
]);

/** File read tools. */
const FILE_READ_TOOLS = new Set(['read_file', 'file_read_file', 'file_read', 'get_file', 'cat_file', 'view_file']);

/** File write/edit tools. */
const FILE_WRITE_TOOLS = new Set([
  'save_file',
  'write_file',
  'file_write',
  'file_edit',
  'edit_file',
  'create_file',
  'patch_file',
  'file_write_file',
]);

/** File search tools (find / grep / glob). */
const FILE_SEARCH_TOOLS = new Set([
  'search_content',
  'search_files',
  'grep',
  'glob',
  'find_file',
  'list_file',
  'file_search',
  'file_search_search',
]);

/** Web search tools. */
const WEB_SEARCH_TOOLS = new Set([
  'web_search',
  'search_web',
  'internet_search',
  'google_search',
  'bing_search',
  'duckduckgo_search',
]);

/** Code execution tools. */
const CODE_TOOLS = new Set([
  'execute_code',
  'run_code',
  'code_exec',
  'python_exec',
  'js_exec',
  'codeexecutor',
  'code_executor',
  'sandbox_exec',
]);

/** Todo management tools. */
const TODO_TOOLS = new Set(['todo_write', 'todo_read', 'todo_update', 'task_create', 'task_update', 'task_complete']);

/** MCP tool name prefix (case-insensitive). */
const MCP_PREFIXES = ['mcp_', 'mcp.', 'mcp/'];

/**
 * Classify a tool by its name. Returns 'other' for unknown tools.
 *
 * TODO: When backend Step.ToolCategory is wired through the v2 API, replace
 * the name-based inference here with a direct field read.
 */
export function classifyTool(toolName: string | undefined | null): ToolCategory {
  if (!toolName) return 'other';
  const name = toolName.toLowerCase();

  if (SHELL_TOOLS.has(name)) return 'shell';
  if (BROWSER_TOOLS.has(name)) return 'browser';
  if (FILE_READ_TOOLS.has(name)) return 'file_read';
  if (FILE_WRITE_TOOLS.has(name)) return 'file_write';
  if (FILE_SEARCH_TOOLS.has(name)) return 'file_search';
  if (WEB_SEARCH_TOOLS.has(name)) return 'web_search';
  if (CODE_TOOLS.has(name)) return 'code';
  if (TODO_TOOLS.has(name)) return 'todo';

  // MCP tools conventionally start with `mcp_` / `mcp.` / `mcp/`.
  if (MCP_PREFIXES.some((p) => name.startsWith(p))) return 'mcp';

  return 'other';
}

/** Emoji icon per tool category (compact header glyph). */
export const TOOL_CATEGORY_ICON: Record<ToolCategory, string> = {
  shell: '⌨️',
  browser: '🌐',
  file_read: '📄',
  file_write: '✏️',
  file_search: '🔍',
  web_search: '🔎',
  mcp: '🔌',
  code: '💻',
  todo: '📋',
  other: '🔧',
};
