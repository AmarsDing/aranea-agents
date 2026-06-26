import { ref } from 'vue';
import { createEnvelopeStream } from '../../realtime/useEnvelopeStream';
import type { ActivityEvent } from '../../realtime/activityEvent';
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

  // ActivityEvent migration: orchestration_agent_status envelopes are now
  // published as ActivityEvent payloads with kind='notice' and
  // stage='orchestration_status' (see internal/team/status_projector.go).
  // The activity.meta carries the same OrchestrationAgentStatusMetadata
  // fields as the legacy envelope metadata.
  function applyActivityEvent(ev: ActivityEvent) {
    if (ev.activity.kind !== 'notice') return;
    if (ev.activity.stage !== 'orchestration_status') return;
    const meta = (ev.activity.meta ?? {}) as OrchestrationAgentStatusMetadata;
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
      onActivityEvent: applyActivityEvent,
    });
    stream.connect();
    connected.value = true;
  }

  return { nodes, connected, seed, disconnect };
}
