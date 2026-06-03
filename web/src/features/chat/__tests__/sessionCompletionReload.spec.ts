import { describe, expect, it, vi } from 'vitest';
import { reloadSessionAfterCompletion } from '../sessionCompletionReload';

describe('reloadSessionAfterCompletion (DECO-R-P2-02)', () => {
  it('uses dropStaleInFlight + afterRevision for agent turns and refreshes sessions', async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const loadAgentSessions = vi.fn().mockResolvedValue(undefined);
    const fetchAndReconcileSession = vi.fn().mockResolvedValue(undefined);
    const clear = vi.fn();

    await reloadSessionAfterCompletion({
      sessionStore: {
        entityKind: 'agent',
        selectedTeamId: '',
        loadAgentSessions,
        loadTeamSessions: vi.fn(),
        fetchAndReconcileSession,
      } as never,
      messageStore: {
        loadMessages,
        sessionRevisionBySession: { 'sess-1': 5 },
      } as never,
      streamingSnapshots: { clear } as never,
      sessionId: 'sess-1',
      resolveAgentId: () => 'agent-1',
    });

    expect(loadMessages).toHaveBeenCalledWith({
      sessionId: 'sess-1',
      dropStaleInFlight: true,
      afterRevision: 5,
    });
    expect(clear).toHaveBeenCalledWith('sess-1');
    expect(fetchAndReconcileSession).toHaveBeenCalledWith('sess-1');
    expect(loadAgentSessions).toHaveBeenCalledWith('agent-1', { refreshOnly: true });
  });

  it('falls back to full load when revision is 0', async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const fetchAndReconcileSession = vi.fn().mockResolvedValue(undefined);
    const clear = vi.fn();

    await reloadSessionAfterCompletion({
      sessionStore: {
        entityKind: 'agent',
        selectedTeamId: '',
        loadAgentSessions: vi.fn(),
        loadTeamSessions: vi.fn(),
        fetchAndReconcileSession,
      } as never,
      messageStore: {
        loadMessages,
        sessionRevisionBySession: {},
      } as never,
      streamingSnapshots: { clear } as never,
      sessionId: 'sess-new',
      resolveAgentId: () => 'agent-1',
    });

    expect(loadMessages).toHaveBeenCalledWith({
      sessionId: 'sess-new',
      dropStaleInFlight: true,
      afterRevision: undefined,
    });
    expect(fetchAndReconcileSession).toHaveBeenCalledWith('sess-new');
  });

  it('uses the same incremental path for team turns', async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const loadTeamSessions = vi.fn().mockResolvedValue(undefined);
    const fetchAndReconcileSession = vi.fn().mockResolvedValue(undefined);
    const clear = vi.fn();

    await reloadSessionAfterCompletion({
      sessionStore: {
        entityKind: 'team',
        selectedTeamId: 'team-1',
        loadAgentSessions: vi.fn(),
        loadTeamSessions,
        fetchAndReconcileSession,
      } as never,
      messageStore: {
        loadMessages,
        sessionRevisionBySession: { 'sess-team': 12 },
      } as never,
      streamingSnapshots: { clear } as never,
      sessionId: 'sess-team',
    });

    expect(loadMessages).toHaveBeenCalledWith({
      sessionId: 'sess-team',
      dropStaleInFlight: true,
      afterRevision: 12,
    });
    expect(clear).toHaveBeenCalledWith('sess-team');
    expect(fetchAndReconcileSession).toHaveBeenCalledWith('sess-team');
    expect(loadTeamSessions).toHaveBeenCalledWith('team-1');
  });
});
