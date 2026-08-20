import { defineStore } from 'pinia';
import { createAgent, deleteAgent, listAgents, updateAgent } from '../features/agents/api';
import type { Agent } from '../features/agents/types';
import { emitSessionMutation, onSessionMutation } from './sessionMutationBus';

export const useAppStore = defineStore('app', {
  state: () => ({
    loading: false,
    agents: [] as Agent[],
    selectedAgent: null as Agent | null,
  }),
  actions: {
    async removeAgentFromList(id: string) {
      await deleteAgent(id);
      this.agents = this.agents.filter((a) => a.id !== id);
      if (this.selectedAgent?.id === id) {
        this.selectedAgent = (this.agents.find((a) => a.agent_key === '__spirit__') || this.agents[0]) ?? null;
        emitSessionMutation({ type: 'agent_removed', agentId: id });
      }
    },
    async updateSelectedAgent(payload: Partial<Agent>) {
      if (!this.selectedAgent) return null;
      const updated = await updateAgent(this.selectedAgent.id, { ...this.selectedAgent, ...payload });
      this.upsertAgent(updated);
      return updated;
    },
    async patchAgent(id: string, patch: Partial<Agent>) {
      const base = this.agents.find((a) => a.id === id) ?? (this.selectedAgent?.id === id ? this.selectedAgent : null);
      if (!base) return null;
      const updated = await updateAgent(id, { ...base, ...patch });
      this.upsertAgent(updated);
      return updated;
    },
    upsertAgent(agent: Agent) {
      const exists = this.agents.some((item) => item.id === agent.id);
      this.agents = exists
        ? this.agents.map((item) => (item.id === agent.id ? { ...item, ...agent } : item))
        : [agent, ...this.agents];
      if (this.selectedAgent?.id === agent.id) {
        this.selectedAgent = { ...this.selectedAgent, ...agent };
      }
    },
    async loadAgents() {
      // High limit so system builtins (精灵助手 etc.) are not pushed off the first page
      // after agency-pack / large catalogs are seeded.
      this.agents = await listAgents({ limit: 500 });
      if (!this.selectedAgent && this.agents.length > 0) {
        this.selectedAgent = this.agents.find((a) => a.agent_key === '__spirit__') || this.agents[0];
      }
    },
    async addAgent(payload: Parameters<typeof createAgent>[0]) {
      const created = await createAgent(payload);
      this.agents.unshift(created);
      this.selectedAgent = created;
      return created;
    },
  },
});

onSessionMutation((mutation) => {
  if (mutation.type === 'agent_updated') {
    useAppStore().upsertAgent(mutation.agent);
  }
});
