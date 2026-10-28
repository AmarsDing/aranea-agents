import { beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope, ref } from 'vue';
import { useChatEntityNav, type EntityNavDeps } from '../useChatEntityNav';
import { createChatFocusCoordinator } from '../../chatFocusCoordinator';
import type { Session } from '../../../session/types';
import type { Agent } from '../../../agents/types';

const fixture = vi.hoisted(() => ({
  fetch: vi.fn(),
  hydrate: vi.fn(),
  router: {
    currentRoute: { value: { name: 'task-workspace', path: '/task-workspace', query: {} as Record<string, string> } },
    replace: vi.fn(),
    push: vi.fn(),
  },
}));
vi.mock('vue-router', () => ({ useRouter: () => fixture.router }));
vi.mock('quasar', () => ({ useQuasar: () => ({ notify: vi.fn() }) }));
vi.mock('../../../../stores/session', () => ({ useSessionStore: () => ({ fetchSession: fixture.fetch }) }));
vi.mock('../../../../stores/teams', () => ({ useTeamsStore: () => ({}) }));
vi.mock('../../../../stores/platform', () => ({ usePlatformStore: () => ({}) }));
vi.mock('../../agentPlannerSettings', () => ({ hydrateAgentSettings: async (agent: Agent) => agent }));
vi.mock('../../channelFocusLoad', () => ({ hydrateSessionForChannelFocus: fixture.hydrate }));
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}
const session = (id: string) => ({ id, agent_id: 'agent', owner_type: 'agent' }) as Session;
function setup() {
  const agent = { id: 'agent', agent_key: 'specialist' } as Agent;
  const deps = {
    appStore: { selectedAgent: agent, agents: [agent], upsertAgent: vi.fn() },
    sessionStore: {
      entityKind: 'agent',
      sessions: [session('A'), session('B')],
      selectedSession: session('A'),
      findSessionById: () => null,
      loadAgentSessions: vi.fn().mockResolvedValue(undefined),
    },
    messageStore: {},
    streamManager: {},
    focusCoordinator: createChatFocusCoordinator(),
    displayAgents: ref([agent]),
    displayTeams: ref([]),
    dialogMode: ref('auto'),
    selectedProviderModel: ref(undefined),
    makeSessionTitle: (v: string) => v,
    t: (key: string) => key,
  } as unknown as EntityNavDeps;
  const scope = effectScope();
  const api = scope.run(() => useChatEntityNav(deps))!;
  return { api, deps, scope };
}
beforeEach(() => {
  vi.clearAllMocks();
  fixture.router.currentRoute.value = { name: 'task-workspace', path: '/task-workspace', query: {} };
  fixture.router.replace.mockImplementation(async (route) => {
    fixture.router.currentRoute.value = route;
  });
  fixture.hydrate.mockResolvedValue(undefined);
});
describe('session navigation intent ownership', () => {
  it('does not let a late A lookup replace the newer B selection', async () => {
    const a = deferred<Session>(),
      b = deferred<Session>();
    fixture.fetch.mockImplementation((sid) => (sid === 'A' ? a.promise : b.promise));
    const c = setup();
    const old = c.api.focusSessionById('A');
    const current = c.api.focusSessionById('B');
    b.resolve(session('B'));
    await current;
    a.resolve(session('A'));
    await old;
    expect(c.deps.sessionStore.selectedSession?.id).toBe('B');
    expect(c.api.navigationLoading.value).toBe(false);
    expect(fixture.hydrate).toHaveBeenCalledTimes(1);
    c.scope.stop();
  });
  it('creates a fresh A request after A to B to A, instead of joining obsolete A', async () => {
    const oldA = deferred<Session>(),
      oldB = deferred<Session>(),
      newA = deferred<Session>();
    fixture.fetch.mockReturnValueOnce(oldA.promise).mockReturnValueOnce(oldB.promise).mockReturnValueOnce(newA.promise);
    const c = setup();
    const first = c.api.focusSessionById('A'),
      second = c.api.focusSessionById('B'),
      third = c.api.focusSessionById('A');
    newA.resolve(session('A'));
    await third;
    oldA.resolve(session('A'));
    oldB.resolve(session('B'));
    await Promise.all([first, second]);
    expect(fixture.fetch).toHaveBeenCalledTimes(3);
    expect(c.deps.sessionStore.selectedSession?.id).toBe('A');
    expect(fixture.hydrate).toHaveBeenCalledTimes(1);
    c.scope.stop();
  });
  it('rejects mismatched explicit ownership without selecting another session', async () => {
    fixture.fetch.mockResolvedValue(session('B'));
    const c = setup();
    await c.api.focusSessionById('B', 'foreign-agent');
    expect(c.api.navigationError.value).toContain('locationScopeMismatch');
    expect(c.deps.sessionStore.selectedSession?.id).toBe('A');
    expect(fixture.hydrate).not.toHaveBeenCalled();
    c.scope.stop();
  });
  it('retains target parameters on mobile owner resolution', async () => {
    fixture.router.currentRoute.value = {
      name: 'mobile-task-workspace',
      path: '/mobile/task-workspace',
      query: { session: 'B', task: 'task', step: 'step', panel: 'chat' },
    };
    fixture.fetch.mockResolvedValue(session('B'));
    const c = setup();
    await c.api.focusSessionById('B');
    expect(fixture.router.replace).toHaveBeenCalledWith({
      name: 'mobile-task-workspace',
      query: { session: 'B', agent: 'agent', task: 'task', step: 'step', panel: 'chat' },
    });
    c.scope.stop();
  });
  it('invalidates in-flight selection when its workspace is disposed', async () => {
    const read = deferred<Session>();
    fixture.fetch.mockReturnValue(read.promise);
    const c = setup();
    const pending = c.api.focusSessionById('B');
    c.scope.stop();
    read.resolve(session('B'));
    await pending;
    expect(c.deps.sessionStore.selectedSession?.id).toBe('A');
    expect(fixture.hydrate).not.toHaveBeenCalled();
  });
  it('resolves a team-owned history target absent from the recent team list', async () => {
    const c = setup();
    const teamSession = { ...session('team-session'), owner_type: 'team', agent_id: '', team_id: 'team' };
    fixture.fetch.mockResolvedValue(teamSession);
    c.deps.displayTeams.value = [{ id: 'team' } as EntityNavDeps['displayTeams']['value'][number]];
    c.deps.sessionStore.teamSessions = {};
    c.deps.sessionStore.resetForTeamSwitch = vi.fn((id) => {
      c.deps.sessionStore.entityKind = 'team';
      c.deps.sessionStore.selectedTeamId = id;
    });
    c.deps.sessionStore.loadTeamSessions = vi.fn().mockResolvedValue(undefined);
    c.deps.messageStore.loadMessages = vi.fn().mockResolvedValue(undefined);
    c.deps.streamManager.ensureTeamStream = vi.fn();
    await c.api.focusSessionById('team-session');
    expect(c.api.navigationError.value).toBe('');
    expect(c.deps.sessionStore.teamSelectedSessionId).toBe('team-session');
    expect(c.deps.sessionStore.teamSessions.team?.[0]?.id).toBe('team-session');
    c.scope.stop();
  });
});
