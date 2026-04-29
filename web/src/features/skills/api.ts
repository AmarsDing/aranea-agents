import { api } from "../../api/http";
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
  SkillRunQuery
} from "./types";

function compactParams(params: Record<string, unknown>) {
  return Object.fromEntries(
    Object.entries(params).filter(([, value]) => value !== undefined && value !== null && value !== "")
  );
}

export async function listSkills(query: SkillListQuery = {}): Promise<PaginatedResponse<Skill>> {
  const { data } = await api.get("/skills", {
    params: compactParams({
      search: query.search,
      tags: query.tags?.join(","),
      enabled: query.enabled,
      status: query.status,
      page: query.page ?? 1,
      page_size: query.page_size ?? 20
    })
  });
  return data;
}

export async function toggleSkillEnabled(id: string, enabled: boolean): Promise<Skill> {
  const { data } = await api.patch(`/skills/${id}/enabled`, { enabled });
  return data;
}

export async function duplicateSkill(id: string): Promise<Skill> {
  const { data } = await api.post(`/skills/${id}/duplicate`);
  return data;
}

export async function deleteSkill(id: string): Promise<void> {
  await api.delete(`/skills/${id}`);
}

export async function listSkillFiles(id: string): Promise<SkillFile[]> {
  const { data } = await api.get(`/skills/${id}/files`);
  return data.items ?? [];
}

export async function readSkillFile(id: string, path: string): Promise<SkillFileContent> {
  const { data } = await api.get(`/skills/${id}/file`, { params: { path } });
  return data;
}

export async function updateSkillFile(id: string, path: string, content: string): Promise<SkillFileContent> {
  const { data } = await api.put(`/skills/${id}/file`, { path, content });
  return data;
}

export async function listSkillRuns(query: SkillRunQuery = {}): Promise<PaginatedResponse<SkillInvocation>> {
  const { data } = await api.get("/skill-runs", {
    params: compactParams({
      skill_id: query.skill_id,
      agent_id: query.agent_id,
      status: query.status,
      from: query.from,
      to: query.to,
      page: query.page ?? 1,
      page_size: query.page_size ?? 20
    })
  });
  return data;
}

export async function uploadSkillZip(file: File): Promise<{ job_id: string }> {
  const form = new FormData();
  form.append("file", file);
  const { data } = await api.post("/skills/import", form, {
    headers: { "Content-Type": "multipart/form-data" }
  });
  return data;
}

export async function getSkillImportJob(jobId: string): Promise<SkillImportJob> {
  const { data } = await api.get(`/skills/import/${jobId}`);
  return data;
}

export async function refineSkillConflictGroup(jobId: string, groupId: string, payload: { provider?: string; model?: string; instructions?: string }): Promise<SkillRefineResult> {
  const { data } = await api.post(`/skills/import/${jobId}/conflict-groups/${groupId}/refine`, payload);
  return data;
}

export async function applySkillImport(jobId: string, decisions: SkillImportDecision[]): Promise<SkillImportApplyResult> {
  const { data } = await api.post(`/skills/import/${jobId}/apply`, { decisions });
  return data;
}
