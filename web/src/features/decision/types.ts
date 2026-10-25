/**
 * 决策记录（decision/v1 ListDecisionRecords）前端类型。
 * 概览页注意力队列（HITL 待审批）消费；字段只保留展示所需子集。
 */

export type DecisionRecordItem = {
  decision_key: string;
  category: string;
  scenario: string;
  reasoning: string;
  outcome: string;
  actor_type: string;
  actor_key: string;
  created_at: string;
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
  /** pagination：1-based page；pageSize ≤100（服务端收敛）。 */
  page?: number;
  pageSize?: number;
};
