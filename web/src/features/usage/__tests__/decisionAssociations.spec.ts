import { beforeEach, describe, expect, it, vi } from 'vitest';

const { listMock } = vi.hoisted(() => ({ listMock: vi.fn() }));
vi.mock('../../../services', () => ({
  createDecisionService: () => ({ ListDecisionRecords: listMock }),
}));

import { listDecisionRecords } from '../../decision/api';

beforeEach(() => listMock.mockReset());

describe('decision optional associations', () => {
  it('maps authoritative IDs without deriving missing links from display text', async () => {
    listMock.mockResolvedValue({
      items: [
        {
          decisionKey: 'linked',
          category: 'hitl_approval',
          sessionId: 'sess-real',
          taskId: 'task-real',
          stepId: 'step-real',
        },
        {
          decisionKey: 'unlinked',
          category: 'hitl_approval',
          scenario: 'sess-guess task-guess step-guess',
          actorKey: 'task-guess',
        },
      ],
      total: 2,
    });
    const page = await listDecisionRecords();
    expect(page.items[0]).toMatchObject({ session_id: 'sess-real', task_id: 'task-real', step_id: 'step-real' });
    expect(page.items[1].session_id).toBeUndefined();
    expect(page.items[1].task_id).toBeUndefined();
    expect(page.items[1].step_id).toBeUndefined();
  });
});
