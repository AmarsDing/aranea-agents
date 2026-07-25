import { createTeamService } from '../../services';
import type {
  CompileTeamGraphResponse,
  CompiledGraphNodeView,
  CompiledGraphEdgeView,
  CompiledGraphConditionalEdgeView,
  CompileTeamGraphValidationIssue,
} from '../../services/kratos/team/v1/index';
import type { GraphDefinition, NodeDef, EdgeDef, ConditionalEdgeDef } from '../graph/types';
import { applyAutoLayout, hasSavedLayout } from '../graph/editor/graphLayout';

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
  // ADR-08 A2: backend template-generated canonical embedded graph spec
  // ("" when the definition carries its own embedded graph).
  definition_graph_json: string;
};

function wireCompile(res: CompileTeamGraphResponse): CompileTeamGraphResult {
  return wireCompileResponse(res);
}

export function wireCompileResponse(res: CompileTeamGraphResponse): CompileTeamGraphResult {
  return {
    template_id: res.templateId ?? '',
    mode: res.mode ?? '',
    entry_point: res.entryPoint ?? '',
    finish_point: res.finishPoint ?? '',
    nodes: res.nodes ?? [],
    edges: res.edges ?? [],
    conditional_edges: res.conditionalEdges ?? [],
    graph_json: res.graphJson ?? '',
    issues: res.issues ?? [],
    valid: res.valid ?? false,
    definition_graph_json: res.definitionGraphJson ?? '',
  };
}

export async function compileTeamGraph(teamId: string, definitionJson?: string): Promise<CompileTeamGraphResult> {
  const svc = createTeamService();
  const res = await svc.CompileTeamGraph({
    teamId,
    definitionJson: definitionJson?.trim() || undefined,
  });
  return wireCompile(res);
}

export function compiledGraphToGraphDef(
  compiled: CompileTeamGraphResult,
  name = 'team-orchestration',
): GraphDefinition {
  const nodes: NodeDef[] = (compiled.nodes ?? []).map((n) => ({
    id: n.id ?? '',
    funcRef: '',
    interruptBefore: false,
    interruptAfter: false,
    type: (n.type ?? 'agent') as NodeDef['type'],
    description: n.agentDisplayName || n.description || '',
    instruction: n.taskPrompt || n.description || '',
    modelName: '',
    toolNames: [],
    agentName: n.agentName ?? '',
    destinations: [],
    requiredRole: n.role ?? '',
    assignmentMode: '',
    assignmentStrategy: '',
    reviewerAgent: '',
    reviewRules: '',
    timeoutSeconds: 0,
    heartbeatIntervalSeconds: 0,
    enableLeaseExtension: false,
    retryMaxAttempts: 0,
    failureAction: '',
    fallbackAgent: '',
    inputMapperJson: '',
    outputMapperJson: '',
    isolatedMessages: false,
    inputFromLastResponse: false,
    cacheEnabled: false,
    cacheTtlSeconds: 0,
  }));
  const edges: EdgeDef[] = (compiled.edges ?? [])
    .map((e) => ({
      from: e.from ?? '',
      to: e.to ?? '',
      kind: e.edgeKind?.trim() || '',
    }))
    .filter((e) => e.from && e.to && e.from !== e.to);
  const conditionalEdges: ConditionalEdgeDef[] = (compiled.conditional_edges ?? []).map((ce) => ({
    from: ce.from ?? '',
    condFuncRef: '',
    pathMap: { ...(ce.pathMap ?? {}) },
  }));
  const result: GraphDefinition = {
    id: '',
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
    executionEngine: 'bsp',
    interruptBefore: [],
    interruptAfter: [],
    metadata: { template_id: compiled.template_id, mode: compiled.mode },
    version: 0,
    sortOrder: 0,
    createdAt: '',
    updatedAt: '',
  };
  if (!hasSavedLayout(result)) {
    applyAutoLayout(result);
  }
  return result;
}
