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
  listSkillTags as listSkillTagsApi,
  createSkillTag as createSkillTagApi,
  renameSkillTag as renameSkillTagApi,
  deleteSkillTag as deleteSkillTagApi,
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
  SkillTagInfo,
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

  // ---- 标签字典 ----
  const skillTags = ref<SkillTagInfo[]>([]);
  const tagsLoading = ref(false);
  let tagsLoaded = false;

  /** 选项源：规范标签名（字典 + 使用中），供 q-select options。 */
  function tagNameOptions(): string[] {
    return skillTags.value.map((t) => t.name);
  }

  /** force=false 时仅首次加载（选项源场景）；管理页传 force=true。 */
  async function loadSkillTags(force = false): Promise<SkillTagInfo[]> {
    if (tagsLoaded && !force) return skillTags.value;
    tagsLoading.value = true;
    try {
      skillTags.value = await listSkillTagsApi();
      tagsLoaded = true;
      return skillTags.value;
    } finally {
      tagsLoading.value = false;
    }
  }

  async function createTag(name: string): Promise<SkillTagInfo> {
    const created = await createSkillTagApi(name);
    await loadSkillTags(true);
    return created;
  }

  /** 改名 + 重写引用；返回重写条数。skills 列表已过期，由调用方决定何时 loadSkills。 */
  async function renameTag(oldName: string, newName: string): Promise<number> {
    const rewritten = await renameSkillTagApi(oldName, newName);
    tagsLoaded = false;
    await loadSkillTags(true);
    return rewritten;
  }

  /** 删除 + 移除引用；返回重写条数。 */
  async function deleteTag(name: string): Promise<number> {
    const rewritten = await deleteSkillTagApi(name);
    tagsLoaded = false;
    await loadSkillTags(true);
    return rewritten;
  }

  /** skill tags 被直接编辑后（MetaDialog 保存等），使选项源缓存失效。 */
  function invalidateSkillTags() {
    tagsLoaded = false;
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
    // ---- 标签字典 ----
    skillTags,
    tagsLoading,
    tagNameOptions,
    loadSkillTags,
    createTag,
    renameTag,
    deleteTag,
    invalidateSkillTags,
  };
});
