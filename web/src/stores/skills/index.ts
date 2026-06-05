import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listSkills,
  listSkillRuns,
  toggleSkillEnabled,
  publishSkill,
  duplicateSkill,
  deleteSkill,
  listSkillFiles,
  readSkillFile,
  updateSkillFile,
  uploadSkillZip,
  getSkillImportJob,
  refineSkillConflictGroup,
  applySkillImport,
  getSkillFilesystemHealth,
  getSkill,
  getSkillHealth,
} from '../../features/skills/api';
import type {
  Skill,
  SkillListQuery,
  SkillRunQuery,
  SkillFilesystemHealth,
  SkillHealthMetric,
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
    listSkillFiles,
    readSkillFile,
    updateSkillFile,
    uploadSkillZip,
    getSkillImportJob,
    refineSkillConflictGroup,
    applySkillImport,
  };
});
