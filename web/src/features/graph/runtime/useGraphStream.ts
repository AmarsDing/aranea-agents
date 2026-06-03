/**
 * Graph-specific envelope stream composable.
 * Lifted from features/chat/useEnvelopeStream.ts to eliminate cross-feature dependency.
 * Chat re-exports this for backward compatibility.
 */
import { ref } from 'vue';
import { useEnvelopeStream } from '../../../realtime/useEnvelopeStream';
import type {
  GraphExecutionState,
  GraphStreamExecutionSummary,
  GraphStreamInterrupt,
} from '../../../realtime/graphState';

function parseGraphStreamSummary(raw: unknown): GraphStreamExecutionSummary | null {
  if (!raw || typeof raw !== 'object') return null;
  const summary = raw as Record<string, unknown>;
  const nodes = Array.isArray(summary.nodes)
    ? summary.nodes.map((node) => {
        const n = node as Record<string, unknown>;
        return {
          nodeId: String(n.node_id ?? n.nodeId ?? ''),
          nodeType: String(n.node_type ?? n.nodeType ?? ''),
          status: String(n.status ?? ''),
          durationMs: Number(n.duration_ms ?? n.durationMs ?? 0),
          error: String(n.error ?? ''),
          stepNumber: Number(n.step_number ?? n.stepNumber ?? 0),
        };
      })
    : [];
  return {
    executionId: String(summary.execution_id ?? summary.executionId ?? ''),
    graphId: String(summary.graph_id ?? summary.graphId ?? ''),
    totalSteps: Number(summary.total_steps ?? summary.totalSteps ?? 0),
    durationMs: Number(summary.duration_ms ?? summary.durationMs ?? 0),
    finalStateKeys: Number(summary.final_state_keys ?? summary.finalStateKeys ?? 0),
    nodes,
  };
}

function parseInterruptPrompt(value: unknown): string {
  if (value == null) return '';
  if (typeof value === 'string') return value.trim();
  if (typeof value === 'object' && value !== null && 'prompt' in value) {
    return String((value as { prompt?: unknown }).prompt ?? '').trim();
  }
  return '';
}

export function useGraphStream(sessionId: string, graphId: string, execId: string) {
  const stream = useEnvelopeStream({
    sessionId,
    channels: ['chat', 'graph', 'system'],
  });

  const execution = ref<GraphExecutionState>({
    executionId: execId,
    graphId,
    status: 'pending',
    nodes: new Map(),
  });

  const executionSummary = ref<GraphStreamExecutionSummary | null>(null);
  const interrupt = ref<GraphStreamInterrupt | null>(null);

  const filterKey = `graph/${graphId}/${execId}`;

  stream.onChannel('graph', (env) => {
    if (env.filter_key && !env.filter_key.startsWith(filterKey)) {
      return;
    }

    switch (env.type) {
      case 'graph_node_start': {
        const nodeId = env.metadata?.node_id as string;
        const nodeType = env.metadata?.node_type as string;
        const stepNumber = env.metadata?.step_number as number;
        if (nodeId) {
          const existing = execution.value.nodes.get(nodeId);
          execution.value.nodes.set(nodeId, {
            nodeId,
            nodeType: nodeType ?? existing?.nodeType ?? 'function',
            status: 'running',
            startTime: env.metadata?.start_time as string,
            stepNumber,
          });
          execution.value.currentNode = nodeId;
          execution.value.status = 'running';
        }
        break;
      }
      case 'graph_node_end': {
        const nodeId = env.metadata?.node_id as string;
        if (nodeId) {
          const existing = execution.value.nodes.get(nodeId);
          execution.value.nodes.set(nodeId, {
            nodeId,
            nodeType: (env.metadata?.node_type as string) ?? existing?.nodeType ?? 'function',
            status: 'completed',
            startTime: existing?.startTime,
            endTime: env.metadata?.end_time as string,
            durationNs: env.metadata?.duration_ns as number,
            stepNumber: env.metadata?.step_number as number,
          });
        }
        break;
      }
      case 'graph_node_error': {
        const nodeId = env.metadata?.node_id as string;
        if (nodeId) {
          const existing = execution.value.nodes.get(nodeId);
          execution.value.nodes.set(nodeId, {
            nodeId,
            nodeType: (env.metadata?.node_type as string) ?? existing?.nodeType ?? 'function',
            status: 'error',
            error: env.metadata?.error as string,
            stepNumber: env.metadata?.step_number as number,
          });
          execution.value.status = 'failed';
        }
        break;
      }
      case 'graph_step': {
        const stepNumber = env.metadata?.step_number as number;
        if (stepNumber !== undefined) {
          execution.value.totalSteps = stepNumber;
        }
        if (env.metadata?.duration_ns) {
          execution.value.durationNs = env.metadata.duration_ns as number;
        }
        break;
      }
      case 'graph_execution_done': {
        execution.value.status = 'completed';
        execution.value.totalSteps = env.metadata?.total_steps as number;
        if (env.metadata?.duration_ns) {
          execution.value.durationNs = env.metadata.duration_ns as number;
        }
        executionSummary.value = parseGraphStreamSummary(env.metadata?.execution_summary);
        break;
      }
      case 'checkpoint': {
        if (env.metadata?.interrupt_key) {
          execution.value.status = 'waiting_human';
          const nodeId = env.metadata?.node_id as string;
          if (nodeId) {
            const existing = execution.value.nodes.get(nodeId);
            execution.value.nodes.set(nodeId, {
              nodeId,
              nodeType: (env.metadata?.node_type as string) ?? existing?.nodeType ?? 'function',
              status: 'interrupted',
              stepNumber: env.metadata?.step_number as number,
            });
          }
          interrupt.value = {
            nodeId: String(env.metadata?.node_id ?? ''),
            interruptKey: String(env.metadata?.interrupt_key ?? ''),
            prompt: parseInterruptPrompt(env.metadata?.interrupt_value),
            checkpointId: String(env.metadata?.checkpoint_id ?? ''),
            lineageId: String(env.metadata?.lineage_id ?? ''),
            interruptValue: env.metadata?.interrupt_value,
          };
        }
        break;
      }
    }
  });

  function clearInterrupt() {
    interrupt.value = null;
  }

  return {
    ...stream,
    execution,
    executionSummary,
    interrupt,
    clearInterrupt,
  };
}
