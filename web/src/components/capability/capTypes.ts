/** 全站状态词表的六种语气（01-视觉与交互基线 第 4 节）：正常 / 关注 / 异常 / 待接入 / 运行中 / 已停用 */
export type CapTone = 'ok' | 'watch' | 'alert' | 'pending' | 'running' | 'off';

export type CapStatus = { tone: CapTone; label: string; hint?: string };

export type CapFact = { label: string; value: string | number; tone?: CapTone; hint?: string };

export type CapTag = { label: string; tone?: CapTone | 'ai' };

export type CapStat = { key: string; label: string; value: string | number; tone?: CapTone; hint?: string };

/** 风险等级 → 语气：高、严重算异常色，中算关注，低算正常 */
export function riskTone(level?: string): CapTone {
  if (level === 'critical' || level === 'high') return 'alert';
  if (level === 'medium') return 'watch';
  return 'ok';
}

/** 成功率（0–1）→ 语气；没有调用时返回待接入 */
export function rateTone(rate: number | null): CapTone {
  if (rate === null) return 'pending';
  if (rate < 0.8) return 'alert';
  if (rate < 0.95) return 'watch';
  return 'ok';
}

export function formatRate(rate: number | null): string {
  return rate === null ? '—' : `${Math.round(rate * 1000) / 10}%`;
}

/** 1234 → 1.2k；小于 1000 原样 */
export function formatCount(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) return '—';
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${Math.round(n / 100) / 10}k`;
  return `${Math.round(n / 100_000) / 10}m`;
}
