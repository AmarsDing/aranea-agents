import type { ActivityKind, ToolUseEvent } from './types';

const builtinLabels: Record<string, string> = {
  read_file: '读取文件',
  save_file: '保存文件',
  file_read_file: '读取文件',
  file_edit: '编辑文件',
  file_write: '写入文件',
  exec_command: '执行命令',
  cli_admin_agent_get: '获取 Agent',
  todo_write: '写入待办',
  skill_load: '加载 Skill',
  skill_run: '运行 Skill',
  mcp_call: 'MCP 调用',
  knowledge_search: '知识库检索',
  grep: '搜索代码',
  search_files: '搜索文件',
  bash: '执行命令',
};

const kindIcons: Record<ActivityKind, string> = {
  tool: 'build',
  skill: 'auto_awesome',
  mcp: 'hub',
  subagent: 'group',
  memory: 'psychology',
  knowledge: 'menu_book',
  session: 'forum',
};

const nameIcons: Record<string, string> = {
  read_file: 'description',
  save_file: 'description',
  write_file: 'description',
  exec_command: 'terminal',
  workspace_exec: 'terminal',
  skill_run: 'play_circle',
  skill_load: 'download',
};

const todoWriteToolNames = new Set(['todo_write', 'TodoWrite']);

/** Check if a tool is a todo_write variant that should render as inline task cards. */
export function isTodoWriteTool(toolName: string): boolean {
  return todoWriteToolNames.has(toolName);
}

export function resolveDisplayLabel(event: ToolUseEvent): string {
  return (
    event.display_label?.trim() ||
    event.tool_label?.trim() ||
    builtinLabels[event.tool_name] ||
    event.tool_name ||
    'tool'
  );
}

export function resolveActivityIcon(event: ToolUseEvent): string {
  if (event.icon_key?.trim()) return event.icon_key.trim();
  const byName = nameIcons[event.tool_name];
  if (byName) return byName;
  // AF: activity_kind is always provided by the backend ActivityProjector
  // (envelopeToolCall.ts / activityMessageAdapter.ts). Default to 'tool' for safety.
  const kind = (event.activity_kind || 'tool') as ActivityKind;
  return kindIcons[kind] || 'build';
}

export function formatDurationLabel(ms?: number): string {
  if (!ms || ms <= 0) return '';
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`;
}

const sensitiveKeyTokens = ['api_key', 'apikey', 'token', 'secret', 'password', 'authorization', 'cookie'];

function isSensitiveKey(key: string): boolean {
  const normalized = key.trim().toLowerCase();
  return sensitiveKeyTokens.some((token) => normalized.includes(token));
}

/** maskSensitiveJSON redacts sensitive keys for display-only JSON views. */
export function maskSensitiveJSON(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => maskSensitiveJSON(item));
  }
  if (value !== null && typeof value === 'object') {
    const out: Record<string, unknown> = {};
    for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
      out[key] = isSensitiveKey(key) ? '***' : maskSensitiveJSON(child);
    }
    return out;
  }
  return value;
}
