/** Harness「大屏显示」各块的展示类型（纯类型）。数值已格式化成文字；拿不到的数据写「—」，不显示成 0。 */
import type { OvTone } from '../overview/ovTypes';

export type HsRange = 'today' | '7d' | '30d' | 'month';

export type HsKpi = {
  key: string;
  label: string;
  value: string;
  caption: string;
  delta?: number | null;
  deltaGoodWhenNegative?: boolean;
  tone: OvTone;
  spark?: number[];
  /** 点了去哪里看明细；没有就不可点。 */
  to?: string;
};

/** 按模型 / 按 Agent 的一行：share 是占比（0–1），按费用算；全部没定价时按 Token 算。 */
export type HsShareRow = {
  key: string;
  label: string;
  cost: string;
  tokens: string;
  calls: string;
  share: number;
  unpriced: boolean;
};

/** 一个工具近 90 天的调用；ratio 是相对第一名的长度（0–1），各段是占这个工具调用数的比例。 */
export type HsToolRow = {
  key: string;
  label: string;
  calls: string;
  ratio: number;
  okPart: number;
  failPart: number;
  blockPart: number;
  failRate: string;
};

/**
 * 积分与额度的一行（预算 / 一个平台）。used 是已用比例（0–1），总额未知时为 null，不画进度条；
 * windows 是编码终端日志里读到的额度窗口（只列已知百分比的）。platform 有值且 editable 时可手填。
 */
export type HsCreditRow = {
  key: string;
  title: string;
  sub: string;
  value: string;
  used: number | null;
  tone: OvTone;
  windows: Array<{ label: string; pct: number; tone: OvTone }>;
  editable: boolean;
  platform?: string;
  missing: boolean;
  /** 近 30 天每天的剩余（≥2 个点才画）。 */
  spark: number[];
  /** 手填超过 7 天没更新，数字可能已经不对。 */
  stale: boolean;
};

/** 手填积分的表单值；total / remaining 为 null 表示不知道。 */
export type HsCreditForm = {
  platform: string;
  title: string;
  plan: string;
  total: number | null;
  remaining: number | null;
  unit: string;
  note: string;
};

export type HsStat = { key: string; label: string; value: string; tone?: OvTone };

export type HsTaskBar = { key: string; label: string; count: number; share: number };
