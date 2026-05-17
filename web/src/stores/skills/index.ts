import { defineStore } from "pinia";
import { ref } from "vue";
import { listSkills, toggleSkillEnabled, publishSkill, duplicateSkill, deleteSkill } from "../../features/skills/api";
import type { Skill, SkillListQuery, PaginatedResponse } from "../../features/skills/types";

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
    } finally {
      loading.value = false;
    }
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

  return { skills, total, loading, loadSkills, toggle, publish, duplicate, remove };
});
