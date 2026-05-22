import { createTeamService } from "../../services";
import type {
  CompileTeamGraphResponse,
  CompiledGraphNodeView,
  CompiledGraphEdgeView,
  CompiledGraphConditionalEdgeView,
  CompileTeamGraphValidationIssue,
} from "../../services/kratos/team/v1/index";
import type { GraphDefinition, NodeDef, EdgeDef, ConditionalEdgeDef } from "../graph/types";

export type CompileTeamGraphResult = {
  template_id: string;
  mode: string;
  entry_point: string;
  finish_point: string;
  nodes: CompiledGraphNodeView[];
  edges: CompiledGraphEdgeView[];
  conditional_edges: CompiledGraphConditionalEdgeView[];
  graph_json: string;
  issues: CompileTeamGraphValidationIssue[];
  valid: boolean;
};

function wireCompile(res: CompileTeamGraphResponse): CompileTeamGraphResult {
  return wireCompileResponse(res);
}

export function wireCompileResponse(res: CompileTeamGraphResponse): CompileTeamGraphResult {
  return {
    template_id: res.templateId ?? "",
    mode: res.mode ?? "",
    entry_point: res.entryPoint ?? "",
    finish_point: res.finishPoint ?? "",
    nodes: res.nodes ?? [],
    edges: res.edges ?? [],
    conditional_edges: res.conditionalEdges ?? [],
    graph_json: res.graphJson ?? "",
    issues: res.issues ?? [],
    valid: res.valid ?? false,
  };
}

export async function compileTeamGraph(
  teamId: string,
  definitionJson?: string,
): Promise<CompileTeamGraphResult> {
  const svc = createTeamService();
  const res = await svc.CompileTeamGraph({
    teamId,
    definitionJson: definitionJson?.trim() || undefined,
  });
  return wireCompile(res);
}

export function compiledGraphToGraphDef(
  compiled: CompileTeamGraphResult,
  name = "team-orchestration",
): GraphDefinition {
  const nodes: NodeDef[] = (compiled.nodes ?? []).map((n) => ({
    id: n.id ?? "",
    funcRef: "",
    interruptBefore: false,
    interruptAfter: false,
    type: (n.type ?? "agent") as NodeDef["type"],
    description: n.description ?? "",
    instruction: "",
    modelName: "",
    toolNames: [],
    agentName: n.agentName ?? "",
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
  const edges: EdgeDef[] = (compiled.edges ?? []).map((e) => ({
    from: e.from ?? "",
    to: e.to ?? "",
    kind: e.edgeKind?.trim() || undefined,
  }));
  const conditionalEdges: ConditionalEdgeDef[] = (compiled.conditional_edges ?? []).map((ce) => ({
    from: ce.from ?? "",
    condFuncRef: "",
    pathMap: { ...(ce.pathMap ?? {}) },
  }));
  return {
    id: "",
    name,
    description: `template: ${compiled.template_id}`,
    stateFields: [],
    nodes,
    edges,
    conditionalEdges,
    subgraphs: [],
    entryPoint: compiled.entry_point,
    finishPoint: compiled.finish_point,
    enableCheckpoint: false,
    executionEngine: "bsp",
    interruptBefore: [],
    interruptAfter: [],
    metadata: { template_id: compiled.template_id, mode: compiled.mode },
    createdAt: "",
    updatedAt: "",
  };
}
