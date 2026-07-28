import { describe, expect, it } from 'vitest';
import { resolveMobileTasksView } from './mobileTasksView';

describe('resolveMobileTasksView', () => {
  it('returns no-session when session id is missing', () => {
    expect(resolveMobileTasksView(null, 3)).toBe('no-session');
    expect(resolveMobileTasksView(undefined, 3)).toBe('no-session');
  });

  it('returns no-session when session id is blank', () => {
    expect(resolveMobileTasksView('', 3)).toBe('no-session');
    expect(resolveMobileTasksView('   ', 3)).toBe('no-session');
  });

  it('returns empty when the session has no tasks', () => {
    expect(resolveMobileTasksView('s-1', 0)).toBe('empty');
  });

  it('returns list when the session has tasks', () => {
    expect(resolveMobileTasksView('s-1', 1)).toBe('list');
    expect(resolveMobileTasksView('s-1', 12)).toBe('list');
  });
});
