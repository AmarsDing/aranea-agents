import { defineStore } from "pinia";
import {
  clearAgentSessions,
  createSession,
  deleteSession,
  listMessages,
  listSessions,
  sendMessage,
  sendMessageStream,
  updateSessionTitle,
  type Message,
  type SendMessageOptions,
  type Session,
  type ToolUseEvent
} from "../features/chat/api";
import { formatToolEventMarkdown } from "../features/chat/toolEventMarkdown";
import { createAgent, deleteAgent, listAgents, updateAgent, type Agent } from "../features/agents/api";

export const useAppStore = defineStore("app", {
  state: () => ({
    loading: false,
    agents: [] as Agent[],
    sessions: [] as Session[],
    messages: [] as Message[],
    selectedAgent: null as Agent | null,
    selectedSession: null as Session | null
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
    /** 聊天侧栏等：按 id 增量更新 Agent，不依赖 selectedAgent。 */
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
      this.messages = await listMessages(this.selectedSession.id);
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
    },
    async sendStream(content: string, options?: SendMessageOptions, signal?: AbortSignal) {
      if (!this.selectedSession || !this.selectedAgent) return;
      const sessionID = this.selectedSession.id;
      const agent = this.selectedAgent;
      let streamingMessageID = "";
      const pendingUserId = `pending-user-${Date.now()}`;
      this.messages = [
        ...this.messages,
        {
          id: pendingUserId,
          session_id: sessionID,
          parent_message_id: "",
          turn_index: this.messages.length + 1,
          role: "user",
          content_markdown: content,
          model_name: "",
          token_in: 0,
          token_out: 0,
          latency_ms: 0,
          status: "ok",
          attachments_count: options?.attachments?.length ?? 0,
          options_json: "",
          error_message: "",
          created_at: new Date().toISOString()
        }
      ];

      const appendMessage = (message: Message) => {
        if (this.messages.some((item) => item.id === message.id)) return;
        this.messages = [...this.messages, message];
      };

      const onUserMessage = (message: Message) => {
        this.messages = this.messages.filter((item) => item.id !== pendingUserId);
        appendMessage(message);
      };

      await sendMessageStream(
        {
          session_id: sessionID,
          agent_key: this.selectedAgent.agent_key,
          content,
          options
        },
        {
          signal,
          onUserMessage,
          onToolEvent: (event) => {
            const message = toolEventMessage(sessionID, event);
            if (this.messages.some((item) => item.id === message.id)) {
              this.messages = this.messages.map((item) => (item.id === message.id ? message : item));
            } else {
              appendMessage(message);
            }
          },
          onDelta: (delta) => {
            if (!delta) return;
            if (!streamingMessageID) {
              streamingMessageID = `stream-${Date.now()}`;
              appendMessage({
                id: streamingMessageID,
                session_id: sessionID,
                parent_message_id: "",
                turn_index: this.messages.length + 1,
                role: "assistant",
                content_markdown: "",
                model_name: "",
                token_in: 0,
                token_out: 0,
                latency_ms: 0,
                status: "streaming",
                attachments_count: 0,
                options_json: JSON.stringify({
                  agent: {
                    agent_id: agent.id,
                    agent_key: agent.agent_key,
                    name: agent.display_name || agent.agent_key,
                    icon: agent.icon || ""
                  }
                }),
                error_message: "",
                created_at: new Date().toISOString()
              });
            }
            this.messages = this.messages.map((message) =>
              message.id === streamingMessageID
                ? { ...message, content_markdown: `${message.content_markdown}${delta}` }
                : message
            );
          },
          onDone: (message) => {
            if (streamingMessageID) {
              this.messages = this.messages.map((item) => (item.id === streamingMessageID ? message : item));
            } else {
              appendMessage(message);
            }
          }
        }
      );
      await this.loadMessages();
      await this.loadSessions();
    }
  }
});

function toolEventMessage(sessionID: string, event: ToolUseEvent): Message {
  const failed = event.status === "failed" || event.status === "error" || event.status === "blocked";
  const status = event.status === "running" ? "tool_running" : failed ? "tool_failed" : "tool_success";
  return {
    id: `tool-${event.agent_id || event.agent_key || "agent"}-${event.id || event.tool_name}`,
    session_id: sessionID,
    parent_message_id: "",
    turn_index: 0,
    role: "assistant",
    content_markdown: formatToolEventMarkdown(event),
    model_name: "",
    token_in: 0,
    token_out: 0,
    latency_ms: event.duration_ms ?? 0,
    status,
    attachments_count: 0,
    options_json: JSON.stringify({
      agent: {
        agent_id: event.agent_id,
        agent_key: event.agent_key,
        name: event.agent_name || event.agent_key,
        icon: event.agent_icon || ""
      },
      tool_event: event
    }),
    error_message: event.error || "",
    created_at: event.occurred_at || new Date().toISOString()
  };
}
