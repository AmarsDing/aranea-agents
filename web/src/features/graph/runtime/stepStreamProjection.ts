import type { GraphStepSnapshot } from "../types";

export type StepStreamEvent = {
  nodeId: string;
  stepIndex: number;
  status: string;
  error?: string;
  timestamp?: string;
};

export function upsertStepFromStreamEvent(
  steps: GraphStepSnapshot[],
  event: StepStreamEvent,
): GraphStepSnapshot[] {
  if (!event.nodeId) return steps;
  const idx = steps.findIndex(
    (step) => step.stepIndex === event.stepIndex && step.nodeId === event.nodeId,
  );
  const prior = idx >= 0 ? steps[idx] : undefined;
  const nextStep: GraphStepSnapshot = {
    nodeId: event.nodeId,
    stepIndex: event.stepIndex,
    inputState: prior?.inputState ?? {},
    outputState: prior?.outputState ?? {},
    status: event.status,
    error: event.error ?? prior?.error ?? "",
    timestamp: event.timestamp ?? prior?.timestamp ?? new Date().toISOString(),
  };
  if (idx >= 0) {
    const next = [...steps];
    next[idx] = nextStep;
    return next;
  }
  return [...steps, nextStep].sort((a, b) => a.stepIndex - b.stepIndex || a.nodeId.localeCompare(b.nodeId));
}

export function stepEventFromEnvelopeMetadata(
  metadata: Record<string, unknown> | undefined,
  status: string,
): StepStreamEvent | null {
  if (!metadata) return null;
  const nodeId = String(metadata.node_id ?? "");
  if (!nodeId) return null;
  const stepIndex = Number(metadata.step_number ?? metadata.step_index ?? 0);
  return {
    nodeId,
    stepIndex: Number.isFinite(stepIndex) ? stepIndex : 0,
    status,
    error: String(metadata.error ?? ""),
    timestamp: String(metadata.end_time ?? metadata.start_time ?? ""),
  };
}
