import type { ActivityKind, ToolUseEvent } from './types';

const builtinLabels: Record<string, string> = {
  read_file: '读取文件',
  save_file: '保存文件',
  exec_command: '执行命令',
  skill_load: '加载 Skill',
  skill_run: '运行 Skill',
  mcp_call: 'MCP 调用',
  knowledge_search: '知识库检索',
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

export function classifyActivityKind(toolName: string): ActivityKind {
  const name = toolName.trim().toLowerCase();
  if (['skill_load', 'skill_run', 'skill_search', 'use_skill'].includes(name) || name.startsWith('skill_')) {
    return 'skill';
  }
  if (
    ['mcp_call', 'mcp_list_tools', 'mcp_list_servers', 'mcp_inspect_tools'].includes(name) ||
    name.startsWith('mcp:') ||
    name.startsWith('mcp_')
  ) {
    return 'mcp';
  }
  if (['transfer_to_agent', 'spawn_subagent', 'call_agent'].includes(name)) return 'subagent';
  if (['load_memory', 'preload_memory'].includes(name) || name.startsWith('memory_')) return 'memory';
  if (name === 'knowledge_search') return 'knowledge';
  if (name === 'await_user_reply') return 'session';
  return 'tool';
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
  const kind = (event.activity_kind || classifyActivityKind(event.tool_name)) as ActivityKind;
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
