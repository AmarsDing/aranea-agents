/**
 * 公司树（经营看板 V2 首屏）的纯函数：把组织树、Agent 名册、收件箱、交办、
 * 定时任务收敛成「我 → 总经理 Agent → 部门 → 岗位」四层。
 *
 * 口径：
 * - 只用已有数据，不估不造：没有来源的东西（收入、目标等）这里没有字段。
 * - 部门/岗位状态只有四种，按严重程度取最重的一种：
 *     attention  有事要我处理，或有 Agent 最近一次运行失败
 *     ok         今天有 Agent 跑过
 *     idle       有 Agent 在岗，但今天没动静
 *     empty      没有 Agent 在岗
 */

export type NodeState = 'attention' | 'ok' | 'idle' | 'empty';

export type OrgTreeInput = {
  node?: {
    id?: string;
    name?: string;
    level?: string;
    enabled?: boolean;
    sortOrder?: number;
    deletedAt?: string;
    deptLeadAgentId?: string;
    metadataJson?: string;
  };
  children?: OrgTreeInput[];
};

export type AgentInput = {
  id: string;
  agent_key: string;
  display_name: string;
  /** 所挂岗位（组织树 position 节点 id） */
  taxonomy_position_id?: string;
  status?: string;
  last_run_status?: string;
  last_run_at?: string;
  deleted_at?: string;
};

export type InboxInput = { agent_key: string; due_at: string; risk_level: string };
export type DelegatedInput = { assignee_kind: string; assignee_id: string };
export type CronInput = { agentId: string; nextRunAt: string };

export type TreeAgent = {
  id: string;
  key: string;
  name: string;
  failed: boolean;
  ranToday: boolean;
  lastRunAt: string;
};

export type PositionLeaf = {
  id: string;
  name: string;
  agents: TreeAgent[];
  state: NodeState;
  /** 等我处理的事项里，由这个岗位的 Agent 提出的条数 */
  waiting: number;
  failed: number;
};

export type DeptNode = {
  id: string;
  name: string;
  lead: TreeAgent | null;
  /** 配置了负责人 id 但名册里找不到（被删或无权查看） */
  leadMissing: boolean;
  agentCount: number;
  activeToday: number;
  failed: number;
  waiting: number;
  /** 我交办出去、还没结掉的任务里，派给这个部门 Agent 的条数 */
  delegated: number;
  state: NodeState;
  lastRunAt: string;
  nextRunAt: string;
  positions: PositionLeaf[];
};

export type CompanyTree = {
  companies: { id: string; name: string }[];
  company: { id: string; name: string } | null;
  me: { waiting: number; highRisk: number; earliestDue: string };
  /** 总经理 Agent；公司节点没有指定时为 null */
  ceo: TreeAgent | null;
  ceoConfigured: boolean;
  depts: DeptNode[];
  totals: { agents: number; activeToday: number; failed: number; unassigned: number };
};

const STATE_RANK: Record<NodeState, number> = { attention: 0, ok: 1, idle: 2, empty: 3 };
const FAILED = /fail|error/i;

function ms(iso: string | undefined): number {
  if (!iso) return NaN;
  return new Date(iso).getTime();
}

function later(a: string, b: string): string {
  return (ms(b) || 0) > (ms(a) || 0) ? b : a;
}

function earlier(a: string, b: string): string {
  if (!Number.isFinite(ms(b))) return a;
  if (!Number.isFinite(ms(a))) return b;
  return ms(b) < ms(a) ? b : a;
}

function sameDay(iso: string | undefined, now: Date): boolean {
  const t = ms(iso);
  if (!Number.isFinite(t)) return false;
  const d = new Date(t);
  return d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate();
}

/** 公司节点的总经理 Agent id 存在 metadata_json.company_lead_agent_id。 */
export function companyLeadId(metadataJson: string | undefined): string {
  if (!metadataJson) return '';
  try {
    const parsed: unknown = JSON.parse(metadataJson);
    const id = (parsed as Record<string, unknown> | null)?.company_lead_agent_id;
    return typeof id === 'string' ? id.trim() : '';
  } catch {
    return '';
  }
}

function stateOf(agentCount: number, activeToday: number, failed: number, waiting: number): NodeState {
  if (agentCount === 0) return waiting > 0 ? 'attention' : 'empty';
  if (failed > 0 || waiting > 0) return 'attention';
  return activeToday > 0 ? 'ok' : 'idle';
}

function alive(node: OrgTreeInput['node']): boolean {
  return Boolean(node?.id) && !node?.deletedAt && node?.enabled !== false;
}

function bySortThenName(a: OrgTreeInput, b: OrgTreeInput): number {
  return (a.node?.sortOrder ?? 0) - (b.node?.sortOrder ?? 0) || (a.node?.name ?? '').localeCompare(b.node?.name ?? '');
}

/** 取某层级的直接后代；容忍公司下直接挂岗位、部门下再套部门的树。 */
function collect(root: OrgTreeInput, level: string): OrgTreeInput[] {
  const out: OrgTreeInput[] = [];
  const walk = (node: OrgTreeInput) => {
    for (const child of node.children ?? []) {
      if (!alive(child.node)) continue;
      if (child.node?.level === level) out.push(child);
      else walk(child);
    }
  };
  walk(root);
  return out.sort(bySortThenName);
}

export function buildCompanyTree(input: {
  orgTree: readonly OrgTreeInput[];
  agents: readonly AgentInput[];
  inbox: readonly InboxInput[];
  inboxTotal: number;
  delegated: readonly DelegatedInput[];
  cron: readonly CronInput[];
  /** 选中的公司；为空或找不到时取第一家 */
  companyId: string;
  now: Date;
}): CompanyTree {
  const { now } = input;
  const companyNodes = input.orgTree.filter((n) => alive(n.node) && n.node?.level === 'company').sort(bySortThenName);
  const companies = companyNodes.map((n) => ({ id: n.node?.id ?? '', name: n.node?.name ?? '' }));
  const root = companyNodes.find((n) => n.node?.id === input.companyId) ?? companyNodes[0];

  const agents = input.agents.filter((a) => a.id && !a.deleted_at);
  const toTreeAgent = (a: AgentInput): TreeAgent => ({
    id: a.id,
    key: a.agent_key,
    name: a.display_name || a.agent_key,
    failed: FAILED.test(a.last_run_status ?? ''),
    ranToday: sameDay(a.last_run_at, now),
    lastRunAt: a.last_run_at ?? '',
  });
  const byId = new Map(agents.map((a) => [a.id, a]));
  const byPosition = new Map<string, TreeAgent[]>();
  for (const a of agents) {
    const pos = a.taxonomy_position_id?.trim();
    if (!pos) continue;
    byPosition.set(pos, [...(byPosition.get(pos) ?? []), toTreeAgent(a)]);
  }

  const waitingByKey = new Map<string, number>();
  let highRisk = 0;
  let earliestDue = '';
  for (const item of input.inbox) {
    if (item.agent_key) waitingByKey.set(item.agent_key, (waitingByKey.get(item.agent_key) ?? 0) + 1);
    if (item.risk_level === 'high') highRisk += 1;
    earliestDue = earlier(earliestDue, item.due_at);
  }
  const delegatedById = new Map<string, number>();
  for (const task of input.delegated) {
    if (task.assignee_kind !== 'agent' || !task.assignee_id) continue;
    delegatedById.set(task.assignee_id, (delegatedById.get(task.assignee_id) ?? 0) + 1);
  }
  const nextRunById = new Map<string, string>();
  for (const job of input.cron) {
    if (!job.agentId || !(ms(job.nextRunAt) >= now.getTime())) continue;
    nextRunById.set(job.agentId, earlier(nextRunById.get(job.agentId) ?? '', job.nextRunAt));
  }

  const me = { waiting: Math.max(input.inboxTotal, input.inbox.length), highRisk, earliestDue };
  if (!root) {
    return {
      companies,
      company: null,
      me,
      ceo: null,
      ceoConfigured: false,
      depts: [],
      totals: { agents: 0, activeToday: 0, failed: 0, unassigned: 0 },
    };
  }

  const placed = new Set<string>();
  const depts: DeptNode[] = collect(root, 'department').map((dept) => {
    const positions: PositionLeaf[] = collect(dept, 'position').map((pos) => {
      const list = byPosition.get(pos.node?.id ?? '') ?? [];
      for (const a of list) placed.add(a.id);
      const waiting = list.reduce((sum, a) => sum + (waitingByKey.get(a.key) ?? 0), 0);
      const failed = list.filter((a) => a.failed).length;
      const active = list.filter((a) => a.ranToday).length;
      return {
        id: pos.node?.id ?? '',
        name: pos.node?.name ?? '',
        agents: list,
        state: stateOf(list.length, active, failed, waiting),
        waiting,
        failed,
      };
    });
    const leadId = dept.node?.deptLeadAgentId?.trim() ?? '';
    const leadRow = leadId ? byId.get(leadId) : undefined;
    const lead = leadRow ? toTreeAgent(leadRow) : null;
    if (lead) placed.add(lead.id);
    // 负责人可能不挂本部门岗位：部门口径 = 岗位上的 Agent ∪ 负责人，按 id 去重。
    const members = new Map<string, TreeAgent>();
    for (const pos of positions) for (const a of pos.agents) members.set(a.id, a);
    // 有的 Agent（如自动建的部门负责人）直接挂在部门节点上，不经过岗位。
    for (const a of byPosition.get(dept.node?.id ?? '') ?? []) {
      members.set(a.id, a);
      placed.add(a.id);
    }
    if (lead) members.set(lead.id, lead);
    const all = [...members.values()];
    const failed = all.filter((a) => a.failed).length;
    const activeToday = all.filter((a) => a.ranToday).length;
    const waiting = all.reduce((sum, a) => sum + (waitingByKey.get(a.key) ?? 0), 0);
    return {
      id: dept.node?.id ?? '',
      name: dept.node?.name ?? '',
      lead,
      leadMissing: Boolean(leadId) && !lead,
      agentCount: all.length,
      activeToday,
      failed,
      waiting,
      delegated: all.reduce((sum, a) => sum + (delegatedById.get(a.id) ?? 0), 0),
      state: stateOf(all.length, activeToday, failed, waiting),
      lastRunAt: all.reduce((latest, a) => later(latest, a.lastRunAt), ''),
      nextRunAt: all.reduce((soonest, a) => earlier(soonest, nextRunById.get(a.id) ?? ''), ''),
      // 有问题的岗位排前面，其次人多的；同档保持组织树里的顺序。
      positions: positions
        .map((p, index) => ({ p, index }))
        .sort(
          (a, b) =>
            STATE_RANK[a.p.state] - STATE_RANK[b.p.state] || b.p.agents.length - a.p.agents.length || a.index - b.index,
        )
        .map(({ p }) => p),
    };
  });

  const ceoId = companyLeadId(root.node?.metadataJson);
  const ceoRow = ceoId ? byId.get(ceoId) : undefined;
  if (ceoRow) placed.add(ceoRow.id);
  // 多公司时名册是全工作区的：只有一家公司才能说「其余都是未编组」。
  const unassigned = companies.length === 1 ? agents.filter((a) => !placed.has(a.id)).length : 0;

  return {
    companies,
    company: { id: root.node?.id ?? '', name: root.node?.name ?? '' },
    me,
    ceo: ceoRow ? toTreeAgent(ceoRow) : null,
    ceoConfigured: Boolean(ceoId),
    depts,
    totals: {
      agents: depts.reduce((sum, d) => sum + d.agentCount, 0) + (ceoRow ? 1 : 0),
      activeToday: depts.reduce((sum, d) => sum + d.activeToday, 0),
      failed: depts.reduce((sum, d) => sum + d.failed, 0),
      unassigned,
    },
  };
}

/** HH:mm；不是今天则带上月-日。无效时间返回空串。 */
export function shortTime(iso: string, now: Date): string {
  const t = ms(iso);
  if (!Number.isFinite(t)) return '';
  const d = new Date(t);
  const pad = (n: number) => String(n).padStart(2, '0');
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  return sameDay(iso, now) ? hm : `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${hm}`;
}
