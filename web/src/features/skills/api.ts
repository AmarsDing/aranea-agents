import { createSkillService, kratosApi } from "../../services";
import type {
  PaginatedResponse,
  Skill,
  SkillFile,
  SkillFileContent,
  SkillImportApplyResult,
  SkillImportDecision,
  SkillImportJob,
  SkillInvocation,
  SkillListQuery,
  SkillRefineResult,
  SkillRunQuery,
  SkillTag
} from "./types";

// ZIP / 冲突消解：`kratosApi` **`/v1/skills/import*`** 由 **`cmd/admin`** 内挂载（multipart + JSON），不经 **`LEGACY_REST_ORIGIN`**。
// 管理列表、启停、文件编辑、运行记录等已接 Kratos `skill/v1`。

function mapSkillTag(raw: unknown): SkillTag {
  const o = raw as Record<string, unknown>;
  return {
    name: String(o.name ?? ""),
    source: (String(o.source ?? "user") || "user") as SkillTag["source"]
  };
}

function mapSkill(row: unknown): Skill {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? "");
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  const b = (snake: string, camel: string) => Boolean(r[snake] ?? r[camel]);
  const rawPerms = r.permissions as Record<string, unknown> | undefined;
  const p = rawPerms ?? {};
  const pb = (snake: string, camel: string) => Boolean(p[snake] ?? p[camel] ?? false);
  const rawTags = r.tags;
  const tags: SkillTag[] = Array.isArray(rawTags) ? rawTags.map(mapSkillTag) : [];
  const cvRaw = r.current_version ?? r.currentVersion;
  const current_version =
    cvRaw && typeof cvRaw === "object"
      ? (() => {
          const c = cvRaw as Record<string, unknown>;
          return {
            id: String(c.id ?? ""),
            version: String(c.version ?? ""),
            validation_status: String(
              c.validation_status ?? c.validationStatus ?? "pass"
            ) as NonNullable<Skill["current_version"]>["validation_status"],
            published_at: String(c.published_at ?? c.publishedAt ?? "")
          };
        })()
      : null;
  const rawAvg = r.avg_duration_ms ?? r.avgDurationMs;
  const avg_duration_ms =
    rawAvg === undefined || rawAvg === null ? null : Number(rawAvg);
  const rawLastDur = r.last_duration_ms ?? r.lastDurationMs;
  const last_duration_ms =
    rawLastDur === undefined || rawLastDur === null ? null : Number(rawLastDur);
  return {
    id: s("id", "id"),
    name: s("name", "name"),
    slug: s("slug", "slug"),
    description: s("description", "description"),
    tags,
    extends_skill_id: s("extends_skill_id", "extendsSkillId") || undefined,
    status: (s("status", "status") || "draft") as Skill["status"],
    enabled: b("enabled", "enabled"),
    current_version,
    invoke_count: n("invoke_count", "invokeCount"),
    success_count: n("success_count", "successCount"),
    failure_count: n("failure_count", "failureCount"),
    usage_count_7d: n("usage_count_7d", "usageCount7d") || undefined,
    avg_duration_ms,
    last_agent_id: s("last_agent_id", "lastAgentId") || undefined,
    last_agent_display_name: s("last_agent_display_name", "lastAgentDisplayName") || undefined,
    last_invoked_at: s("last_invoked_at", "lastInvokedAt") || undefined,
    last_duration_ms,
    created_at: s("created_at", "createdAt"),
    updated_at: s("updated_at", "updatedAt"),
    permissions: {
      can_edit: pb("can_edit", "canEdit"),
      can_delete: pb("can_delete", "canDelete"),
      can_toggle_enabled: pb("can_toggle_enabled", "canToggleEnabled"),
      can_duplicate: pb("can_duplicate", "canDuplicate")
    }
  };
}

function mapSkillFile(row: unknown): SkillFile {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? "");
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  return {
    path: s("path", "path"),
    name: s("name", "name"),
    language: s("language", "language"),
    size: n("size", "size"),
    updated_at: s("updated_at", "updatedAt")
  };
}

function mapSkillFileContent(row: unknown): SkillFileContent {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? "");
  return {
    path: s("path", "path"),
    content: s("content", "content"),
    language: s("language", "language")
  };
}

function mapSkillInvocation(row: unknown): SkillInvocation {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? "");
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  const rawPerms = r.permissions as Record<string, unknown> | undefined;
  const p = rawPerms ?? {};
  const pb = (snake: string, camel: string) => Boolean(p[snake] ?? p[camel] ?? false);
  return {
    id: s("id", "id"),
    skill_id: s("skill_id", "skillId"),
    skill_name: s("skill_name", "skillName"),
    skill_version: s("skill_version", "skillVersion"),
    agent_id: s("agent_id", "agentId"),
    agent_display_name: s("agent_display_name", "agentDisplayName"),
    user_id: s("user_id", "userId") || undefined,
    session_id: s("session_id", "sessionId") || undefined,
    status: (s("status", "status") || "pending") as SkillInvocation["status"],
    duration_ms: n("duration_ms", "durationMs"),
    started_at: s("started_at", "startedAt"),
    ended_at: s("ended_at", "endedAt") || undefined,
    input_preview: s("input_preview", "inputPreview") || undefined,
    input_hash: s("input_hash", "inputHash") || undefined,
    output_preview: s("output_preview", "outputPreview") || undefined,
    error_code: s("error_code", "errorCode") || undefined,
    error_message: s("error_message", "errorMessage") || undefined,
    permissions: {
      can_view_detail: pb("can_view_detail", "canViewDetail")
    }
  };
}

export async function listSkills(query: SkillListQuery = {}): Promise<PaginatedResponse<Skill>> {
  const svc = createSkillService();
  let enabled: string | undefined;
  if (query.enabled === true) enabled = "true";
  else if (query.enabled === false) enabled = "false";
  const page = query.page ?? 1;
  const pageSize = query.page_size ?? 20;
  const res = await svc.ListSkills({
    search: query.search?.trim() || undefined,
    tags: query.tags?.length ? query.tags.join(",") : undefined,
    enabled,
    status: query.status?.trim() || undefined,
    page,
    pageSize
  });
  return {
    items: (res.items ?? []).map(mapSkill),
    total: Number(res.total ?? 0),
    page: Number(res.page ?? page),
    page_size: Number(res.pageSize ?? pageSize)
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
    pageSize
  });
  return {
    items: (res.items ?? []).map(mapSkillInvocation),
    total: Number(res.total ?? 0),
    page: Number(res.page ?? page),
    page_size: Number(res.pageSize ?? pageSize)
  };
}

export async function uploadSkillZip(file: File): Promise<{ job_id: string }> {
  const form = new FormData();
  form.append("file", file);
  const { data } = await kratosApi.post("/v1/skills/import", form);
  const d = data as Record<string, unknown>;
  return { job_id: String(d.job_id ?? d.jobId ?? "") };
}

export async function getSkillImportJob(jobId: string): Promise<SkillImportJob> {
  const { data } = await kratosApi.get(`/v1/skills/import/${jobId}`);
  return data as SkillImportJob;
}

export async function refineSkillConflictGroup(
  jobId: string,
  groupId: string,
  payload: { provider?: string; model?: string; instructions?: string }
): Promise<SkillRefineResult> {
  const { data } = await kratosApi.post(
    `/v1/skills/import/${jobId}/conflict-groups/${groupId}/refine`,
    payload
  );
  return data as SkillRefineResult;
}

export async function applySkillImport(
  jobId: string,
  decisions: SkillImportDecision[]
): Promise<SkillImportApplyResult> {
  const { data } = await kratosApi.post(`/v1/skills/import/${jobId}/apply`, { decisions });
  return data as SkillImportApplyResult;
}
