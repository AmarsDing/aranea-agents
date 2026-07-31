import type { GraphNodeState } from '../../../realtime/graphState';
import type { AgentNodeState } from '../../orchestration/types';
import type { GraphStepSnapshot, NodeDef } from '../types';

export type ExecNodeState = {
  status: string;
  fineStatus?: string;
};

export type GraphInterruptInfo = {
  nodeId: string;
  interruptKey: string;
  prompt: string;
  checkpointId: string;
  lineageId: string;
  interruptValue?: unknown;
};

export function graphNodeStatusToExecStatus(status: GraphNodeState['status']): string {
  switch (status) {
    case 'running':
      return 'running';
    case 'completed':
      return 'completed';
    case 'error':
      return 'failed';
    case 'interrupted':
      return 'interrupted';
    case 'pending':
      return 'waiting';
    default:
      return 'idle';
  }
}

export function stepStatusToGraphNodeStatus(status: string): GraphNodeState['status'] {
  const normalized = status.trim().toLowerCase();
  if (normalized === 'completed' || normalized === 'success') return 'completed';
  if (normalized === 'running') return 'running';
  if (normalized === 'error' || normalized === 'failed') return 'error';
  if (normalized === 'interrupted' || normalized === 'waiting_human') return 'interrupted';
  return 'pending';
}

export function buildExecNodeStatesFromGraphNodes(nodes: Map<string, GraphNodeState>): Map<string, ExecNodeState> {
  const out = new Map<string, ExecNodeState>();
  for (const [id, node] of nodes.entries()) {
    out.set(id, { status: graphNodeStatusToExecStatus(node.status) });
  }
  return out;
}

export function seedGraphNodeStatesFromSteps(steps: GraphStepSnapshot[]): Map<string, GraphNodeState> {
  const nodes = new Map<string, GraphNodeState>();
  for (const step of steps) {
    if (!step.nodeId) continue;
    nodes.set(step.nodeId, {
      nodeId: step.nodeId,
      nodeType: 'function',
      status: stepStatusToGraphNodeStatus(step.status),
      stepNumber: step.stepIndex,
      error: step.error || undefined,
    });
  }
  return nodes;
}

export function parseInterruptPrompt(value: unknown): string {
  if (value == null) return '';
  if (typeof value === 'string') return value.trim();
  if (typeof value === 'object' && value !== null && 'prompt' in value) {
    return String((value as { prompt?: unknown }).prompt ?? '').trim();
  }
  return '';
}

export function buildResumePayload(
  interrupt: GraphInterruptInfo,
  approved: boolean,
  advancedJson?: Record<string, unknown>,
): Record<string, unknown> {
  if (advancedJson && Object.keys(advancedJson).length > 0) {
    return advancedJson;
  }
  const payload: Record<string, unknown> = {};
  if (interrupt.lineageId) {
    payload.lineage_id = interrupt.lineageId;
  }
  if (interrupt.checkpointId) {
    payload.checkpoint_id = interrupt.checkpointId;
  }
  if (interrupt.interruptKey) {
    payload.resume_map = { [interrupt.interruptKey]: approved };
  }
  return payload;
}

// ── M53 Phase 11 F7：GraphRunPage Kanban 视角（team 执行 / 资产删除降级） ──

const KANBAN_PREVIEW_MAX_LEN = 160;

function kanbanPreview(value: Record<string, unknown> | undefined): string | undefined {
  if (!value || Object.keys(value).length === 0) return undefined;
  const raw = JSON.stringify(value);
  return raw.length > KANBAN_PREVIEW_MAX_LEN ? `${raw.slice(0, KANBAN_PREVIEW_MAX_LEN)}…` : raw;
}

function execStatusToKanban(execStatus: string): Pick<AgentNodeState, 'status' | 'display_status' | 'phase'> {
  switch (execStatus) {
    case 'completed':
      return { status: 'success', display_status: 'success', phase: 'delivered' };
    case 'running':
      return { status: 'running', display_status: 'active', phase: 'doing' };
    case 'failed':
      return { status: 'failed', display_status: 'failed', phase: 'doing' };
    case 'interrupted':
      return { status: 'waiting_input', display_status: 'suspended', phase: 'doing' };
    default:
      return { status: 'idle', display_status: 'waiting', phase: 'received' };
  }
}

/**
 * 将 Graph 执行 steps + 实时节点状态 + 图定义投影为 Kanban 卡片所需的 AgentNodeState。
 * 节点全集 = 图定义节点（保持定义顺序） ∪ steps/exec 中出现但定义缺失的节点（按 stepIndex 升序，
 * 覆盖 team 图资产已删除的降级场景）；live execNodeStates 状态优先于 step 状态。
 */
export function buildGraphRunKanbanNodes(
  steps: GraphStepSnapshot[],
  execNodeStates: Map<string, ExecNodeState>,
  graphNodes: NodeDef[],
): AgentNodeState[] {
  const latestStepByNode = new Map<string, GraphStepSnapshot>();
  for (const step of steps) {
    if (!step.nodeId) continue;
    const prev = latestStepByNode.get(step.nodeId);
    if (!prev || step.stepIndex >= prev.stepIndex) {
      latestStepByNode.set(step.nodeId, step);
    }
  }

  const orderedIds: string[] = [];
  const seen = new Set<string>();
  for (const node of graphNodes) {
    if (seen.has(node.id)) continue;
    seen.add(node.id);
    orderedIds.push(node.id);
  }
  const extraIds = [...latestStepByNode.keys()]
    .filter((id) => !seen.has(id))
    .sort((a, b) => (latestStepByNode.get(a)?.stepIndex ?? 0) - (latestStepByNode.get(b)?.stepIndex ?? 0));
  orderedIds.push(...extraIds);

  const graphNodeById = new Map(graphNodes.map((n) => [n.id, n]));

  return orderedIds.map((nodeId) => {
    const step = latestStepByNode.get(nodeId);
    const liveStatus = execNodeStates.get(nodeId)?.status;
    const stepExecStatus = step ? graphNodeStatusToExecStatus(stepStatusToGraphNodeStatus(step.status)) : 'idle';
    const mapped = execStatusToKanban(liveStatus ?? stepExecStatus);
    const agentName = graphNodeById.get(nodeId)?.agentName?.trim();
    return {
      node_id: nodeId,
      agent_name: agentName || nodeId,
      ...mapped,
      input_preview: kanbanPreview(step?.inputState),
      output_preview: kanbanPreview(step?.outputState),
      error_message: step?.error || undefined,
    };
  });
}

/**
 * 悬空 graph_id 降级：资产已删除时从执行 steps 合成只读画布节点（按 stepIndex 升序）。
 * 仅保留画布渲染所需的最小字段；team 图节点恒为 agent 类型。
 */
export function synthesizeGraphNodesFromSteps(steps: GraphStepSnapshot[]): NodeDef[] {
  const firstStepIndexByNode = new Map<string, number>();
  for (const step of steps) {
    if (!step.nodeId) continue;
    const prev = firstStepIndexByNode.get(step.nodeId);
    if (prev === undefined || step.stepIndex < prev) {
      firstStepIndexByNode.set(step.nodeId, step.stepIndex);
    }
  }
  return [...firstStepIndexByNode.entries()]
    .sort((a, b) => a[1] - b[1])
    .map(([id]) => ({
      id,
      funcRef: '',
      interruptBefore: false,
      interruptAfter: false,
      type: 'agent' as const,
      description: '',
      instruction: '',
      modelName: '',
      toolNames: [],
      agentName: '',
      destinations: [],
      requiredRole: '',
      assignmentMode: 'static',
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
}
