import { defineStore } from "pinia";
import { ref } from "vue";
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
  applySkillImport
} from "../../features/skills/api";
import type { Skill, SkillListQuery, SkillRunQuery, PaginatedResponse } from "../../features/skills/types";

export const useSkillsStore = defineStore("skills", () => {
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
    listSkillFiles,
    readSkillFile,
    updateSkillFile,
    uploadSkillZip,
    getSkillImportJob,
    refineSkillConflictGroup,
    applySkillImport
  };
});
