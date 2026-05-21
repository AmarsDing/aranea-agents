import { defineStore } from "pinia";
import {
  clearAgentSessions,
  createSession,
  deleteSession,
  listSessionChatMessages as listMessages,
  listSessions,
  updateSessionTitle,
  type Session
} from "../features/session/api";
import {
  sendMessage,
  type Message,
  type SendMessageOptions,
  type IntentPassResult
} from "../features/chat/api";
import { mergeSessionMessages } from "../features/chat/mergeSessionMessages";
import { createAgent, deleteAgent, listAgents, updateAgent, type Agent } from "../features/agents/api";

export const useAppStore = defineStore("app", {
  state: () => ({
    loading: false,
    agents: [] as Agent[],
    sessions: [] as Session[],
    messages: [] as Message[],
    selectedAgent: null as Agent | null,
    selectedSession: null as Session | null,
    lastIntentPass: null as IntentPassResult | null
  }),
  actions: {
    async removeAgentFromList(id: string) {
      await deleteAgent(id);
      this.agents = this.agents.filter((a) => a.id !== id);
      if (this.selectedAgent?.id === id) {
        this.selectedAgent = this.agents[0] ?? null;
        this.sessions = [];
        this.selectedSession = null;
        this.messages = [];
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
      this.agents = exists ? this.agents.map((item) => (item.id === agent.id ? { ...item, ...agent } : item)) : [agent, ...this.agents];
      if (this.selectedAgent?.id === agent.id) {
        this.selectedAgent = { ...this.selectedAgent, ...agent };
      }
    },
    async clearAllSessions() {
      if (!this.selectedAgent) return;
      await clearAgentSessions(this.selectedAgent.id);
      this.sessions = [];
      this.selectedSession = null;
      this.messages = [];
    },
    async removeSessionLocal(id: string) {
      await deleteSession(id);
      this.sessions = this.sessions.filter((s) => s.id !== id);
      if (this.selectedSession?.id === id) {
        this.selectedSession = this.sessions[0] ?? null;
      }
      this.messages = [];
    },
    async renameSessionLocal(id: string, title: string) {
      const updated = await updateSessionTitle(id, title);
      this.sessions = this.sessions.map((session) => (session.id === id ? updated : session));
      if (this.selectedSession?.id === id) {
        this.selectedSession = updated;
      }
      return updated;
    },
    async loadAgents() {
      this.agents = await listAgents();
      if (!this.selectedAgent && this.agents.length > 0) {
        this.selectedAgent = this.agents[0];
      }
    },
    async addAgent(payload: Parameters<typeof createAgent>[0]) {
      const created = await createAgent(payload);
      this.agents.unshift(created);
      this.selectedAgent = created;
      return created;
    },
    async loadSessions() {
      if (!this.selectedAgent) return;
      const selectedID = this.selectedSession?.id;
      this.sessions = await listSessions(this.selectedAgent.id);
      if (selectedID) {
        this.selectedSession = this.sessions.find((session) => session.id === selectedID) ?? this.sessions[0] ?? null;
      } else if (!this.selectedSession && this.sessions.length > 0) {
        this.selectedSession = this.sessions[0];
      }
    },
    async addSession(title: string, options?: { dialog_mode?: string; default_provider?: string; default_model?: string }) {
      if (!this.selectedAgent) return null;
      const created = await createSession({ agent_id: this.selectedAgent.id, title, ...options });
      this.sessions.unshift(created);
      this.selectedSession = created;
      return created;
    },
    async loadMessages() {
      if (!this.selectedSession) return;
      const server = await listMessages(this.selectedSession.id);
      this.messages = mergeSessionMessages(server, this.messages);
    },
    async send(content: string, options?: SendMessageOptions) {
      if (!this.selectedSession || !this.selectedAgent) return;
      const result = await sendMessage({
        session_id: this.selectedSession.id,
        agent_key: this.selectedAgent.agent_key,
        content,
        options
      });
      const returnedMessages = [result.user_message, result.agent_message].filter(Boolean);
      const existing = new Set(this.messages.map((message) => message.id));
      this.messages = [...this.messages, ...returnedMessages.filter((message) => !existing.has(message.id))];
      await this.loadMessages();
      await this.loadSessions();
      return result;
    }
  }
});
