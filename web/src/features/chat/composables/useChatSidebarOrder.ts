import type { Ref } from "vue";
import type { TeamRow } from "../../../components/chat/types";
import type { Agent } from "../../agents/types";
import { LS_AG_ORDER, LS_TM_ORDER, loadAgentOrder, loadTeamOrder } from "./chatWorkspaceUtils";

export function useChatSidebarOrder(
  displayAgents: Ref<Agent[]>,
  displayTeams: Ref<TeamRow[]>,
  defaultAgentId: Ref<string | null>,
  defaultTeamId: Ref<string>
) {
  function onEndAgent() {
    if (defaultAgentId.value) {
      const current = displayAgents.value;
      if (current[0] && current[0].id !== defaultAgentId.value) {
        const fixed = current.find((agent) => agent.id === defaultAgentId.value);
        if (fixed) displayAgents.value = [fixed, ...current.filter((agent) => agent.id !== fixed.id)];
      }
    }
    try {
      localStorage.setItem(LS_AG_ORDER, JSON.stringify(displayAgents.value.map((agent) => agent.id)));
    } catch {
      /* ignore */
    }
  }

  function onEndTeam() {
    const current = displayTeams.value;
    if (current[0] && current[0].id !== defaultTeamId.value) {
      const fixed = current.find((team) => team.id === defaultTeamId.value);
      if (fixed) displayTeams.value = [fixed, ...current.filter((team) => team.id !== fixed.id)];
    }
    try {
      localStorage.setItem(LS_TM_ORDER, JSON.stringify(displayTeams.value.map((team) => team.id)));
    } catch {
      /* ignore */
    }
  }

  return {
    loadAgentOrder,
    loadTeamOrder,
    onEndAgent,
    onEndTeam,
  };
}
