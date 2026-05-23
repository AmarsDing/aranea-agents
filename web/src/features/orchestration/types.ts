/** Orchestration observability types (M53). Align with internal/biz/orchestration_status.go */

export type AgentNodeStatus =
  | "idle"
  | "queued"
  | "scheduled"
  | "running"
  | "thinking"
  | "tool_running"
  | "transferring"
  | "retrying"
  | "waiting_input"
  | "waiting_review"
  | "waiting_assign"
  | "blocked"
  | "success"
  | "failed"
  | "skipped"
  | "cancelled"
  | "timed_out";

export type DisplayStatus =
  | "waiting"
  | "active"
  | "suspended"
  | "success"
  | "failed"
  | "skipped"
  | "cancelled";

export type WorkPhase = "received" | "doing" | "delivered";

export type ActivitySnapshot = {
  kind?: string;
  display_label?: string;
  tool_name?: string;
  status?: string;
  summary?: string;
  arguments_json?: string;
  result_json?: string;
  started_at?: string;
  finished_at?: string;
  duration_ms?: number;
  error_code?: string;
};

export type AgentNodeState = {
  run_id?: string;
  team_id?: string;
  node_id: string;
  agent_id?: string;
  agent_key?: string;
  agent_name?: string;
  role?: string;
  status: AgentNodeStatus;
  display_status: DisplayStatus;
  phase: WorkPhase;
  retry_count?: number;
  input_preview?: string;
  output_preview?: string;
  error_message?: string;
  current_activity?: ActivitySnapshot;
  activity_history?: ActivitySnapshot[];
};

export type OrchestrationAgentStatusMetadata = {
  run_id?: string;
  team_id?: string;
  node_id?: string;
  agent_id?: string;
  agent_key?: string;
  agent_name?: string;
  role?: string;
  status?: AgentNodeStatus;
  display_status?: DisplayStatus;
  phase?: WorkPhase;
  retry_count?: number;
  input_preview?: string;
  output_preview?: string;
  error_message?: string;
  current_activity?: ActivitySnapshot;
  activity_history?: ActivitySnapshot[];
};

export type TeamRunObservatory = {
  run_id: string;
  team_id: string;
  session_id: string;
  status: string;
  mode: string;
  graph_execution_id?: string;
  trace_id?: string;
  definition_snapshot_json?: string;
  compiled_topology?: import("./compileApi").CompileTeamGraphResult;
  nodes: AgentNodeState[];
};

export type ActivityTimelineRow = {
  node_id: string;
  kind?: string;
  display_label?: string;
  status?: string;
  started_at?: string;
  finished_at?: string;
  duration_ms?: number;
  trace_id?: string;
};

export function agentNodeStateFromMetadata(meta: OrchestrationAgentStatusMetadata): AgentNodeState {
  return {
    run_id: meta.run_id,
    team_id: meta.team_id,
    node_id: String(meta.node_id ?? ""),
    agent_id: meta.agent_id,
    agent_key: meta.agent_key,
    agent_name: meta.agent_name,
    role: meta.role,
    status: meta.status ?? "idle",
    display_status: meta.display_status ?? "waiting",
    phase: meta.phase ?? "received",
    retry_count: meta.retry_count,
    input_preview: meta.input_preview,
    output_preview: meta.output_preview,
    error_message: meta.error_message,
    current_activity: meta.current_activity,
    activity_history: meta.activity_history,
  };
}
