import { describe, expect, it } from 'vitest';
import type { LocationQuery } from 'vue-router';
import {
  chatBreakpointTarget,
  parseWorkspaceLocation,
  safeWorkspaceReturn,
  sessionNavigationQuery,
  workspaceLocationQuery,
} from '../taskWorkspaceNavigation';

describe('workspace location contracts', () => {
  it('preserves first version zero and only serializes target IDs', () => {
    const target = {
      sessionId: 'S',
      taskId: 'T',
      stepId: 'step',
      panel: 'artifacts' as const,
      artifactId: 'file',
      version: 0,
    };
    expect(parseWorkspaceLocation(workspaceLocationQuery(target) as Record<string, string>)).toEqual(target);
  });
  const invalidQueries: LocationQuery[] = [
    { session: ['A', 'B'] },
    { session: 'A', step: 'step' },
    { session: 'A', version: '1' },
    { session: 'A', artifact: 'file', version: '-1' },
    { session: 'A', artifact: 'file', version: '9007199254740992' },
    { session: 'A', panel: ['chat'] },
    { session: 'A', task: '<script>' },
    { session: 'A', agent: ['a', 'b'] },
    { session: 'A', agent: 'a', team: 'b' },
  ];
  it.each(invalidQueries)('rejects malformed or ambiguous targets: %j', (query) => {
    expect(() => parseWorkspaceLocation(query)).toThrow();
  });
  it('does not carry an old task or file into a different session', () => {
    const query = { session: 'A', task: 'T', step: 'step', artifact: 'file', version: '0', panel: 'chat' };
    expect(sessionNavigationQuery(query, 'A', { agent: 'agent' })).toMatchObject(query);
    expect(sessionNavigationQuery(query, 'B', { team: 'team' })).toEqual({ session: 'B', team: 'team' });
  });
  it('maps both workspace and legacy chat links across the breakpoint without losing queries', () => {
    const query = { session: 'S', task: 'T', step: 'step', panel: 'chat' };
    expect(chatBreakpointTarget('/task-workspace', query, true)).toEqual({ path: '/mobile/task-workspace', query });
    expect(chatBreakpointTarget('/mobile/task-workspace', query, false)).toEqual({ path: '/task-workspace', query });
    expect(chatBreakpointTarget('/chat', query, true)).toEqual({ path: '/mobile/chat', query });
    expect(chatBreakpointTarget('/mobile/chat', query, false)).toEqual({ path: '/chat', query });
    expect(chatBreakpointTarget('/task-workspace', query, false)).toBeNull();
  });
  it('allows only local chat returns and drops unrelated data', () => {
    expect(safeWorkspaceReturn('/task-workspace?session=S&artifact=file&version=0&secret=discard')).toBe(
      '/task-workspace?session=S&artifact=file&version=0',
    );
    for (const value of [
      'https://example.com',
      '//example.com',
      '/task-workspace/../admin',
      '/login',
      '/chat?session=A&session=B',
      '/chat?step=x',
    ])
      expect(safeWorkspaceReturn(value)).toBeNull();
  });
});
