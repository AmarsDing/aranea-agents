/**
 * Chat domain types. Shared types (Message, RunStatus, RunStatusValue) have
 * been lifted to domain/types.ts. This file re-exports them for backward
 * compatibility and defines chat-specific types.
 */

import type { ComputedRef, InjectionKey } from 'vue';

// Re-export shared domain types
export type { Message, MessageOrigin, RunStatus, RunStatusValue } from '../../domain/types';

export const TOOL_DISPLAY_KEY: InjectionKey<ComputedRef<{ showToolCalls: boolean }>> = Symbol('tool-display-key');

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
};

import type { Message } from '../../domain/types';

export type SendMessageResult = {
  user_message: Message;
  agent_message: Message;
};

export type ActivityKind = 'tool' | 'skill' | 'mcp' | 'subagent' | 'memory' | 'knowledge' | 'session';

export type ToolUseEvent = {
  id: string;
  phase: 'before' | 'after' | string;
  status: 'running' | 'success' | 'failed' | 'blocked' | 'cancelled' | string;
  agent_id: string;
  agent_key: string;
  agent_name: string;
  tool_name: string;
  tool_label: string;
  arguments?: unknown;
  result?: unknown;
  error?: string;
  occurred_at: string;
  duration_ms?: number;
  is_long_running?: boolean;
  activity_kind?: ActivityKind;
  display_label?: string;
  icon_key?: string;
  summary?: string;
  started_at?: string;
  finished_at?: string;
  error_code?: string;
  i18n_key?: string;
  run_id?: string;
  trace_id?: string;
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

export type PendingMessage = {
  id: string;
  content: string;
  status: string;
  created_at: string;
};

export interface EnqueueUserMessageResult {
  accepted: boolean;
  queued: boolean;
  pendingId: string;
}

export type ChatBackgroundJobRow = {
  id: string;
  source: string;
  session_id: string;
  agent_id: string;
  status: string;
  target_type: string;
  target_id: string;
  graph_id?: string;
  turn_id?: string;
  session_run_id?: string;
  phase?: string;
  created_at: string;
  updated_at: string;
  summary?: string;
  error_message?: string;
  channel_id: string;
};
