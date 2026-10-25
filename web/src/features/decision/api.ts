/**
 * decision/v1 HTTP 门面：只谈请求与类型映射，不持有业务状态。
 * 现仅供概览页注意力队列（HITL 待审批计数）使用。
 */
import { createDecisionService } from '../../services';
import type { DecisionRecordItem, DecisionRecordPage, DecisionRecordQuery } from './types';

function wireString(row: Record<string, unknown>, camel: string, snake: string): string {
  const v = row[camel] ?? row[snake];
  return typeof v === 'string' ? v : '';
}

function wireDecision(row: Record<string, unknown>): DecisionRecordItem {
  return {
    decision_key: wireString(row, 'decisionKey', 'decision_key'),
    category: wireString(row, 'category', 'category'),
    scenario: wireString(row, 'scenario', 'scenario'),
    reasoning: wireString(row, 'reasoning', 'reasoning'),
    outcome: wireString(row, 'outcome', 'outcome'),
    actor_type: wireString(row, 'actorType', 'actor_type'),
    actor_key: wireString(row, 'actorKey', 'actor_key'),
    created_at: wireString(row, 'createdAt', 'created_at'),
  };
}

export async function listDecisionRecords(query: DecisionRecordQuery = {}): Promise<DecisionRecordPage> {
  const svc = createDecisionService();
  const res = await svc.ListDecisionRecords({
    // protogen 请求类型的字段是「必带键、值为 undefined」而非 optional，全部显式给出。
    actorKey: undefined,
    entityType: undefined,
    entityKey: undefined,
    sourceRunId: undefined,
    sourceSessionId: undefined,
    timeFrom: query.time_from?.trim() || undefined,
    timeTo: query.time_to?.trim() || undefined,
    category: query.category?.trim() || undefined,
    page: query.page ?? 1,
    pageSize: query.pageSize ?? 50,
  });
  const total = Number(res.total ?? 0);
  return {
    items: (res.items ?? []).map(wireDecision),
    total: Number.isFinite(total) ? total : 0,
  };
}
