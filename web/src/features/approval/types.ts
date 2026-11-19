// E 阶段 W1b 统一审批中心（需求文档 §5，拍板 A-1/A-4）页面类型。
// ApprovalItem 统一形状来自 approval.v1；决议出口仍走各领域 RPC
// （任务验收→agentbridge ReviewTask，进化建议→agent Apply/Reject，
// 技能审批→skill Approve/Reject）——聚合端点只读，不另开写旁路。

export type ApprovalKindKey =
  | 'task_review'
  | 'mask_proposal'
  | 'offboard_export'
  | 'evolution_suggestion'
  | 'skill_approval'
  | 'schedule_draft'
  | 'memory_fact';

export type RiskLevelKey = 'low' | 'medium' | 'high';

/** 决议动作。archive 仅 task_review 支持（ReviewTask 三动作契约）。 */
export type DecideAction = 'approve' | 'reject' | 'archive';

export type ApprovalItemRow = {
  kind: ApprovalKindKey;
  id: string;
  title: string;
  summary: string;
  /** context_json 解析后的对象；解析失败为 null（页面回退展示原文）。 */
  context: Record<string, unknown> | null;
  context_raw: string;
  risk_level: RiskLevelKey | '';
  requester: string;
  /** 归因专项 agent_key；空串 = 该源无法归因（业务线按 — 处理，不摊派） */
  agent_key: string;
  created_at: string;
};

export type RiskMatrixRowModel = {
  matcher_key: string;
  risk: RiskLevelKey;
  default_mode: string; // never|auto|ask|trusted（与 approval-policy 四档一致）
  auto_approve_times: number;
  updated_by: string;
  updated_at: string;
};

/** 聚合器实际覆盖的四类（task_review/evolution_suggestion/skill_approval/
 *  schedule_draft，W1c 起）；其余三类为 proto 预留，列表恒空。 */
export const APPROVAL_KINDS: ApprovalKindKey[] = [
  'task_review',
  'evolution_suggestion',
  'skill_approval',
  'mask_proposal',
  'offboard_export',
  'schedule_draft',
  'memory_fact',
];

export const RISK_LEVELS: RiskLevelKey[] = ['low', 'medium', 'high'];

export const MATRIX_MODES = ['never', 'auto', 'ask', 'trusted'] as const;

/** 聚合器有写出口的四类；其余为 proto 预留，列表恒空，不可勾选/批量裁决。 */
export const DECIDABLE_KINDS: ApprovalKindKey[] = [
  'task_review',
  'evolution_suggestion',
  'skill_approval',
  'schedule_draft',
];

export function approvalItemKey(item: Pick<ApprovalItemRow, 'kind' | 'id'>): string {
  return `${item.kind}:${item.id}`;
}

/**
 * 该条审批是否支持某动作。archive 仅 task_review（ReviewTask 三动作契约）；
 * 其余可裁决类支持 approve / reject。
 */
export function canDecideApproval(kind: ApprovalKindKey, action: DecideAction = 'approve'): boolean {
  if (!DECIDABLE_KINDS.includes(kind)) return false;
  if (action === 'archive') return kind === 'task_review';
  return true;
}

export function emptyMatrixRow(): RiskMatrixRowModel {
  return {
    matcher_key: '',
    risk: 'medium',
    default_mode: 'ask',
    auto_approve_times: 0,
    updated_by: '',
    updated_at: '',
  };
}
