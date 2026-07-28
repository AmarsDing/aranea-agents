import { describe, expect, it, vi } from 'vitest';
import {
  buildAgentMap,
  buildSpiritStatusBarModel,
  patchMemberSessionStatus,
  resolveActiveMember,
  resolveMemberChatSessionId,
} from './useChatMessagePanelBindings';
import type { Agent } from '../../agents/types';
import type { SpiritMember, SpiritTeam } from '../../spirit/types';
import type { MemberSession } from '../v2Types';

function makeAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: 'agent-1',
    agent_key: 'agent_key_1',
    display_name: 'Agent One',
    ...overrides,
  } as Agent;
}

function makeMember(overrides: Partial<SpiritMember> = {}): SpiritMember {
  return {
    agentId: 'agent-1',
    agentKey: 'ak_1',
    displayName: 'Member One',
    role: 'worker',
    status: 'running',
    avatarUrl: '',
    ...overrides,
  };
}

function makeTeam(overrides: Partial<SpiritTeam> = {}): SpiritTeam {
  return {
    id: 'team-1',
    teamName: 'Team One',
    taskSummary: '',
    status: 'running',
    mode: 'sequential',
    memberAvatars: [],
    completedSteps: 0,
    totalSteps: 0,
    progressPct: 0,
    durationMs: 0,
    spiritSessionId: 'spirit-sess-1',
    teamSessionId: 'team-sess-1',
    members: [],
    sharedAgentIds: [],
    createdAt: 0,
    ...overrides,
  };
}

function makeMemberSession(overrides: Partial<MemberSession> = {}): MemberSession {
  return {
    ID: 'ms-1',
    TeamRunID: 'tr-1',
    TeamStageID: 'ts-1',
    TaskID: 'task-1',
    SessionID: 'sess-1',
    SpiritSessionID: 'spirit-sess-1',
    AgentKey: 'ak_1',
    AgentName: 'Member One',
    AvatarURL: '',
    Status: 'running',
    Seq: 1,
    Version: 1,
    StartedAt: '2026-07-28T10:00:00.000Z',
    FinishedAt: null,
    Error: '',
    ...overrides,
  };
}

describe('buildAgentMap', () => {
  it('maps agent_key to display info', () => {
    const map = buildAgentMap([makeAgent({ agent_key: 'ak_1', display_name: 'Alpha' })]);
    expect(map.get('ak_1')).toEqual({ displayName: 'Alpha', agentKey: 'ak_1' });
  });

  it('falls back to agent_key when display_name is empty', () => {
    const map = buildAgentMap([makeAgent({ agent_key: 'ak_2', display_name: '' })]);
    expect(map.get('ak_2')?.displayName).toBe('ak_2');
  });

  it('skips agents without agent_key', () => {
    const map = buildAgentMap([makeAgent({ agent_key: '' })]);
    expect(map.size).toBe(0);
  });
});

describe('resolveActiveMember', () => {
  it('returns null when team or memberId is missing', () => {
    expect(resolveActiveMember(null, 'agent-1')).toBeNull();
    expect(resolveActiveMember(makeTeam(), null)).toBeNull();
  });

  it('returns the matching member', () => {
    const member = makeMember({ agentId: 'agent-9' });
    const team = makeTeam({ members: [makeMember(), member] });
    expect(resolveActiveMember(team, 'agent-9')).toBe(member);
  });

  it('returns null when memberId is not in the team', () => {
    const team = makeTeam({ members: [makeMember()] });
    expect(resolveActiveMember(team, 'agent-unknown')).toBeNull();
  });
});

describe('buildSpiritStatusBarModel', () => {
  it('counts running/pending as running, plus interrupted and completed', () => {
    const model = buildSpiritStatusBarModel({
      teams: [
        makeTeam({ id: 't1', status: 'running' }),
        makeTeam({ id: 't2', status: 'pending' }),
        makeTeam({ id: 't3', status: 'interrupted' }),
        makeTeam({ id: 't4', status: 'completed' }),
        makeTeam({ id: 't5', status: 'completed' }),
        makeTeam({ id: 't6', status: 'failed' }),
      ],
      activeTeam: null,
      usageSnapshot: null,
    });
    expect(model.runningTeamCount).toBe(2);
    expect(model.interruptedTeamCount).toBe(1);
    expect(model.completedTeamCount).toBe(2);
    expect(model.totalTeamCount).toBe(6);
  });

  it('prefers active team token usage over the session snapshot', () => {
    const model = buildSpiritStatusBarModel({
      teams: [],
      activeTeam: makeTeam({ tokenIn: 100, tokenOut: 40 }),
      usageSnapshot: {
        contextRatio: 0.5,
        inputTokens: 999,
        outputTokens: 999,
        totalTokens: 1998,
        totalCostMicroUsd: 0,
      },
    });
    expect(model.tokenUsage).toEqual({ in: 100, out: 40 });
  });

  it('falls back to session snapshot token usage when team has none', () => {
    const model = buildSpiritStatusBarModel({
      teams: [],
      activeTeam: makeTeam(),
      usageSnapshot: {
        contextRatio: 0.5,
        inputTokens: 10,
        outputTokens: 0,
        totalTokens: 10,
        totalCostMicroUsd: 0,
      },
    });
    expect(model.tokenUsage).toEqual({ in: 10, out: 0 });
  });

  it('returns null tokenUsage when neither source has tokens', () => {
    const model = buildSpiritStatusBarModel({
      teams: [],
      activeTeam: null,
      usageSnapshot: {
        contextRatio: 0.5,
        inputTokens: 0,
        outputTokens: 0,
        totalTokens: 0,
        totalCostMicroUsd: 0,
      },
    });
    expect(model.tokenUsage).toBeNull();
  });

  it('maps context and orchestration metadata fields', () => {
    const model = buildSpiritStatusBarModel({
      teams: [],
      activeTeam: null,
      usageSnapshot: {
        contextRatio: 0.7,
        contextUsedTokens: 700,
        contextWindow: 1000,
        inputTokens: 0,
        outputTokens: 0,
        totalTokens: 0,
        totalCostMicroUsd: 0,
      },
      complexityLevel: 'high',
      complexityReason: 'many teams',
      checkpointStep: 'step-3',
      dqScore: 0.82,
    });
    expect(model.contextRatio).toBe(0.7);
    expect(model.contextUsedTokens).toBe(700);
    expect(model.contextWindow).toBe(1000);
    expect(model.complexityLevel).toBe('high');
    expect(model.complexityReason).toBe('many teams');
    expect(model.checkpointStep).toBe('step-3');
    expect(model.dqScore).toBe(0.82);
  });

  it('defaults optional metadata fields to null', () => {
    const model = buildSpiritStatusBarModel({ teams: [], activeTeam: null, usageSnapshot: null });
    expect(model.contextRatio).toBeNull();
    expect(model.contextUsedTokens).toBeNull();
    expect(model.contextWindow).toBeNull();
    expect(model.complexityLevel).toBeNull();
    expect(model.complexityReason).toBeNull();
    expect(model.checkpointStep).toBeNull();
    expect(model.dqScore).toBeNull();
  });
});

describe('resolveMemberChatSessionId', () => {
  it('prefers member.chatSessionId when present', () => {
    const team = makeTeam({ members: [makeMember({ agentKey: 'ak_1', chatSessionId: 'chat-sess-1' })] });
    const id = resolveMemberChatSessionId({ teams: [team], agentKey: 'ak_1' });
    expect(id).toBe('chat-sess-1');
  });

  it('falls back to the session tree lookup with team.spiritSessionId', () => {
    const findMemberSessionId = vi.fn().mockReturnValue('tree-sess-1');
    const team = makeTeam({ members: [makeMember({ agentKey: 'ak_1' })] });
    const id = resolveMemberChatSessionId({ teams: [team], agentKey: 'ak_1', findMemberSessionId });
    expect(id).toBe('tree-sess-1');
    expect(findMemberSessionId).toHaveBeenCalledWith('spirit-sess-1', 'ak_1', 'team-sess-1');
  });

  it('uses fallbackSpiritSessionId when the team has none', () => {
    const findMemberSessionId = vi.fn().mockReturnValue('tree-sess-2');
    const team = makeTeam({ spiritSessionId: '', members: [makeMember({ agentKey: 'ak_1' })] });
    const id = resolveMemberChatSessionId({
      teams: [team],
      agentKey: 'ak_1',
      fallbackSpiritSessionId: 'fallback-spirit',
      findMemberSessionId,
    });
    expect(id).toBe('tree-sess-2');
    expect(findMemberSessionId).toHaveBeenCalledWith('fallback-spirit', 'ak_1', 'team-sess-1');
  });

  it('returns null when the member is not found in any team', () => {
    expect(resolveMemberChatSessionId({ teams: [makeTeam()], agentKey: 'ak_missing' })).toBeNull();
  });

  it('returns null when no chatSessionId and no tree lookup available', () => {
    const team = makeTeam({ members: [makeMember({ agentKey: 'ak_1' })] });
    expect(resolveMemberChatSessionId({ teams: [team], agentKey: 'ak_1' })).toBeNull();
  });
});

describe('patchMemberSessionStatus', () => {
  function makeStore(sessions: MemberSession[]) {
    const memberSessions = new Map(sessions.map((ms) => [ms.ID, ms]));
    return {
      memberSessions,
      upsertMemberSession: vi.fn((ms: MemberSession) => {
        memberSessions.set(ms.ID, ms);
      }),
    };
  }

  it('patches the matching member session and returns the previous status', () => {
    const store = makeStore([makeMemberSession({ SessionID: 'sess-1', Status: 'running' })]);
    const prev = patchMemberSessionStatus(store, 'sess-1', 'paused');
    expect(prev).toBe('running');
    expect(store.upsertMemberSession).toHaveBeenCalledWith(expect.objectContaining({ Status: 'paused' }));
  });

  it('returns null and does not patch when no member session matches', () => {
    const store = makeStore([makeMemberSession({ SessionID: 'sess-other' })]);
    const prev = patchMemberSessionStatus(store, 'sess-1', 'paused');
    expect(prev).toBeNull();
    expect(store.upsertMemberSession).not.toHaveBeenCalled();
  });
});
