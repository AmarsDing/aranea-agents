/**
 * Shared Graph execution types — the single source of truth for graph
 * node/execution state used by both chat (useEnvelopeStream) and
 * graph runtime (graphExecutionProjection).
 *
 * Previously these types lived in features/chat/useEnvelopeStream.ts;
 * they have been lifted to this shared location so that the graph
 * feature doesn't need to reach into the chat domain for shared types.
 */

export type GraphNodeState = {
  nodeId: string;
  nodeType: string;
  status: 'pending' | 'running' | 'completed' | 'error' | 'interrupted';
  startTime?: string;
  endTime?: string;
  durationNs?: number;
  error?: string;
  stepNumber?: number;
};

export type GraphExecutionState = {
  executionId: string;
  graphId: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'waiting_human';
  currentNode?: string;
  totalSteps?: number;
  durationNs?: number;
  nodes: Map<string, GraphNodeState>;
};

export type GraphStreamInterrupt = {
  nodeId: string;
  interruptKey: string;
  prompt: string;
  checkpointId: string;
  lineageId: string;
  interruptValue?: unknown;
};

export type GraphStreamExecutionSummary = {
  executionId: string;
  graphId: string;
  totalSteps: number;
  durationMs: number;
  finalStateKeys: number;
  nodes: Array<{
    nodeId: string;
    nodeType: string;
    status: string;
    durationMs: number;
    error: string;
    stepNumber: number;
  }>;
};
