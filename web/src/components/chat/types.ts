/**
 * Chat-specific UI types. Cross-domain types (Agent, Message, Session, Team)
 * should be imported directly from their respective feature modules — this file
 * no longer re-exports them to avoid a cross-domain type barrel.
 */

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
  context_status?: string;
  context_used_tokens?: number;
  last_context_window_tokens?: number;
  input_tokens?: number;
  output_tokens?: number;
  total_tokens?: number;
  total_cost_micro_usd?: number;
  at: string;
  timeline_at?: string;
  agent_id?: string;
  status?: string;
  pinned_at?: string;
  metadata_json?: string;
};

export type ChatAttachment = {
  id: string;
  name: string;
  progress: number;
  timer?: ReturnType<typeof setInterval>;
};

export type ChatEntityKind = "agent" | "team";
export type DeleteKind = ChatEntityKind | "session" | "all";
