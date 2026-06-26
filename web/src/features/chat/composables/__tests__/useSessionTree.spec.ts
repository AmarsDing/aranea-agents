import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../../../session/api', () => ({
  getSessionTree: vi.fn(),
}));

import { getSessionTree } from '../../../session/api';
import { useSessionTree } from '../useSessionTree';

const fakeTree = {
  session: { id: 'spirit-1', title: 'Spirit 1', owner_type: 'spirit', agent_id: '' },
  children: [
    {
      session: {
        id: 'team-1',
        title: 'Team 1',
        parent_session_id: 'spirit-1',
        agent_id: '',
      },
      children: [
        {
          session: {
            id: 'agent-1',
            title: 'Agent A',
            parent_session_id: 'team-1',
            agent_id: 'agent-key-A',
          },
          children: [],
        },
      ],
    },
  ],
};

describe('useSessionTree', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(getSessionTree).mockResolvedValue(fakeTree as never);
  });

  it('loads tree for a spirit session', async () => {
    const tree = useSessionTree();
    await tree.loadTreeFor('spirit-1');
    expect(tree.spiritTreeNodes.value).toHaveLength(1);
    expect(tree.spiritTreeNodes.value[0].session.id).toBe('spirit-1');
    expect(tree.findMemberSessionId('spirit-1', 'agent-key-A')).toBe('agent-1');
  });

  it('returns null when member not found', async () => {
    const tree = useSessionTree();
    await tree.loadTreeFor('spirit-1');
    expect(tree.findMemberSessionId('spirit-1', 'nope')).toBeNull();
  });

  it('does not refetch when tree already cached', async () => {
    const tree = useSessionTree();
    await tree.loadTreeFor('spirit-1');
    await tree.loadTreeFor('spirit-1');
    expect(getSessionTree).toHaveBeenCalledTimes(1);
  });
});
