/**
 * Graph execution WS stream — consumes v2 `system.notice` directly.
 * Backend graph bridge publishes NoticeType = stage (node_start/end/…)
 * with Meta.filter_key = graph/{graphId}/{execId}.
 */
import { ref } from 'vue';
import { useEnvelopeStream } from '../../../realtime/useEnvelopeStream';
import type { SystemNoticeEventPayload, V2WsEnvelope } from '../../chat/v2Types';
import type {
  GraphExecutionState,
  GraphStreamExecutionSummary,
  GraphStreamInterrupt,
} from '../../../realtime/graphState';

export type GraphNoticeListener = (noticeType: string, meta: Record<string, unknown>) => void;

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

function resolveStage(noticeType: string, meta: Record<string, unknown>): string {
  if (noticeType === 'graph_task_status') return 'task_status';
  if (noticeType === 'checkpoint' || meta.interrupt_key != null) return 'checkpoint';
  return noticeType;
}

export function useGraphStream(sessionId: string, graphId: string, execId: string) {
  const noticeListeners: GraphNoticeListener[] = [];

  const execution = ref<GraphExecutionState>({
    executionId: execId,
    graphId,
    status: 'pending',
    nodes: new Map(),
  });

  const executionSummary = ref<GraphStreamExecutionSummary | null>(null);
  const interrupt = ref<GraphStreamInterrupt | null>(null);

  const filterKey = `graph/${graphId}/${execId}`;

  function matchesFilter(meta: Record<string, unknown>): boolean {
    const metaFilterKey = meta.filter_key;
    if (typeof metaFilterKey === 'string' && metaFilterKey !== '' && !metaFilterKey.startsWith(filterKey)) {
      return false;
    }
    return true;
  }

  function notifyListeners(noticeType: string, meta: Record<string, unknown>) {
    for (const listener of noticeListeners) {
      listener(noticeType, meta);
    }
  }

  function handleGraphStage(stage: string, meta: Record<string, unknown>) {
    switch (stage) {
      case 'node_start': {
        const nodeId = String(meta.node_id ?? '');
        const nodeTypeRaw = typeof meta.node_type === 'string' ? meta.node_type : '';
        const stepNumber = Number(meta.step_number ?? 0);
        if (nodeId) {
          const existing = execution.value.nodes.get(nodeId);
          execution.value.nodes.set(nodeId, {
            nodeId,
            nodeType: nodeTypeRaw || existing?.nodeType || 'function',
            status: 'running',
            startTime: typeof meta.start_time === 'string' ? meta.start_time : undefined,
            stepNumber,
          });
          execution.value.currentNode = nodeId;
          execution.value.status = 'running';
        }
        break;
      }
      case 'node_end': {
        const nodeId = String(meta.node_id ?? '');
        if (nodeId) {
          const existing = execution.value.nodes.get(nodeId);
          execution.value.nodes.set(nodeId, {
            nodeId,
            nodeType: typeof meta.node_type === 'string' ? meta.node_type : (existing?.nodeType ?? 'function'),
            status: 'completed',
            startTime: existing?.startTime,
            endTime: typeof meta.end_time === 'string' ? meta.end_time : undefined,
            durationNs: typeof meta.duration_ns === 'number' ? meta.duration_ns : undefined,
            stepNumber: Number(meta.step_number ?? 0),
          });
        }
        break;
      }
      case 'node_error': {
        const nodeId = String(meta.node_id ?? '');
        if (nodeId) {
          const existing = execution.value.nodes.get(nodeId);
          execution.value.nodes.set(nodeId, {
            nodeId,
            nodeType: typeof meta.node_type === 'string' ? meta.node_type : (existing?.nodeType ?? 'function'),
            status: 'error',
            error: typeof meta.error === 'string' ? meta.error : undefined,
            stepNumber: Number(meta.step_number ?? 0),
          });
          execution.value.status = 'failed';
        }
        break;
      }
      case 'step': {
        const stepNumber = Number(meta.step_number ?? 0);
        if (Number.isFinite(stepNumber) && stepNumber > 0) {
          execution.value.totalSteps = stepNumber;
        }
        if (typeof meta.duration_ns === 'number') {
          execution.value.durationNs = meta.duration_ns;
        }
        break;
      }
      case 'execution_done': {
        execution.value.status = 'completed';
        const totalSteps = Number(meta.total_steps ?? 0);
        if (Number.isFinite(totalSteps)) {
          execution.value.totalSteps = totalSteps;
        }
        if (typeof meta.duration_ns === 'number') {
          execution.value.durationNs = meta.duration_ns;
        }
        executionSummary.value = parseGraphStreamSummary(meta.execution_summary);
        break;
      }
      default:
        break;
    }
  }

  function handleCheckpoint(meta: Record<string, unknown>) {
    if (!meta.interrupt_key) return;
    execution.value.status = 'waiting_human';
    const nodeId = String(meta.node_id ?? '');
    if (nodeId) {
      const existing = execution.value.nodes.get(nodeId);
      execution.value.nodes.set(nodeId, {
        nodeId,
        nodeType: typeof meta.node_type === 'string' ? meta.node_type : (existing?.nodeType ?? 'function'),
        status: 'interrupted',
        stepNumber: Number(meta.step_number ?? 0),
      });
    }
    interrupt.value = {
      nodeId: String(meta.node_id ?? ''),
      interruptKey: String(meta.interrupt_key ?? ''),
      prompt: parseInterruptPrompt(meta.interrupt_value),
      checkpointId: String(meta.checkpoint_id ?? ''),
      lineageId: String(meta.lineage_id ?? ''),
      interruptValue: meta.interrupt_value,
    };
  }

  function handleV2Event(envelope: V2WsEnvelope) {
    if (envelope.kind !== 'system.notice') return;
    const payload = envelope.payload as SystemNoticeEventPayload;
    const noticeType = String(payload.NoticeType ?? '');
    const meta = { ...(payload.Meta ?? {}) } as Record<string, unknown>;
    if (!matchesFilter(meta)) return;

    const stage = resolveStage(noticeType, meta);
    const kind = String(meta.activity_kind ?? '');

    if (stage === 'checkpoint' || (kind === 'session' && stage === 'checkpoint')) {
      handleCheckpoint(meta);
      notifyListeners(stage, meta);
      return;
    }
    if (kind === 'graph_stage' || !kind || stage === 'task_status') {
      handleGraphStage(stage, meta);
      notifyListeners(stage, meta);
    }
  }

  const stream = useEnvelopeStream({
    sessionId,
    channels: ['chat', 'graph', 'system'],
    onV2Event: handleV2Event,
  });

  function clearInterrupt() {
    interrupt.value = null;
  }

  /** Register a listener after internal state updates (graph_stage / checkpoint). */
  function onGraphNotice(listener: GraphNoticeListener): void {
    noticeListeners.push(listener);
  }

  return {
    ...stream,
    execution,
    executionSummary,
    interrupt,
    clearInterrupt,
    onGraphNotice,
  };
}
