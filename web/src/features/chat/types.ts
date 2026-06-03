/**
 * Chat domain types. Shared types (Message, RunStatus, RunStatusValue) have
 * been lifted to domain/types.ts. This file re-exports them for backward
 * compatibility and defines chat-specific types.
 */

import type { ReactStep } from './reactPlannerTypes';

// Re-export shared domain types
export type { Message, RunStatus, RunStatusValue } from '../../domain/types';

export type ChatOption = {
  type: string;
  key: string;
  label: string;
  enabled: boolean;
  sort_order: number;
  metadata_json: string;
};

export type SendMessageOptions = {
  dialog_mode?: string;
  provider?: string;
  model?: string;
  attachments?: Array<{ id: string }>;
  knowledge_bases?: string[];
  context_refs?: ContextRef[];
};

import type { Message } from '../../domain/types';

export type SendMessageResult = {
  user_message: Message;
  agent_message: Message;
};

export type IntentPassResult = {
  outcome: string;
  duration_ms: number;
  session_id?: string;
  agent_id?: string;
  intent_kind?: string;
  refined_goal_len?: number;
  search_hints_count?: number;
};

export type ActivityKind = 'tool' | 'skill' | 'mcp' | 'subagent' | 'memory' | 'knowledge' | 'session';

export type ToolUseEvent = {
  id: string;
  phase: 'before' | 'after' | string;
  status: 'running' | 'success' | 'error' | 'failed' | 'blocked' | string;
  agent_id: string;
  agent_key: string;
  agent_name: string;
  agent_icon: string;
  tool_name: string;
  tool_label: string;
  arguments?: Record<string, unknown>;
  result?: Record<string, unknown>;
  error?: string;
  occurred_at: string;
  duration_ms?: number;
  is_long_running?: boolean;
  message_hint?: string;
  activity_kind?: ActivityKind;
  display_label?: string;
  icon_key?: string;
  summary?: string;
  started_at?: string;
  finished_at?: string;
  error_code?: string;
  run_id?: string;
  trace_id?: string;
  expanded?: boolean;
};

export type DiffEditHunk = {
  search: string;
  replace: string;
  replace_all?: boolean;
};

export type DiffEditArguments = {
  file_name: string;
  edits: DiffEditHunk[];
  expected_mtime_ms?: number;
};

export type PatchFileArguments = {
  file_name: string;
  patch?: string;
  hunks?: Array<Record<string, unknown>>;
  expected_mtime_ms?: number;
};

export type FileEditResult = {
  applied_edits?: number;
  applied_hunks?: number;
  total_replacements?: number;
  file_name?: string;
  content?: string;
  error?: string;
};

export type ContextRefKind = 'file' | 'folder' | 'knowledge_base' | 'artifact';

export type ContextRefItem = {
  key: string;
  kind: ContextRefKind;
  label: string;
  description: string;
  icon: string;
  iconColor: string;
};

export type ContextRef = {
  kind: ContextRefKind;
  ref_id: string;
  label: string;
};

/** ReAct ACTION step with linked `chat.activity/v1` tool rows (see `reactPlannerToolLink`). */
export type ReactStepWithTools = ReactStep & {
  linkedTools: ToolUseEvent[];
};

/** Session-level cache: one O(n) pass over `displayMessages` for ReAct ↔ tool dedupe. */
export type ReactToolLinkIndex = {
  linkedToolIds: ReadonlySet<string>;
  stepsByAssistantIndex: ReadonlyMap<number, ReactStepWithTools[]>;
};

export type PendingMessage = {
  id: string;
  content: string;
  status: string;
  created_at: string;
};
