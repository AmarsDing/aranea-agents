import { defineStore } from 'pinia';
import { ref } from 'vue';
import { listTeams, createTeam, updateTeam, duplicateTeam, deleteTeam } from '../../features/teams/api';
import type { Team } from '../../features/teams/types';

export const useTeamsStore = defineStore('teams', () => {
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

  async function fetchTeam(id: string) {
    let team = teams.value.find((t) => t.id === id) ?? null;
    if (!team) {
      await loadTeams();
      team = teams.value.find((t) => t.id === id) ?? null;
    }
    if (!team) {
      throw new Error('Team not found');
    }
    activeTeam.value = team;
    return team;
  }

  function setActiveTeam(t: Team | null) {
    activeTeam.value = t;
  }

  return { teams, activeTeam, loading, loadTeams, fetchTeam, addTeam, editTeam, copy, remove, setActiveTeam };
});
