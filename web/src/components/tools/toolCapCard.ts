// 工具 → 构件卡（CapCard）的取值：状态、三个关键事实、标签、图标。纯函数，单测见 __tests__/toolCapCard.spec.ts。
import type { Tool } from '../../features/tools/types';
import {
  formatCount,
  formatRate,
  rateTone,
  riskTone,
  type CapFact,
  type CapStatus,
  type CapTag,
} from '../capability/capTypes';

type T = (key: string, named?: Record<string, unknown>) => string;

// 分类 → 图标；没登记的分类用通用的扳手
const CATEGORY_ICON: Record<string, string> = {
  browser: 'travel_explore',
  web: 'public',
  filesystem: 'folder',
  memory: 'psychology',
  media: 'image',
  integration: 'hub',
  system: 'settings',
  cli_admin: 'terminal',
  spirit: 'auto_awesome',
  computeruse: 'desktop_windows',
  orchestration: 'account_tree',
  office: 'work',
  coding: 'code',
  runtime: 'memory',
  session: 'forum',
  client: 'devices',
  sandbox: 'science',
  skill: 'school',
  twinops: 'dns',
  messaging: 'chat',
};

export function toolIcon(tool: Pick<Tool, 'category'>): string {
  return CATEGORY_ICON[tool.category] ?? 'build';
}

export function riskText(level: string, t: T): string {
  const key = { low: 'riskLow', medium: 'riskMedium', high: 'riskHigh', critical: 'riskCritical' }[level];
  return key ? t(`cap.common.${key}`) : level || '—';
}

/** 停用优先；启用后看运行时有没有实现、最近一次调用是否失败 */
export function toolStatus(tool: Tool, t: T): CapStatus {
  if (!tool.enabled) return { tone: 'off', label: t('cap.tools.status.off') };
  if (tool.runtime_status === 'needs_config') {
    return { tone: 'watch', label: t('cap.tools.status.needsConfig'), hint: t('cap.tools.status.needsConfigHint') };
  }
  if (tool.runtime_status === 'registered_only') {
    return {
      tone: 'pending',
      label: t('cap.tools.status.registeredOnly'),
      hint: t('cap.tools.status.registeredOnlyHint'),
    };
  }
  if (tool.last_status === 'error' || tool.last_status === 'failed' || tool.last_status === 'blocked') {
    return { tone: 'alert', label: t('cap.tools.status.lastFailed') };
  }
  return { tone: 'ok', label: t('cap.tools.status.ready') };
}

/** 能用它的 Agent 数：后端没算出来时写「—」，不写 0 */
export function agentCountText(n: number | null | undefined): string {
  return n === null || n === undefined ? '—' : String(n);
}

/**
 * 三个关键事实。没被调用过的工具，用量全是 0 和 —，对判断没帮助：这时第三格换成「调用前要不要人工确认」；
 * 有调用后第三格是成功率。第二格始终是「多少 Agent 能用」。
 */
export function toolFacts(tool: Tool, t: T): CapFact[] {
  const agents: CapFact = {
    label: t('cap.tools.agentsCanUse'),
    value: agentCountText(tool.allowed_agent_count),
    tone: tool.allowed_agent_count === 0 && tool.enabled ? 'watch' : undefined,
    hint: t('cap.tools.agentsCanUseHint'),
  };
  const risk: CapFact = {
    label: t('cap.common.risk'),
    value: riskText(tool.risk_level, t),
    tone: riskTone(tool.risk_level),
  };
  if (tool.invoke_count > 0) {
    const rate = tool.success_count / tool.invoke_count;
    return [risk, agents, { label: t('cap.common.successRate'), value: formatRate(rate), tone: rateTone(rate) }];
  }
  return [
    risk,
    agents,
    tool.requires_confirmation
      ? { label: t('cap.tools.confirmMode'), value: t('cap.tools.confirmAsk'), tone: 'watch' }
      : { label: t('cap.tools.confirmMode'), value: t('cap.tools.confirmDirect') },
  ];
}

export function toolTags(tool: Tool, t: T): CapTag[] {
  const tags: CapTag[] = [];
  if (tool.category) tags.push({ label: tool.category });
  if (tool.source && tool.source !== 'builtin') tags.push({ label: tool.source, tone: 'pending' });
  // 没调用过时「确认方式」已在事实里，不再重复打标签
  if (tool.requires_confirmation && tool.invoke_count > 0)
    tags.push({ label: t('cap.tools.tagConfirm'), tone: 'watch' });
  if (tool.readonly) tags.push({ label: t('cap.tools.tagReadonly') });
  return tags;
}

/** 底部一句：有调用时说用量；没有时说单独设置或「还没被调用过」 */
export function toolRelation(tool: Tool, t: T): string {
  if (tool.invoke_count > 0) {
    return t('cap.tools.relationUsage', {
      day: formatCount(tool.invoke_count_24h),
      total: formatCount(tool.invoke_count),
    });
  }
  return tool.agent_override_count > 0
    ? t('cap.tools.relationOverrides', { n: tool.agent_override_count })
    : t('cap.tools.relationNeverCalled');
}
