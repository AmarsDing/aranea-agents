/**
 * 作战总台纯函数层：lane 聚合 / 归因 / 状态词推导 / 走廊合并。
 * 无 vue 依赖，单元测试直接打这里（CS-F5）。
 */
import type { DecisionRecordItem } from '../decision/types';
import type { InboxItemRow } from '../approval/api';
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
  YouSeverity,
} from './types';

// ——— outcome 判定（与 useAttentionBoard 同形态）———
// E 阶段 W1b 顺手修复（需求文档 §5.2 关键缺口②）：decision_records 实际
// outcome 词表为 asked/approved/rejected/timeout/resumed
// （tool_confirmation_part2.go 待决写 asked），旧过滤词 ''/pending/awaiting
// 与真实数据零交集导致 trade lane 恒空——对齐为 asked。
export function isPendingDecision(outcome: string): boolean {
  const o = (outcome ?? '').toLowerCase();
  return o === 'asked';
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

const SEVERITY_RANK: Record<YouSeverity, number> = { high: 3, medium: 2, low: 1 };

/** 归因 agent_key（related_entities 中 entity_type=agent） */
export function decisionAgentKey(d: DecisionRecordItem): string {
  const hit = (d.related_entities ?? []).find((e) => (e.entity_type ?? '').toLowerCase() === 'agent');
  return (hit?.entity_key ?? '').trim();
}

function bySeverityThenTime(a: YouItem, b: YouItem): number {
  const rank = SEVERITY_RANK[b.severity] - SEVERITY_RANK[a.severity];
  if (rank !== 0) return rank;
  return (b.when || '').localeCompare(a.when || '');
}

/**
 * 按请求人分组（审批 lane 多人协作场景）：组间按「组内最高风险级 → 最新时间」
 * 排序，组内仍按风险级 → 时间排序；空请求人归入「—」组排最后。
 */
export type RequesterGroup = { requester: string; items: YouItem[] };

export function groupByRequester(items: readonly YouItem[]): RequesterGroup[] {
  const by = new Map<string, YouItem[]>();
  for (const it of items) {
    const key = (it.who || '').trim() || '—';
    const list = by.get(key) ?? [];
    list.push(it);
    by.set(key, list);
  }
  const groups: RequesterGroup[] = [...by.entries()].map(([requester, list]) => ({
    requester,
    items: [...list].sort(bySeverityThenTime),
  }));
  groups.sort((a, b) => {
    if (a.requester === '—') return 1;
    if (b.requester === '—') return -1;
    const rankDiff = SEVERITY_RANK[b.items[0]?.severity ?? 'low'] - SEVERITY_RANK[a.items[0]?.severity ?? 'low'];
    if (rankDiff !== 0) return rankDiff;
    return (b.items[0]?.when ?? '').localeCompare(a.items[0]?.when ?? '');
  });
  return groups;
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

// ——— 统一收件箱 → YOU lane（服务端聚合后的唯一映射；排序与口径以服务端为准）———
const INBOX_SOURCE_LANE: Record<string, YouItem['lane']> = {
  hitl_confirm: 'trade',
  publish_review: 'publish',
  opportunity: 'listing',
  dead_letter: 'alerts',
  cron_failure: 'alerts',
};

/** 展示层沿用的来源标签（告警筛选 dead/cron、审批类别名等） */
const INBOX_SOURCE_TAG: Record<string, string> = {
  hitl_confirm: 'hitl',
  publish_review: 'review',
  opportunity: 'opportunity',
  dead_letter: 'dead',
  cron_failure: 'cron',
};

/** 未列出的来源一律归审批 lane（七类审批及未来新增类别）。 */
export function inboxSourceLane(source: string): YouItem['lane'] {
  return INBOX_SOURCE_LANE[source] ?? 'approval';
}

const LANE_IDS: readonly YouItem['lane'][] = ['approval', 'trade', 'publish', 'listing', 'alerts'];

export function inboxRowToYouItem(row: InboxItemRow): YouItem {
  const lane = inboxSourceLane(row.source);
  const severity: YouSeverity = row.risk_level || 'medium';
  // 首个非驳回动作即主动作；无可执行动作的条目只下钻
  const primary = row.actions.find((a) => a !== 'reject');
  return {
    id: row.id,
    lane,
    agentKey: row.agent_key,
    who: row.requester || '—',
    when: row.created_at,
    summary: (row.title || row.summary || row.id).slice(0, 80),
    detail: row.summary.slice(0, 160),
    action: primary ?? 'goto',
    to: row.link || (primary ? undefined : '/approvals'),
    danger: severity === 'high',
    severity,
    source: INBOX_SOURCE_TAG[row.source] ?? row.source,
  };
}

/**
 * 收件箱一次读取 → 五条 lane。
 * count 取服务端各来源计数之和（含未展示条目）；任一所属来源读取失败即标降级，
 * 并保留该 lane 上一次成功刷新的时间（不伪造新鲜度）。
 */
export function buildLanesFromInbox(
  rows: readonly InboxItemRow[],
  counts: Readonly<Record<string, number>>,
  degradedSources: readonly string[],
  now: string,
  prevUpdatedAt: Readonly<Partial<Record<YouItem['lane'], string>>> = {},
): YouLaneState[] {
  const degraded = new Set(degradedSources.map(inboxSourceLane));
  return LANE_IDS.map((lane) => {
    const all = rows.filter((r) => inboxSourceLane(r.source) === lane).map(inboxRowToYouItem);
    const agentCounts: Record<string, number> = {};
    for (const it of all) {
      if (it.agentKey) agentCounts[it.agentKey] = (agentCounts[it.agentKey] ?? 0) + 1;
    }
    let count = 0;
    for (const [source, n] of Object.entries(counts)) {
      if (inboxSourceLane(source) === lane) count += Number(n) || 0;
    }
    const isDegraded = degraded.has(lane);
    return {
      lane,
      items: all.slice(0, MAX_ROWS_PER_LANE),
      count,
      updatedAt: isDegraded ? (prevUpdatedAt[lane] ?? '') : now,
      degraded: isDegraded,
      agentCounts,
    };
  });
}

export const RECENT_TOOLS_CAP = 8;
export const LOG_LINES_CAP = 80;
export const CORRIDOR_ACTIVE_WINDOW_MS = 3000;
