import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listSkills,
  listSkillRuns,
  toggleSkillEnabled,
  publishSkill,
  duplicateSkill,
  deleteSkill,
  uploadSkillZip as uploadSkillZipApi,
  getSkillImportJob,
  refineSkillConflictGroup as refineSkillConflictGroupApi,
  applySkillImport as applySkillImportApi,
  getSkillFilesystemHealth,
  getSkill,
  getSkillHealth,
  updateSkillFile as updateSkillFileApi,
  createSkill as createSkillApi,
  updateSkill as updateSkillApi,
  getSkillVersions as getSkillVersionsApi,
  rollbackSkillVersion as rollbackSkillVersionApi,
} from '../../features/skills/api';
import type {
  Skill,
  SkillListQuery,
  SkillRunQuery,
  SkillFilesystemHealth,
  SkillHealthMetric,
  SkillImportDecision,
  SkillImportApplyResult,
  SkillRefineResult,
  SkillFileContent,
  SkillVersionDetail,
  PaginatedResponse,
} from '../../features/skills/types';

export const useSkillsStore = defineStore('skills', () => {
  const skills = ref<Skill[]>([]);
  const total = ref(0);
  const loading = ref(false);

  async function loadSkills(query?: SkillListQuery) {
    loading.value = true;
    try {
      const result: PaginatedResponse<Skill> = await listSkills(query);
      skills.value = result.items ?? [];
      total.value = result.total ?? skills.value.length;
      return result;
    } finally {
      loading.value = false;
    }
  }

  async function loadSkillRuns(query?: SkillRunQuery) {
    return listSkillRuns(query);
  }

  async function toggle(id: string, enabled: boolean) {
    const updated = await toggleSkillEnabled(id, enabled);
    skills.value = skills.value.map((s) => (s.id === id ? updated : s));
    return updated;
  }

  async function publish(id: string) {
    const updated = await publishSkill(id);
    skills.value = skills.value.map((s) => (s.id === id ? updated : s));
    return updated;
  }

  async function duplicate(id: string) {
    const copy = await duplicateSkill(id);
    skills.value.push(copy);
    return copy;
  }

  async function remove(id: string) {
    await deleteSkill(id);
    skills.value = skills.value.filter((s) => s.id !== id);
  }

  async function loadFilesystemHealth(): Promise<SkillFilesystemHealth> {
    return getSkillFilesystemHealth();
  }

  async function loadSkillHealth(skillId: string): Promise<SkillHealthMetric> {
    return getSkillHealth(skillId);
  }

  async function loadSkill(id: string): Promise<{ skill: Skill; bodyMarkdown: string }> {
    return getSkill(id);
  }

  async function uploadSkillZip(file: File): Promise<{ job_id: string }> {
    const result = await uploadSkillZipApi(file);
    await loadSkills();
    return result;
  }

  async function applySkillImport(jobId: string, decisions: SkillImportDecision[]): Promise<SkillImportApplyResult> {
    const result = await applySkillImportApi(jobId, decisions);
    await loadSkills();
    return result;
  }

  async function refineSkillConflictGroup(
    jobId: string,
    groupId: string,
    payload: { provider?: string; model?: string; instructions?: string },
  ): Promise<SkillRefineResult> {
    return refineSkillConflictGroupApi(jobId, groupId, payload);
  }

  async function updateSkillFile(id: string, path: string, content: string): Promise<SkillFileContent> {
    return updateSkillFileApi(id, path, content);
  }

  async function create(payload: {
    name: string;
    description?: string;
    slug?: string;
    tags?: string[];
    bodyMarkdown?: string;
  }): Promise<Skill> {
    const created = await createSkillApi(payload);
    skills.value = [created, ...skills.value];
    total.value += 1;
    return created;
  }

  async function update(
    id: string,
    payload: { name?: string; description?: string; tags?: string[]; bodyMarkdown?: string },
  ): Promise<Skill> {
    const updated = await updateSkillApi(id, payload);
    skills.value = skills.value.map((s) => (s.id === updated.id ? updated : s));
    return updated;
  }

  async function loadVersions(id: string, page = 1, pageSize = 20): Promise<PaginatedResponse<SkillVersionDetail>> {
    return getSkillVersionsApi(id, page, pageSize);
  }

  async function rollbackVersion(id: string, versionId: string): Promise<Skill> {
    const updated = await rollbackSkillVersionApi(id, versionId);
    skills.value = skills.value.map((s) => (s.id === updated.id ? updated : s));
    return updated;
  }

  return {
    skills,
    total,
    loading,
    loadSkills,
    loadSkillRuns,
    toggle,
    publish,
    duplicate,
    remove,
    loadFilesystemHealth,
    loadSkillHealth,
    loadSkill,
    uploadSkillZip,
    getSkillImportJob,
    refineSkillConflictGroup,
    applySkillImport,
    updateSkillFile,
    create,
    update,
    loadVersions,
    rollbackVersion,
  };
});
