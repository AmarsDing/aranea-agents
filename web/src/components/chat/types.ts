/**
 * Chat-specific UI types. Cross-domain types (Agent, Message, Session, Team)
 * should be imported directly from their respective feature modules — this file
 * no longer re-exports them to avoid a cross-domain type barrel.
 */

import type { ContextBudgetSnapshot } from '../../features/session/types';

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
  tool_call_count?: number;
  message_count?: number;
  at: string;
  timeline_at?: string;
  agent_id?: string;
  status?: string;
  archived_at?: string;
  pinned_at?: string;
  metadata_json?: string;
  /** Prompt-assembly breakdown pushed via context_usage WS events (WS-only). */
  context_budget?: ContextBudgetSnapshot | null;
};

export type ChatAttachment = {
  id: string;
  name: string;
  mime_type?: string;
  progress: number;
  timer?: ReturnType<typeof setInterval>;
};

export type ChatEntityKind = 'agent' | 'team';
export type DeleteKind = ChatEntityKind | 'session' | 'all';
