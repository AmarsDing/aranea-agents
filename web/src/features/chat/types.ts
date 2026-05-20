/** Chat：消息列表走 `session/v1` `GET /v1/sessions/{id}/messages`；发送 / 流式 / options 见 **`features/chat/api.ts`** → **`/v1/chat/*`**，由 admin 进程内 trpc-agent-go 运行时处理。 */

export type Message = {
  id: string;
  session_id: string;
  parent_message_id: string;
  turn_index: number;
  role: string;
  content_markdown: string;
  model_name: string;
  token_in: number;
  token_out: number;
  latency_ms: number;
  status: string;
  attachments_count: number;
  options_json: string;
  error_message: string;
  created_at: string;
};

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
};

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

export type ActivityKind = "tool" | "skill" | "mcp" | "subagent" | "memory" | "knowledge" | "session";

export type ToolUseEvent = {
  id: string;
  phase: "before" | "after" | string;
  status: "running" | "success" | "error" | "failed" | "blocked" | string;
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
