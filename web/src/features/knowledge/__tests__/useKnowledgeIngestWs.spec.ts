import { describe, expect, it } from 'vitest';
import { isKnowledgeIngestNotice } from '../useKnowledgeIngestWs';
import type { V2WsEnvelope } from '../../chat/v2Types';

function notice(meta: Record<string, string>, type = 'knowledge_ingest'): V2WsEnvelope {
  return {
    kind: 'system.notice',
    payload: { NoticeType: type, Meta: meta },
  } as V2WsEnvelope;
}

describe('isKnowledgeIngestNotice', () => {
  it('accepts matching collection ingest notices', () => {
    expect(isKnowledgeIngestNotice(notice({ collection_id: 'c1' }), 'c1')).toBe(true);
  });

  it('rejects other collections and notice types', () => {
    expect(isKnowledgeIngestNotice(notice({ collection_id: 'c2' }), 'c1')).toBe(false);
    expect(isKnowledgeIngestNotice(notice({ collection_id: 'c1' }, 'other'), 'c1')).toBe(false);
    expect(isKnowledgeIngestNotice({ kind: 'chat.delta', payload: {} } as V2WsEnvelope, 'c1')).toBe(false);
  });
});
