import { describe, expect, it, vi } from 'vitest';
import { reloadSessionAfterCompletion } from '../sessionCompletionReload';

/**
 * T7.3c: Legacy reload path removed. AF mode is the only path.
 * Activity events are the source of truth for assistant content — the
 * streaming state IS the final state, so no message reload is needed.
 * Only session metadata (token usage, context ratio, session list) is refreshed.
 */
describe('reloadSessionAfterCompletion (DECO-R-P2-02)', () => {
  it('refreshes session metadata + agent session list (no message reload)', async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const loadSessions = vi.fn().mockResolvedValue(undefined);
    const fetchAndReconcileSession = vi.fn().mockResolvedValue(undefined);
    const clear = vi.fn();

    await reloadSessionAfterCompletion({
      sessionStore: {
        entityKind: 'agent',
        selectedTeamId: '',
        loadSessions,
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

    // T7.3c: Legacy message reload removed — AF streaming state is final.
    expect(loadMessages).not.toHaveBeenCalled();
    expect(clear).not.toHaveBeenCalled();
    // Session metadata + session list are still refreshed.
    expect(fetchAndReconcileSession).toHaveBeenCalledWith('sess-1');
    expect(loadSessions).toHaveBeenCalledWith('agent-1', { refreshOnly: true });
  });

  it('skips session list refresh when agentId is empty', async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const loadSessions = vi.fn().mockResolvedValue(undefined);
    const fetchAndReconcileSession = vi.fn().mockResolvedValue(undefined);

    await reloadSessionAfterCompletion({
      sessionStore: {
        entityKind: 'agent',
        selectedTeamId: '',
        loadSessions,
        fetchAndReconcileSession,
      } as never,
      messageStore: {
        loadMessages,
        sessionRevisionBySession: {},
      } as never,
      streamingSnapshots: { clear: vi.fn() } as never,
      sessionId: 'sess-new',
      resolveAgentId: () => '',
    });

    expect(loadMessages).not.toHaveBeenCalled();
    expect(fetchAndReconcileSession).toHaveBeenCalledWith('sess-new');
    expect(loadSessions).not.toHaveBeenCalled();
  });

  it('refreshes team session list for team turns', async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const loadSessions = vi.fn().mockResolvedValue(undefined);
    const fetchAndReconcileSession = vi.fn().mockResolvedValue(undefined);

    await reloadSessionAfterCompletion({
      sessionStore: {
        entityKind: 'team',
        selectedTeamId: 'team-1',
        loadSessions,
        fetchAndReconcileSession,
      } as never,
      messageStore: {
        loadMessages,
        sessionRevisionBySession: { 'sess-team': 12 },
      } as never,
      streamingSnapshots: { clear: vi.fn() } as never,
      sessionId: 'sess-team',
    });

    // T7.3c: Legacy message reload removed — AF streaming state is final.
    expect(loadMessages).not.toHaveBeenCalled();
    expect(fetchAndReconcileSession).toHaveBeenCalledWith('sess-team');
    expect(loadSessions).toHaveBeenCalledWith('team-1');
  });

  it('no-ops when sessionId is empty', async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const loadSessions = vi.fn().mockResolvedValue(undefined);
    const fetchAndReconcileSession = vi.fn().mockResolvedValue(undefined);

    await reloadSessionAfterCompletion({
      sessionStore: {
        entityKind: 'agent',
        selectedTeamId: '',
        loadSessions,
        fetchAndReconcileSession,
      } as never,
      messageStore: {
        loadMessages,
        sessionRevisionBySession: {},
      } as never,
      streamingSnapshots: { clear: vi.fn() } as never,
      sessionId: '  ',
    });

    expect(loadMessages).not.toHaveBeenCalled();
    expect(fetchAndReconcileSession).not.toHaveBeenCalled();
    expect(loadSessions).not.toHaveBeenCalled();
  });
});
