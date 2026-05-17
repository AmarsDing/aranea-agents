import { defineStore } from "pinia";
import { ref } from "vue";
import {
  discoverAgents,
  getAgentCard,
  invokeA2A,
  listA2AAudit,
  updateAgentCard
} from "../../features/a2a/api";
import type {
  A2AAgentCard,
  A2AAuditEntry,
  A2AInvokeInput,
  A2AInvokeResult,
  DiscoverParams,
  ListAuditParams,
  ListAuditResult,
  UpdateAgentCardInput
} from "../../features/a2a/types";

export const useA2AStore = defineStore("a2a", () => {
  const agentCards = ref<A2AAgentCard[]>([]);
  const auditLog = ref<A2AAuditEntry[]>([]);
  const auditTotal = ref(0);
  const loading = ref(false);

  async function discover(params: DiscoverParams = {}): Promise<A2AAgentCard[]> {
    loading.value = true;
    try {
      agentCards.value = await discoverAgents(params);
      return agentCards.value;
    } finally {
      loading.value = false;
    }
  }

  async function refreshCard(agentId: string): Promise<A2AAgentCard> {
    const card = await getAgentCard(agentId);
    agentCards.value = agentCards.value.map((c) => (c.agent_id === agentId ? card : c));
    return card;
  }

  async function updateCard(agentId: string, input: UpdateAgentCardInput): Promise<A2AAgentCard> {
    const updated = await updateAgentCard(agentId, input);
    agentCards.value = agentCards.value.map((c) => (c.agent_id === agentId ? updated : c));
    return updated;
  }

  /** 发起 A2A 调用（当前为演示状态，EP-A2A-01）。 */
  async function invoke(input: A2AInvokeInput): Promise<A2AInvokeResult> {
    return invokeA2A(input);
  }

  async function loadAudit(params: ListAuditParams = {}): Promise<ListAuditResult> {
    const result = await listA2AAudit(params);
    auditLog.value = result.items;
    auditTotal.value = result.total;
    return result;
  }

  return {
    agentCards,
    auditLog,
    auditTotal,
    loading,
    discover,
    refreshCard,
    updateCard,
    invoke,
    loadAudit
  };
});
