import { onUnmounted, ref, watch } from 'vue';
import { createEnvelopeStream } from '../../realtime/useEnvelopeStream';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import type { ActivityEvent } from '../../realtime/activityEvent';

/** Subscribe to knowledge ingest progress over /v1/ws (EP-KN-02). */
export function useKnowledgeIngestWs(collectionId: () => string, onProgress: () => void) {
  const connected = ref(false);
  let stream: ReturnType<typeof createEnvelopeStream> | null = null;

  function disconnect() {
    stream?.disconnect();
    stream = null;
    connected.value = false;
  }

  function connect() {
    const cid = collectionId().trim();
    if (!cid) {
      disconnect();
      return;
    }
    disconnect();

    // ActivityEvent migration: knowledge_ingest is now published as an
    // ActivityEvent with kind='notice' and meta.event_type='knowledge_ingest'
    // (see internal/service/knowledge.go publishKnowledgeIngest). The backend
    // does not set SessionID (system-domain event), so session-filtered WS
    // connections cannot receive it. We subscribe via the shared global WS hub
    // (session_id=*) which bypasses the session filter, and filter by
    // meta.collection_id client-side to only react to this collection.
    function applyActivityEvent(ev: ActivityEvent) {
      if (ev.activity.kind !== 'notice') return;
      const meta = (ev.activity.meta ?? {}) as Record<string, unknown>;
      if (String(meta.event_type ?? '') !== 'knowledge_ingest') return;
      if (String(meta.collection_id ?? '') !== cid) return;
      onProgress();
    }

    stream = createEnvelopeStream({
      sessionId: GLOBAL_WS_SESSION_ID,
      channels: ['chat', 'system'],
      autoConnect: false,
      onActivityEvent: applyActivityEvent,
      onConnected: () => {
        connected.value = true;
      },
      onDisconnected: () => {
        connected.value = false;
      },
    });
    stream.connect();
  }

  watch(collectionId, () => connect(), { immediate: true });
  onUnmounted(disconnect);

  return { connected, reconnect: connect, disconnect };
}
