import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatSessionStore } from '../chat/sessionStore';

vi.mock('../../features/session/api', () => ({
  listSessions: vi.fn().mockResolvedValue([]),
  listTeamSessions: vi.fn().mockResolvedValue([]),
  createSession: vi.fn(),
  deleteSession: vi.fn().mockResolvedValue(undefined),
  getSession: vi.fn(),
  updateSessionTitle: vi.fn(),
  clearAgentSessions: vi.fn().mockResolvedValue(undefined),
  compactSession: vi.fn(),
  pinSession: vi.fn(),
  unpinSession: vi.fn(),
}));

vi.mock('../sessionSync', () => ({
  emitSessionMutation: vi.fn(),
  onSessionMutation: vi.fn().mockReturnValue(() => {}),
}));

vi.mock('../../features/chat/sessionContextPatch', () => ({
  reconcilePatchFromServer: vi.fn().mockReturnValue({}),
}));

vi.mock('../../features/chat/composables/chatWorkspaceUtils', () => ({
  formatSessionTime: vi.fn((v: string) => v || ''),
}));

vi.mock('../../features/session/sessionSort', () => ({
  sortSessionsForDisplay: vi.fn(<T>(rows: T[]) => rows),
}));

const mockSession = (overrides: Record<string, unknown> = {}): any => ({
  id: 's1',
  owner_type: 'agent',
  agent_id: 'agent-1',
  team_id: '',
  title: 'Test Session',
  summary: '',
  context_used_ratio: 0,
  max_context_used_ratio: 0,
  context_status: '',
  dialog_mode: '',
  provider: '',
  model: '',
  status: 'idle',
  status_reason: '',
  status_changed_at: '',
  message_count: 0,
  run_count: 0,
  model_call_count: 0,
  tool_call_count: 0,
  skill_call_count: 0,
  mcp_call_count: 0,
  input_tokens: 0,
  output_tokens: 0,
  total_tokens: 0,
  total_cost_micro_usd: 0,
  last_message_at: '2025-01-01T00:00:00Z',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
  archived_at: '',
  deleted_at: '',
  pinned_at: '',
  ...overrides,
});

describe('useChatSessionStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('initialises with empty state', () => {
    const store = useChatSessionStore();
    expect(store.sessions).toEqual([]);
    expect(store.selectedSession).toBeNull();
    expect(store.teamSessions).toEqual({});
    expect(store.error).toBeNull();
    expect(store.entityKind).toBe('agent');
    expect(store.selectedTeamId).toBeNull();
    expect(store.teamSelectedSessionId).toBeNull();
  });

  it('loadAgentSessions populates sessions and auto-selects first', async () => {
    const { listSessions } = await import('../../features/session/api');
    const s1 = mockSession({ id: 's1' });
    const s2 = mockSession({ id: 's2' });
    (listSessions as ReturnType<typeof vi.fn>).mockResolvedValueOnce([s1, s2]);

    const store = useChatSessionStore();
    await store.loadAgentSessions('agent-1');

    expect(store.sessions).toHaveLength(2);
    expect(store.selectedSession).toEqual(s1);
    expect(store.error).toBeNull();
  });

  it('loadAgentSessions sets error on failure', async () => {
    const { listSessions } = await import('../../features/session/api');
    (listSessions as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network'));

    const store = useChatSessionStore();
    await expect(store.loadAgentSessions('agent-1')).rejects.toThrow('network');
    expect(store.error).toBe('network');
  });

  it('addAgentSession prepends and selects created session', async () => {
    const { createSession } = await import('../../features/session/api');
    const { emitSessionMutation } = await import('../sessionSync');
    const created = mockSession({ id: 'new-1' });
    (createSession as ReturnType<typeof vi.fn>).mockResolvedValueOnce(created);

    const store = useChatSessionStore();
    const result = await store.addAgentSession('agent-1', 'New Chat');

    expect(result).toEqual(created);
    expect(store.sessions[0]).toEqual(created);
    expect(store.selectedSession).toEqual(created);
    expect(emitSessionMutation).toHaveBeenCalledWith({ type: 'update', id: 'new-1', session: created });
  });

  it('removeSessionLocal removes session and emits mutation', async () => {
    const { deleteSession } = await import('../../features/session/api');
    const { emitSessionMutation } = await import('../sessionSync');

    const store = useChatSessionStore();
    store.sessions = [mockSession({ id: 's1' }), mockSession({ id: 's2' })] as any;
    store.selectedSession = store.sessions[0];

    await store.removeSessionLocal('s1');

    expect(deleteSession).toHaveBeenCalledWith('s1');
    expect(store.sessions).toHaveLength(1);
    expect(store.sessions[0].id).toBe('s2');
    expect(emitSessionMutation).toHaveBeenCalledWith({ type: 'remove', id: 's1' });
  });

  it('removeSessionLocal falls back to next session when selected is removed', async () => {
    const store = useChatSessionStore();
    store.sessions = [mockSession({ id: 's1' }), mockSession({ id: 's2' })] as any;
    store.selectedSession = store.sessions[0];

    await store.removeSessionLocal('s1');

    expect(store.selectedSession?.id).toBe('s2');
  });

  it('renameSessionLocal updates session and emits mutation', async () => {
    const { updateSessionTitle } = await import('../../features/session/api');
    const { emitSessionMutation } = await import('../sessionSync');
    const updated = mockSession({ id: 's1', title: 'Renamed' });
    (updateSessionTitle as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updated);

    const store = useChatSessionStore();
    store.sessions = [mockSession({ id: 's1', title: 'Old' })] as any;
    store.selectedSession = store.sessions[0];

    const result = await store.renameSessionLocal('s1', 'Renamed');

    expect(result.title).toBe('Renamed');
    expect(emitSessionMutation).toHaveBeenCalledWith({ type: 'update', id: 's1', session: updated });
  });

  it('clearAllAgentSessions clears sessions and emits refresh', async () => {
    const { clearAgentSessions } = await import('../../features/session/api');
    const { emitSessionMutation } = await import('../sessionSync');

    const store = useChatSessionStore();
    store.sessions = [mockSession({ id: 's1' })] as any;
    store.selectedSession = store.sessions[0];

    await store.clearAllAgentSessions('agent-1');

    expect(clearAgentSessions).toHaveBeenCalledWith('agent-1');
    expect(store.sessions).toEqual([]);
    expect(store.selectedSession).toBeNull();
    expect(emitSessionMutation).toHaveBeenCalledWith({ type: 'refresh' });
  });

  it('setSessionPinnedLocal pins and emits update', async () => {
    const { pinSession } = await import('../../features/session/api');
    const { emitSessionMutation } = await import('../sessionSync');
    const pinned = mockSession({ id: 's1', pinned_at: '2025-01-01T00:00:00Z' });
    (pinSession as ReturnType<typeof vi.fn>).mockResolvedValueOnce(pinned);

    const store = useChatSessionStore();
    store.sessions = [mockSession({ id: 's1' })] as any;

    const result = await store.setSessionPinnedLocal('s1', true);

    expect(result.pinned_at).toBe('2025-01-01T00:00:00Z');
    expect(emitSessionMutation).toHaveBeenCalledWith({ type: 'update', id: 's1', session: pinned });
  });

  it('resetForAgentSwitch resets to agent mode', () => {
    const store = useChatSessionStore();
    store.entityKind = 'team';
    store.selectedTeamId = 'team-1';
    store.teamSelectedSessionId = 'ts1';

    store.resetForAgentSwitch();

    expect(store.entityKind).toBe('agent');
    expect(store.selectedTeamId).toBeNull();
    expect(store.teamSelectedSessionId).toBeNull();
  });

  it('resetForTeamSwitch sets team mode', () => {
    const store = useChatSessionStore();
    store.selectedSession = mockSession({ id: 's1' });

    store.resetForTeamSwitch('team-2');

    expect(store.entityKind).toBe('team');
    expect(store.selectedTeamId).toBe('team-2');
    expect(store.selectedSession).toBeNull();
    expect(store.teamSelectedSessionId).toBeNull();
  });

  it('currentSessionId returns selected session id in agent mode', () => {
    const store = useChatSessionStore();
    store.selectedSession = mockSession({ id: 's1' });

    expect(store.currentSessionId()).toBe('s1');
  });

  it('currentSessionId returns teamSelectedSessionId in team mode', () => {
    const store = useChatSessionStore();
    store.entityKind = 'team';
    store.teamSelectedSessionId = 'ts1';

    expect(store.currentSessionId()).toBe('ts1');
  });

  it('findSessionById finds from agent sessions', () => {
    const store = useChatSessionStore();
    store.sessions = [mockSession({ id: 's1' })] as any;

    expect(store.findSessionById('s1')).toBeDefined();
    expect(store.findSessionById('nonexistent')).toBeUndefined();
  });

  it('loadAgentSessions returns early when agentId is empty', async () => {
    const { listSessions } = await import('../../features/session/api');
    const store = useChatSessionStore();
    await store.loadAgentSessions('');
    expect(listSessions).not.toHaveBeenCalled();
  });

  it('addAgentSession returns null when agentId is empty', async () => {
    const store = useChatSessionStore();
    const result = await store.addAgentSession('', 'Title');
    expect(result).toBeNull();
  });

  // P0 #4: 验证快速切换 Agent↔Team 时，旧请求不污染状态
  it('loadAgentSessions is invalidated by resetForTeamSwitch during in-flight request', async () => {
    const { listSessions } = await import('../../features/session/api');
    const s1 = mockSession({ id: 's1' });
    let resolveList: (rows: any[]) => void = () => {};
    (listSessions as ReturnType<typeof vi.fn>).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveList = resolve;
      }),
    );

    const store = useChatSessionStore();
    const loadPromise = store.loadAgentSessions('agent-1');

    // 在 loadAgentSessions 在途时切换到 Team 模式
    store.resetForTeamSwitch('team-1');
    expect(store.entityKind).toBe('team');

    // 现在 resolve 旧的 loadAgentSessions 请求
    resolveList([s1]);
    await loadPromise;

    // 旧请求不应污染 sessions / selectedSession（已切换到 team 模式）
    expect(store.sessions).toEqual([]);
    expect(store.selectedSession).toBeNull();
  });

  it('loadTeamSessions is invalidated by resetForAgentSwitch during in-flight request', async () => {
    const { listTeamSessions } = await import('../../features/session/api');
    let resolveList: (rows: any[]) => void = () => {};
    (listTeamSessions as ReturnType<typeof vi.fn>).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveList = resolve;
      }),
    );

    const store = useChatSessionStore();
    store.resetForTeamSwitch('team-1');
    const loadPromise = store.loadTeamSessions('team-1');

    // 在途时切回 Agent 模式
    store.resetForAgentSwitch();
    expect(store.entityKind).toBe('agent');

    resolveList([mockSession({ id: 'ts1', team_id: 'team-1' })]);
    await loadPromise;

    // 旧请求不应写入 teamSessions
    expect(store.teamSessions['team-1']).toBeUndefined();
  });
});
