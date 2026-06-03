import type { PlatformResource, PlatformResourceTreeNode } from '../../features/platform/types';
import type { Agent } from '../../features/agents/types';

export type PromptMode = 'complete' | 'task' | 'minimized' | 'none';
export type EvolutionKey =
  | 'self_evolve'
  | 'skill_evolve'
  | 'evolution_metrics_enabled'
  | 'evolution_suggestions_enabled';

export type AgentFile = {
  id?: string;
  name: string;
  caption: string;
  body: string;
  /** optional=true means the file is not created by default; user adds it explicitly (PGO-1). */
  optional?: boolean;
};

export const promptModes = [
  { value: 'complete' as PromptMode, label: '完整', caption: '交互聊天 + 完整人格类能力', tokens: '~8K tokens' },
  { value: 'task' as PromptMode, label: '任务', caption: '企业自动化、记忆、进化', tokens: '~5K tokens' },
  { value: 'minimized' as PromptMode, label: '最小化', caption: '后台任务、核心规则、仅观察', tokens: '~2K tokens' },
  { value: 'none' as PromptMode, label: '无', caption: '纯工具调用自动化', tokens: '~2K tokens' },
];

export const statusOptions = [
  { label: '活跃', value: 'active' },
  { label: '停用', value: 'inactive' },
];

export function statusLabel(value: string) {
  return statusOptions.find((opt) => opt.value === value)?.label ?? value;
}

export const toolOptions = [
  'browser',
  'replace_content',
  'list_file',
  'read_file',
  'save_file',
  'create_image',
  'create_video',
  'stt',
];

// PGO-1 V2: 5 core files + 1 optional. Removed SOUL.md (merged → IDENTITY.md ## Persona),
// HEARTBEAT.md (deprecated), USER.md, USER_PREDEFINED.md.
// Must match internal/biz.pgoDefaultFilesV2 — validated by make fieldguide-lint.
export const defaultAgentFiles: AgentFile[] = [
  {
    name: 'AGENTS_CORE.md',
    caption: '通用操作规则、语言跟随、工具保存约束',
    body: '# AGENTS_CORE\n遵循用户语言，保存变更必须通过文件工具。',
  },
  {
    name: 'AGENTS_TASK.md',
    caption: '任务模式规则、memory、cron 与隐私约定',
    body: '# AGENTS_TASK\n执行企业任务时保持可追踪、可恢复。',
  },
  {
    name: 'IDENTITY.md',
    caption: '身份 + 人格（含 ## Persona 段，替代旧 SOUL.md）',
    body: '# IDENTITY\n\n## Persona\n保持专业、清晰、克制。',
  },
  {
    name: 'RULE.md',
    caption: '硬性规则、约束与禁止项',
    body: '# RULE\n不得越权操作。',
  },
  {
    name: 'CAPABILITIES.md',
    caption: '能力描述、工具使用说明与边界',
    body: '# CAPABILITIES\n可进行分析、执行和复盘。',
  },
  {
    name: 'USER_CONTEXT.md',
    caption: '用户上下文（可选）：稳定偏好、背景说明',
    body: '# USER_CONTEXT\n记录稳定偏好与背景信息。',
    optional: true,
  },
];

export function promptModeLabel(value: string) {
  return promptModes.find((mode) => mode.value === value)?.label ?? '完整';
}

export function tokenEstimateFor(value: string) {
  return Math.ceil((value || '').length / 4);
}

export function tokenText(value: string) {
  const count = tokenEstimateFor(value);
  return count > 0 ? `估计 ${count} token` : '空';
}

export function formatContext(value?: number) {
  if (!value) return '默认 ctx';
  if (value >= 1_000_000) return `${Math.round(value / 1_000_000)}M ctx`;
  if (value >= 1000) return `${Math.round(value / 1000)}K ctx`;
  return `${value} ctx`;
}

export function flattenCategoryPositions(
  nodes: PlatformResourceTreeNode[],
  prefix = '',
): Array<{ label: string; value: string }> {
  return nodes.flatMap((node) => {
    const label = prefix ? `${prefix} / ${node.name}` : node.name;
    if (node.level === 'position' || !node.children?.length) {
      if (node.level === 'position') return [{ label, value: node.id }];
      return [];
    }
    return flattenCategoryPositions(node.children, label);
  });
}

export function parseAvatarIcon(row: PlatformResource) {
  try {
    return JSON.parse(row.config_json || '{}').icon as string | undefined;
  } catch {
    return undefined;
  }
}

export function selfEvolveEnabled(agent: Agent) {
  if (agent.settings) {
    return agent.settings.self_evolve || agent.settings.evolution_self_evolve;
  }
  try {
    return Boolean(JSON.parse(agent.config_json || '{}').self_evolve);
  } catch {
    return false;
  }
}

const runStatusLabels: Record<string, string> = {
  idle: '空闲',
  pending: '排队',
  running: '运行中',
  awaiting_user: '等待回复',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
};

export function formatLastRunStatus(status?: string) {
  const key =
    String(status ?? 'idle')
      .trim()
      .toLowerCase() || 'idle';
  return runStatusLabels[key] ?? status ?? '空闲';
}

export function formatLastRunContext(agent: Agent) {
  const label = formatLastRunStatus(agent.last_run_status);
  const at = String(agent.last_run_at ?? '').trim();
  if (at) {
    const short = at.length >= 16 ? at.slice(0, 16).replace('T', ' ') : at;
    return `${label} · ${short}`;
  }
  return label;
}

/** 进化中：自我进化开启且存在待处理建议（列表 enrichment 或设置页 suggestions）。 */
export function isAgentEvolving(agent: Agent, pendingSuggestions = agent.pending_evolution_count ?? 0) {
  return selfEvolveEnabled(agent) && pendingSuggestions > 0;
}
