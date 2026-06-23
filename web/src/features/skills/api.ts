import {
  createSkillService,
  createSkillIntelligenceService,
  createSkillEvolutionSuggestionService,
  createSkillEvolutionService,
  kratosApi,
} from '../../services';
import axios from 'axios';
import type {
  PaginatedResponse,
  Skill,
  SkillFilesystemHealth,
  SkillFile,
  SkillFileContent,
  SkillHealthMetric,
  SkillImportApplyResult,
  SkillImportDecision,
  SkillImportJob,
  SkillInvocation,
  SkillListQuery,
  SkillRefineResult,
  SkillRunQuery,
  SkillTag,
  ExperienceReportView,
  ExperienceReportListResult,
  EvolutionSuggestionView,
  SkillEvolutionView,
  EvolutionActionType,
} from './types';

// ZIP / 冲突消解：`kratosApi` **`/v1/skills/import*`** 由 **`cmd/admin`** 内挂载（multipart + JSON）。
// 管理列表、启停、文件编辑、运行记录等已接 Kratos `skill/v1`。

function mapSkillTag(raw: unknown): SkillTag {
  const o = raw as Record<string, unknown>;
  return {
    name: String(o.name ?? ''),
    source: (String(o.source ?? 'user') || 'user') as SkillTag['source'],
  };
}

function mapSkill(row: unknown): Skill {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? '');
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  const b = (snake: string, camel: string) => Boolean(r[snake] ?? r[camel]);
  const rawPerms = r.permissions as Record<string, unknown> | undefined;
  const p = rawPerms ?? {};
  const pb = (snake: string, camel: string) => Boolean(p[snake] ?? p[camel] ?? false);
  const rawTags = r.tags;
  const tags: SkillTag[] = Array.isArray(rawTags) ? rawTags.map(mapSkillTag) : [];
  const cvRaw = r.current_version ?? r.currentVersion;
  const current_version =
    cvRaw && typeof cvRaw === 'object'
      ? (() => {
          const c = cvRaw as Record<string, unknown>;
          return {
            id: String(c.id ?? ''),
            version: String(c.version ?? ''),
            validation_status: String(c.validation_status ?? c.validationStatus ?? 'pass') as NonNullable<
              Skill['current_version']
            >['validation_status'],
            published_at: String(c.published_at ?? c.publishedAt ?? ''),
          };
        })()
      : null;
  const rawAvg = r.avg_duration_ms ?? r.avgDurationMs;
  const avg_duration_ms = rawAvg === undefined || rawAvg === null ? null : Number(rawAvg);
  const rawLastDur = r.last_duration_ms ?? r.lastDurationMs;
  const last_duration_ms = rawLastDur === undefined || rawLastDur === null ? null : Number(rawLastDur);
  return {
    id: s('id', 'id'),
    name: s('name', 'name'),
    slug: s('slug', 'slug'),
    description: s('description', 'description'),
    tags,
    extends_skill_id: s('extends_skill_id', 'extendsSkillId') || undefined,
    status: (s('status', 'status') || 'draft') as Skill['status'],
    enabled: b('enabled', 'enabled'),
    current_version,
    invoke_count: n('invoke_count', 'invokeCount'),
    success_count: n('success_count', 'successCount'),
    failure_count: n('failure_count', 'failureCount'),
    usage_count_7d: n('usage_count_7d', 'usageCount7d') || 0,
    avg_duration_ms,
    last_agent_id: s('last_agent_id', 'lastAgentId') || undefined,
    last_agent_display_name: s('last_agent_display_name', 'lastAgentDisplayName') || undefined,
    last_invoked_at: s('last_invoked_at', 'lastInvokedAt') || undefined,
    last_duration_ms,
    created_at: s('created_at', 'createdAt'),
    updated_at: s('updated_at', 'updatedAt'),
    permissions: {
      can_edit: pb('can_edit', 'canEdit'),
      can_delete: pb('can_delete', 'canDelete'),
      can_toggle_enabled: pb('can_toggle_enabled', 'canToggleEnabled'),
      can_duplicate: pb('can_duplicate', 'canDuplicate'),
    },
    filesystem_missing: b('filesystem_missing', 'filesystemMissing'),
    sync_origin: s('sync_origin', 'syncOrigin') || undefined,
  };
}

function mapSkillFile(row: unknown): SkillFile {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? '');
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  return {
    path: s('path', 'path'),
    name: s('name', 'name'),
    language: s('language', 'language'),
    size: n('size', 'size'),
    updated_at: s('updated_at', 'updatedAt'),
  };
}

function mapSkillFileContent(row: unknown): SkillFileContent {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? '');
  return {
    path: s('path', 'path'),
    content: s('content', 'content'),
    language: s('language', 'language'),
  };
}

function mapSkillInvocation(row: unknown): SkillInvocation {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? '');
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  const rawPerms = r.permissions as Record<string, unknown> | undefined;
  const p = rawPerms ?? {};
  const pb = (snake: string, camel: string) => Boolean(p[snake] ?? p[camel] ?? false);
  return {
    id: s('id', 'id'),
    skill_id: s('skill_id', 'skillId'),
    skill_name: s('skill_name', 'skillName'),
    skill_version: s('skill_version', 'skillVersion'),
    agent_id: s('agent_id', 'agentId'),
    agent_display_name: s('agent_display_name', 'agentDisplayName'),
    user_id: s('user_id', 'userId') || undefined,
    session_id: s('session_id', 'sessionId') || undefined,
    status: (s('status', 'status') || 'pending') as SkillInvocation['status'],
    duration_ms: n('duration_ms', 'durationMs'),
    started_at: s('started_at', 'startedAt'),
    ended_at: s('ended_at', 'endedAt') || undefined,
    input_preview: s('input_preview', 'inputPreview') || undefined,
    input_hash: s('input_hash', 'inputHash') || undefined,
    output_preview: s('output_preview', 'outputPreview') || undefined,
    error_code: s('error_code', 'errorCode') || undefined,
    error_message: s('error_message', 'errorMessage') || undefined,
    permissions: {
      can_view_detail: pb('can_view_detail', 'canViewDetail'),
    },
  };
}

export async function listSkills(query: SkillListQuery = {}): Promise<PaginatedResponse<Skill>> {
  const svc = createSkillService();
  let enabled: string | undefined;
  if (query.enabled === true) enabled = 'true';
  else if (query.enabled === false) enabled = 'false';
  let filesystemMissing: string | undefined;
  if (query.filesystem_missing === true) filesystemMissing = 'true';
  else if (query.filesystem_missing === false) filesystemMissing = 'false';
  const page = query.page ?? 1;
  const pageSize = query.page_size ?? 20;
  const res = await svc.ListSkills({
    search: query.search?.trim() || undefined,
    tags: query.tags?.length ? query.tags.join(',') : undefined,
    enabled,
    status: query.status?.trim() || undefined,
    filesystemMissing,
    syncOrigin: query.sync_origin?.trim() || undefined,
    page,
    pageSize,
  });
  return {
    items: (res.items ?? []).map(mapSkill),
    total: Number(res.total ?? 0),
    page: Number(res.page ?? page),
    page_size: Number(res.pageSize ?? pageSize),
  };
}

export async function getSkillFilesystemHealth(): Promise<SkillFilesystemHealth> {
  const { data } = await kratosApi.get('/v1/skills/filesystem-health');
  const d = data as Record<string, unknown>;
  return {
    root_accessible: Boolean(d.root_accessible ?? d.rootAccessible ?? true),
    resolved_root: String(d.resolved_root ?? d.resolvedRoot ?? ''),
    missing_count: Number(d.missing_count ?? d.missingCount ?? 0),
    pending_filesystem_count: Number(d.pending_filesystem_count ?? d.pendingFilesystemCount ?? 0),
  };
}

export async function toggleSkillEnabled(id: string, enabled: boolean): Promise<Skill> {
  const row = await createSkillService().ToggleSkillEnabled({ id, enabled });
  return mapSkill(row);
}

/** POST /v1/skills/{id}/publish — 草稿 → 已发布。 A-04 fix: use generated service client. */
export async function publishSkill(id: string): Promise<Skill> {
  const row = await createSkillService().PublishSkill({ id });
  return mapSkill(row);
}

export async function duplicateSkill(id: string): Promise<Skill> {
  const row = await createSkillService().DuplicateSkill({ id });
  return mapSkill(row);
}

export async function deleteSkill(id: string): Promise<void> {
  await createSkillService().DeleteSkill({ id });
}

export async function listSkillFiles(id: string): Promise<SkillFile[]> {
  const res = await createSkillService().ListSkillFiles({ id });
  return (res.items ?? []).map(mapSkillFile);
}

export async function readSkillFile(id: string, path: string): Promise<SkillFileContent> {
  const row = await createSkillService().GetSkillFile({ id, path });
  return mapSkillFileContent(row);
}

export async function updateSkillFile(id: string, path: string, content: string): Promise<SkillFileContent> {
  const row = await createSkillService().UpdateSkillFile({ id, path, content });
  return mapSkillFileContent(row);
}

export async function listSkillRuns(query: SkillRunQuery = {}): Promise<PaginatedResponse<SkillInvocation>> {
  const svc = createSkillService();
  const page = query.page ?? 1;
  const pageSize = query.page_size ?? 20;
  const res = await svc.ListSkillRuns({
    skillId: query.skill_id?.trim() || undefined,
    agentId: query.agent_id?.trim() || undefined,
    sessionId: undefined,
    status: query.status?.trim() || undefined,
    from: query.from?.trim() || undefined,
    to: query.to?.trim() || undefined,
    page,
    pageSize,
  });
  return {
    items: (res.items ?? []).map(mapSkillInvocation),
    total: Number(res.total ?? 0),
    page: Number(res.page ?? page),
    page_size: Number(res.pageSize ?? pageSize),
  };
}

export async function uploadSkillZip(file: File): Promise<{ job_id: string }> {
  const form = new FormData();
  form.append('file', file);
  const { data } = await kratosApi.post('/v1/skills/import', form);
  const d = data as Record<string, unknown>;
  return { job_id: String(d.job_id ?? d.jobId ?? '') };
}

export async function getSkillImportJob(jobId: string): Promise<SkillImportJob> {
  const { data } = await kratosApi.get(`/v1/skills/import/${jobId}`);
  return data as SkillImportJob;
}

export async function refineSkillConflictGroup(
  jobId: string,
  groupId: string,
  payload: { provider?: string; model?: string; instructions?: string },
): Promise<SkillRefineResult> {
  const { data } = await kratosApi.post(`/v1/skills/import/${jobId}/conflict-groups/${groupId}/refine`, payload);
  return data as SkillRefineResult;
}

export async function applySkillImport(
  jobId: string,
  decisions: SkillImportDecision[],
): Promise<SkillImportApplyResult> {
  const { data } = await kratosApi.post(`/v1/skills/import/${jobId}/apply`, { decisions });
  return data as SkillImportApplyResult;
}

export async function getSkill(id: string): Promise<{ skill: Skill; bodyMarkdown: string }> {
  const res = await createSkillService().GetSkill({ id });
  const r = res as Record<string, unknown>;
  const rawSkill = r.skill as Record<string, unknown> | undefined;
  const skill = rawSkill ? mapSkill(rawSkill) : null!;
  const bodyMarkdown = String(r.bodyMarkdown ?? r.body_markdown ?? '');
  return { skill, bodyMarkdown };
}

export async function getSkillHealth(skillId: string): Promise<SkillHealthMetric> {
  const row = await createSkillService().GetSkillHealth({ skillId });
  const r = row as Record<string, unknown>;
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  const rawDaily = r.daily_metrics ?? r.dailyMetrics;
  const dailyMetrics: SkillHealthMetric['daily_metrics'] = Array.isArray(rawDaily)
    ? rawDaily.map((d: unknown) => {
        const dm = d as Record<string, unknown>;
        return {
          date: String(dm.date ?? ''),
          invocations: Number(dm.invocations ?? 0),
          successes: Number(dm.successes ?? 0),
          avg_duration_ms: Number(dm.avg_duration_ms ?? dm.avgDurationMs ?? 0),
          routed_count: Number(dm.routed_count ?? dm.routedCount ?? 0),
          loaded_count: Number(dm.loaded_count ?? dm.loadedCount ?? 0),
        };
      })
    : [];
  return {
    skill_id: String(r.skill_id ?? r.skillId ?? ''),
    total_invocations_7d: n('total_invocations_7d', 'totalInvocations7d'),
    success_count_7d: n('success_count_7d', 'successCount7d'),
    success_rate_7d: n('success_rate_7d', 'successRate7d'),
    p95_duration_ms_7d: n('p95_duration_ms_7d', 'p95DurationMs7d'),
    total_invocations_30d: n('total_invocations_30d', 'totalInvocations30d'),
    success_count_30d: n('success_count_30d', 'successCount30d'),
    success_rate_30d: n('success_rate_30d', 'successRate30d'),
    p95_duration_ms_30d: n('p95_duration_ms_30d', 'p95DurationMs30d'),
    route_hit_rate_7d: n('route_hit_rate_7d', 'routeHitRate7d'),
    route_hit_rate_30d: n('route_hit_rate_30d', 'routeHitRate30d'),
    daily_metrics: dailyMetrics,
  };
}

// ── Experience Report (Skill Intelligence) ──────────────────────

function mapExperienceReport(raw: unknown): ExperienceReportView {
  const r = raw as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? '');
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  const b = (snake: string, camel: string) => Boolean(r[snake] ?? r[camel]);
  const rawTags = r.failure_tags ?? r.failureTags;
  const failureTags: string[] = Array.isArray(rawTags) ? rawTags.map(String) : [];
  const rawSnapshot = r.selection_snapshot ?? r.selectionSnapshot;
  const selectionSnapshot: Record<string, unknown> =
    rawSnapshot && typeof rawSnapshot === 'object' && !Array.isArray(rawSnapshot)
      ? (rawSnapshot as Record<string, unknown>)
      : {};
  return {
    id: s('id', 'id'),
    tenantId: s('tenant_id', 'tenantId'),
    sessionId: s('session_id', 'sessionId'),
    invocationId: s('invocation_id', 'invocationId'),
    skillId: s('skill_id', 'skillId'),
    skillName: s('skill_name', 'skillName'),
    isSuccess: b('is_success', 'isSuccess'),
    score: n('score', 'score'),
    failureTags,
    flowSummary: s('flow_summary', 'flowSummary'),
    rootCauseAnalysis: s('root_cause_analysis', 'rootCauseAnalysis'),
    suggestedFix: s('suggested_fix', 'suggestedFix'),
    optimizationAdvice: s('optimization_advice', 'optimizationAdvice'),
    selectionSnapshot,
    generatedSuggestionId: s('generated_suggestion_id', 'generatedSuggestionId'),
    createdAt: s('created_at', 'createdAt'),
  };
}

function mapFailureTagCount(raw: unknown): { tag: string; count: number } {
  const r = raw as Record<string, unknown>;
  return {
    tag: String(r.tag ?? ''),
    count: Number(r.count ?? 0),
  };
}

export async function listExperienceReports(params: {
  skillId?: string;
  startTime?: string;
  endTime?: string;
  page?: number;
  pageSize?: number;
}): Promise<ExperienceReportListResult> {
  const svc = createSkillIntelligenceService();
  const res = await svc.ListExperienceReports({
    skillId: params.skillId || undefined,
    startTime: params.startTime || undefined,
    endTime: params.endTime || undefined,
    page: params.page,
    pageSize: params.pageSize,
  });
  return {
    items: (res.items ?? []).map(mapExperienceReport),
    total: Number(res.total ?? 0),
    page: Number(res.page ?? params.page ?? 1),
    page_size: Number(res.pageSize ?? params.pageSize ?? 20),
    failureTagCounts: (res.failureTagCounts ?? []).map(mapFailureTagCount),
    rootCauseReports: (res.rootCauseReports ?? []).map(mapExperienceReport),
  };
}

// ── Evolution Suggestion ────────────────────────────────────────

function mapEvolutionSuggestion(raw: unknown): EvolutionSuggestionView {
  const r = raw as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? '');
  const rawReportIds = r.source_report_ids ?? r.sourceReportIds;
  const sourceReportIds: string[] = Array.isArray(rawReportIds) ? rawReportIds.map(String) : [];
  const toStruct = (snake: string, camel: string): Record<string, unknown> => {
    const v = r[snake] ?? r[camel];
    return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
  };
  // Proto bool defaults to false; use lifecycleStatus to distinguish
  // "not yet validated" from "validation failed".
  const lifecycleStatus = String(r['lifecycle_status'] ?? r['lifecycleStatus'] ?? '');
  const rawSandboxPassed = r['sandbox_passed'] ?? r['sandboxPassed'];
  const sandboxPassed: boolean | null = (() => {
    if (rawSandboxPassed === true) return true;
    if (
      rawSandboxPassed === false &&
      (lifecycleStatus === 'validating' || lifecycleStatus === 'ready' || lifecycleStatus === 'applied')
    )
      return false;
    return null; // not yet validated
  })();
  return {
    id: s('id', 'id'),
    skillId: s('skill_id', 'skillId'),
    type: s('type', 'type'),
    status: s('status', 'status'),
    triggerReason: s('trigger_reason', 'triggerReason'),
    sourceReportIds,
    draftSkillBody: s('draft_skill_body', 'draftSkillBody'),
    sandboxPassed,
    sandboxResult: toStruct('sandbox_result', 'sandboxResult'),
    preVerifyResult: toStruct('pre_verify_result', 'preVerifyResult'),
    parentVersionId: s('parent_version_id', 'parentVersionId'),
    draftVersionId: s('draft_version_id', 'draftVersionId'),
    evolutionReason: s('evolution_reason', 'evolutionReason'),
    lifecycleStatus,
    approvedBy: s('approved_by', 'approvedBy'),
    rejectedBy: s('rejected_by', 'rejectedBy'),
    rejectionReason: s('rejection_reason', 'rejectionReason'),
    resolvedAt: s('resolved_at', 'resolvedAt'),
    createdAt: s('created_at', 'createdAt'),
  };
}

export async function listEvolutionSuggestions(params: {
  skillId?: string;
  status?: string;
  page?: number;
  pageSize?: number;
}): Promise<PaginatedResponse<EvolutionSuggestionView>> {
  const svc = createSkillEvolutionSuggestionService();
  const res = await svc.ListSkillEvolutionSuggestions({
    skillId: params.skillId || undefined,
    status: params.status || undefined,
    page: params.page,
    pageSize: params.pageSize,
  });
  return {
    items: (res.items ?? []).map(mapEvolutionSuggestion),
    total: Number(res.total ?? 0),
    page: Number(res.page ?? params.page ?? 1),
    page_size: Number(res.pageSize ?? params.pageSize ?? 20),
  };
}

export async function approveEvolutionSuggestion(id: string, approvedBy: string): Promise<void> {
  const svc = createSkillEvolutionSuggestionService();
  await svc.ApproveSkillEvolutionSuggestion({ id, approvedBy });
}

export async function rejectEvolutionSuggestion(
  id: string,
  rejectedBy: string,
  rejectionReason?: string,
): Promise<void> {
  const svc = createSkillEvolutionSuggestionService();
  await svc.RejectSkillEvolutionSuggestion({
    id,
    rejectedBy,
    rejectionReason: rejectionReason || undefined,
  });
}

export async function triggerCuratorFlow(skillId: string): Promise<EvolutionSuggestionView | null> {
  const svc = createSkillEvolutionSuggestionService();
  const res = await svc.TriggerCuratorFlow({ skillId });
  if (res.suggestion) {
    return mapEvolutionSuggestion(res.suggestion);
  }
  return null;
}

// ── A-02 fix: add missing Skill RPC wrappers ──────────────────────

function toSkillTags(tags?: string[]): SkillTag[] | undefined {
  if (!tags || tags.length === 0) return undefined;
  return tags.map((name) => ({ name, source: 'user' }));
}

export async function createSkill(payload: {
  name: string;
  description?: string;
  slug?: string;
  tags?: string[];
  bodyMarkdown?: string;
}): Promise<Skill> {
  const row = await createSkillService().CreateSkill({
    name: payload.name,
    description: payload.description || undefined,
    slug: payload.slug || undefined,
    bodyMarkdown: payload.bodyMarkdown || undefined,
    tags: toSkillTags(payload.tags),
  });
  return mapSkill(row);
}

export async function updateSkill(
  id: string,
  payload: { name?: string; description?: string; tags?: string[]; bodyMarkdown?: string },
): Promise<Skill> {
  const row = await createSkillService().UpdateSkill({
    id,
    name: payload.name || undefined,
    description: payload.description || undefined,
    bodyMarkdown: payload.bodyMarkdown || undefined,
    replaceTags: true,
    tags: toSkillTags(payload.tags),
  });
  return mapSkill(row);
}

export async function deleteSkillFile(id: string, path: string): Promise<void> {
  await createSkillService().DeleteSkillFile({ id, path });
}

export type SkillRuntimePreview = {
  resolved_storage_root: string;
  enabled_published_count: number;
  enabled_skill_slugs: string[];
  reasons: Record<string, string>;
};

export async function previewSkillRuntime(id: string): Promise<SkillRuntimePreview> {
  const res = await createSkillService().PreviewSkillRuntime({ agentId: id, userQuery: undefined });
  const r = res as Record<string, unknown>;
  return {
    resolved_storage_root: String(r.resolvedStorageRoot ?? r.resolved_storage_root ?? ''),
    enabled_published_count: Number(r.enabledPublishedCount ?? r.enabled_published_count ?? 0),
    enabled_skill_slugs: Array.isArray(r.enabledSkillSlugs ?? r.enabled_skill_slugs)
      ? ((r.enabledSkillSlugs ?? r.enabled_skill_slugs) as string[])
      : [],
    reasons: (r.reasons ?? {}) as Record<string, string>,
  };
}

export async function getSkillVersions(id: string, page = 1, pageSize = 20): Promise<PaginatedResponse<unknown>> {
  const res = await createSkillService().GetSkillVersions({ skillId: id, page, pageSize });
  const r = res as Record<string, unknown>;
  const items = (r.items ?? []) as unknown[];
  return {
    items,
    total: Number(r.total ?? 0),
    page: Number(r.page ?? page),
    page_size: Number(r.pageSize ?? pageSize),
  };
}

export async function rollbackSkillVersion(id: string, versionId: string): Promise<Skill> {
  const row = await createSkillService().RollbackSkillVersion({ skillId: id, versionId });
  return mapSkill(row);
}

// ── Unified Evolution API ──

/** Check if an error is a 404 NOT_FOUND from Kratos (gRPC code 5 or HTTP 404). */
function isNotFoundError(err: unknown): boolean {
  if (axios.isAxiosError(err)) {
    return err.response?.status === 404;
  }
  if (err && typeof err === 'object') {
    const e = err as Record<string, unknown>;
    // Kratos gRPC-gateway error: code 5 = NOT_FOUND
    if (e.code === 5 || e.code === '5') return true;
    if (typeof e.message === 'string' && e.message.includes('NOT_FOUND')) return true;
  }
  return false;
}

export async function listUnifiedEvolutionSuggestions(params: {
  targetType?: string;
  targetId?: string;
  status?: string;
  page?: number;
  pageSize?: number;
}): Promise<{ items: SkillEvolutionView[]; total: number; skillTotal: number; agentTotal: number }> {
  const items: SkillEvolutionView[] = [];
  let skillTotal = 0;
  let agentTotal = 0;

  // When targetType is specified, only request that data source — pagination is correct.
  // When targetType is unspecified, both sources are queried with the same page params;
  // the combined total is approximate (sum of both totals) and pagination may be
  // inconsistent across the merge boundary. Consumers should use skillTotal / agentTotal
  // for display and prefer specifying targetType when accurate pagination is needed.
  if (!params.targetType || params.targetType === 'skill') {
    const client = createSkillEvolutionSuggestionService();
    const skillRes = await client.ListSkillEvolutionSuggestions({
      skillId: params.targetId || undefined,
      status: params.status || undefined,
      page: params.page,
      pageSize: params.pageSize,
    });
    for (const item of skillRes.items || []) {
      items.push(mapProtoEvolutionSuggestionToView(item));
    }
    skillTotal = Number(skillRes.total || 0);
  }

  // Fetch from SkillEvolutionService (agent-level)
  if (!params.targetType || params.targetType === 'agent') {
    const evoClient = createSkillEvolutionService();
    const agentRes = await evoClient.ListSkillProposals({
      agentId: params.targetId || undefined,
      status: params.status || undefined,
      page: params.page,
      pageSize: params.pageSize,
    });
    for (const item of agentRes.items || []) {
      items.push(mapProtoSkillProposalToView(item));
    }
    agentTotal = Number(agentRes.total || (agentRes.items || []).length);
  }

  return { items, total: skillTotal + agentTotal, skillTotal, agentTotal };
}

export async function approveUnifiedEvolutionSuggestion(id: string, approvedBy: string): Promise<void> {
  // Try skill-level service first, then agent-level — but only fall through on 404 (NOT_FOUND).
  // Real errors (500, 403, etc.) must propagate to the caller.
  try {
    const client = createSkillEvolutionSuggestionService();
    await client.ApproveSkillEvolutionSuggestion({ id, approvedBy });
    return;
  } catch (err) {
    if (!isNotFoundError(err)) throw err;
    // Fall through to agent-level
  }
  const evoClient = createSkillEvolutionService();
  await evoClient.ApproveSkillProposal({ id, approvedBy });
}

export async function rejectUnifiedEvolutionSuggestion(id: string, rejectedBy: string, reason: string): Promise<void> {
  // Try skill-level service first, then agent-level — but only fall through on 404 (NOT_FOUND).
  // Real errors (500, 403, etc.) must propagate to the caller.
  try {
    const client = createSkillEvolutionSuggestionService();
    await client.RejectSkillEvolutionSuggestion({ id, rejectedBy, rejectionReason: reason });
    return;
  } catch (err) {
    if (!isNotFoundError(err)) throw err;
    // Fall through to agent-level
  }
  const evoClient = createSkillEvolutionService();
  await evoClient.RejectSkillProposal({ id, rejectedBy });
}

export async function registerUnifiedEvolutionSuggestion(id: string): Promise<void> {
  const evoClient = createSkillEvolutionService();
  await evoClient.RegisterSkillProposal({ id });
}

// ── Proto-to-View mapping helpers ──

function mapProtoEvolutionSuggestionToView(item: Record<string, unknown>): SkillEvolutionView {
  const s = (snake: string, camel: string) => String(item[snake] ?? item[camel] ?? '');
  const rawSandboxPassed = item['sandbox_passed'] ?? item['sandboxPassed'];
  const sandboxPassed: boolean = rawSandboxPassed === true;
  const rawSandboxResult = item['sandbox_result'] ?? item['sandboxResult'];
  const sandboxResult: Record<string, unknown> | null =
    rawSandboxResult && typeof rawSandboxResult === 'object' && !Array.isArray(rawSandboxResult)
      ? (rawSandboxResult as Record<string, unknown>)
      : null;
  const rawMetadata = item['metadata'] ?? item['Metadata'];
  const metadata: Record<string, unknown> | null =
    rawMetadata && typeof rawMetadata === 'object' && !Array.isArray(rawMetadata)
      ? (rawMetadata as Record<string, unknown>)
      : null;
  return {
    id: s('id', 'id'),
    targetType: 'skill',
    targetId: s('skill_id', 'skillId'),
    targetName: '', // TODO: backend SkillEvolutionSuggestionMsg lacks skill_name; frontend should resolve via skill list lookup
    actionType: mapSuggestionTypeToAction(s('type', 'type')),
    triggerSource: 'health',
    triggerReason: s('trigger_reason', 'triggerReason'),
    status: (s('status', 'status') || 'pending') as SkillEvolutionView['status'],
    priority: s('type', 'type') === 'fix_failure' ? 2 : 1,
    draftBody: s('draft_skill_body', 'draftSkillBody'),
    draftName: '',
    mergeTargetId: '',
    lifecycleStatus: (s('lifecycle_status', 'lifecycleStatus') || 'draft') as SkillEvolutionView['lifecycleStatus'],
    sandboxPassed,
    sandboxResult,
    metadata,
    createdAt: s('created_at', 'createdAt'),
    approvedBy: s('approved_by', 'approvedBy'),
    appliedAt: null,
  };
}

function mapProtoSkillProposalToView(item: Record<string, unknown>): SkillEvolutionView {
  const s = (snake: string, camel: string) => String(item[snake] ?? item[camel] ?? '');
  return {
    id: s('id', 'id'),
    targetType: 'agent',
    targetId: s('agent_id', 'agentId'),
    targetName: s('agent_id', 'agentId'),
    actionType: 'create_skill',
    triggerSource: 'pattern', // TODO: backend SkillProposal lacks trigger_source; default to 'pattern' until field is added
    triggerReason: s('pattern_desc', 'patternDesc'),
    status: (s('status', 'status') || 'pending') as SkillEvolutionView['status'],
    priority: 1,
    draftBody: s('skill_md', 'skillMd'),
    draftName: s('skill_name', 'skillName'),
    mergeTargetId: '',
    lifecycleStatus: 'draft',
    sandboxPassed: false,
    sandboxResult: null,
    metadata: null,
    createdAt: s('created_at', 'createdAt'),
    approvedBy: s('approved_by', 'approvedBy'),
    appliedAt: null,
  };
}

function mapSuggestionTypeToAction(suggestionType: string): EvolutionActionType {
  switch (suggestionType) {
    case 'fix_failure':
      return 'improve_skill';
    case 'boost_efficiency':
      return 'improve_skill';
    case 'merge_duplicate':
      return 'merge_skill';
    default:
      return 'improve_skill';
  }
}
