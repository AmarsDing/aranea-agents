import { describe, expect, it } from 'vitest';
import { parseMemoryCenterDeepLink } from '../memoryCenterDeepLink';

describe('parseMemoryCenterDeepLink', () => {
  it('defaults to panorama when query is empty', () => {
    const link = parseMemoryCenterDeepLink({}, false);
    expect(link.tab).toBe('panorama');
    expect(link.layer).toBe('');
    expect(link.agentId).toBeNull();
  });

  it('falls ops back to trust for non-admins', () => {
    const link = parseMemoryCenterDeepLink({ tab: 'ops' }, false);
    expect(link.tab).toBe('governance');
  });

  it('keeps ops for platform admins', () => {
    expect(parseMemoryCenterDeepLink({ tab: 'ops' }, true).tab).toBe('ops');
  });

  it('opens browse and fact keyword when factId is set', () => {
    const link = parseMemoryCenterDeepLink(
      { tab: 'governance', factId: 'fact-1', agentId: 'a1', sessionId: 's1' },
      false,
    );
    expect(link.tab).toBe('browse');
    expect(link.factId).toBe('fact-1');
    expect(link.keyword).toBe('fact-1');
    expect(link.clearFactStatus).toBe(true);
    expect(link.agentId).toBe('a1');
    expect(link.sessionId).toBe('s1');
  });

  it('routes L4 layer to graph', () => {
    const link = parseMemoryCenterDeepLink({ layer: 'L4' }, true);
    expect(link.tab).toBe('graph');
    expect(link.layer).toBe('L4');
  });

  it('routes L3 layer to browse unless tab is explicit', () => {
    expect(parseMemoryCenterDeepLink({ layer: 'L3' }, false).tab).toBe('browse');
    expect(parseMemoryCenterDeepLink({ tab: 'panorama', layer: 'L1' }, false).tab).toBe('panorama');
  });
});
