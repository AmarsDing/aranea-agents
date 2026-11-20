import { formatUsdCompact } from '../usage/moneyFormat';

/**
 * 看板总览（一张图管全部公司）的纯函数：把组织树、Agent 名册、收件箱、交办登记、
 * 定时任务、今日用量收敛成「我 → 公司（含总经理 Agent）→ 部门 → Agent」。
 *
 * 每个节点讲同一件事——电商闭环「建议 → 拍板 → 动手 → 验收」落到系统里的真实对象：
 *   waiting  等你拍板：收件箱里由这个节点的 Agent 提出的事
 *   running  在做：交给这个节点 Agent 的事，执行中
 *   stuck    卡住：交办的事停在等待 / 失败 / 取消
 *   review   待验收：交办的事做完了，等你验收
 * 外加「今天谁跑过」（最近会话或今日用量）和今日 AI 花费。
 *
 * 口径：
 * - 只用已有数据，不估不造：收入、目标这类经营数字没有来源，这里没有字段。
 * - 节点状态四种，按严重程度取最重的一种：
 *     attention  有事等我（拍板 / 验收），或有事卡住、有 Agent 最近一次运行失败
 *     ok         今天有 Agent 跑过，或有事在做
 *     idle       有 Agent 在岗，但今天没动静
 *     empty      没有 Agent 在岗
 */

export type NodeState = 'attention' | 'ok' | 'idle' | 'empty';
export type WorkState = 'running' | 'stuck' | 'review';
/** 读取失败的补充来源（页面据此说明哪些数字不可信） */
export type PartialSource = 'inbox' | 'delegated' | 'cron' | 'usage' | 'signals';

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
  /** 所挂岗位（组织树 position 节点 id；部门负责人可能直接挂部门节点） */
  taxonomy_position_id?: string;
  status?: string;
  last_run_status?: string;
  last_run_at?: string;
  deleted_at?: string;
};

export type InboxInput = {
  id: string;
  title: string;
  agent_key: string;
  due_at: string;
  risk_level: string;
  link?: string;
};
/** 一条交办：已按承载会话的执行态归成三态（见 today/logic 的 classifyOfficeDelegated） */
export type WorkInput = {
  id: string;
  title: string;
  assignee_kind: string;
  assignee_id: string;
  state: WorkState;
  at: string;
  to: string;
};
export type CronInput = { agentId: string; nextRunAt: string };
export type UsageInput = { agentId: string; calls: number; costMicro: number };

export type Loop = { waiting: number; running: number; stuck: number; review: number };

export type BoardAgent = {
  id: string;
  key: string;
  name: string;
  /** 岗位名；部门负责人直接挂部门时为空 */
  position: string;
  failed: boolean;
  ranToday: boolean;
  lastRunAt: string;
  nextRunAt: string;
  calls: number;
  costMicro: number;
  state: NodeState;
  loop: Loop;
  waitingItems: InboxInput[];
  workItems: WorkInput[];
};

type Rollup = {
  agentCount: number;
  activeToday: number;
  failed: number;
  calls: number;
  costMicro: number;
  loop: Loop;
  state: NodeState;
  lastRunAt: string;
  nextRunAt: string;
  waitingItems: InboxInput[];
  workItems: WorkInput[];
};

export type DeptNode = Rollup & {
  id: string;
  name: string;
  lead: BoardAgent | null;
  /** 配置了负责人 id 但名册里找不到（被删或无权查看） */
  leadMissing: boolean;
  positions: number;
  emptyPositions: number;
  /** 有问题的排前面，其次今天跑过的；同档保持组织树里的顺序 */
  agents: BoardAgent[];
};

/** 总经理的巡视与汇报（只有后端 org-graph 给得出；前端回落拼装时没有）。 */
export type GmStatus = {
  /** 巡视 / 汇报两条定时任务至少一条在启用 */
  enabled: boolean;
  lastPatrolAt: string;
  nextPatrolAt: string;
  lastReport?: {
    runId: string;
    at: string;
    conclusion: string;
    reasons: string[];
    /** model = 总经理的模型写的；rule = 模型没回答，按规则生成 */
    writer: string;
    acked: boolean;
  };
  nextReportAt: string;
};

export type CompanyNode = Rollup & {
  id: string;
  name: string;
  /** 总经理 Agent；公司节点没有指定或名册里找不到时为 null */
  ceo: BoardAgent | null;
  ceoConfigured: boolean;
  depts: DeptNode[];
  gm?: GmStatus;
};

/** 收件箱里总经理的汇报来源：要读的，不算「等你拍板」（与后端 org-graph 同一口径）。 */
export const GM_REPORT_SOURCE = 'gm_report';

export type Board = {
  companies: CompanyNode[];
  me: {
    loop: Loop;
    highRisk: number;
    earliestDue: string;
    /** 全部等我拍板的事（含归不到任何公司的） */
    waitingItems: InboxInput[];
    workItems: WorkInput[];
    /** 归不到任何公司的待办（来源不带 Agent，或 Agent 不在组织里） */
    unattributed: number;
    /** 名册里有、但没挂到任何公司的 Agent */
    unassigned: number;
    agentCount: number;
    activeToday: number;
    calls: number;
    costMicro: number;
  };
};

const STATE_RANK: Record<NodeState, number> = { attention: 0, ok: 1, idle: 2, empty: 3 };
const FAILED = /fail|error/i;
const EMPTY_LOOP: Loop = { waiting: 0, running: 0, stuck: 0, review: 0 };

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

function stateOf(agentCount: number, activeToday: number, failed: number, loop: Loop): NodeState {
  const needsMe = loop.waiting + loop.stuck + loop.review > 0;
  if (agentCount === 0) return needsMe ? 'attention' : 'empty';
  if (failed > 0 || needsMe) return 'attention';
  return activeToday > 0 || loop.running > 0 ? 'ok' : 'idle';
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

/** 一组 Agent（按 id 去重后）汇总成节点口径。 */
function rollup(members: readonly BoardAgent[]): Rollup {
  const loop = { ...EMPTY_LOOP };
  const waitingItems: InboxInput[] = [];
  const workItems: WorkInput[] = [];
  let activeToday = 0;
  let failed = 0;
  let calls = 0;
  let costMicro = 0;
  let lastRunAt = '';
  let nextRunAt = '';
  for (const a of members) {
    loop.waiting += a.loop.waiting;
    loop.running += a.loop.running;
    loop.stuck += a.loop.stuck;
    loop.review += a.loop.review;
    waitingItems.push(...a.waitingItems);
    workItems.push(...a.workItems);
    if (a.ranToday) activeToday += 1;
    if (a.failed) failed += 1;
    calls += a.calls;
    costMicro += a.costMicro;
    lastRunAt = later(lastRunAt, a.lastRunAt);
    nextRunAt = earlier(nextRunAt, a.nextRunAt);
  }
  return {
    agentCount: members.length,
    activeToday,
    failed,
    calls,
    costMicro,
    loop,
    state: stateOf(members.length, activeToday, failed, loop),
    lastRunAt,
    nextRunAt,
    waitingItems,
    workItems,
  };
}

function uniq(list: readonly (BoardAgent | null | undefined)[]): BoardAgent[] {
  const seen = new Map<string, BoardAgent>();
  for (const a of list) if (a && !seen.has(a.id)) seen.set(a.id, a);
  return [...seen.values()];
}

export function buildBoard(input: {
  orgTree: readonly OrgTreeInput[];
  agents: readonly AgentInput[];
  inbox: readonly InboxInput[];
  inboxTotal: number;
  work: readonly WorkInput[];
  cron: readonly CronInput[];
  usage: readonly UsageInput[];
  now: Date;
}): Board {
  const { now } = input;

  const waitingByKey = new Map<string, InboxInput[]>();
  let highRisk = 0;
  let earliestDue = '';
  for (const item of input.inbox) {
    if (item.agent_key) waitingByKey.set(item.agent_key, [...(waitingByKey.get(item.agent_key) ?? []), item]);
    if (item.risk_level === 'high') highRisk += 1;
    earliestDue = earlier(earliestDue, item.due_at);
  }
  const workById = new Map<string, WorkInput[]>();
  for (const w of input.work) {
    if (w.assignee_kind !== 'agent' || !w.assignee_id) continue;
    workById.set(w.assignee_id, [...(workById.get(w.assignee_id) ?? []), w]);
  }
  const nextRunById = new Map<string, string>();
  for (const job of input.cron) {
    if (!job.agentId || !(ms(job.nextRunAt) >= now.getTime())) continue;
    nextRunById.set(job.agentId, earlier(nextRunById.get(job.agentId) ?? '', job.nextRunAt));
  }
  const usageById = new Map(input.usage.map((u) => [u.agentId, u]));

  // 岗位 id → 岗位名，部门负责人可能直接挂部门节点（名为空）
  const positionName = new Map<string, string>();
  const index = (node: OrgTreeInput) => {
    if (node.node?.level === 'position' && node.node.id) positionName.set(node.node.id, node.node.name ?? '');
    for (const child of node.children ?? []) index(child);
  };
  input.orgTree.forEach(index);

  const agentById = new Map<string, BoardAgent>();
  const byPosition = new Map<string, BoardAgent[]>();
  for (const a of input.agents) {
    if (!a.id || a.deleted_at) continue;
    const waitingItems = waitingByKey.get(a.agent_key) ?? [];
    const workItems = workById.get(a.id) ?? [];
    const use = usageById.get(a.id);
    const failed = FAILED.test(a.last_run_status ?? '');
    const loop: Loop = {
      waiting: waitingItems.length,
      running: workItems.filter((w) => w.state === 'running').length,
      stuck: workItems.filter((w) => w.state === 'stuck').length,
      review: workItems.filter((w) => w.state === 'review').length,
    };
    const ranToday = sameDay(a.last_run_at, now) || (use?.calls ?? 0) > 0;
    const pos = a.taxonomy_position_id?.trim() ?? '';
    const agent: BoardAgent = {
      id: a.id,
      key: a.agent_key,
      name: a.display_name || a.agent_key,
      position: positionName.get(pos) ?? '',
      failed,
      ranToday,
      lastRunAt: a.last_run_at ?? '',
      nextRunAt: nextRunById.get(a.id) ?? '',
      calls: use?.calls ?? 0,
      costMicro: use?.costMicro ?? 0,
      state: stateOf(1, ranToday ? 1 : 0, failed ? 1 : 0, loop),
      loop,
      waitingItems,
      workItems,
    };
    agentById.set(a.id, agent);
    if (pos) byPosition.set(pos, [...(byPosition.get(pos) ?? []), agent]);
  }

  const placed = new Set<string>();
  const companyNodes = input.orgTree.filter((n) => alive(n.node) && n.node?.level === 'company').sort(bySortThenName);
  const companies: CompanyNode[] = companyNodes.map((root) => {
    const depts: DeptNode[] = collect(root, 'department').map((dept) => {
      const positions = collect(dept, 'position');
      const onPositions = positions.flatMap((p) => byPosition.get(p.node?.id ?? '') ?? []);
      // 有的 Agent（如自动建的部门负责人）直接挂在部门节点上，不经过岗位。
      const onDept = byPosition.get(dept.node?.id ?? '') ?? [];
      const leadId = dept.node?.deptLeadAgentId?.trim() ?? '';
      const lead = leadId ? (agentById.get(leadId) ?? null) : null;
      const members = uniq([lead, ...onDept, ...onPositions]);
      for (const a of members) placed.add(a.id);
      const order = new Map(members.map((a, i) => [a.id, i]));
      return {
        id: dept.node?.id ?? '',
        name: dept.node?.name ?? '',
        lead,
        leadMissing: Boolean(leadId) && !lead,
        positions: positions.length,
        emptyPositions: positions.filter((p) => !(byPosition.get(p.node?.id ?? '') ?? []).length).length,
        agents: [...members].sort(
          (a, b) => STATE_RANK[a.state] - STATE_RANK[b.state] || (order.get(a.id) ?? 0) - (order.get(b.id) ?? 0),
        ),
        ...rollup(members),
      };
    });
    const ceoId = companyLeadId(root.node?.metadataJson);
    const ceo = ceoId ? (agentById.get(ceoId) ?? null) : null;
    if (ceo) placed.add(ceo.id);
    return {
      id: root.node?.id ?? '',
      name: root.node?.name ?? '',
      ceo,
      ceoConfigured: Boolean(ceoId),
      depts,
      ...rollup(uniq([ceo, ...depts.flatMap((d) => d.agents)])),
    };
  });

  const inCompany = uniq(companies.flatMap((c) => [c.ceo, ...c.depts.flatMap((d) => d.agents)]));
  const all = rollup(inCompany);
  const attributed = new Set(inCompany.flatMap((a) => a.waitingItems.map((i) => i.id)));
  const allWork = input.work;
  return {
    companies,
    me: {
      loop: {
        waiting: Math.max(input.inboxTotal, input.inbox.length),
        running: allWork.filter((w) => w.state === 'running').length,
        stuck: allWork.filter((w) => w.state === 'stuck').length,
        review: allWork.filter((w) => w.state === 'review').length,
      },
      highRisk,
      earliestDue,
      waitingItems: [...input.inbox],
      workItems: [...allWork],
      unattributed: input.inbox.filter((i) => !attributed.has(i.id)).length,
      unassigned: [...agentById.keys()].filter((id) => !placed.has(id)).length,
      agentCount: all.agentCount,
      activeToday: all.activeToday,
      calls: all.calls,
      costMicro: all.costMicro,
    },
  };
}

/** 今日花费：0 就写 $0，不写一串零；其余同用量页的紧凑美元格式。 */
export function money(micro: number): string {
  return micro > 0 ? formatUsdCompact(micro) : '$0';
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
