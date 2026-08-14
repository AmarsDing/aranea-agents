import { onUnmounted, ref, watch } from 'vue';
import { createV2EventStream } from '../../realtime/useV2EventStream';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import type { SystemNoticeEventPayload, V2WsEnvelope } from '../chat/v2Types';

/** Subscribe to knowledge ingest progress over /v1/ws (EP-KN-02). */
export function useKnowledgeIngestWs(collectionId: () => string, onProgress: () => void) {
  const connected = ref(false);
  let stream: ReturnType<typeof createV2EventStream> | null = null;

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

    // Backend publishes NewSystemNoticeEvent("", "knowledge_ingest", …).
    // Subscribe via GLOBAL_WS_SESSION_ID and filter by Meta.collection_id.
    function applyV2(envelope: V2WsEnvelope) {
      if (envelope.kind !== 'system.notice') return;
      const payload = envelope.payload as SystemNoticeEventPayload;
      if (payload.NoticeType !== 'knowledge_ingest') return;
      const meta = payload.Meta ?? {};
      if (String(meta.collection_id ?? '') !== cid) return;
      onProgress();
    }

    stream = createV2EventStream({
      sessionId: GLOBAL_WS_SESSION_ID,
      channels: ['chat', 'system'],
      autoConnect: false,
      onV2Event: applyV2,
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
