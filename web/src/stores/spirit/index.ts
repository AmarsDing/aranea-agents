import { defineStore } from "pinia";
import { ref, computed } from "vue";
import { listSpiritTeams } from "../../features/spirit/api";
import type { SpiritTeam, SpiritPanelMode } from "../../features/spirit/types";

export const useSpiritTeamStore = defineStore("spiritTeam", () => {
  const teams = ref<SpiritTeam[]>([]);
  const expandedTeamIds = ref<Set<string>>(new Set());
  const activePanelMode = ref<SpiritPanelMode>("spirit");
  const activeTeamId = ref<string | null>(null);
  const activeMemberId = ref<string | null>(null);
  const loading = ref(false);

  const activeTeam = computed(() =>
    teams.value.find((t) => t.id === activeTeamId.value) ?? null
  );

  const activeTeams = computed(() =>
    teams.value.filter((t) => t.status !== "completed")
  );

  const completedTeams = computed(() =>
    teams.value.filter((t) => t.status === "completed")
  );

  async function loadSpiritTeams(spiritSessionId: string) {
    loading.value = true;
    try {
      teams.value = await listSpiritTeams(spiritSessionId);
    } finally {
      loading.value = false;
    }
  }

  function selectTeam(teamId: string) {
    activeTeamId.value = teamId;
    activePanelMode.value = "team";
    activeMemberId.value = null;
  }

  function selectMember(memberId: string) {
    activeMemberId.value = memberId;
    activePanelMode.value = "member";
  }

  function returnToSpirit() {
    activePanelMode.value = "spirit";
    activeTeamId.value = null;
    activeMemberId.value = null;
  }

  function toggleTeamExpand(teamId: string) {
    const next = new Set(expandedTeamIds.value);
    if (next.has(teamId)) {
      next.delete(teamId);
    } else {
      next.add(teamId);
    }
    expandedTeamIds.value = next;
  }

  async function archiveTeam(teamId: string) {
    teams.value = teams.value.filter((t) => t.id !== teamId);
    if (activeTeamId.value === teamId) {
      returnToSpirit();
    }
  }

  return {
    teams,
    expandedTeamIds,
    activePanelMode,
    activeTeamId,
    activeMemberId,
    loading,
    activeTeam,
    activeTeams,
    completedTeams,
    loadSpiritTeams,
    selectTeam,
    selectMember,
    returnToSpirit,
    toggleTeamExpand,
    archiveTeam,
  };
});
