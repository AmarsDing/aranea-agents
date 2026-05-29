/**
 * Shared Envelope types — the single source of truth for all realtime
 * envelope definitions. Both chat and non-chat features (monitor, teams,
 * orchestration, graph, knowledge) import from here.
 *
 * Previously these types lived in features/chat/envelope.ts; they have
 * been lifted to this shared location so that features don't need to
 * reach into the chat domain for protocol-level types.
 */

export type EnvelopeType =
  | "text_delta"
  | "text_done"
  | "tool_call"
  | "tool_result"
  | "state_delta"
  | "transfer"
  | "runner_completion"
  | "context_usage"
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
  | "user_feedback"
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
  code?: string;
  message: string;
  hint?: string;
  pending_id?: string;
};

export type PromptTokenBreakdown = {
  system_prompt?: number;
  skills?: number;
  memory?: number;
  intent_pass?: number;
  session_summary?: number;
  tool_results?: number;
  history?: number;
  user_message?: number;
};

export type EnvelopeUsage = {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  max_tokens?: number;
  /** Max prompt tokens in the turn — context window fill (ReAct uses peak prompt). */
  context_prompt_tokens?: number;
  turn_total_tokens?: number;
  /** Category-level prompt token breakdown (CC-E-02 Phase 2). When present, frontend uses this instead of estimation. */
  prompt_breakdown?: PromptTokenBreakdown;
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

export function resolveEnvelopeTurnId(env: Envelope): string {
  return (
    (env.turn_id ?? "").trim() ||
    stringValue(env.metadata?.turn_id) ||
    stringValue(env.metadata?.run_id) ||
    (env.request_id ?? "").trim() ||
    env.id
  );
}

export function resolveEnvelopeSource(env: Envelope): string {
  return ((env.source ?? "").trim() || stringValue(env.metadata?.source));
}

export function resolveEnvelopeRevision(env: Envelope): number {
  if (typeof env.session_revision === "number" && env.session_revision > 0) return env.session_revision;
  const fromMeta = env.metadata?.session_revision;
  if (typeof fromMeta === "number" && fromMeta > 0) return fromMeta;
  return 0;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}
