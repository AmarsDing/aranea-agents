import { onUnmounted, ref } from 'vue';
import { createV2EventStream } from '../../realtime/useV2EventStream';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import type { SystemNoticeEventPayload, V2WsEnvelope } from '../chat/v2Types';
import { debounceTrailing } from './timing';

/** G-38 W8：SourceSync 轮次帧过滤（NoticeType=knowledge_source_sync；
 *  connectorId 空串=不过滤——面板列表页收全部连接器帧）。 */
export function isSourceSyncNotice(envelope: V2WsEnvelope, connectorId = ''): boolean {
  if (envelope.kind !== 'system.notice') return false;
  const payload = envelope.payload as SystemNoticeEventPayload;
  if (payload.NoticeType !== 'knowledge_source_sync') return false;
  if (!connectorId) return true;
  return String(payload.Meta?.connector_id ?? '') === connectorId;
}

/** Subscribe to source-sync lifecycle frames over /v1/ws（useKnowledgeIngestWs 同款：
 *  GLOBAL 会话 + system channel + 800ms 尾触发防抖——一轮 started/finished 双帧
 *  收敛为单次列表刷新）。 */
export function useSourceSyncWs(connectorId: () => string, onProgress: () => void) {
  const connected = ref(false);
  let stream: ReturnType<typeof createV2EventStream> | null = null;

  const debouncedProgress = debounceTrailing(onProgress, 800);

  function disconnect() {
    stream?.disconnect();
    stream = null;
    connected.value = false;
    debouncedProgress.cancel();
  }

  function connect() {
    disconnect();
    stream = createV2EventStream({
      sessionId: GLOBAL_WS_SESSION_ID,
      channels: ['chat', 'system'],
      autoConnect: false,
      onV2Event: (envelope: V2WsEnvelope) => {
        if (!isSourceSyncNotice(envelope, connectorId())) return;
        debouncedProgress.call();
      },
      onConnected: () => {
        connected.value = true;
      },
      onDisconnected: () => {
        connected.value = false;
      },
    });
    stream.connect();
  }

  connect();
  onUnmounted(disconnect);

  return { connected, reconnect: connect, disconnect };
}
