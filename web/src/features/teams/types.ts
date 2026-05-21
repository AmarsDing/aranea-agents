/** Teams UI / WS Envelope snake_case 形状（与 Kratos wire 对齐）。 */

export type Team = {
  id: string;
  team_key: string;
  display_name: string;
  status: string;
  is_default: boolean;
  definition_json: string;
  app_name: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
};

export type TeamDefinitionMember = {
  agent_id: string;
  role: "coordinator" | "worker" | "synthesizer" | "critic" | "generator" | string;
  name: string;
  enabled: boolean;
  sort_order: number;
};

export type TeamDefinitionGraphNode = {
  id: string;
  type: "start" | "agent" | "join" | "end" | string;
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

export type TeamDefinition = {
  version: number;
  description?: string;
  mode: "sequential" | "parallel" | "coordinator" | "critic_loop" | "adaptive" | string;
  max_concurrency?: number;
  timeout_seconds?: number;
  /** coordinator / adaptive：外圈 LoopAgent 迭代上限；0 表示后端默认 3 */
  loop_max_iterations?: number;
  /** 可选：intent 与 user options 锚定的启用成员 agent_id；默认首位启用成员 */
  intent_anchor_agent_id?: string;
  members: TeamDefinitionMember[];
  a2a?: {
    enabled?: boolean;
    envelope_version?: string;
    message_format?: "markdown_json" | "plain" | string;
    include_trace?: boolean;
    max_payload_chars?: number;
  };
  graph?: {
    version?: number;
    layout?: "linear" | "parallel" | "loop" | "coordinator" | string;
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
