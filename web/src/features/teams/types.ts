/** Teams UI / WS Envelope snake_case 形状（与 Kratos wire 对齐）。 */

export type { OrchestrationSpec } from './orchestrationSpec';
export { toOrchestrationSpec, fromOrchestrationSpec } from './orchestrationSpec';

export type Team = {
  id: string;
  team_key: string;
  display_name: string;
  status: string;
  is_default: boolean;
  definition_json: string;
  app_name: string;
  linked_graph_id: string;
  has_active_run: boolean;
  taxonomy_industry_id: string;
  readonly?: boolean;
  source?: string; // user | system | imported
  created_at: string;
  updated_at: string;
  deleted_at: string;
};

export type TeamDefinitionMember = {
  agent_id: string;
  role: 'coordinator' | 'worker' | 'synthesizer' | 'critic' | 'generator' | string;
  name: string;
  task_prompt?: string;
  enabled: boolean;
  sort_order: number;
};

export type TeamDefinitionGraphNode = {
  id: string;
  type: 'start' | 'agent' | 'join' | 'end' | string;
  label: string;
  agent_id?: string;
  role?: string;
  x?: number;
  y?: number;
};

export type TeamDefinitionGraphEdge = {
  id: string;
  source: string;
  target: string;
  label?: string;
  condition?: string;
};

export type TeamFailurePolicy = {
  default?: 'retry_then_block' | 'skip' | 'fail_fast' | string;
  parallel_fail?: 'continue' | 'abort' | string;
  retry?: {
    max_attempts?: number;
    initial_interval_ms?: number;
    backoff_factor?: number;
    max_interval_ms?: number;
  };
  node_overrides?: Record<
    string,
    {
      policy?: string;
      retry?: TeamFailurePolicy['retry'];
      fallback_agent?: string;
    }
  >;
  circuit_breaker?: {
    failure_threshold?: number;
    reset_timeout_seconds?: number;
    fallback_node?: string;
  };
  on_error?: 'await_review' | 'halt' | string;
};

export type TeamDefinition = {
  version: number;
  description?: string;
  /** Graph 执行引擎（默认 graph）；Native 仅在后端 ARANEA_TEAM_NATIVE=1 应急时可用 */
  runtime_engine?: 'native' | 'graph' | string;
  /** 兼容旧字段，等效 runtime_engine=graph */
  team_graph_runtime?: boolean;
  /** 关联 persisted graph 资产 id（M53 linked_graph） */
  linked_graph_id?: string;
  failure_policy?: TeamFailurePolicy;
  mode: 'sequential' | 'parallel' | 'coordinator' | 'critic_loop' | 'adaptive' | string;
  max_concurrency?: number;
  timeout_seconds?: number;
  /** coordinator / adaptive：外圈 LoopAgent 迭代上限；0 表示后端默认 3 */
  loop_max_iterations?: number;
  /** 可选：intent 与 user options 锚定的启用成员 agent_id；默认首位启用成员 */
  intent_anchor_agent_id?: string;
  members: TeamDefinitionMember[];
  enable_checkpoint?: boolean;
  a2a?: {
    enabled?: boolean;
    envelope_version?: string;
    message_format?: 'markdown_json' | 'plain' | string;
    include_trace?: boolean;
    max_payload_chars?: number;
  };
  graph?: {
    version?: number;
    layout?: 'linear' | 'parallel' | 'loop' | 'coordinator' | string;
    nodes: TeamDefinitionGraphNode[];
    edges: TeamDefinitionGraphEdge[];
  };
  synthesizer_agent_id?: string;
  critic_loop?: {
    max_iterations?: number;
    score_threshold?: number;
  };
};

export type TeamRun = {
  id: string;
  team_id: string;
  session_id: string;
  message_id: string;
  mode: string;
  status: string;
  input_preview: string;
  output_preview: string;
  token_in: number;
  token_out: number;
  cost_micro_usd: number;
  duration_ms: number;
  error_message: string;
  topology_json: string;
  graph_execution_id: string;
  started_at: string;
  finished_at: string;
  created_at: string;
  updated_at: string;
};

export type TeamRunStep = {
  id: string;
  run_id: string;
  team_id: string;
  agent_id: string;
  agent_key: string;
  agent_name: string;
  role: string;
  sort_order: number;
  status: string;
  input_preview: string;
  output_preview: string;
  token_in: number;
  token_out: number;
  cost_micro_usd: number;
  duration_ms: number;
  error_message: string;
  started_at: string;
  finished_at: string;
  created_at: string;
  tool_call_count?: number;
};

export type TeamRunMemberSummary = {
  agent_id: string;
  agent_key: string;
  agent_name: string;
  role: string;
  sort_order: number;
  status: string;
  token_in: number;
  token_out: number;
  duration_ms: number;
  cost_micro_usd: number;
  output_preview: string;
  tool_call_count: number;
};

export type TeamRunSummary = {
  run_id: string;
  team_id: string;
  session_id: string;
  mode: string;
  status: string;
  duration_ms: number;
  token_in: number;
  token_out: number;
  cost_micro_usd: number;
  member_count: number;
  tool_call_count: number;
  output_preview: string;
  error_message: string;
  members: TeamRunMemberSummary[];
};

export type TeamRunEvent = {
  type: string;
  team_id: string;
  run_id: string;
  session_id?: string;
  run?: TeamRun;
  step?: TeamRunStep;
  payload?: Record<string, unknown>;
};

export type TaskDeadLetterRow = {
  id: string;
  source_type: string;
  source_id: string;
  team_id: string;
  team_run_id: string;
  session_id: string;
  graph_execution_id: string;
  error_message: string;
  payload_json: string;
  status: string;
  created_at: string;
  resolved_at: string;
};
