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
  | 'text_delta'
  | 'text_done'
  | 'tool_call'
  | 'tool_result'
  | 'state_delta'
  | 'transfer'
  | 'runner_completion'
  | 'context_usage'
  | 'run_status'
  | 'error'
  | 'log'
  | 'flow_log'
  | 'graph_node_start'
  | 'graph_node_end'
  | 'checkpoint'
  | 'intent_pass'
  | 'member_message_start'
  | 'member_delta'
  | 'member_message_done'
  | 'team_run_started'
  | 'team_run_finished'
  | 'team_run_failed'
  | 'team_summary'
  | 'team_step_started'
  | 'team_step_finished'
  | 'graph_step'
  | 'graph_execution_done'
  | 'graph_node_error'
  | 'graph_node_custom'
  | 'graph_task_status'
  | 'knowledge_ingest'
  | 'user_feedback'
  | 'mcp.session.reconnect'
  | 'mcp.health.alert'
  | 'orchestration_agent_status'
  | 'alert.notify'
  | 'session.status_changed'
  | 'spirit_team_assembled'
  | 'spirit_team_completed'
  | 'spirit_team_failed'
  | 'spirit_team_cancelled'
  | 'spirit_team_interrupted'
  | 'spirit_team_progress'
  | 'spirit_teams_all_completed'
  | 'spirit_synthesis_completed'
  | 'spirit_plan_created'
  | 'spirit_allocation_created'
  | 'spirit_orchestration_started'
  | 'spirit_orchestration_checkpoint'
  | 'spirit_orchestration_interrupted'
  | 'token_usage'
  | 'butler.orchestration.started'
  | 'butler.orchestration.completed'
  | 'butler.orchestration.failed'
  | 'skill.health_changed'
  | 'skill.evolution_proposed'
  | 'monitor.auto_healed'
  | 'monitor.self_check_completed'
  | 'metrics_updated';

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

export type EnvelopeUsage = {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  max_tokens?: number;
  /** Max prompt tokens in the turn — context window fill (ReAct uses peak prompt). */
  context_prompt_tokens?: number;
  turn_total_tokens?: number;
};

export type EnvelopeTokenUsage = {
  id: string;
  occurred_at: string;
  date_key: string;
  hour_key: string;
  workspace_id: string;
  user_id: string;
  team_id: string;
  agent_id: string;
  agent_key: string;
  session_id: string;
  message_id: string;
  request_id: string;
  provider_code: string;
  canonical_provider_code: string;
  provider_type: string;
  provider_display_name: string;
  model_api_id: string;
  model_display_name: string;
  model_category_json: string;
  usage_kind: string;
  call_count: number;
  input_tokens: number;
  output_tokens: number;
  cached_input_tokens: number;
  cache_write_tokens: number;
  reasoning_tokens: number;
  embedding_tokens: number;
  total_tokens: number;
  input_price_micro_usd_per_1k: number;
  output_price_micro_usd_per_1k: number;
  cached_input_price_micro_usd_per_1k: number;
  cache_write_price_micro_usd_per_1k: number;
  reasoning_price_micro_usd_per_1k: number;
  embedding_price_micro_usd_per_1k: number;
  input_cost_micro_usd: number;
  output_cost_micro_usd: number;
  cached_input_cost_micro_usd: number;
  cache_write_cost_micro_usd: number;
  reasoning_cost_micro_usd: number;
  embedding_cost_micro_usd: number;
  total_cost_micro_usd: number;
  latency_ms: number;
  time_to_first_token_ms: number;
  tokens_per_second: number;
  status: string;
  error_code: string;
  error_message: string;
  retry_count: number;
  prompt_mode: string;
  max_output_tokens: number;
  context_window_k: number;
  stream_enabled: boolean;
  metadata_json: string;
  created_at: string;
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
  token_usage?: EnvelopeTokenUsage;
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
  direction: 'server_to_client';
  channel: string;
  type?: string;
  payload?: unknown;
  envelope?: Envelope;
};

export type WsUpstream = {
  direction: 'client_to_server';
  channel: string;
  type: string;
  request_id?: string;
  payload?: unknown;
};

export function resolveEnvelopeTurnId(env: Envelope): string {
  return (
    (env.turn_id ?? '').trim() ||
    stringValue(env.metadata?.turn_id) ||
    stringValue(env.metadata?.run_id) ||
    (env.request_id ?? '').trim() ||
    env.id
  );
}

export function resolveEnvelopeSource(env: Envelope): string {
  return (env.source ?? '').trim() || stringValue(env.metadata?.source);
}

export function resolveEnvelopeRevision(env: Envelope): number {
  if (typeof env.session_revision === 'number' && env.session_revision > 0) return env.session_revision;
  const fromMeta = env.metadata?.session_revision;
  if (typeof fromMeta === 'number' && fromMeta > 0) return fromMeta;
  return 0;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

// --- Spirit Orchestration Event Payloads ---

export type SpiritPlanCreatedPayload = {
  plan_id: string;
  spirit_session_id: string;
  complexity_level: string;
  complexity_score: number;
  strategy: string;
  strategy_reason: string;
  topology_hint: string;
  subtask_count: number;
};

export type SpiritAllocationCreatedPayload = {
  allocation_id: string;
  task_plan_id: string;
  spirit_session_id: string;
  allocation_count: number;
  status: string;
};

export type SpiritOrchestrationStartedPayload = {
  orchestration_id: string;
  spirit_session_id: string;
  strategy: string;
  status: string;
  task_plan_id: string;
  allocation_id: string;
  team_ids?: string[];
  max_concurrent_teams?: number;
};

export type SpiritOrchestrationCheckpointPayload = {
  orchestration_id: string;
  spirit_session_id: string;
  checkpoint_id: string;
  step: string;
  status: string;
};

export type SpiritOrchestrationInterruptedPayload = {
  orchestration_id: string;
  spirit_session_id: string;
  status: string;
};
