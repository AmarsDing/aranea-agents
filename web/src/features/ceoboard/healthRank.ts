// 健康度档位与折叠规则（整合 P2-8，自 ceo-dashboard web/src/healthRank.mjs
// 语义等价移植为 TS；与后端 internal/biz/ceoboard/agg 同口径，§15.1）。

import type { Overview } from './types';

export type RankedHealth = 'red' | 'yellow' | 'green' | 'gray' | (string & {});

const RANK: Record<string, number> = { red: 0, yellow: 1, green: 2, gray: 3 };

export const healthRank = (h: RankedHealth): number => RANK[h] ?? 3;

// 仅当组内全部绿/灰时可折叠；红/黄组永不折叠。
export function laneCollapsible(healths: string[]): boolean {
  return healths.every((h) => h === 'green' || h === 'gray');
}

export interface SortableWorkflow {
  id: string;
  health: string;
  alerts?: number;
  volume_1h?: number;
}

// 组内排序：健康度 → 告警数 → 业务量 → id。
export function sortWorkflows<T extends SortableWorkflow>(ws: T[]): T[] {
  return [...ws].sort(
    (a, b) =>
      healthRank(a.health) - healthRank(b.health) ||
      (b.alerts ?? 0) - (a.alerts ?? 0) ||
      (b.volume_1h ?? 0) - (a.volume_1h ?? 0) ||
      String(a.id).localeCompare(String(b.id)),
  );
}

// 全页整体结论的唯一口径：Hero 与公司运转图 CEO 卡共用，避免同屏两个「整体」打架。
export function overviewLevel(ov: Overview): 'red' | 'yellow' | 'green' {
  const wfs = ov.lanes.flatMap((l) => l.workflows);
  if (ov.platforms.some((p) => p.health === 'red') || wfs.some((w) => w.health === 'red')) return 'red';
  if (wfs.some((w) => w.health === 'yellow') || ov.approvals.count > 0) return 'yellow';
  return 'green';
}
