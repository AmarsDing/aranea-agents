/**
 * 作战总台纯函数层：lane 聚合 / 归因 / 状态词推导 / 走廊合并。
 * 无 vue 依赖，单元测试直接打这里（CS-F5）。
 */
import type { DecisionRecordItem } from '../decision/types';
import type { Opportunity, ReviewTask } from '../mvp/api';
import type {
  AgentLiveState,
  BizLineId,
  LiveKind,
  LogLine,
  RosterAgent,
  RosterDept,
  RosterModel,
  RosterPosition,
  ToolBeat,
  YouItem,
  YouLaneState,
} from './types';

// ——— outcome 判定（与 useAttentionBoard 同形态）———
export function isPendingDecision(outcome: string): boolean {
  const o = (outcome ?? '').toLowerCase();
  return o === '' || o === 'pending' || o === 'awaiting';
}

// ——— 业务线归因（M67/M78：按既有组织树关键词绑定，不发明新组织模型）———
const BIZLINE_KEYWORDS: ReadonlyArray<[BizLineId, readonly string[]]> = [
  ['novatrader', ['novatrader', '股票', '交易', '盯盘', 'quant']],
  ['hotcraft', ['hotcraft', '热点', '二创', '内容', 'content']],
  ['assistant', ['助手', '助理', '综合', '办公', '秘书', 'assistant']],
  ['ecmove', ['商品', '搬运', '电商', '选品', '刊登', 'ec-']],
];

export function mapNameToBizLine(name: string): BizLineId | '' {
  const n = (name ?? '').toLowerCase();
  for (const [line, kws] of BIZLINE_KEYWORDS) {
    if (kws.some((k) => n.includes(k.toLowerCase()))) return line;
  }
  return '';
}

// ——— 花名册分组（组织树 + Agent 列表 → RosterModel）———
type OrgNodeInput = {
  id: string;
  name: string;
};

type AgentInput = {
  agent_key: string;
  display_name: string;
  icon: string;
  org_node_id?: string;
  deleted_at?: string;
};

export function groupRoster(
  departments: readonly OrgNodeInput[],
  positions: readonly (OrgNodeInput & { parentId: string })[],
  agents: readonly AgentInput[],
): RosterModel {
  const aliveAgents = agents.filter((a) => !a.deleted_at);
  const byPosition = new Map<string, RosterAgent[]>();
  const byDeptFloat = new Map<string, RosterAgent[]>();
  const unassigned: RosterAgent[] = [];

  const posIndex = new Map(positions.map((p) => [p.id, p]));
  const deptIndex = new Map(departments.map((d) => [d.id, d]));
  const deptBiz = new Map(departments.map((d) => [d.id, mapNameToBizLine(d.name)]));

  for (const a of aliveAgents) {
    const nodeId = (a.org_node_id ?? '').trim();
    const pos = nodeId ? posIndex.get(nodeId) : undefined;
    const dept = nodeId ? deptIndex.get(nodeId) : undefined;
    if (pos) {
      const deptId = pos.parentId;
      const ra: RosterAgent = {
        key: a.agent_key,
        name: a.display_name || a.agent_key,
        icon: a.icon || '',
        positionId: pos.id,
        positionName: pos.name,
        deptId,
        deptName: deptIndex.get(deptId)?.name ?? '',
        bizLine: deptBiz.get(deptId) ?? '',
      };
      const list = byPosition.get(pos.id) ?? [];
      list.push(ra);
      byPosition.set(pos.id, list);
    } else if (dept) {
      const ra: RosterAgent = {
        key: a.agent_key,
        name: a.display_name || a.agent_key,
        icon: a.icon || '',
        positionId: '',
        positionName: '',
        deptId: dept.id,
        deptName: dept.name,
        bizLine: deptBiz.get(dept.id) ?? '',
      };
      const list = byDeptFloat.get(dept.id) ?? [];
      list.push(ra);
      byDeptFloat.set(dept.id, list);
    } else {
      unassigned.push({
        key: a.agent_key,
        name: a.display_name || a.agent_key,
        icon: a.icon || '',
        positionId: '',
        positionName: '',
        deptId: '',
        deptName: '',
        bizLine: mapNameToBizLine(a.display_name || a.agent_key),
      });
    }
  }

  const depts: RosterDept[] = departments.map((d) => {
    const ps: RosterPosition[] = positions
      .filter((p) => p.parentId === d.id)
      .map((p) => ({ id: p.id, name: p.name, agents: byPosition.get(p.id) ?? [] }));
    return {
      id: d.id,
      name: d.name,
      bizLine: deptBiz.get(d.id) ?? '',
      positions: ps,
      floatAgents: byDeptFloat.get(d.id) ?? [],
    };
  });
  return { depts, unassigned };
}

// ——— 状态词推导（step → 角色行状态；word 文案交给展示层 i18n）———
export function liveKindFromStep(kind: string, status: string, errorCode: string, waitingConfirm: boolean): LiveKind {
  if (waitingConfirm) return 'waiting';
  if (status === 'failed' || errorCode) return 'error';
  switch (kind) {
    case 'thinking':
      return 'thinking';
    case 'action':
    case 'reply':
      return 'working';
    default:
      return 'idle';
  }
}

export function stepToToolBeat(step: {
  ToolName: string;
  ToolDurationMs: number;
  ToolErrorCode: string;
  CompletedAt: string | null;
  StartedAt: string;
  Status: string;
}): ToolBeat | null {
  if (!step.ToolName) return null;
  return {
    toolName: step.ToolName,
    durationMs: step.ToolDurationMs ?? 0,
    errorCode: step.ToolErrorCode ?? '',
    at: step.CompletedAt ?? step.StartedAt ?? '',
  };
}

// ——— ACTIVITY 卡 ticker 行（tool_name · 耗时；11px 单行）———
export function toolBeatText(b: ToolBeat): string {
  const sec = (b.durationMs / 1000).toFixed(1).replace(/\.0$/, '');
  return `${b.toolName} · ${sec}s`;
}

// ——— 日志行构造（回执/事件 → 卷宗行）———
export function makeLogLine(id: string, at: string, kind: LogLine['kind'], agentKey: string, text: string): LogLine {
  return { id, at, kind, agentKey, text };
}

// ——— YOU lane 聚合 ———
const MAX_ROWS_PER_LANE = 5;

export function buildTradeLane(decisions: readonly DecisionRecordItem[], now: string, degraded: boolean): YouLaneState {
  const pending = decisions.filter((d) => isPendingDecision(d.outcome));
  const items: YouItem[] = pending.slice(0, MAX_ROWS_PER_LANE).map((d) => ({
    id: `hitl:${d.decision_key}`,
    lane: 'trade' as const,
    agentKey: d.actor_key ?? '',
    who: d.actor_key || '?',
    when: d.created_at ?? '',
    summary: (d.reasoning || d.scenario || d.category || '').slice(0, 80),
    // 决策记录无写路径（写路径在 team run 域）→ 只下钻，诚实导航
    action: 'goto' as const,
    to: '/team',
    danger: true,
  }));
  return { lane: 'trade', items, count: pending.length, updatedAt: now, degraded };
}

export function buildPublishLane(reviews: readonly ReviewTask[], now: string, degraded: boolean): YouLaneState {
  const pend = reviews.filter((r) => (r.Status ?? '').toLowerCase() === 'pending');
  const items: YouItem[] = pend.slice(0, MAX_ROWS_PER_LANE).map((r) => ({
    id: `review:${r.ID}`,
    lane: 'publish' as const,
    agentKey: '',
    who: (r.MachineFlags ?? []).join(' ') || 'machine',
    when: '',
    summary: r.DraftTitle || r.DraftBody?.slice(0, 60) || r.ID,
    action: 'approve' as const,
  }));
  return { lane: 'publish', items, count: pend.length, updatedAt: now, degraded };
}

export function buildListingLane(opps: readonly Opportunity[], now: string, degraded: boolean): YouLaneState {
  const pend = opps.filter((o) => ['pending', 'new', 'open', ''].includes((o.Status ?? '').toLowerCase()));
  const items: YouItem[] = pend.slice(0, MAX_ROWS_PER_LANE).map((o) => ({
    id: `opp:${o.ID}`,
    lane: 'listing' as const,
    agentKey: '',
    who: o.Audience || 'sourcing',
    when: '',
    summary: `${o.Title}${o.Score ? ` · ${o.Score.toFixed(2)}` : ''}`,
    action: 'ack' as const,
  }));
  return { lane: 'listing', items, count: pend.length, updatedAt: now, degraded };
}

export function buildAlertsLane(
  deadLetters: readonly { id: string; source_type: string; error_message: string; created_at: string }[],
  cronFailures: readonly { id: string; task_name: string; error_message: string; started_at: string }[],
  now: string,
  degraded: boolean,
): YouLaneState {
  const items: YouItem[] = [
    ...deadLetters.map((d) => ({
      id: `dead:${d.id}`,
      lane: 'alerts' as const,
      agentKey: '',
      who: d.source_type || 'task',
      when: d.created_at ?? '',
      summary: (d.error_message || '').slice(0, 80),
      action: 'goto' as const,
      to: '/team',
      danger: true,
    })),
    ...cronFailures.map((c) => ({
      id: `cron:${c.id}`,
      lane: 'alerts' as const,
      agentKey: '',
      who: c.task_name || 'cron',
      when: c.started_at ?? '',
      summary: (c.error_message || '').slice(0, 80),
      action: 'goto' as const,
      to: '/cron',
    })),
  ].slice(0, MAX_ROWS_PER_LANE);
  return {
    lane: 'alerts',
    items,
    count: deadLetters.length + cronFailures.length,
    updatedAt: now,
    degraded,
  };
}

// ——— 走廊合并口径（同屏汇聚 ≤8 条，超出按业务线合一为 ×N）———
export const CORRIDOR_MERGE_THRESHOLD = 8;

export function corridorKey(agentKey: string, bizLine: BizLineId | '', activeCount: number): string {
  if (activeCount <= CORRIDOR_MERGE_THRESHOLD) return agentKey;
  return `line:${bizLine || 'misc'}`;
}

// ——— Live 附加值 ———
export function newLiveState(agentKey: string): AgentLiveState {
  return {
    agentKey,
    statusKind: 'idle',
    statusWord: '',
    lastToolName: '',
    lastDurationMs: 0,
    lastErrorCode: '',
    lastEventAt: '',
    recentTools: [],
    lastThinking: '',
  };
}

export const RECENT_TOOLS_CAP = 8;
export const LOG_LINES_CAP = 80;
export const CORRIDOR_ACTIVE_WINDOW_MS = 3000;
