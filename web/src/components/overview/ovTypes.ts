/** 概览页组件共享的展示类型（纯类型，无运行时依赖）。 */

export type OvTone = 'ok' | 'warn' | 'danger' | 'info' | 'neutral';

export type OvVerdict = 'ok' | 'warning' | 'critical' | 'unknown';

export type OvVerdictChip = { key: string; icon: string; label: string; severity: OvVerdict };

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

/** 一个编码终端的用量；percent 是额度已用百分比（0–100），-1 表示终端日志里没有额度信息。 */
export type OvTerminal = {
  agent: string;
  sessions: number;
  source: string;
  windows: Array<{ label: string; tokens: number; percent: number; resetsAt: string }>;
};

/** 基座总览的领域卡；value 为 undefined 表示这个数没读到，界面写「—」。 */
export type OvDomainCard = {
  key: string;
  icon: string;
  title: string;
  question: string;
  to: string;
  facts: Array<{ label: string; value?: string | number }>;
};

export type OvCacheHit = { denied: boolean; samples: number; p50: number; low: boolean };

export type OvWorstAgent = { key: string; name: string; calls: number; successRate: number };

export type OvSource = { label: string; time: string };
