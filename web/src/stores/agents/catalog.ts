import { defineStore } from "pinia";
import { listAgents } from "../../features/agents/api";
import type { Agent, AgentListQuery } from "../../features/agents/types";

/** 轻量 Agent 目录：下拉/筛选等场景拉全量或限量列表。 */
export const useAgentsCatalogStore = defineStore("agentsCatalog", () => {
  async function fetchAgents(query: AgentListQuery = {}): Promise<Agent[]> {
    return listAgents(query);
  }

  return { fetchAgents };
});
