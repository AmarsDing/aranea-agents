import { onUnmounted, ref, watch } from "vue";
import { createWsTransport } from "../chat/ws-transport";
import type { Envelope } from "../chat/envelope";

/** Subscribe to knowledge ingest progress over /v1/ws (EP-KN-02). */
export function useKnowledgeIngestWs(collectionId: () => string, onProgress: () => void) {
  const connected = ref(false);
  let transport: ReturnType<typeof createWsTransport> | null = null;

  function disconnect() {
    transport?.disconnect();
    transport = null;
    connected.value = false;
  }

  function connect() {
    const cid = collectionId().trim();
    if (!cid) {
      disconnect();
      return;
    }
    disconnect();
    transport = createWsTransport({
      sessionId: cid,
      logEnabled: false,
      onEnvelope: (env: Envelope) => {
        if (env.type !== "knowledge_ingest") return;
        const meta = env.metadata as Record<string, unknown> | undefined;
        if (String(meta?.collection_id ?? "") !== cid) return;
        onProgress();
      },
      onConnected: () => {
        connected.value = true;
        transport?.subscribe("knowledge");
      },
      onDisconnected: () => {
        connected.value = false;
      }
    });
    transport.connect();
  }

  watch(collectionId, () => connect(), { immediate: true });
  onUnmounted(disconnect);

  return { connected, reconnect: connect, disconnect };
}
