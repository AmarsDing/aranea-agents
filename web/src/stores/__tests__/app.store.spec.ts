import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useAppStore } from '../app';
import { useChatSessionStore } from '../chat/sessionStore';
import { useChatMessageStore } from '../chat/messageStore';
import { useChatRuntimeStore } from '../chat/runtimeStore';

vi.mock('../../features/session/api', () => ({
  listSessions: vi.fn().mockResolvedValue([]),
  createSession: vi.fn().mockResolvedValue({ id: 'sess-1', title: 'Test Session' }),
  deleteSession: vi.fn().mockResolvedValue(undefined),
  clearAgentSessions: vi.fn().mockResolvedValue(undefined),
  updateSessionTitle: vi.fn().mockResolvedValue({ id: 'sess-1', title: 'Renamed' }),
  listSessionChatMessages: vi.fn().mockResolvedValue({ items: [], currentRevision: 0 }),
  listTeamSessions: vi.fn().mockResolvedValue([]),
}));

vi.mock('../../features/agents/api', () => ({
  listAgents: vi.fn().mockResolvedValue([{ id: 'agent-1', agent_key: 'test-agent', display_name: 'Test Agent' }]),
  createAgent: vi.fn().mockResolvedValue({ id: 'agent-2', agent_key: 'new', display_name: 'New' }),
  updateAgent: vi.fn().mockImplementation((_id, patch) => Promise.resolve(patch)),
  deleteAgent: vi.fn().mockResolvedValue(undefined),
}));

describe('useAppStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('initialises with empty state', () => {
    const store = useAppStore();
    expect(store.agents).toEqual([]);
    expect(store.selectedAgent).toBeNull();
  });

  it('loadAgents populates agents and selects the first', async () => {
    const store = useAppStore();
    await store.loadAgents();
    expect(store.agents).toHaveLength(1);
    expect(store.selectedAgent?.id).toBe('agent-1');
  });

  it('upsertAgent updates existing agent in-place', async () => {
    const store = useAppStore();
    await store.loadAgents();
    store.upsertAgent({ id: 'agent-1', agent_key: 'test-agent', display_name: 'Updated' } as any);
    expect(store.agents[0].display_name).toBe('Updated');
  });

  it('removeAgentFromList removes and clears selection', async () => {
    const store = useAppStore();
    const session = useChatSessionStore();
    await store.loadAgents();
    await store.removeAgentFromList('agent-1');
    expect(store.agents).toHaveLength(0);
    expect(store.selectedAgent).toBeNull();
    expect(session.sessions).toEqual([]);
  });

  it('addAgent creates agent and prepends to list', async () => {
    const store = useAppStore();
    await store.loadAgents();
    const created = await store.addAgent({ agent_key: 'new', display_name: 'New' } as any);
    expect(created?.id).toBe('agent-2');
    expect(store.agents[0].id).toBe('agent-2');
    expect(store.selectedAgent?.id).toBe('agent-2');
  });
});

describe('chat sub-stores', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('stores messages by session id', async () => {
    const message = useChatMessageStore();
    message.setMessages('sess-1', [{ id: 'm1' } as any]);
    expect(message.getMessages('sess-1')).toHaveLength(1);
  });

  it('tracks ws connected per session', () => {
    const runtime = useChatRuntimeStore();
    runtime.setWsConnected('sess-1', true);
    expect(runtime.isWsConnected('sess-1')).toBe(true);
    expect(runtime.isWsConnected('sess-2')).toBe(false);
  });

  it('clearTeamSessions only removes that team session caches', () => {
    const session = useChatSessionStore();
    const message = useChatMessageStore();
    session.teamSessions['team-1'] = [{ id: 't-s1', at: '' } as any, { id: 't-s2', at: '' } as any];
    session.teamSessions['team-2'] = [{ id: 't-s3', at: '' } as any];
    message.setMessages('t-s1', [{ id: 'm1' } as any]);
    message.setMessages('t-s2', [{ id: 'm2' } as any]);
    message.setMessages('t-s3', [{ id: 'm3' } as any]);
    message.setMessages('agent-s1', [{ id: 'm4' } as any]);
    (session as any).teamSelectedSessionId = 't-s1';

    session.clearTeamSessions('team-1');
    const runtime = useChatRuntimeStore();
    for (const sid of ['t-s1', 't-s2']) {
      message.deleteSessionMessages(sid);
      runtime.deleteSessionRuntime(sid);
    }

    expect(session.teamSessions['team-1']).toEqual([]);
    expect(session.teamSelectedSessionId).toBeNull();
    expect(message.getMessages('t-s1')).toEqual([]);
    expect(message.getMessages('t-s2')).toEqual([]);
    expect(message.getMessages('t-s3')).toHaveLength(1);
    expect(message.getMessages('agent-s1')).toHaveLength(1);
  });

  it('clearAllAgentSessions only clears that agent session caches', async () => {
    const { clearAgentSessions } = await import('../../features/session/api');
    const session = useChatSessionStore();
    const message = useChatMessageStore();
    const runtime = useChatRuntimeStore();
    (session as any).sessions = [{ id: 'agent-s1', title: 'A' } as any, { id: 'agent-s2', title: 'B' } as any];
    message.setMessages('agent-s1', [{ id: 'm1' } as any]);
    message.setMessages('agent-s2', [{ id: 'm2' } as any]);
    message.setMessages('team-s1', [{ id: 'm3' } as any]);
    message.sessionRevisionBySession['agent-s1'] = 3;
    runtime.wsConnectedBySession['agent-s2'] = true;

    const sessionIds = session.sessions.map((s) => s.id);
    await session.clearAllAgentSessions('agent-1');
    for (const sid of sessionIds) {
      message.deleteSessionMessages(sid);
      runtime.deleteSessionRuntime(sid);
    }

    expect(clearAgentSessions).toHaveBeenCalledWith('agent-1');
    expect(session.sessions).toEqual([]);
    expect(session.selectedSession).toBeNull();
    expect(message.getMessages('agent-s1')).toEqual([]);
    expect(message.getMessages('agent-s2')).toEqual([]);
    expect(message.sessionRevisionBySession['agent-s1']).toBeUndefined();
    expect(runtime.wsConnectedBySession['agent-s2']).toBeUndefined();
    expect(message.getMessages('team-s1')).toHaveLength(1);
  });

  it('removeTeamSessionLocal deletes session and clears caches', async () => {
    const { deleteSession } = await import('../../features/session/api');
    const session = useChatSessionStore();
    const message = useChatMessageStore();
    const runtime = useChatRuntimeStore();
    session.teamSessions['team-1'] = [{ id: 't-s1', at: '' } as any, { id: 't-s2', at: '' } as any];
    (session as any).teamSelectedSessionId = 't-s1';
    message.setMessages('t-s1', [{ id: 'm1' } as any]);
    message.sessionRevisionBySession['t-s1'] = 2;
    runtime.wsConnectedBySession['t-s1'] = true;

    await session.removeTeamSessionLocal('team-1', 't-s1');
    message.deleteSessionMessages('t-s1');
    runtime.deleteSessionRuntime('t-s1');

    expect(deleteSession).toHaveBeenCalledWith('t-s1');
    expect(session.teamSessions['team-1']).toHaveLength(1);
    expect(session.teamSessions['team-1']![0].id).toBe('t-s2');
    expect(session.teamSelectedSessionId).toBe('t-s2');
    expect(message.getMessages('t-s1')).toEqual([]);
    expect(message.sessionRevisionBySession['t-s1']).toBeUndefined();
    expect(runtime.wsConnectedBySession['t-s1']).toBeUndefined();
  });
});
