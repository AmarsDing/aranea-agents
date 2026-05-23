export type EnvelopeType =
  | "text_delta"
  | "text_done"
  | "tool_call"
  | "tool_result"
  | "state_delta"
  | "transfer"
  | "runner_completion"
  | "run_status"
  | "error"
  | "log"
  | "flow_log"
  | "graph_node_start"
  | "graph_node_end"
  | "checkpoint"
  | "intent_pass"
  | "member_message_start"
  | "member_delta"
  | "member_message_done"
  | "team_run_started"
  | "team_run_finished"
  | "team_run_failed"
  | "team_summary"
  | "team_step_started"
  | "team_step_finished"
  | "graph_step"
  | "graph_execution_done"
  | "graph_node_error"
  | "graph_node_custom"
  | "graph_task_status"
  | "knowledge_ingest"
  | "mcp.session.reconnect"
  | "alert.notify";

export type EnvelopeContent = {
  text: string;
  reasoning?: string;
  is_partial: boolean;
};

export type EnvelopeToolCall = {
  id: string;
  name: string;
  arguments_json: string;
  result_json?: string;
  status: string;
  duration_ms?: number;
  is_long_running?: boolean;
  activity_kind?: string;
  display_label?: string;
  icon_key?: string;
  summary?: string;
  started_at?: string;
  finished_at?: string;
  error_code?: string;
  agent_key?: string;
  agent_id?: string;
  agent_name?: string;
  run_id?: string;
  trace_id?: string;
};

export type EnvelopeStateDelta = {
  operation: string;
  path: string;
  value_json: string;
};

export type EnvelopeTransfer = {
  from_agent: string;
  to_agent: string;
};

export type EnvelopeError = {
  type: string;
  message: string;
  pending_id?: string;
};

export type EnvelopeUsage = {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
};

export type EnvelopeActions = {
  skip_summarization?: boolean;
};

export type EnvelopeTrace = {
  agent_name: string;
  invocation_id: string;
  step_count: number;
  duration_ms?: number;
};

export type Envelope = {
  id: string;
  type: EnvelopeType;
  author: string;
  session_id: string;
  team_id?: string;
  request_id?: string;
  invocation_id?: string;
  parent_invocation_id?: string;
  branch?: string;
  filter_key?: string;
  tag?: string;
  timestamp: string;
  version: number;
  channel?: string;

  content?: EnvelopeContent;
  tool_call?: EnvelopeToolCall;
  state_delta?: EnvelopeStateDelta;
  transfer?: EnvelopeTransfer;
  error?: EnvelopeError;
  usage?: EnvelopeUsage;
  extensions?: Record<string, string>;
  actions?: EnvelopeActions;
  trace?: EnvelopeTrace;
  metadata?: Record<string, unknown>;

  session_revision?: number;
  source?: string;
  job_id?: string;
  turn_id?: string;
};

export type WsDownstream = {
  direction: "server_to_client";
  channel: string;
  type?: string;
  payload?: unknown;
  envelope?: Envelope;
};

export type WsUpstream = {
  direction: "client_to_server";
  channel: string;
  type: string;
  request_id?: string;
  payload?: unknown;
};
