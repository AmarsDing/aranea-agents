import { describe, expect, it, vi } from 'vitest';
import type { Envelope } from '../envelope';
import { SESSION_RUN_STATUS } from '../sessionRunStatus';
import { shouldGlobalHubFinalizeTurn, shouldGlobalHubHandleStream } from '../inboundSyncRouting';

function env(partial: Partial<Envelope>): Envelope {
  return {
    id: 'e1',
    type: 'run_status',
    author: 'test',
    session_id: 'sess-1',
    timestamp: '',
    version: 1,
    ...partial,
  };
}

describe('inbound turn-complete routing (DECO-R-P2-03)', () => {
  it('channel turn complete on current session finalizes via global hub', () => {
    const done = env({
      source: 'channel',
      metadata: { status: SESSION_RUN_STATUS.COMPLETED, run_id: 'run-1' },
    });
    expect(shouldGlobalHubFinalizeTurn(true, true, true)).toBe(true);
    expect(shouldGlobalHubHandleStream(true, 'agent', done)).toBe(false);
  });

  it('web turn complete on current session skips global hub finalize', () => {
    expect(shouldGlobalHubFinalizeTurn(false, true, true)).toBe(false);
  });

  it('background channel turn still finalizes via global hub', () => {
    expect(shouldGlobalHubFinalizeTurn(true, false, true)).toBe(true);
  });

  it('T7.3c: completion helpers skip message reload (AF streaming state is final)', async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const { reloadSessionAfterCompletion } = await import('../sessionCompletionReload');
    await reloadSessionAfterCompletion({
      sessionStore: {
        entityKind: 'agent',
        selectedTeamId: '',
        loadSessions: vi.fn().mockResolvedValue(undefined),
        fetchAndReconcileSession: vi.fn().mockResolvedValue(undefined),
      } as never,
      messageStore: {
        loadMessages,
        sessionRevisionBySession: { 'sess-1': 3 },
      } as never,
      streamingSnapshots: { clear: vi.fn() } as never,
      sessionId: 'sess-1',
      resolveAgentId: () => 'agent-1',
    });
    // T7.3c: Legacy reload path removed — loadMessages must NOT be called.
    expect(loadMessages).not.toHaveBeenCalled();
  });
});
