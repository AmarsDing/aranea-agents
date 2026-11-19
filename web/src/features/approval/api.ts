import { AxiosError } from 'axios';
import {
  createAgentBridgeService,
  createAgentService,
  createApprovalCenterService,
  createOfficeScheduleService,
  createSkillEvolutionSuggestionService,
} from '../../services';
import type { ApprovalItemRow, ApprovalKindKey, DecideAction, RiskLevelKey, RiskMatrixRowModel } from './types';

// ---------- proto 枚举 ↔ 页面键 ----------

const KIND_TO_PROTO: Record<ApprovalKindKey, string> = {
  task_review: 'APPROVAL_KIND_TASK_REVIEW',
  mask_proposal: 'APPROVAL_KIND_MASK_PROPOSAL',
  offboard_export: 'APPROVAL_KIND_OFFBOARD_EXPORT',
  evolution_suggestion: 'APPROVAL_KIND_EVOLUTION_SUGGESTION',
  skill_approval: 'APPROVAL_KIND_SKILL_APPROVAL',
  schedule_draft: 'APPROVAL_KIND_SCHEDULE_DRAFT',
  memory_fact: 'APPROVAL_KIND_MEMORY_FACT',
};

const KIND_FROM_PROTO: Record<string, ApprovalKindKey> = Object.fromEntries(
  Object.entries(KIND_TO_PROTO).map(([k, v]) => [v, k as ApprovalKindKey]),
) as Record<string, ApprovalKindKey>;

const RISK_FROM_PROTO: Record<string, RiskLevelKey> = {
  RISK_LEVEL_LOW: 'low',
  RISK_LEVEL_MEDIUM: 'medium',
  RISK_LEVEL_HIGH: 'high',
};

const RISK_TO_PROTO: Record<RiskLevelKey, string> = {
  low: 'RISK_LEVEL_LOW',
  medium: 'RISK_LEVEL_MEDIUM',
  high: 'RISK_LEVEL_HIGH',
};

function wireToItem(raw: Record<string, unknown>): ApprovalItemRow {
  const contextRaw = String(raw.contextJson ?? raw.context_json ?? '');
  let context: Record<string, unknown> | null = null;
  if (contextRaw) {
    try {
      const parsed = JSON.parse(contextRaw);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        context = parsed as Record<string, unknown>;
      }
    } catch {
      context = null; // 解析失败回退原文展示（诚实失败，不吞形状）
    }
  }
  return {
    kind: KIND_FROM_PROTO[String(raw.kind ?? '')] ?? 'task_review',
    id: String(raw.id ?? ''),
    title: String(raw.title ?? ''),
    summary: String(raw.summary ?? ''),
    context,
    context_raw: context ? '' : contextRaw,
    risk_level: RISK_FROM_PROTO[String(raw.riskLevel ?? raw.risk_level ?? '')] ?? '',
    requester: String(raw.requester ?? ''),
    agent_key: String(raw.agentKey ?? raw.agent_key ?? ''),
    created_at: String(raw.createdAt ?? raw.created_at ?? ''),
  };
}

function wireToMatrixRow(raw: Record<string, unknown>): RiskMatrixRowModel {
  return {
    matcher_key: String(raw.matcherKey ?? raw.matcher_key ?? ''),
    risk: RISK_FROM_PROTO[String(raw.risk ?? '')] ?? 'medium',
    default_mode: String(raw.defaultMode ?? raw.default_mode ?? 'ask'),
    auto_approve_times: Number(raw.autoApproveTimes ?? raw.auto_approve_times ?? 0),
    updated_by: String(raw.updatedBy ?? raw.updated_by ?? ''),
    updated_at: String(raw.updatedAt ?? raw.updated_at ?? ''),
  };
}

// ---------- 聚合端点（只读） ----------

export type ApprovalListResult = {
  items: ApprovalItemRow[];
  total: number;
};

export async function listApprovals(query: {
  kind?: ApprovalKindKey | '';
  /** 精确请求人筛选（服务端按 requester 过滤后再分页/计数）；空 = 不筛选 */
  requester?: string;
  page?: number;
  pageSize?: number;
}): Promise<ApprovalListResult> {
  const svc = createApprovalCenterService();
  const res = await svc.ListApprovals({
    kind: query.kind ? (KIND_TO_PROTO[query.kind] as never) : undefined,
    requester: query.requester?.trim() || undefined,
    page: query.page ?? 1,
    pageSize: query.pageSize ?? 20,
  });
  return {
    items: (res.items ?? []).map((row) => wireToItem(row as Record<string, unknown>)),
    total: Number(res.total ?? 0),
  };
}

/** 铃铛/分组计数。返回 kind → count（未列出的类视为 0）。 */
export async function getApprovalSummary(): Promise<{
  counts: Partial<Record<ApprovalKindKey, number>>;
  total: number;
}> {
  const svc = createApprovalCenterService();
  const res = await svc.GetApprovalSummary({});
  const counts: Partial<Record<ApprovalKindKey, number>> = {};
  for (const c of res.counts ?? []) {
    const rec = c as Record<string, unknown>;
    const k = KIND_FROM_PROTO[String(rec.kind ?? '')];
    if (k) counts[k] = Number(rec.count ?? 0);
  }
  return { counts, total: Number(res.total ?? 0) };
}

// ---------- 决议出口（各领域 RPC，聚合端点无写路径） ----------

export type DecideInput = {
  item: ApprovalItemRow;
  action: DecideAction;
  note: string;
  /** 当前登录用户 id 字符串（技能审批 RPC 的 approvedBy/rejectedBy 仍按
   *  既有契约由调用方传入；review.v1 收编面已改服务端 JWT 取值）。 */
  actor: string;
};

export async function decideApproval({ item, action, note, actor }: DecideInput): Promise<void> {
  switch (item.kind) {
    case 'task_review': {
      // ReviewTask 契约：approve/reject/archive；reject 时 comment 必填（服务端 400）。
      await createAgentBridgeService().ReviewTask({ id: item.id, action, comment: note });
      return;
    }
    case 'evolution_suggestion': {
      const agentId = String(item.context?.target_id ?? '');
      if (!agentId) throw new Error('evolution suggestion missing context.target_id');
      const svc = createAgentService();
      if (action === 'approve') {
        await svc.ApplyEvolutionSuggestion({ agentId, suggestionId: item.id });
      } else {
        await svc.RejectEvolutionSuggestion({ agentId, suggestionId: item.id, reason: note });
      }
      return;
    }
    case 'skill_approval': {
      const svc = createSkillEvolutionSuggestionService();
      if (action === 'approve') {
        await svc.ApproveSkillEvolutionSuggestion({ id: item.id, approvedBy: actor });
      } else {
        await svc.RejectSkillEvolutionSuggestion({ id: item.id, rejectedBy: actor, rejectionReason: note });
      }
      return;
    }
    case 'schedule_draft': {
      // W1c 日程草案决议出口（office 域 RPC；批准=单事务逐项 409 复检+落库，
      // 拒绝须理由——服务端 400/409 语义与上一致）。
      const svc = createOfficeScheduleService();
      if (action === 'approve') {
        await svc.ApproveScheduleDraft({ id: item.id });
      } else {
        await svc.RejectScheduleDraft({ id: item.id, reason: note });
      }
      return;
    }
    default:
      // mask_proposal / offboard_export / memory_fact 一期无聚合源（列表
      // 恒空）；防御分支，理论上不可达。
      throw new Error(`approval kind ${item.kind} has no decide path`);
  }
}

/** 决议冲突（并发决议恰一个成功，其余 409）判定——用于「已被处理」提示语义。 */
export function isConflictError(e: unknown): boolean {
  return e instanceof AxiosError && e.response?.status === 409;
}

// ---------- 风险矩阵（A-4；写路径 approval.policy 硬判） ----------

export async function getRiskMatrix(): Promise<RiskMatrixRowModel[]> {
  const svc = createApprovalCenterService();
  const res = await svc.GetRiskMatrix({});
  return (res.rows ?? []).map((row) => wireToMatrixRow(row as Record<string, unknown>));
}

/** 全量替换语义（DELETE+INSERT）；返回服务端回读的最新表。 */
export async function putRiskMatrix(rows: RiskMatrixRowModel[]): Promise<RiskMatrixRowModel[]> {
  const svc = createApprovalCenterService();
  const res = await svc.PutRiskMatrix({
    rows: rows.map((r) => ({
      matcherKey: r.matcher_key,
      risk: RISK_TO_PROTO[r.risk] as never,
      defaultMode: r.default_mode,
      autoApproveTimes: r.auto_approve_times,
      // updatedBy/updatedAt 由服务端统一落列（取 JWT 主体），客户端不传。
      updatedBy: undefined,
      updatedAt: undefined,
    })),
  });
  return (res.rows ?? []).map((row) => wireToMatrixRow(row as Record<string, unknown>));
}

// ---------- 统一收件箱（服务端聚合；全站唯一待办口径） ----------

export type InboxAction = 'approve' | 'reject' | 'ack' | 'resolve';

export type InboxItemRow = {
  id: string;
  source: string;
  kind: string;
  title: string;
  summary: string;
  risk_level: RiskLevelKey | '';
  requester: string;
  agent_key: string;
  created_at: string;
  due_at: string;
  actions: InboxAction[];
  reject_requires_note: boolean;
  link: string;
};

export type InboxListResult = {
  items: InboxItemRow[];
  total: number;
  /** 来源 → 待办数（已注册来源全量，含 0） */
  counts: Record<string, number>;
  /** 本次读取失败的来源（计数不可信） */
  degraded: string[];
};

const INBOX_ACTIONS: ReadonlySet<string> = new Set(['approve', 'reject', 'ack', 'resolve']);

function wireToInboxItem(raw: Record<string, unknown>): InboxItemRow {
  const actions = Array.isArray(raw.actions) ? raw.actions : [];
  return {
    id: String(raw.id ?? ''),
    source: String(raw.source ?? ''),
    kind: String(raw.kind ?? ''),
    title: String(raw.title ?? ''),
    summary: String(raw.summary ?? ''),
    risk_level: RISK_FROM_PROTO[String(raw.riskLevel ?? raw.risk_level ?? '')] ?? '',
    requester: String(raw.requester ?? ''),
    agent_key: String(raw.agentKey ?? raw.agent_key ?? ''),
    created_at: String(raw.createdAt ?? raw.created_at ?? ''),
    due_at: String(raw.dueAt ?? raw.due_at ?? ''),
    actions: actions.map(String).filter((a): a is InboxAction => INBOX_ACTIONS.has(a)),
    reject_requires_note: Boolean(raw.rejectRequiresNote ?? raw.reject_requires_note ?? false),
    link: String(raw.link ?? ''),
  };
}

function wireToInboxCounts(raw: unknown): Record<string, number> {
  const out: Record<string, number> = {};
  for (const c of Array.isArray(raw) ? raw : []) {
    const rec = c as Record<string, unknown>;
    const source = String(rec.source ?? '');
    if (source) out[source] = Number(rec.count ?? 0);
  }
  return out;
}

export async function listInbox(
  query: { sources?: string[]; page?: number; pageSize?: number } = {},
): Promise<InboxListResult> {
  const res = (await createApprovalCenterService().ListInbox({
    sources: query.sources?.length ? query.sources : undefined,
    page: query.page ?? 1,
    pageSize: query.pageSize ?? 50,
  })) as unknown as Record<string, unknown>;
  const items = Array.isArray(res.items) ? res.items : [];
  return {
    items: items.map((row) => wireToInboxItem(row as Record<string, unknown>)),
    total: Number(res.total ?? 0),
    counts: wireToInboxCounts(res.counts),
    degraded: (Array.isArray(res.degradedSources) ? res.degradedSources : []).map(String),
  };
}

/** 全站待办数（侧栏角标 / 工作台 / 经营看板同源）。 */
export async function getInboxSummary(): Promise<{
  total: number;
  highRisk: number;
  counts: Record<string, number>;
  degraded: string[];
}> {
  const res = (await createApprovalCenterService().GetInboxSummary({})) as unknown as Record<string, unknown>;
  return {
    total: Number(res.total ?? 0),
    highRisk: Number(res.highRisk ?? 0),
    counts: wireToInboxCounts(res.counts),
    degraded: (Array.isArray(res.degradedSources) ? res.degradedSources : []).map(String),
  };
}

/** 统一决议：服务端按 id 回源解析后分发到各领域写路径；条目已被处理时 409。 */
export async function decideInboxItem(id: string, action: InboxAction, note = ''): Promise<void> {
  await createApprovalCenterService().DecideInboxItem({ id, action, note: note.trim() || undefined });
}
