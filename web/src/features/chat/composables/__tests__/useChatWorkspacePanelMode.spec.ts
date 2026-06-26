import { describe, it, expect } from 'vitest';
import { resolvePanelSessionId } from '../useChatWorkspace';

describe('resolvePanelSessionId (Phase B-3)', () => {
  const findMember = (spirit: string, agentKey: string) =>
    spirit === 'spirit-1' && agentKey === 'agent-key-A' ? 'agent-A' : null;

  it('returns spirit session id in spirit mode', () => {
    expect(
      resolvePanelSessionId({
        mode: 'spirit',
        spiritSessionId: 'spirit-1',
        activeTeamSessionId: 'team-1',
        activeMemberAgentKey: 'agent-key-A',
        findMemberSessionId: findMember,
      }),
    ).toBe('spirit-1');
  });

  it('returns team session id in team mode', () => {
    expect(
      resolvePanelSessionId({
        mode: 'team',
        spiritSessionId: 'spirit-1',
        activeTeamSessionId: 'team-1',
        activeMemberAgentKey: 'agent-key-A',
        findMemberSessionId: findMember,
      }),
    ).toBe('team-1');
  });

  it('returns member session id resolved from tree in member mode', () => {
    expect(
      resolvePanelSessionId({
        mode: 'member',
        spiritSessionId: 'spirit-1',
        activeTeamSessionId: 'team-1',
        activeMemberAgentKey: 'agent-key-A',
        findMemberSessionId: findMember,
      }),
    ).toBe('agent-A');
  });

  it('returns null in member mode when member agentKey is null', () => {
    expect(
      resolvePanelSessionId({
        mode: 'member',
        spiritSessionId: 'spirit-1',
        activeTeamSessionId: 'team-1',
        activeMemberAgentKey: null,
        findMemberSessionId: findMember,
      }),
    ).toBeNull();
  });

  it('returns null in member mode when tree has no matching agent', () => {
    expect(
      resolvePanelSessionId({
        mode: 'member',
        spiritSessionId: 'spirit-1',
        activeTeamSessionId: 'team-1',
        activeMemberAgentKey: 'agent-key-UNKNOWN',
        findMemberSessionId: findMember,
      }),
    ).toBeNull();
  });
});
