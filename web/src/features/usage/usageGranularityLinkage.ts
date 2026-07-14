// usageGranularityLinkage.ts
//
// 维度智能联动：range → granularity 的允许列表与默认值。
//
// 设计动机（P1-3）：原 UI 的 range 和 granularity 两个选择器完全独立，
// 用户可以在 today 下选 day（不合理：当天只看小时趋势），也可以在 30d
// 下选 hour（不合理：720 个点）。本模块提供纯函数，让 composable 根据
// 当前 range 自动决定 granularity 的可选项与默认值。
//
// 当 P2-1 引入 5min/week/month 粒度后，只需扩展 RANGE_GRANULARITY_MAP。

export type Granularity = 'day' | 'hour';
export type RangeKey = 'today' | '7d' | '30d' | 'month';

// RANGE_GRANULARITY_MAP 是 range → 允许 granularity 列表的映射。
// 顺序即默认优先级：第一项为该 range 的默认 granularity。
//
// 设计依据（点数合理性）：
//   - today:  24 小时点（hour 合理；day 仅 1 点无趋势意义）
//   - 7d:     7 日点（day）或 168 小时点（hour），均可视化
//   - 30d:    30 日点（day 合理；hour=720 点过密）
//   - month:  同 30d
const RANGE_GRANULARITY_MAP: Record<RangeKey, Granularity[]> = {
  today: ['hour'],
  '7d': ['day', 'hour'],
  '30d': ['day'],
  month: ['day'],
};

// allowedGranularitiesForRange 返回该 range 下可选的 granularity 列表。
// 未知 range 回退到 ['day']（保守默认）。
export function allowedGranularitiesForRange(range: string): Granularity[] {
  return RANGE_GRANULARITY_MAP[range as RangeKey] ?? ['day'];
}

// defaultGranularityForRange 返回该 range 的默认 granularity（列表第一项）。
export function defaultGranularityForRange(range: string): Granularity {
  return allowedGranularitiesForRange(range)[0];
}

// isGranularityValidForRange 判断给定 granularity 在该 range 下是否合法。
export function isGranularityValidForRange(granularity: Granularity, range: string): boolean {
  return allowedGranularitiesForRange(range).includes(granularity);
}

// resolveGranularityForRange 在 range 切换时解析新 granularity：
//   - 若当前 granularity 在新 range 下仍合法，保留之
//   - 否则回退到新 range 的默认 granularity
// 这样切换 range 时不会丢失用户的 granularity 选择（只要仍合法）。
export function resolveGranularityForRange(current: Granularity, range: string): Granularity {
  return isGranularityValidForRange(current, range) ? current : defaultGranularityForRange(range);
}
