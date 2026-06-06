import {
  createSkillService,
  createSkillIntelligenceService,
  createSkillEvolutionSuggestionService,
  kratosApi,
} from '../../services';
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

/** POST /v1/skills/{id}/publish — 草稿 → 已发布（与 codegen 无关，避免客户端未同步时缺方法）。 */
export async function publishSkill(id: string): Promise<Skill> {
  const { data } = await kratosApi.post(`/v1/skills/${encodeURIComponent(id)}/publish`, {});
  return mapSkill(data);
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
