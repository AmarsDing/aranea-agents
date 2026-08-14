import { onUnmounted } from 'vue';
import { createV2EventStream } from '../../realtime/useV2EventStream';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import type { SystemNoticeEventPayload, V2WsEnvelope } from '../chat/v2Types';
import { parseGraphDeltaMeta, type KnowledgeGraphDelta } from './graphDelta';

/**
 * SP1-D/I-4：订阅 knowledge.graph.delta WS 增量（EP-KN-02 同款 SystemNotice 通道）。
 * 事件分级 Informational（WS-only 不持久化，丢失可容忍）；到达后由调用方失效
 * 反链/悬空链缓存并重载受影响视图。页面级单订阅（KnowledgePage 挂载期间存活）。
 */
export function useKnowledgeGraphDeltaWs(onDelta: (delta: KnowledgeGraphDelta) => void) {
  let stream: ReturnType<typeof createV2EventStream> | null = null;

  function applyV2(envelope: V2WsEnvelope) {
    if (envelope.kind !== 'system.notice') return;
    const payload = envelope.payload as SystemNoticeEventPayload;
    if (payload.NoticeType !== 'knowledge.graph.delta') return;
    const delta = parseGraphDeltaMeta(payload.Meta);
    if (!delta || (!delta.added.length && !delta.removed.length)) return;
    onDelta(delta);
  }

  stream = createV2EventStream({
    sessionId: GLOBAL_WS_SESSION_ID,
    channels: ['chat', 'system'],
    autoConnect: true,
    onV2Event: applyV2,
  });

  onUnmounted(() => {
    stream?.disconnect();
    stream = null;
  });
}
