import type { GraphNodeState } from "../../../realtime/graphState";
import type { GraphStepSnapshot } from "../types";

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

export function graphNodeStatusToExecStatus(status: GraphNodeState["status"]): string {
  switch (status) {
    case "running":
      return "running";
    case "completed":
      return "completed";
    case "error":
      return "failed";
    case "interrupted":
      return "interrupted";
    case "pending":
      return "waiting";
    default:
      return "idle";
  }
}

export function stepStatusToGraphNodeStatus(status: string): GraphNodeState["status"] {
  const normalized = status.trim().toLowerCase();
  if (normalized === "completed" || normalized === "success") return "completed";
  if (normalized === "running") return "running";
  if (normalized === "error" || normalized === "failed") return "error";
  if (normalized === "interrupted" || normalized === "waiting_human") return "interrupted";
  return "pending";
}

export function buildExecNodeStatesFromGraphNodes(
  nodes: Map<string, GraphNodeState>,
): Map<string, ExecNodeState> {
  const out = new Map<string, ExecNodeState>();
  for (const [id, node] of nodes.entries()) {
    out.set(id, { status: graphNodeStatusToExecStatus(node.status) });
  }
  return out;
}

export function seedGraphNodeStatesFromSteps(
  steps: GraphStepSnapshot[],
): Map<string, GraphNodeState> {
  const nodes = new Map<string, GraphNodeState>();
  for (const step of steps) {
    if (!step.nodeId) continue;
    nodes.set(step.nodeId, {
      nodeId: step.nodeId,
      nodeType: "function",
      status: stepStatusToGraphNodeStatus(step.status),
      stepNumber: step.stepIndex,
      error: step.error || undefined,
    });
  }
  return nodes;
}

export function parseInterruptPrompt(value: unknown): string {
  if (value == null) return "";
  if (typeof value === "string") return value.trim();
  if (typeof value === "object" && value !== null && "prompt" in value) {
    return String((value as { prompt?: unknown }).prompt ?? "").trim();
  }
  return "";
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
