import { defineStore } from "pinia";
import { ref } from "vue";
import {
  discoverAgents,
  discoverRemoteAgent,
  deleteRemoteAgent,
  getA2AConfig,
  getAgentCard,
  invokeA2A,
  listA2AAudit,
  listRemoteAgents,
  registerRemoteAgent,
  updateAgentCard
} from "../../features/a2a/api";
import type {
  A2AAgentCard,
  A2AAuditEntry,
  A2AInvokeInput,
  A2AInvokeResult,
  A2ARemoteAgent,
  A2ARuntimeConfig,
  DiscoverParams,
  DiscoverRemoteInput,
  ListAuditParams,
  ListAuditResult,
  RegisterRemoteAgentInput,
  UpdateAgentCardInput
} from "../../features/a2a/types";

export const useA2AStore = defineStore("a2a", () => {
  const agentCards = ref<A2AAgentCard[]>([]);
  const auditLog = ref<A2AAuditEntry[]>([]);
  const auditTotal = ref(0);
  const remoteAgents = ref<A2ARemoteAgent[]>([]);
  const loading = ref(false);
  const runtimeConfig = ref<A2ARuntimeConfig | null>(null);

  async function loadRuntimeConfig() {
    try {
      runtimeConfig.value = await getA2AConfig();
    } catch {
      runtimeConfig.value = null;
    }
    return runtimeConfig.value;
  }

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

  /** 发起 A2A 调用（EP-A2A-01：经 ChatService 派发目标 Agent）。 */
  async function invoke(input: A2AInvokeInput): Promise<A2AInvokeResult> {
    return invokeA2A(input);
  }

  async function loadAudit(params: ListAuditParams = {}): Promise<ListAuditResult> {
    const result = await listA2AAudit(params);
    auditLog.value = result.items;
    auditTotal.value = result.total;
    return result;
  }

  async function loadRemoteAgents(workspace = ""): Promise<A2ARemoteAgent[]> {
    remoteAgents.value = await listRemoteAgents(workspace);
    return remoteAgents.value;
  }

  async function registerRemote(input: RegisterRemoteAgentInput): Promise<A2ARemoteAgent> {
    const created = await registerRemoteAgent(input);
    remoteAgents.value = [created, ...remoteAgents.value.filter((r) => r.id !== created.id)];
    return created;
  }

  async function removeRemote(id: string): Promise<void> {
    await deleteRemoteAgent(id);
    remoteAgents.value = remoteAgents.value.filter((r) => r.id !== id);
  }

  async function previewRemote(input: DiscoverRemoteInput): Promise<A2AAgentCard> {
    return discoverRemoteAgent(input);
  }

  return {
    agentCards,
    auditLog,
    auditTotal,
    remoteAgents,
    loading,
    runtimeConfig,
    loadRuntimeConfig,
    discover,
    refreshCard,
    updateCard,
    invoke,
    loadAudit,
    loadRemoteAgents,
    registerRemote,
    removeRemote,
    previewRemote
  };
});
