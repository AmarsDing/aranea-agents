import { ref } from "vue";
import { createEnvelopeStream } from "../../realtime/useEnvelopeStream";
import type { Envelope } from "../../realtime/envelope";
import { ORCHESTRATION_STATUS_ENVELOPE } from "./agentNodeStatusStyles";
import { agentNodeStateFromMetadata, type AgentNodeState, type OrchestrationAgentStatusMetadata } from "./types";

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

  function applyEnvelope(env: Envelope) {
    const meta = (env.metadata ?? {}) as OrchestrationAgentStatusMetadata;
    if (String(meta.run_id ?? "") !== runId) return;
    const state = agentNodeStateFromMetadata(meta);
    if (!state.node_id) return;
    const next = new Map(nodes.value);
    const prev = next.get(state.node_id);
    next.set(state.node_id, {
      ...prev,
      ...state,
      run_id: runId,
      activity_history: state.activity_history?.length
        ? state.activity_history
        : prev?.activity_history,
    });
    nodes.value = next;
  }

  if (sessionId.trim() && runId.trim()) {
    stream = createEnvelopeStream({
      sessionId,
      channels: ["team", "graph", "monitor"],
      autoConnect: false,
    });
    stream.onType([ORCHESTRATION_STATUS_ENVELOPE] as any, applyEnvelope);
    stream.connect();
    connected.value = true;
  }

  return { nodes, connected, seed, disconnect };
}
