import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';
import { createChatFocusCoordinator } from '../../chatFocusCoordinator';
import { useChatEntityNav, type EntityNavDeps } from '../useChatEntityNav';
import type { Agent } from '../../../agents/types';
import type { Session } from '../../../session/types';

const router = vi.hoisted(() => ({
  currentRoute: { value: { name: 'task-workspace', query: { session: 'A', agent: 'agent' } } },
  replace: vi.fn(),
  push: vi.fn(),
}));
vi.mock('vue-router', () => ({ useRouter: () => router }));
vi.mock('quasar', () => ({ useQuasar: () => ({ notify: vi.fn() }) }));
vi.mock('../../../../stores/session', () => ({ useSessionStore: () => ({}) }));
vi.mock('../../../../stores/teams', () => ({ useTeamsStore: () => ({}) }));
vi.mock('../../../../stores/platform', () => ({ usePlatformStore: () => ({}) }));
vi.mock('../../agentPlannerSettings', () => ({ hydrateAgentSettings: async (agent: Agent) => agent }));
vi.mock('../../channelFocusLoad', () => ({ hydrateSessionForChannelFocus: vi.fn() }));

beforeEach(() => {
  vi.clearAllMocks();
  router.currentRoute.value = { name: 'task-workspace', query: { session: 'A', agent: 'agent' } };
  router.replace.mockImplementation(async (route) => {
    router.currentRoute.value = route;
  });
});

describe('chat route preservation', () => {
  it.each(['task-workspace', 'chat'])('keeps %s when selecting another session', async (name) => {
    router.currentRoute.value.name = name;
    const agent = { id: 'agent', agent_key: 'specialist' } as Agent;
    const selected = { id: 'B', agent_id: 'agent' } as Session;
    const deps = {
      appStore: { selectedAgent: agent, agents: [agent], upsertAgent: vi.fn() },
      sessionStore: {
        entityKind: 'agent',
        sessions: [selected],
        selectedSession: { id: 'A' },
        findSessionById: vi.fn(() => selected),
        loadAgentSessions: vi.fn(),
      },
      messageStore: {},
      streamManager: {},
      focusCoordinator: createChatFocusCoordinator(),
      displayAgents: ref([agent]),
      displayTeams: ref([]),
      dialogMode: ref('auto'),
      selectedProviderModel: ref(undefined),
      makeSessionTitle: (text: string) => text,
      t: (key: string) => key,
    } as unknown as EntityNavDeps;
    await useChatEntityNav(deps).onSelectSession('B');
    expect(router.replace).toHaveBeenCalledWith({ name, query: { session: 'B', agent: 'agent' } });
    expect(router.push).not.toHaveBeenCalled();
    expect(deps.sessionStore.selectedSession?.id).toBe('B');
  });
});
