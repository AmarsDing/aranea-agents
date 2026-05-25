/**
 * Memory：`createMemoryService()` → 网关 **`/v1/...`**。
 * 服务端 **`memory/v1`** 由 **`cmd/admin`** 内 SQLite（`internal/data/sessionmemory`）实现；查询参数与响应字段按 proto **snake_case** 与网关 JSON 对齐。
 */
export type {
  AgentIdentity,
  AgentSkillStat,
  AgentStrategyProfile,
  EvolutionEvent,
  EvolutionMetricsReport,
  EvolutionProposal,
  GraphNeighborhood,
  L0AssemblySegment,
  L0AssemblySnapshot,
  L1Field,
  L1Task,
  MemoryEntity,
  MemoryFact,
  MemoryFactListQuery,
  MemoryFactListResult,
  MemoryRelation,
  CascadeProposal,
  MemoryRecallHit,
  CompositeSearchHit,
  MemoryWorkerStatus,
  MemoryPlatformSettings
} from "./types";

export { memoryEndpoints } from "./memoryEndpoints";

import { createMemoryService } from "../../services/index";
import type { AppendEvolutionEventRequest, MemoryFact as PbMemoryFact } from "../../services/kratos/memory/v1/index";
import type {
  AgentIdentity,
  AgentSkillStat,
  AgentStrategyProfile,
  EvolutionEvent,
  EvolutionMetricsReport,
  EvolutionProposal,
  GraphNeighborhood,
  L0AssemblySnapshot,
  L1Field,
  L1Task,
  MemoryEntity,
  MemoryFact,
  MemoryFactListQuery,
  MemoryFactListResult,
  MemoryRelation,
  CascadeProposal,
  MemoryRecallHit,
  CompositeSearchHit,
  MemoryWorkerStatus,
  MemoryPlatformSettings
} from "./types";
import { asRecord, mapStringFloat, pickBool, pickI32, pickNum, pickOptionalI32, pickStr, pickStrArray } from "../../shared/wireJson";
import { kratosApi } from "../../services/axiosHandler";
import { memoryEndpoints } from "./memoryEndpoints";

const memory = createMemoryService();

function mapL0(raw: unknown): L0AssemblySnapshot {
  const s = asRecord(raw);
  return {
    id: pickStr(s, "id", "id"),
    session_id: pickStr(s, "session_id", "sessionId"),
    run_id: pickStr(s, "run_id", "runId"),
    turn_id: pickStr(s, "turn_id", "turnId"),
    span_id: pickStr(s, "span_id", "spanId"),
    agent_id: pickStr(s, "agent_id", "agentId"),
    team_id: pickStr(s, "team_id", "teamId"),
    provider: pickStr(s, "provider", "provider"),
    model: pickStr(s, "model", "model"),
    context_window_tokens: pickI32(s, "context_window_tokens", "contextWindowTokens"),
    budget_tokens: pickI32(s, "budget_tokens", "budgetTokens"),
    recent_window_turns: pickI32(s, "recent_window_turns", "recentWindowTurns"),
    recent_window_tokens: pickI32(s, "recent_window_tokens", "recentWindowTokens"),
    summary_token_estimate: pickI32(s, "summary_token_estimate", "summaryTokenEstimate"),
    l1_field_count: pickI32(s, "l1_field_count", "l1FieldCount"),
    l1_token_estimate: pickI32(s, "l1_token_estimate", "l1TokenEstimate"),
    l3_chunk_count: pickI32(s, "l3_chunk_count", "l3ChunkCount"),
    l3_token_estimate: pickI32(s, "l3_token_estimate", "l3TokenEstimate"),
    l4_path_count: pickI32(s, "l4_path_count", "l4PathCount"),
    l4_token_estimate: pickI32(s, "l4_token_estimate", "l4TokenEstimate"),
    prompt_token_estimate: pickI32(s, "prompt_token_estimate", "promptTokenEstimate"),
    prompt_token_actual: pickI32(s, "prompt_token_actual", "promptTokenActual"),
    used_ratio: pickNum(s, "used_ratio", "usedRatio"),
    truncate_strategy: pickStr(s, "truncate_strategy", "truncateStrategy"),
    truncated_message_count: pickI32(s, "truncated_message_count", "truncatedMessageCount"),
    summarized_turn_from: pickI32(s, "summarized_turn_from", "summarizedTurnFrom"),
    summarized_turn_to: pickI32(s, "summarized_turn_to", "summarizedTurnTo"),
    segments_json: pickStr(s, "segments_json", "segmentsJson"),
    warning_codes_json: pickStr(s, "warning_codes_json", "warningCodesJson"),
    metadata_json: pickStr(s, "metadata_json", "metadataJson"),
    created_at: pickStr(s, "created_at", "createdAt")
  };
}

function mapL1Task(raw: unknown): L1Task {
  const t = asRecord(raw);
  return {
    id: pickStr(t, "id", "id"),
    session_id: pickStr(t, "session_id", "sessionId"),
    run_id: pickStr(t, "run_id", "runId"),
    team_id: pickStr(t, "team_id", "teamId"),
    agent_id: pickStr(t, "agent_id", "agentId"),
    task_key: pickStr(t, "task_key", "taskKey"),
    task_title: pickStr(t, "task_title", "taskTitle"),
    task_goal: pickStr(t, "task_goal", "taskGoal"),
    status: pickStr(t, "status", "status"),
    schema_version: pickI32(t, "schema_version", "schemaVersion"),
    budget_tokens: pickI32(t, "budget_tokens", "budgetTokens"),
    used_tokens: pickI32(t, "used_tokens", "usedTokens"),
    parent_task_id: pickStr(t, "parent_task_id", "parentTaskId"),
    shared_with_json: pickStr(t, "shared_with_json", "sharedWithJson") || undefined,
    started_at: pickStr(t, "started_at", "startedAt"),
    ended_at: pickStr(t, "ended_at", "endedAt"),
    archived_at: pickStr(t, "archived_at", "archivedAt"),
    metadata_json: pickStr(t, "metadata_json", "metadataJson"),
    created_at: pickStr(t, "created_at", "createdAt"),
    updated_at: pickStr(t, "updated_at", "updatedAt")
  };
}

function mapL1Field(raw: unknown): L1Field {
  const f = asRecord(raw);
  return {
    id: pickStr(f, "id", "id"),
    task_id: pickStr(f, "task_id", "taskId"),
    session_id: pickStr(f, "session_id", "sessionId"),
    agent_id: pickStr(f, "agent_id", "agentId"),
    field_path: pickStr(f, "field_path", "fieldPath"),
    field_kind: pickStr(f, "field_kind", "fieldKind"),
    visibility: pickStr(f, "visibility", "visibility"),
    pin_to_prompt: pickBool(f, "pin_to_prompt", "pinToPrompt"),
    is_required: pickBool(f, "is_required", "isRequired"),
    value_text: pickStr(f, "value_text", "valueText"),
    value_json: pickStr(f, "value_json", "valueJson"),
    value_ref: pickStr(f, "value_ref", "valueRef"),
    preview: pickStr(f, "preview", "preview"),
    token_estimate: pickI32(f, "token_estimate", "tokenEstimate"),
    source: pickStr(f, "source", "source"),
    source_ref: pickStr(f, "source_ref", "sourceRef"),
    ttl_seconds: pickI32(f, "ttl_seconds", "ttlSeconds"),
    expires_at: pickStr(f, "expires_at", "expiresAt"),
    revision: pickI32(f, "revision", "revision"),
    last_read_at: pickStr(f, "last_read_at", "lastReadAt"),
    read_count: pickI32(f, "read_count", "readCount"),
    metadata_json: pickStr(f, "metadata_json", "metadataJson"),
    created_at: pickStr(f, "created_at", "createdAt"),
    updated_at: pickStr(f, "updated_at", "updatedAt")
  };
}

function mapFact(raw: unknown): MemoryFact {
  const f = asRecord(raw);
  return {
    id: pickStr(f, "id", "id"),
    scope_type: pickStr(f, "scope_type", "scopeType"),
    scope_id: pickStr(f, "scope_id", "scopeId"),
    workspace_id: pickStr(f, "workspace_id", "workspaceId"),
    user_id: pickStr(f, "user_id", "userId"),
    team_id: pickStr(f, "team_id", "teamId"),
    agent_id: pickStr(f, "agent_id", "agentId"),
    statement: pickStr(f, "statement", "statement"),
    details_markdown: pickStr(f, "details_markdown", "detailsMarkdown"),
    fact_kind: pickStr(f, "fact_kind", "factKind"),
    tags_json: pickStr(f, "tags_json", "tagsJson"),
    confidence: pickNum(f, "confidence", "confidence"),
    importance: pickNum(f, "importance", "importance"),
    use_count: pickI32(f, "use_count", "useCount"),
    hit_count: pickI32(f, "hit_count", "hitCount"),
    positive_feedback_count: pickI32(f, "positive_feedback_count", "positiveFeedbackCount"),
    negative_feedback_count: pickI32(f, "negative_feedback_count", "negativeFeedbackCount"),
    conflict_count: pickI32(f, "conflict_count", "conflictCount"),
    source_kind: pickStr(f, "source_kind", "sourceKind"),
    source_episode_id: pickStr(f, "source_episode_id", "sourceEpisodeId"),
    source_session_id: pickStr(f, "source_session_id", "sourceSessionId"),
    source_message_id: pickStr(f, "source_message_id", "sourceMessageId"),
    version: pickI32(f, "version", "version"),
    status: pickStr(f, "status", "status"),
    pii_flag: pickBool(f, "pii_flag", "piiFlag"),
    created_at: pickStr(f, "created_at", "createdAt"),
    updated_at: pickStr(f, "updated_at", "updatedAt")
  };
}

function mapEntity(raw: unknown): MemoryEntity {
  const e = asRecord(raw);
  const aliasesRaw = e.aliases ?? e.Aliases;
  let aliases: string[] | undefined;
  if (Array.isArray(aliasesRaw)) {
    aliases = aliasesRaw.filter((x): x is string => typeof x === "string");
  }
  return {
    id: pickStr(e, "id", "id"),
    scope_type: pickStr(e, "scope_type", "scopeType"),
    scope_id: pickStr(e, "scope_id", "scopeId"),
    workspace_id: pickStr(e, "workspace_id", "workspaceId") || undefined,
    user_id: pickStr(e, "user_id", "userId") || undefined,
    entity_type: pickStr(e, "entity_type", "entityType"),
    name: pickStr(e, "name", "name"),
    name_normalized: pickStr(e, "name_normalized", "nameNormalized") || undefined,
    aliases,
    description: pickStr(e, "description", "description") || undefined,
    importance: pickNum(e, "importance", "importance"),
    confidence: pickNum(e, "confidence", "confidence"),
    use_count: pickI32(e, "use_count", "useCount"),
    source_kind: pickStr(e, "source_kind", "sourceKind"),
    status: pickStr(e, "status", "status"),
    created_at: pickStr(e, "created_at", "createdAt") || undefined,
    updated_at: pickStr(e, "updated_at", "updatedAt") || undefined
  };
}

function mapRelation(raw: unknown): MemoryRelation {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    source_id: pickStr(r, "source_id", "sourceId"),
    target_id: pickStr(r, "target_id", "targetId"),
    relation_type: pickStr(r, "relation_type", "relationType"),
    weight: pickNum(r, "weight", "weight"),
    confidence: pickNum(r, "confidence", "confidence"),
    status: pickStr(r, "status", "status")
  };
}

function mapGraphNH(raw: unknown): GraphNeighborhood {
  const g = asRecord(raw);
  const centerRaw = g.center;
  const entitiesRaw = g.entities ?? g.Entities;
  const relationsRaw = g.relations ?? g.Relations;
  return {
    center: centerRaw ? mapEntity(centerRaw) : ({} as MemoryEntity),
    hops: pickI32(g, "hops", "hops"),
    entities: Array.isArray(entitiesRaw) ? entitiesRaw.map(mapEntity) : [],
    relations: Array.isArray(relationsRaw) ? relationsRaw.map(mapRelation) : []
  };
}

function mapAgentIdentity(raw: unknown): AgentIdentity {
  const a = asRecord(raw);
  return {
    agent_id: pickStr(a, "agent_id", "agentId"),
    persona: pickStr(a, "persona", "persona"),
    values: pickStrArray(a, "values", "values"),
    tone: pickStr(a, "tone", "tone"),
    domains: pickStrArray(a, "domains", "domains"),
    user_expectations: pickStr(a, "user_expectations", "userExpectations"),
    current_phase: pickStr(a, "current_phase", "currentPhase"),
    version: pickI32(a, "version", "version")
  };
}

function mapAgentStrategy(raw: unknown): AgentStrategyProfile {
  const a = asRecord(raw);
  const toolPref = a.tool_preference ?? a.toolPreference;
  const provPref = a.provider_preference ?? a.providerPreference;
  const modelPref = a.model_preference ?? a.modelPreference;
  return {
    agent_id: pickStr(a, "agent_id", "agentId"),
    exploration: pickNum(a, "exploration", "exploration"),
    conciseness: pickNum(a, "conciseness", "conciseness"),
    caution: pickNum(a, "caution", "caution"),
    delegation: pickNum(a, "delegation", "delegation"),
    tool_preference: mapStringFloat(toolPref),
    tool_blacklist: pickStrArray(a, "tool_blacklist", "toolBlacklist"),
    provider_preference: mapStringFloat(provPref),
    model_preference: mapStringFloat(modelPref),
    version: pickI32(a, "version", "version")
  };
}

function mapEvolutionProposal(raw: unknown): EvolutionProposal {
  const p = asRecord(raw);
  return {
    id: pickStr(p, "id", "id"),
    agent_id: pickStr(p, "agent_id", "agentId"),
    proposal_kind: pickStr(p, "proposal_kind", "proposalKind") || undefined,
    kind: pickStr(p, "kind", "kind") || undefined,
    target_field: pickStr(p, "target_field", "targetField"),
    rationale: pickStr(p, "rationale", "rationale"),
    expected_impact: pickStr(p, "expected_impact", "expectedImpact"),
    risk_level: pickStr(p, "risk_level", "riskLevel"),
    status: pickStr(p, "status", "status"),
    created_at: pickStr(p, "created_at", "createdAt")
  };
}

function mapEvolutionEvent(raw: unknown): EvolutionEvent {
  const e = asRecord(raw);
  return {
    id: pickStr(e, "id", "id"),
    agent_id: pickStr(e, "agent_id", "agentId"),
    event_kind: pickStr(e, "event_kind", "eventKind") || undefined,
    kind: pickStr(e, "kind", "kind") || undefined,
    target_field: pickStr(e, "target_field", "targetField"),
    reason: pickStr(e, "reason", "reason"),
    reverted: pickBool(e, "reverted", "reverted"),
    created_at: pickStr(e, "created_at", "createdAt")
  };
}

function mapSkillStat(raw: unknown): AgentSkillStat {
  const s = asRecord(raw);
  return {
    agent_id: pickStr(s, "agent_id", "agentId"),
    tool_key: pickStr(s, "tool_key", "toolKey"),
    invocations: pickI32(s, "invocations", "invocations"),
    successes: pickI32(s, "successes", "successes"),
    failures: pickI32(s, "failures", "failures"),
    preference_score: pickNum(s, "preference_score", "preferenceScore"),
    last_used_at: pickStr(s, "last_used_at", "lastUsedAt")
  };
}

function mapEvolutionMetrics(raw: unknown): EvolutionMetricsReport {
  const m = asRecord(raw);
  const byRaw = m.proposals_by_status ?? m.proposalsByStatus;
  const out: Record<string, number> = {};
  if (byRaw && typeof byRaw === "object" && !Array.isArray(byRaw)) {
    for (const [k, v] of Object.entries(byRaw as Record<string, unknown>)) {
      out[k] = typeof v === "number" ? v : Number(v) || 0;
    }
  }
  const statsRaw = m.skill_stats ?? m.skillStats;
  const skill_stats: AgentSkillStat[] = Array.isArray(statsRaw) ? statsRaw.map(mapSkillStat) : [];
  return {
    events_total: pickI32(m, "events_total", "eventsTotal"),
    events_reverted: pickI32(m, "events_reverted", "eventsReverted"),
    proposals_total: pickI32(m, "proposals_total", "proposalsTotal"),
    proposals_by_status: out,
    skill_stats
  };
}

export async function listL0Snapshots(sessionID: string, limit = 20): Promise<L0AssemblySnapshot[]> {
  const res = asRecord(await memory.ListL0Snapshots({ sessionId: sessionID, limit }));
  const items = res.items ?? res.Items;
  return Array.isArray(items) ? items.map(mapL0) : [];
}

export async function listL1Tasks(
  sessionID: string,
  params: { agent_id?: string; status?: string; include_ended?: boolean } = {}
): Promise<L1Task[]> {
  const res = asRecord(
    await memory.ListL1Tasks({
      sessionId: sessionID,
      agentId: params.agent_id,
      status: params.status,
      includeEnded: params.include_ended === undefined ? undefined : String(params.include_ended)
    })
  );
  const items = res.items ?? res.Items;
  return Array.isArray(items) ? items.map(mapL1Task) : [];
}

export async function listL1Fields(sessionID: string, taskID: string, includeInternal = true): Promise<L1Field[]> {
  const res = asRecord(
    await memory.ListL1Fields({
      sessionId: sessionID,
      taskId: taskID,
      includeInternal: includeInternal ? "true" : "false"
    })
  );
  const items = res.items ?? res.Items;
  return Array.isArray(items) ? items.map(mapL1Field) : [];
}

export async function listMemoryFacts(query: MemoryFactListQuery = {}): Promise<MemoryFactListResult> {
  const res = asRecord(
    await memory.ListMemoryFacts({
      scopeType: query.scope_type,
      scopeId: query.scope_id,
      kind: query.kind,
      status: query.status,
      keyword: query.keyword,
      limit: query.limit,
      offset: query.offset
    })
  );
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapFact) : [];
  const total = pickOptionalI32(res, "total", "total") ?? items.length;
  const limit = pickOptionalI32(res, "limit", "limit") ?? query.limit ?? items.length;
  const offset = pickOptionalI32(res, "offset", "offset") ?? query.offset ?? 0;
  return { items, total, limit, offset };
}

export async function listMemoryEntities(
  query: Record<string, string | number | undefined> = {}
): Promise<{ items: MemoryEntity[]; total: number }> {
  const res = asRecord(
    await memory.ListMemoryEntities({
      scopeType: query.scope_type as string | undefined,
      scopeId: query.scope_id as string | undefined,
      workspaceId: query.workspace_id as string | undefined,
      userId: query.user_id as string | undefined,
      entityType: query.entity_type as string | undefined,
      status: query.status as string | undefined,
      keyword: query.keyword as string | undefined,
      limit: query.limit as number | undefined,
      offset: query.offset as number | undefined
    })
  );
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapEntity) : [];
  return { items, total: pickOptionalI32(res, "total", "total") ?? items.length };
}

export async function getMemoryNeighborhood(
  centerID: string,
  params: { hops?: number; max_nodes?: number } = {}
): Promise<GraphNeighborhood> {
  const raw = await memory.GetMemoryNeighborhood({
    centerId: centerID,
    hops: params.hops,
    maxNodes: params.max_nodes,
    queryAt: undefined
  });
  return mapGraphNH(raw);
}

export async function getAgentIdentity(agentID: string): Promise<AgentIdentity> {
  const raw = await memory.GetAgentIdentity({ agentId: agentID });
  return mapAgentIdentity(raw);
}

export async function getAgentStrategy(agentID: string): Promise<AgentStrategyProfile> {
  const raw = await memory.GetAgentStrategy({ agentId: agentID });
  return mapAgentStrategy(raw);
}

export async function listEvolutionProposals(
  agentID: string,
  params: { status?: string; limit?: number } = {}
): Promise<EvolutionProposal[]> {
  const res = asRecord(await memory.ListEvolutionProposals({ agentId: agentID, status: params.status, limit: params.limit }));
  const items = res.items ?? res.Items;
  return Array.isArray(items) ? items.map(mapEvolutionProposal) : [];
}

export async function listEvolutionEvents(agentID: string, params: { limit?: number } = {}): Promise<EvolutionEvent[]> {
  const res = asRecord(await memory.ListEvolutionEvents({ agentId: agentID, limit: params.limit }));
  const items = res.items ?? res.Items;
  return Array.isArray(items) ? items.map(mapEvolutionEvent) : [];
}

export async function getEvolutionMetrics(agentID: string): Promise<EvolutionMetricsReport> {
  const raw = await memory.GetEvolutionMetrics({ agentId: agentID, range: "30d" });
  return mapEvolutionMetrics(raw);
}

export async function upsertMemoryFact(fact: Partial<PbMemoryFact>): Promise<MemoryFact> {
  const res = asRecord(await memory.UpsertMemoryFact({ fact: fact as PbMemoryFact }));
  const raw = res.fact ?? res.Fact;
  return mapFact(raw);
}

export async function appendEvolutionEvent(req: AppendEvolutionEventRequest): Promise<EvolutionEvent> {
  const res = asRecord(await memory.AppendEvolutionEvent(req));
  const raw = res.event ?? res.Event;
  return mapEvolutionEvent(raw);
}

function mapCascadeAffected(raw: unknown) {
  const a = asRecord(raw);
  return {
    entity_id: pickStr(a, "entity_id", "entityId"),
    entity_name: pickStr(a, "entity_name", "entityName"),
    entity_type: pickStr(a, "entity_type", "entityType"),
    relation_type: pickStr(a, "relation_type", "relationType"),
    hops: pickI32(a, "hops", "hops")
  };
}

function mapCascadeProposal(raw: unknown): CascadeProposal {
  const p = asRecord(raw);
  const affectedRaw = p.affected_entities ?? p.affectedEntities;
  const affected = Array.isArray(affectedRaw) ? affectedRaw.map(mapCascadeAffected) : [];
  return {
    id: pickStr(p, "id", "id"),
    agent_id: pickStr(p, "agent_id", "agentId"),
    trigger_entity_id: pickStr(p, "trigger_entity_id", "triggerEntityId"),
    trigger_entity_name: pickStr(p, "trigger_entity_name", "triggerEntityName"),
    trigger_attribute: pickStr(p, "trigger_attribute", "triggerAttribute"),
    old_value: pickStr(p, "old_value", "oldValue"),
    new_value: pickStr(p, "new_value", "newValue"),
    affected_entities: affected,
    status: pickStr(p, "status", "status"),
    risk_level: pickStr(p, "risk_level", "riskLevel"),
    rationale: pickStr(p, "rationale", "rationale"),
    created_at: pickStr(p, "created_at", "createdAt"),
    updated_at: pickStr(p, "updated_at", "updatedAt")
  };
}

export async function listCascadeProposals(
  agentID: string,
  params: { status?: string; limit?: number } = {}
): Promise<CascadeProposal[]> {
  const res = await kratosApi.get(memoryEndpoints.listCascadeProposals(agentID), {
    params: { status: params.status ?? "pending", limit: params.limit ?? 20 }
  });
  const body = asRecord(res.data);
  const items = body.items ?? body.Items;
  return Array.isArray(items) ? items.map(mapCascadeProposal) : [];
}

export async function approveCascadeProposal(id: string, reviewer = "admin"): Promise<CascadeProposal> {
  const res = await kratosApi.post(memoryEndpoints.approveCascadeProposal(id), { reviewer });
  const body = asRecord(res.data);
  return mapCascadeProposal(body.proposal ?? body.Proposal);
}

export async function rejectCascadeProposal(id: string, reviewer = "admin", reason = ""): Promise<CascadeProposal> {
  const res = await kratosApi.post(memoryEndpoints.rejectCascadeProposal(id), { reviewer, reason });
  const body = asRecord(res.data);
  return mapCascadeProposal(body.proposal ?? body.Proposal);
}

function mapRecallScores(raw: unknown) {
  const s = asRecord(raw);
  return {
    keyword: pickNum(s, "keyword", "keyword"),
    vector: pickNum(s, "vector", "vector"),
    importance: pickNum(s, "importance", "importance"),
    recency: pickNum(s, "recency", "recency"),
    cross_encoder: pickNum(s, "cross_encoder", "crossEncoder"),
    total: pickNum(s, "total", "total")
  };
}

function mapRecallHit(raw: unknown): MemoryRecallHit {
  const h = asRecord(raw);
  return {
    layer: pickStr(h, "layer", "layer"),
    id: pickStr(h, "id", "id"),
    title: pickStr(h, "title", "title") || undefined,
    summary: pickStr(h, "summary", "summary") || undefined,
    statement: pickStr(h, "statement", "statement") || undefined,
    scores: mapRecallScores(h.scores ?? h.Scores)
  };
}

export async function debugMemoryRecall(params: {
  agent_id: string;
  session_id?: string;
  user_id?: string;
  query: string;
  l2_limit?: number;
  l3_limit?: number;
}): Promise<{ l2_hits: MemoryRecallHit[]; l3_hits: MemoryRecallHit[] }> {
  const res = asRecord(
    await memory.DebugMemoryRecall({
      agentId: params.agent_id,
      sessionId: params.session_id,
      userId: params.user_id,
      query: params.query,
      l2Limit: params.l2_limit,
      l3Limit: params.l3_limit
    })
  );
  const l2Raw = res.l2_hits ?? res.l2Hits;
  const l3Raw = res.l3_hits ?? res.l3Hits;
  return {
    l2_hits: Array.isArray(l2Raw) ? l2Raw.map(mapRecallHit) : [],
    l3_hits: Array.isArray(l3Raw) ? l3Raw.map(mapRecallHit) : []
  };
}

function mapCompositeHit(raw: unknown): CompositeSearchHit {
  const h = asRecord(raw);
  return {
    layer: pickStr(h, "layer", "layer"),
    id: pickStr(h, "id", "id"),
    text: pickStr(h, "text", "text"),
    score: pickNum(h, "score", "score")
  };
}

export async function compositeSearchMemories(params: {
  agent_id: string;
  session_id?: string;
  user_id?: string;
  query: string;
  limit?: number;
}): Promise<CompositeSearchHit[]> {
  const res = asRecord(
    await memory.CompositeSearchMemories({
      agentId: params.agent_id,
      sessionId: params.session_id,
      userId: params.user_id,
      query: params.query,
      limit: params.limit
    })
  );
  const items = res.items ?? res.Items;
  return Array.isArray(items) ? items.map(mapCompositeHit) : [];
}

export async function getMemoryWorkerStatus(): Promise<MemoryWorkerStatus> {
  const raw = asRecord(await memory.GetMemoryWorkerStatus({}));
  return {
    jobs_done: pickI32(raw, "jobs_done", "jobsDone"),
    jobs_dead: pickI32(raw, "jobs_dead", "jobsDead"),
    llm_fallback_total: pickI32(raw, "llm_fallback_total", "llmFallbackTotal"),
    avg_extraction_seconds: pickNum(raw, "avg_extraction_seconds", "avgExtractionSeconds"),
    episode_backfill_total: pickI32(raw, "episode_backfill_total", "episodeBackfillTotal")
  };
}

function mapMemoryPlatformSettings(raw: unknown): MemoryPlatformSettings {
  const s = asRecord(raw);
  return {
    policy_strict: pickBool(s, "policy_strict", "policyStrict"),
    episode_backfill_disabled: pickBool(s, "episode_backfill_disabled", "episodeBackfillDisabled"),
    env_policy_strict_override: pickBool(s, "env_policy_strict_override", "envPolicyStrictOverride"),
    env_episode_backfill_disabled_override: pickBool(s, "env_episode_backfill_disabled_override", "envEpisodeBackfillDisabledOverride")
  };
}

export async function getMemoryPlatformSettings(): Promise<MemoryPlatformSettings> {
  const raw = await memory.GetMemoryPlatformSettings({});
  return mapMemoryPlatformSettings(raw);
}

export async function updateMemoryPlatformSettings(input: {
  policy_strict: boolean;
  episode_backfill_disabled: boolean;
}): Promise<MemoryPlatformSettings> {
  const raw = await memory.UpdateMemoryPlatformSettings({
    policyStrict: input.policy_strict,
    episodeBackfillDisabled: input.episode_backfill_disabled
  });
  return mapMemoryPlatformSettings(raw);
}
