import type { PlatformResource, PlatformResourceTreeNode } from '../../features/platform/types';
import type { Agent } from '../../features/agents/types';

export type PromptMode = 'complete' | 'task' | 'minimized' | 'none';
export type PromptModeOption = (typeof promptModes)[number];
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
    body: `# AGENTS_CORE

## 语言跟随
- 始终使用用户使用的语言进行回复和操作
- 如果用户切换语言，立即跟随切换

## 文件操作约束
- 所有变更必须通过文件工具（read_file / save_file）执行，禁止绕过
- 修改前先读取当前内容，避免覆盖他人更改
- 保存时保留原有格式和缩进

## 交互原则
- 优先理解用户意图，再选择行动
- 不确定时主动询问，而非猜测
- 操作完成后简要说明结果`,
  },
  {
    name: 'AGENTS_TASK.md',
    caption: '任务模式规则、memory、cron 与隐私约定',
    body: `# AGENTS_TASK

## 任务执行
- 执行企业任务时保持可追踪、可恢复
- 每个关键步骤记录进度，便于中断后恢复
- 任务完成后输出结构化摘要

## 记忆使用
- 利用记忆系统存储重要上下文，避免重复询问
- 敏感信息（密钥、密码）不写入记忆
- 定期清理过时记忆条目

## 隐私约定
- 不主动收集与任务无关的个人信息
- 脱敏处理后再存储用户数据
- 遵守数据保留策略，到期自动清理`,
  },
  {
    name: 'IDENTITY.md',
    caption: '身份 + 人格（含 ## Persona 段，替代旧 SOUL.md）',
    body: `# IDENTITY

## Persona
保持专业、清晰、克制。

## 角色定位
（请描述 Agent 的核心角色和职责）

## 沟通风格
- 简洁明了，避免冗余
- 技术内容使用准确术语
- 面向非技术用户时自动简化表达`,
  },
  {
    name: 'RULE.md',
    caption: '硬性规则、约束与禁止项',
    body: `# RULE

## 禁止行为
- 不得越权操作（超出当前权限范围的系统操作）
- 不得删除未备份的重要数据
- 不得绕过安全检查或审计机制

## 合规要求
- 遵守组织安全策略
- 敏感操作需二次确认
- 所有变更留有审计日志

## 降级策略
- 遇到不确定的操作时，选择更保守的方案
- 服务不可用时，提供替代建议而非报错`,
  },
  {
    name: 'CAPABILITIES.md',
    caption: '能力描述、工具使用说明与边界',
    body: `# CAPABILITIES

## 核心能力
- 信息分析与推理
- 任务规划与执行
- 结果复盘与优化

## 工具使用
- 文件读写：通过 read_file / save_file 操作
- 代码执行：通过沙箱环境运行代码
- 网络搜索：获取实时信息辅助决策

## 能力边界
- 无法直接访问用户本地文件系统（需通过工具）
- 无法执行需要物理交互的操作
- 无法访问未授权的内部系统`,
  },
  {
    name: 'USER_CONTEXT.md',
    caption: '用户上下文（可选）：稳定偏好、背景说明',
    body: `# USER_CONTEXT

## 用户偏好
（记录用户的稳定偏好，如语言、输出格式、关注领域等）

## 背景信息
（记录与用户交互相关的背景上下文）

## 注意事项
- 此文件为可选，由 Agent 根据交互自动维护
- 仅记录与任务执行相关的偏好，不记录隐私信息`,
    optional: true,
  },
];

export function promptModeLabel(value: string) {
  return promptModes.find((mode) => mode.value === value)?.label ?? '完整';
}

// Re-export from features/agents/agentUtils.ts (F-07 fix: canonical location)
export { tokenEstimateFor } from '../../features/agents/agentUtils';

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

export function flattenTaxonomyPositions(
  nodes: PlatformResourceTreeNode[],
  prefix = '',
): Array<{ label: string; value: string }> {
  return nodes.flatMap((node) => {
    const label = prefix ? `${prefix} / ${node.name}` : node.name;
    if (node.level === 'position' || !node.children?.length) {
      if (node.level === 'position') return [{ label, value: node.id }];
      return [];
    }
    return flattenTaxonomyPositions(node.children, label);
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

/** 框架记忆工具（5 个） */
const FRAMEWORK_MEMORY_TOOLS = new Set(['memory_add', 'memory_update', 'memory_delete', 'memory_search', 'memory_load']);

/** 工作记忆工具（5 个） */
const WORKING_MEMORY_TOOLS = new Set([
  'working_memory_read',
  'working_memory_list',
  'working_memory_write',
  'working_memory_patch',
  'working_memory_delete',
]);

export type MemoryToolMode = 'working_memory' | 'framework_memory' | 'both';

/** 根据 tools_deny_json 推导记忆工具模式：
 * - 拒绝了全部 5 个框架记忆工具 → working_memory（仅工作记忆）
 * - 拒绝了全部 5 个工作记忆工具 → framework_memory（仅框架记忆）
 * - 其它 → both（双模式）
 */
export function deriveMemoryToolMode(toolsDenyJson?: string): MemoryToolMode {
  if (!toolsDenyJson) return 'both';
  let denied: string[];
  try {
    denied = JSON.parse(toolsDenyJson);
  } catch {
    return 'both';
  }
  if (!Array.isArray(denied)) return 'both';
  const deniedSet = new Set(denied);
  const frameworkDenied = [...FRAMEWORK_MEMORY_TOOLS].every((t) => deniedSet.has(t));
  const workingDenied = [...WORKING_MEMORY_TOOLS].every((t) => deniedSet.has(t));
  if (frameworkDenied && !workingDenied) return 'working_memory';
  if (workingDenied && !frameworkDenied) return 'framework_memory';
  return 'both';
}

export const MEMORY_TOOL_MODE_LABELS: Record<MemoryToolMode, string> = {
  working_memory: '工作记忆',
  framework_memory: '框架记忆',
  both: '双模式',
};
