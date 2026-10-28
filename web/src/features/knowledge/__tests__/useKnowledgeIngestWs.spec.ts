import { describe, expect, it } from 'vitest';
import { isKnowledgeIngestNotice } from '../useKnowledgeIngestWs';
import type { V2WsEnvelope } from '../../chat/v2Types';

function notice(meta: Record<string, string>, type = 'knowledge_ingest'): V2WsEnvelope {
  return {
    type: 'v2_event',
    kind: 'system.notice',
    payload: { NoticeType: type, Message: '', Meta: meta },
  };
}

describe('isKnowledgeIngestNotice', () => {
  it('accepts matching collection ingest notices', () => {
    expect(isKnowledgeIngestNotice(notice({ collection_id: 'c1' }), 'c1')).toBe(true);
  });

  it('rejects other collections and notice types', () => {
    expect(isKnowledgeIngestNotice(notice({ collection_id: 'c2' }), 'c1')).toBe(false);
    expect(isKnowledgeIngestNotice(notice({ collection_id: 'c1' }, 'other'), 'c1')).toBe(false);
    expect(
      isKnowledgeIngestNotice(
        { type: 'v2_event', kind: 'system.heartbeat', payload: { Message: '', Meta: null } },
        'c1',
      ),
    ).toBe(false);
  });
});
