import { describe, expect, it } from 'vitest';
import { isSourceSyncNotice } from '../useSourceSyncWs';
import type { V2WsEnvelope } from '../../chat/v2Types';

function notice(meta: Record<string, unknown>, type = 'knowledge_source_sync'): V2WsEnvelope {
  return {
    type: 'v2_event',
    kind: 'system.notice',
    payload: { NoticeType: type, Message: '', Meta: meta as never },
  };
}

describe('isSourceSyncNotice', () => {
  it('accepts connector-scoped frames', () => {
    expect(isSourceSyncNotice(notice({ connector_id: 'c1' }), 'c1')).toBe(true);
    expect(isSourceSyncNotice(notice({ connector_id: 'c2' }), 'c1')).toBe(false);
  });

  it('empty scope accepts all connectors (panel list view)', () => {
    expect(isSourceSyncNotice(notice({ connector_id: 'c1' }), '')).toBe(true);
  });

  it('rejects other notice types and non-notice kinds', () => {
    expect(isSourceSyncNotice(notice({ connector_id: 'c1' }, 'knowledge_ingest'), '')).toBe(false);
    expect(
      isSourceSyncNotice({ type: 'v2_event', kind: 'system.heartbeat', payload: { Message: '', Meta: null } }, ''),
    ).toBe(false);
  });
});
