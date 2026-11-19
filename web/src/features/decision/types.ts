/**
 * 决策记录（decision/v1 ListDecisionRecords）前端类型。
 * 概览页注意力队列（HITL 待审批）消费；字段只保留展示所需子集。
 */

export type DecisionEntityRef = {
  entity_type: string;
  entity_key: string;
};

export type DecisionRecordItem = {
  decision_key: string;
  category: string;
  scenario: string;
  reasoning: string;
  outcome: string;
  actor_type: string;
  actor_key: string;
  created_at: string;
  session_id?: string;
  task_id?: string;
  step_id?: string;
  /** 来源引用（HITL 记录携带 session_id + tool_invocation_id） */
  source_ref?: { session_id?: string; tool_invocation_id?: string };
  /** 记录附加元数据（HITL 的 correlation_id 是 asked/decided 配对键） */
  metadata?: Record<string, unknown>;
  /** 关联实体（归因 agent 用） */
  related_entities?: DecisionEntityRef[];
};

export type DecisionRecordPage = {
  items: DecisionRecordItem[];
  total: number;
};

export type DecisionRecordQuery = {
  category?: string;
  /** 时间窗起点（RFC3339）；服务端按 created_at ≥ time_from 过滤。 */
  time_from?: string;
  /** 时间窗终点（RFC3339）。 */
  time_to?: string;
  /** 按 metadata.correlation_id 批量过滤（HITL 配对兜底，绕开时间窗/分页上限）。 */
  correlation_ids?: string[];
  /** pagination：1-based page；pageSize ≤100（服务端收敛）。 */
  page?: number;
  pageSize?: number;
};
