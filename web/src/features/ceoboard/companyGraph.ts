// 公司运转图图模型（2026-11-10 方案 §4）：把 GetOverview 物化数据收敛为
// 店主 → CEO 汇总 → lane → workflow → node 的只读图。纯逻辑、无 Vue 依赖，
// 健康度口径与泳道卡一致（healthRank worst）；整体结论与 Hero 共用 overviewLevel。

import type { Overview, SourceView } from './types';
import { healthRank, overviewLevel } from './healthRank';

// 「正在干活」= 此刻有数据流动：事件轨/对账轨任一新鲜即转圈。
// 不按 consuming 二分——consuming 仅 NATS/hybrid 填充，webhook 型只有 last_event_at。
export const WORKING_FRESH_MS = 5 * 60_000;

export interface GraphNode {
  id: string;
  name: string;
  order: number;
  health: string;
  working: boolean;
  reason: string;
}
export interface GraphWorkflow {
  id: string;
  name: string;
  health: string;
  alerts: number;
  volume1h: number;
  working: boolean;
  worstHealth: string;
  worstReason: string;
  nodes: GraphNode[];
}
export interface GraphLane {
  bizLine: string;
  health: string;
  working: boolean;
  volume1h: number;
  alerts: number;
  workflows: GraphWorkflow[];
}
export interface CompanyGraph {
  lanes: GraphLane[];
  totalWorkflows: number;
  totalNodes: number;
  workingNodes: number;
  alertCount: number;
  approvalCount: number;
  level: 'red' | 'yellow' | 'green';
}

function freshAt(at: string | undefined, asOfMs: number): boolean {
  if (!at) return false;
  const t = new Date(at).getTime();
  if (Number.isNaN(t)) return false;
  const delta = asOfMs - t;
  // 负差（时钟偏移导致事件晚于数据时点）保守按不在干活处理。
  return delta >= 0 && delta <= WORKING_FRESH_MS;
}

export function isNodeWorking(source: SourceView | undefined, asOf: string): boolean {
  if (!source || source.skipped || source.unwired) return false;
  const asOfMs = new Date(asOf).getTime();
  if (Number.isNaN(asOfMs)) return false;
  return freshAt(source.last_event_at, asOfMs) || freshAt(source.last_ok_at, asOfMs);
}

export function buildCompanyGraph(ov: Overview): CompanyGraph {
  const lanes: GraphLane[] = ov.lanes.map((lane) => {
    const workflows: GraphWorkflow[] = lane.workflows.map((wf) => {
      const nodes: GraphNode[] = wf.nodes.map((nd) => ({
        id: nd.id,
        name: nd.name,
        order: nd.order,
        health: nd.health,
        working: isNodeWorking(nd.source, ov.as_of),
        reason: nd.reason,
      }));
      const worst = wf.nodes.slice().sort((a, b) => healthRank(a.health) - healthRank(b.health))[0];
      const worstBad = worst !== undefined && (worst.health === 'red' || worst.health === 'yellow');
      return {
        id: wf.id,
        name: wf.name,
        health: wf.health,
        alerts: wf.alerts,
        volume1h: wf.volume_1h,
        working: nodes.some((nd) => nd.working),
        worstHealth: worst?.health ?? 'gray',
        worstReason: worstBad ? worst.reason : '',
        nodes,
      };
    });
    const laneHealth = workflows.map((w) => w.health).sort((a, b) => healthRank(a) - healthRank(b))[0] ?? 'gray';
    return {
      bizLine: lane.biz_line,
      health: laneHealth,
      working: workflows.some((w) => w.working),
      volume1h: workflows.reduce((s, w) => s + w.volume1h, 0),
      alerts: workflows.reduce((s, w) => s + w.alerts, 0),
      workflows,
    };
  });
  const allWfs = lanes.flatMap((l) => l.workflows);
  return {
    lanes,
    totalWorkflows: allWfs.length,
    totalNodes: allWfs.reduce((s, w) => s + w.nodes.length, 0),
    workingNodes: allWfs.reduce((s, w) => s + w.nodes.filter((nd) => nd.working).length, 0),
    alertCount: ov.alerts.length,
    approvalCount: ov.approvals.count,
    level: overviewLevel(ov),
  };
}
