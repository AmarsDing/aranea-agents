/**
 * useVoiceButlerBinding 单测（M74 V9-T4，设计 74 §15.4-E）。
 *
 * 进入语音模式必须绑定 agent_id 属于 __voice_butler__ 的会话（Turn 执行按
 * sess.AgentID hydrate agent，评审 R5）；退出语音模式恢复先前选择。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';

import { useAppStore } from '../../../stores/app';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import type { Agent } from '../../agents/types';
import type { Session } from '../../session/types';
import type { TeamRow } from '../../../components/chat/types';
import { createSession, listSessions } from '../../session/api';
import { useVoiceButlerBinding, VOICE_BUTLER_AGENT_KEY } from './useVoiceButlerBinding';

vi.mock('../../session/api', () => ({
  listSessions: vi.fn(),
  createSession: vi.fn(),
}));

const listSessionsMock = vi.mocked(listSessions);
const createSessionMock = vi.mocked(createSession);

function fakeAgent(id: string, key: string): Agent {
  return { id, agent_key: key, display_name: key, provider: 'p', model: 'm' } as unknown as Agent;
}

function fakeSession(id: string, agentId: string): Session {
  return { id, agent_id: agentId, title: id } as unknown as Session;
}

const BUTLER = () => fakeAgent('agent___voice_butler__', VOICE_BUTLER_AGENT_KEY);
const SPIRIT = () => fakeAgent('agent___spirit__', '__spirit__');

type Deps = Parameters<typeof useVoiceButlerBinding>[0];

function makeDeps(overrides: Partial<Deps> = {}) {
  const selectAgent = vi.fn(async (agent: Agent, options?: { sessionId?: string }) => {
    const appStore = useAppStore();
    const chatStore = useChatSessionStore();
    appStore.selectedAgent = agent;
    chatStore.entityKind = 'agent';
    chatStore.selectedSession = fakeSession(options?.sessionId ?? `${agent.id}-s1`, agent.id);
  });
  const selectTeam = vi.fn(async (_team: TeamRow, _options?: { sessionId?: string }) => {});
  const deps: Deps = {
    selectAgent,
    selectTeam,
    displayTeams: () => [],
    sessionTitle: () => '语音助手',
    ...overrides,
  };
  return { deps, selectAgent, selectTeam };
}

describe('useVoiceButlerBinding', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    const appStore = useAppStore();
    appStore.agents = [SPIRIT(), BUTLER()];
    appStore.selectedAgent = appStore.agents[0] ?? null;
    const chatStore = useChatSessionStore();
    chatStore.entityKind = 'agent';
    chatStore.selectedSession = fakeSession('spirit-s1', 'agent___spirit__');
  });

  it('语音助手 agent 缺失 → null，无副作用', async () => {
    const appStore = useAppStore();
    appStore.agents = [SPIRIT()];
    const { deps, selectAgent } = makeDeps();
    const binding = useVoiceButlerBinding(deps);

    await expect(binding.bindVoiceButlerSession()).resolves.toBeNull();
    expect(selectAgent).not.toHaveBeenCalled();
    expect(listSessionsMock).not.toHaveBeenCalled();
  });

  it('已有语音助手会话 → 直接选中复用，不创建', async () => {
    listSessionsMock.mockResolvedValue([fakeSession('vb-s1', 'agent___voice_butler__')]);
    const { deps, selectAgent } = makeDeps();
    const binding = useVoiceButlerBinding(deps);

    const sid = await binding.bindVoiceButlerSession();
    expect(sid).toBe('agent___voice_butler__-s1'); // makeDeps selectAgent 默认选中
    expect(createSessionMock).not.toHaveBeenCalled();
    expect(selectAgent).toHaveBeenCalledTimes(1);
    // 未显式指定 sessionId（让 selectAgent 自然挑选最新会话）
    expect(selectAgent.mock.calls[0]?.[1]).toBeUndefined();
  });

  it('无语音助手会话 → 纯 API 创建（不钉 provider/model）后选中', async () => {
    listSessionsMock.mockResolvedValue([]);
    createSessionMock.mockResolvedValue(fakeSession('vb-new', 'agent___voice_butler__'));
    const { deps, selectAgent } = makeDeps();
    const binding = useVoiceButlerBinding(deps);

    const sid = await binding.bindVoiceButlerSession();
    expect(createSessionMock).toHaveBeenCalledTimes(1);
    const payload = createSessionMock.mock.calls[0]?.[0];
    expect(payload).toEqual({ agent_id: 'agent___voice_butler__', title: '语音助手' });
    expect(payload).not.toHaveProperty('default_provider');
    expect(payload).not.toHaveProperty('default_model');
    // 新建会话未必排在展示排序首位，必须显式指定
    expect(selectAgent).toHaveBeenCalledWith(expect.anything(), { sessionId: 'vb-new' });
    expect(sid).toBe('vb-new');
  });

  it('幂等：已在语音助手会话 → 直接复用，不切选不覆盖快照', async () => {
    const appStore = useAppStore();
    const chatStore = useChatSessionStore();
    appStore.selectedAgent = BUTLER();
    chatStore.selectedSession = fakeSession('vb-cur', 'agent___voice_butler__');
    const { deps, selectAgent } = makeDeps();
    const binding = useVoiceButlerBinding(deps);

    await expect(binding.bindVoiceButlerSession()).resolves.toBe('vb-cur');
    expect(selectAgent).not.toHaveBeenCalled();
    expect(listSessionsMock).not.toHaveBeenCalled();
  });

  it('listSessions 失败 → null（不抛出，由调用方走 NO_SESSION 错误路径）', async () => {
    listSessionsMock.mockRejectedValue(new Error('network'));
    const { deps } = makeDeps();
    const binding = useVoiceButlerBinding(deps);
    await expect(binding.bindVoiceButlerSession()).resolves.toBeNull();
  });

  it('退出恢复：从精灵会话进入 → 恢复精灵 agent + 原会话', async () => {
    listSessionsMock.mockResolvedValue([fakeSession('vb-s1', 'agent___voice_butler__')]);
    const { deps, selectAgent } = makeDeps();
    const binding = useVoiceButlerBinding(deps);

    await binding.bindVoiceButlerSession();
    expect(useAppStore().selectedAgent?.agent_key).toBe(VOICE_BUTLER_AGENT_KEY);

    await binding.restorePreviousSelection();
    // 第二次 selectAgent = 恢复调用：精灵 agent + 原会话 id
    expect(selectAgent).toHaveBeenCalledTimes(2);
    const [agent, options] = selectAgent.mock.calls[1] ?? [];
    expect(agent?.agent_key).toBe('__spirit__');
    expect(options).toEqual({ sessionId: 'spirit-s1' });
  });

  it('退出恢复：从 team 进入 → 恢复 team 选择', async () => {
    const chatStore = useChatSessionStore();
    chatStore.entityKind = 'team';
    chatStore.selectedTeamId = 'team-1';
    chatStore.teamSelectedSessionId = 'team-s1';
    listSessionsMock.mockResolvedValue([fakeSession('vb-s1', 'agent___voice_butler__')]);
    const team = { id: 'team-1' } as unknown as TeamRow;
    const { deps, selectTeam } = makeDeps({ displayTeams: () => [team] });
    const binding = useVoiceButlerBinding(deps);

    await binding.bindVoiceButlerSession();
    await binding.restorePreviousSelection();
    expect(selectTeam).toHaveBeenCalledWith(team, { sessionId: 'team-s1' });
  });

  it('退出恢复：语音模式中用户已手动切走 → 不打扰', async () => {
    listSessionsMock.mockResolvedValue([fakeSession('vb-s1', 'agent___voice_butler__')]);
    const { deps, selectAgent } = makeDeps();
    const binding = useVoiceButlerBinding(deps);

    await binding.bindVoiceButlerSession();
    // 用户手动切回精灵（漂移）
    useAppStore().selectedAgent = SPIRIT();
    await binding.restorePreviousSelection();
    expect(selectAgent).toHaveBeenCalledTimes(1); // 仅进入那次
  });

  it('无快照（直接进入即退出边界）→ 恢复为空操作', async () => {
    const { deps, selectAgent, selectTeam } = makeDeps();
    const binding = useVoiceButlerBinding(deps);
    await binding.restorePreviousSelection();
    expect(selectAgent).not.toHaveBeenCalled();
    expect(selectTeam).not.toHaveBeenCalled();
  });
});
