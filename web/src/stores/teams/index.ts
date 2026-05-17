import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listTeams,
  createTeam,
  updateTeam,
  duplicateTeam,
  deleteTeam,
  type Team
} from "../../features/teams/api";

export const useTeamsStore = defineStore("teams", () => {
  const teams = ref<Team[]>([]);
  const activeTeam = ref<Team | null>(null);
  const loading = ref(false);

  async function loadTeams() {
    loading.value = true;
    try {
      teams.value = await listTeams();
    } finally {
      loading.value = false;
    }
  }

  async function addTeam(payload: Partial<Team>) {
    const created = await createTeam(payload);
    teams.value.unshift(created);
    activeTeam.value = created;
    return created;
  }

  async function editTeam(id: string, payload: Partial<Team>) {
    const updated = await updateTeam(id, payload);
    teams.value = teams.value.map((t) => (t.id === id ? updated : t));
    if (activeTeam.value?.id === id) activeTeam.value = updated;
    return updated;
  }

  async function copy(id: string) {
    const copy = await duplicateTeam(id);
    teams.value.push(copy);
    return copy;
  }

  async function remove(id: string) {
    await deleteTeam(id);
    teams.value = teams.value.filter((t) => t.id !== id);
    if (activeTeam.value?.id === id) activeTeam.value = null;
  }

  function setActiveTeam(t: Team | null) {
    activeTeam.value = t;
  }

  return { teams, activeTeam, loading, loadTeams, addTeam, editTeam, copy, remove, setActiveTeam };
});
