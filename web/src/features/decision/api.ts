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

function wireOptionalString(row: Record<string, unknown>, camel: string, snake: string): string | undefined {
  const value = row[camel] ?? row[snake];
  return typeof value === 'string' && value ? value : undefined;
}

function wireRecordObject(row: Record<string, unknown>): Record<string, unknown> | undefined {
  const v = row.metadata;
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : undefined;
}

function wireEntities(row: Record<string, unknown>): DecisionRecordItem['related_entities'] {
  const raw = row.relatedEntities ?? row.related_entities;
  if (!Array.isArray(raw)) return undefined;
  return raw
    .filter((e): e is Record<string, unknown> => !!e && typeof e === 'object')
    .map((e) => ({
      // 后端 decision.EntityRef 的 JSON 是 {type,key}（internal/biz/decision/record.go）；
      // camel/snake 形态仅作兼容兜底。
      entity_type: String(e.type ?? e.entityType ?? e.entity_type ?? ''),
      entity_key: String(e.key ?? e.entityKey ?? e.entity_key ?? ''),
    }));
}

function wireSourceRef(row: Record<string, unknown>): DecisionRecordItem['source_ref'] {
  const raw = row.sourceRef ?? row.source_ref;
  if (!raw || typeof raw !== 'object') return undefined;
  const rec = raw as Record<string, unknown>;
  return {
    session_id: wireOptionalString(rec, 'sessionId', 'session_id'),
    tool_invocation_id: wireOptionalString(rec, 'toolInvocationId', 'tool_invocation_id'),
  };
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
    session_id: wireOptionalString(row, 'sessionId', 'session_id'),
    task_id: wireOptionalString(row, 'taskId', 'task_id'),
    step_id: wireOptionalString(row, 'stepId', 'step_id'),
    source_ref: wireSourceRef(row),
    metadata: wireRecordObject(row),
    related_entities: wireEntities(row),
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
    correlationIds: query.correlation_ids?.length ? query.correlation_ids : undefined,
    page: query.page ?? 1,
    pageSize: query.pageSize ?? 50,
  });
  const total = Number(res.total ?? 0);
  return {
    items: (res.items ?? []).map(wireDecision),
    total: Number.isFinite(total) ? total : 0,
  };
}
