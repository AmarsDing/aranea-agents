import type { Agent } from "../../features/agents/api";
import type { Message, Session } from "../../features/chat/api";
import type { Team } from "../../features/teams/api";

export type TeamRow = {
  id: string;
  team_key?: string;
  display_name: string;
  status?: string;
  isDefault: boolean;
  isWorking: boolean;
  definition_json?: string;
};

export type SessionView = {
  id: string;
  title: string;
  context_used_ratio: number;
  at: string;
  timeline_at?: string;
  agent_id?: string;
};

export type ChatAttachment = {
  id: string;
  name: string;
  progress: number;
  timer?: ReturnType<typeof setInterval>;
};

export type ChatEntityKind = "agent" | "team";
export type DeleteKind = ChatEntityKind | "session" | "all";

export type { Agent, Message, Session, Team };
