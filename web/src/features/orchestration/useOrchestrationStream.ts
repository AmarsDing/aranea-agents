import { ref } from 'vue';
import { createEnvelopeStream } from '../../realtime/useEnvelopeStream';
import type { SystemNoticeEventPayload, V2WsEnvelope } from '../chat/v2Types';
import { agentNodeStateFromMetadata, type AgentNodeState, type OrchestrationAgentStatusMetadata } from './types';

export function useOrchestrationStream(sessionId: string, runId: string) {
  const nodes = ref(new Map<string, AgentNodeState>());
  const connected = ref(false);
  let stream: ReturnType<typeof createEnvelopeStream> | null = null;

  function seed(items: AgentNodeState[]) {
    const next = new Map<string, AgentNodeState>();
    for (const item of items) {
      if (item.node_id) next.set(item.node_id, { ...item, run_id: runId });
    }
    nodes.value = next;
  }

  function disconnect() {
    stream?.disconnect();
    stream = null;
    connected.value = false;
  }

  // Backend: NewSystemNoticeEvent(sessionID, "orchestration_status", "", meta)
  function applyV2(envelope: V2WsEnvelope) {
    if (envelope.kind !== 'system.notice') return;
    const payload = envelope.payload as SystemNoticeEventPayload;
    if (payload.NoticeType !== 'orchestration_status') return;
    const meta = (payload.Meta ?? {}) as OrchestrationAgentStatusMetadata;
    if (String(meta.run_id ?? '') !== runId) return;
    const state = agentNodeStateFromMetadata(meta);
    if (!state.node_id) return;
    const next = new Map(nodes.value);
    const prev = next.get(state.node_id);
    next.set(state.node_id, {
      ...prev,
      ...state,
      run_id: runId,
      activity_history: state.activity_history?.length ? state.activity_history : prev?.activity_history,
    });
    nodes.value = next;
  }

  if (sessionId.trim() && runId.trim()) {
    stream = createEnvelopeStream({
      sessionId,
      channels: ['team', 'graph', 'monitor'],
      autoConnect: false,
      onV2Event: applyV2,
    });
    stream.connect();
    connected.value = true;
  }

  return { nodes, connected, seed, disconnect };
}
