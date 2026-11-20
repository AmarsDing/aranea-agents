import { describe, expect, it, vi } from 'vitest';

const svc = {
  ListDeadLetters: vi.fn(),
  ListConsumers: vi.fn(),
  ReplayDeadLetter: vi.fn(),
};
vi.mock('../../../services', () => ({ createCeoboardService: () => svc }));

import { useFlowOps } from '../useFlowOps';

describe('useFlowOps', () => {
  it('消费者：断开的排前面，其次按积压从多到少；积压与断开数汇总', async () => {
    svc.ListConsumers.mockResolvedValue({
      items: [
        { stream: 'A', durable: 'a', numPending: 5, connected: true },
        { stream: 'B', durable: 'b', numPending: 0, connected: false },
        { stream: 'C', durable: 'c', numPending: 50, connected: true },
      ],
    });
    const ops = useFlowOps();
    await ops.loadConsumers();
    expect(ops.consumers.value.map((c) => c.stream)).toEqual(['B', 'C', 'A']);
    expect(ops.backlog.value).toBe(55);
    expect(ops.disconnected.value).toBe(1);
  });

  it('死信没权限（403）时标记 forbidden，不当成普通故障', async () => {
    svc.ListDeadLetters.mockRejectedValue({ response: { status: 403, data: { code: 403, reason: 'PERMISSION_DENIED' } } });
    const ops = useFlowOps();
    await ops.loadDeadLetters();
    expect(ops.deadState.value.forbidden).toBe(true);
  });

  it('重放成功后重读列表，并把「已投递过」如实返回', async () => {
    svc.ListDeadLetters.mockResolvedValue({ pending: 0, items: [] });
    svc.ReplayDeadLetter.mockResolvedValue({ replayed: true, deduped: true });
    const ops = useFlowOps();
    const res = await ops.replay(7);
    expect(svc.ReplayDeadLetter).toHaveBeenCalledWith({ id: 7 });
    expect(res).toEqual({ ok: true, deduped: true });
    expect(svc.ListDeadLetters).toHaveBeenCalled();
    expect(ops.replaying.value).toBeNull();
  });
});
