// 插件 → 构件卡（CapCard）的取值。纯函数，单测见 __tests__/pluginCapCard.spec.ts。
import type { Plugin } from '../../features/plugins/types';
import { formatCount, riskTone, type CapFact, type CapStatus, type CapTag } from '../capability/capTypes';

type T = (key: string, named?: Record<string, unknown>) => string;

const CATEGORY_ICON: Record<string, string> = {
  observability: 'visibility',
  tracking: 'track_changes',
  reliability: 'autorenew',
  security: 'shield',
  governance: 'gavel',
  routing: 'alt_route',
};

/** 安全类：关掉后运行链路上就没有这层防护，页面要单独提醒 */
export const SECURITY_CATEGORY = 'security';

export function pluginIcon(plugin: Pick<Plugin, 'category'>): string {
  return CATEGORY_ICON[plugin.category] ?? 'extension';
}

export function pluginStatus(plugin: Plugin, t: T): CapStatus {
  if (!plugin.enabled) return { tone: 'off', label: t('cap.common.disabled') };
  const last = (plugin.last_status || '').toLowerCase();
  if (last.includes('error') || last.includes('fail')) return { tone: 'alert', label: t('cap.plugins.lastError') };
  return { tone: 'ok', label: t('cap.common.enabled') };
}

export function pluginFacts(plugin: Plugin, t: T): CapFact[] {
  return [
    {
      label: t('cap.common.risk'),
      value: t(`cap.common.risk${cap(plugin.risk_level || 'low')}`),
      tone: riskTone(plugin.risk_level),
    },
    {
      label: t('cap.plugins.hooks'),
      value: (plugin.callback_points ?? []).length,
      hint: (plugin.callback_points ?? []).join(', '),
    },
    {
      label: t('cap.plugins.runs'),
      value: formatCount(plugin.invoke_count),
      tone: plugin.error_count > 0 ? 'alert' : plugin.block_count > 0 ? 'watch' : undefined,
      hint: t('cap.plugins.runsHint', { block: plugin.block_count, error: plugin.error_count }),
    },
  ];
}

export function pluginTags(plugin: Plugin): CapTag[] {
  const tags: CapTag[] = [{ label: plugin.category }];
  for (const point of (plugin.callback_points ?? []).slice(0, 2)) tags.push({ label: point, tone: 'pending' });
  const more = (plugin.callback_points ?? []).length - 2;
  if (more > 0) tags.push({ label: `+${more}` });
  return tags;
}

/** 作用域：空或 global = 全部 Agent；否则是某个 Agent 的 ID（与后端 PluginMatchesScope 一致） */
export function pluginScopeText(plugin: Plugin, t: T, agentName: (id: string) => string): string {
  const scope = (plugin.scope || '').trim();
  if (scope === '' || scope.toLowerCase() === 'global') return t('cap.plugins.scopeGlobal');
  return t('cap.plugins.scopeAgent', { agent: agentName(scope) });
}

function cap(level: string): string {
  return level.charAt(0).toUpperCase() + level.slice(1);
}
