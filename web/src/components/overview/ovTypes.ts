/** 概览页组件共享的展示类型（纯类型，无运行时依赖）。 */

export type OvTone = 'ok' | 'warn' | 'danger' | 'info' | 'neutral';

export type OvVerdict = 'ok' | 'warning' | 'critical';

export type OvVerdictChip = { key: string; icon: string; label: string; severity: 'critical' | 'warning' | 'ok' };

export type OvRedTile = {
  id: string;
  label: string;
  value: string;
  caption?: string;
  delta?: number | null;
  deltaGoodWhenNegative?: boolean;
  tone?: OvTone;
  spark?: number[];
  clickable?: boolean;
};

export type OvBudget = {
  configured: boolean;
  pct: number;
  spentUsd: string;
  capUsd: string;
  forecastUsd: string | null;
  lowConfidence: boolean;
  tone: OvTone;
};

export type OvTopBar = { key: string; label: string; costUsd: number; ratio: number; unpriced: boolean };

export type OvCacheHit = { denied: boolean; samples: number; p50: number; low: boolean };

export type OvWorstAgent = { key: string; name: string; calls: number; successRate: number };

export type OvSource = { label: string; time: string };
