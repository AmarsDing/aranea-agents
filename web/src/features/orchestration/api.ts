import { createTeamService } from "../../services";
import type { AgentNodeState, ActivitySnapshot, TeamRunObservatory } from "./types";
import { wireCompileResponse } from "./compileApi";

export type { TeamRunObservatory } from "./types";
import type {
  AgentNodeStateView,
  ActivitySnapshotView,
  GetTeamRunObservatoryResponse,
} from "../../services/kratos/team/v1/index";

function wireActivity(a: ActivitySnapshotView | null | undefined): ActivitySnapshot | undefined {
  if (!a) return undefined;
  return {
    kind: a.kind,
    display_label: a.displayLabel,
    tool_name: a.toolName,
    status: a.status,
    summary: a.summary,
    arguments_json: a.argumentsJson,
    result_json: a.resultJson,
    started_at: a.startedAt,
    finished_at: a.finishedAt,
    duration_ms: a.durationMs,
    error_code: a.errorCode,
  };
}

function wireNode(n: AgentNodeStateView): AgentNodeState {
  return {
    run_id: undefined,
    team_id: undefined,
    node_id: n.nodeId ?? "",
    agent_id: n.agentId,
    agent_key: n.agentKey,
    agent_name: n.agentName,
    role: n.role,
    status: (n.status ?? "idle") as AgentNodeState["status"],
    display_status: (n.displayStatus ?? "waiting") as AgentNodeState["display_status"],
    phase: (n.phase ?? "received") as AgentNodeState["phase"],
    retry_count: n.retryCount,
    input_preview: n.inputPreview,
    output_preview: n.outputPreview,
    error_message: n.errorMessage,
    current_activity: wireActivity(n.currentActivity),
  };
}

function wireObservatory(res: GetTeamRunObservatoryResponse): TeamRunObservatory {
  return {
    run_id: res.runId ?? "",
    team_id: res.teamId ?? "",
    session_id: res.sessionId ?? "",
    status: res.status ?? "",
    mode: res.mode ?? "",
    definition_snapshot_json: res.definitionSnapshotJson ?? "",
    compiled_topology: res.compiledTopology ? wireCompileResponse(res.compiledTopology) : undefined,
    nodes: (res.nodes ?? []).map(wireNode),
  };
}

export async function getTeamRunObservatory(runId: string): Promise<TeamRunObservatory> {
  const svc = createTeamService();
  const res = await svc.GetTeamRunObservatory({ runId });
  return wireObservatory(res);
}
