import type { TeamDefinition } from "../teams/types";
import type { GraphDefinition, NodeDef, EdgeDef } from "../graph/types";
import { buildGraphFromDefinition } from "../../components/teams/teamUtils";
import type { DisplayStatus, AgentNodeState } from "./types";

export function teamDefinitionToGraphDef(definition: TeamDefinition): GraphDefinition {
  const graph = buildGraphFromDefinition(definition);
  const nodes: NodeDef[] = graph.nodes
    .filter((n) => n.type === "agent")
    .map((n) => ({
      id: n.id,
      funcRef: "",
      interruptBefore: false,
      interruptAfter: false,
      type: "agent",
      description: n.role ?? "",
      instruction: "",
      modelName: "",
      toolNames: [],
      agentName: n.agent_id ?? n.label,
      destinations: [],
      requiredRole: n.role ?? "",
      assignmentMode: "",
      assignmentStrategy: "",
      reviewerAgent: "",
      reviewRules: "",
      timeoutSeconds: 0,
      heartbeatIntervalSeconds: 0,
      enableLeaseExtension: false,
    }));
  const edges: EdgeDef[] = graph.edges.map((e) => ({ from: e.source, to: e.target }));
  return {
    id: "",
    name: "team-orchestration",
    description: "",
    stateFields: [],
    nodes,
    edges,
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: graph.nodes.find((n) => n.type === "start")?.id ?? "",
    finishPoint: graph.nodes.find((n) => n.type === "end")?.id ?? "",
    enableCheckpoint: false,
    executionEngine: "bsp",
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    createdAt: "",
    updatedAt: "",
  };
}

export function displayStatusToExecStatus(display: DisplayStatus | string): string {
  switch (display) {
    case "active":
      return "running";
    case "success":
      return "completed";
    case "failed":
      return "failed";
    case "suspended":
      return "interrupted";
    case "cancelled":
      return "failed";
    case "skipped":
      return "waiting";
    default:
      return "idle";
  }
}

export type ExecNodeStateView = {
  status: string;
  fineStatus?: string;
  inputPreview?: string;
  outputPreview?: string;
  currentActivity?: string;
};

export function buildExecNodeStates(
  nodeStates: Map<string, AgentNodeState>,
): Map<string, ExecNodeStateView> {
  const out = new Map<string, ExecNodeStateView>();
  for (const [id, st] of nodeStates.entries()) {
    const activity = st.current_activity;
    out.set(id, {
      status: displayStatusToExecStatus(st.display_status),
      fineStatus: st.status,
      inputPreview: st.input_preview,
      outputPreview: st.output_preview,
      currentActivity:
        activity?.display_label?.trim() ||
        activity?.tool_name?.trim() ||
        activity?.kind?.trim() ||
        undefined,
    });
  }
  return out;
}
