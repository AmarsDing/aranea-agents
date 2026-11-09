import { AxiosError } from 'axios';
import {
  createAgentBridgeService,
  createAgentService,
  createApprovalCenterService,
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
  page?: number;
  pageSize?: number;
}): Promise<ApprovalListResult> {
  const svc = createApprovalCenterService();
  const res = await svc.ListApprovals({
    kind: query.kind ? (KIND_TO_PROTO[query.kind] as never) : undefined,
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
    default:
      // mask_proposal / offboard_export / schedule_draft / memory_fact 一期
      // 无聚合源（列表恒空）；防御分支，理论上不可达。
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
