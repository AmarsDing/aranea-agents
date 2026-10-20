// SP3-P3 审查待办徽章：fetch 口径（home 回退 / 单项失败独立结算）+ 轮询 / 聚焦刷新 / stop 语义。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick, ref } from 'vue';
import {
  fetchReviewBadgeCounts,
  useReviewBadge,
  REVIEW_BADGE_POLL_INTERVAL_MS,
  type ReviewBadgeFetchers,
} from '../useReviewBadge';

function makeFetchers(overrides?: Partial<ReviewBadgeFetchers>): ReviewBadgeFetchers {
  return {
    getWriteBackHome: vi.fn().mockResolvedValue({ found: true, collection_id: 'col-home' }),
    // G-30：治理分页契约 {items,total}——角标取 total（不受页大小截断）。
    listGovernancePending: vi.fn().mockResolvedValue({ items: [{ id: 1 }, { id: 2 }], total: 2 }),
    listWriteBackPending: vi.fn().mockResolvedValue([{ id: 'f1' }]),
    ...overrides,
  };
}

describe('fetchReviewBadgeCounts', () => {
  it('当前库有治理条目：不回退 home；写回吐向 home 计数', async () => {
    const f = makeFetchers();
    const got = await fetchReviewBadgeCounts('col-1', f);
    expect(got).toEqual({ governancePending: 2, writebackPending: 1 });
    expect(f.listGovernancePending).toHaveBeenCalledTimes(1);
    expect(f.listGovernancePending).toHaveBeenCalledWith('col-1');
    expect(f.listWriteBackPending).toHaveBeenCalledWith('col-home');
  });

  it('当前库治理为空且 home 不同：回退 home 计数（与治理对话框口径一致）', async () => {
    const listGovernancePending = vi
      .fn()
      .mockResolvedValueOnce({ items: [], total: 0 }) // 当前库
      .mockResolvedValueOnce({ items: [{ id: 7 }], total: 1 }); // home
    const got = await fetchReviewBadgeCounts('col-1', makeFetchers({ listGovernancePending }));
    expect(got.governancePending).toBe(1);
    expect(listGovernancePending).toHaveBeenNthCalledWith(1, 'col-1');
    expect(listGovernancePending).toHaveBeenNthCalledWith(2, 'col-home');
  });

  it('当前库为空且 home 与当前库一致：不再回退请求', async () => {
    const listGovernancePending = vi.fn().mockResolvedValue({ items: [], total: 0 });
    const got = await fetchReviewBadgeCounts('col-home', makeFetchers({ listGovernancePending }));
    expect(got.governancePending).toBe(0);
    expect(listGovernancePending).toHaveBeenCalledTimes(1);
    expect(got.writebackPending).toBe(1);
  });

  it('治理拉取失败：仅治理置 null，写回正常结算（单项失败不拖垮另一项）', async () => {
    const got = await fetchReviewBadgeCounts(
      'col-1',
      makeFetchers({ listGovernancePending: vi.fn().mockRejectedValue(new Error('boom')) }),
    );
    expect(got).toEqual({ governancePending: null, writebackPending: 1 });
  });

  it('home 拉取失败：写回置 null，治理仍用当前库结算', async () => {
    const f = makeFetchers({ getWriteBackHome: vi.fn().mockRejectedValue(new Error('down')) });
    const got = await fetchReviewBadgeCounts('col-1', f);
    expect(got).toEqual({ governancePending: 2, writebackPending: null });
    expect(f.listWriteBackPending).not.toHaveBeenCalled();
  });

  it('无写回 home：写回 0（对话框「home missing」口径）且不请求待审列表', async () => {
    const f = makeFetchers({ getWriteBackHome: vi.fn().mockResolvedValue({ found: false, collection_id: '' }) });
    const got = await fetchReviewBadgeCounts('col-1', f);
    expect(got.writebackPending).toBe(0);
    expect(f.listWriteBackPending).not.toHaveBeenCalled();
  });

  it('写回列表失败：写回置 null，治理不受影响', async () => {
    const got = await fetchReviewBadgeCounts(
      'col-1',
      makeFetchers({ listWriteBackPending: vi.fn().mockRejectedValue(new Error('boom')) }),
    );
    expect(got).toEqual({ governancePending: 2, writebackPending: null });
  });
});

describe('useReviewBadge', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('start 立即刷新并汇总 total；之后按 60s 轮询', async () => {
    const f = makeFetchers();
    const badge = useReviewBadge(ref('col-1'), { fetchers: f });
    badge.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(badge.counts.value).toEqual({ governancePending: 2, writebackPending: 1 });
    expect(badge.total.value).toBe(3);
    expect(badge.lastRefreshAt.value).not.toBeNull();
    expect(f.getWriteBackHome).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(REVIEW_BADGE_POLL_INTERVAL_MS);
    expect(f.getWriteBackHome).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(REVIEW_BADGE_POLL_INTERVAL_MS);
    expect(f.getWriteBackHome).toHaveBeenCalledTimes(3);
    badge.stop();
  });

  it('窗口聚焦立即刷新', async () => {
    const f = makeFetchers();
    const badge = useReviewBadge(ref('col-1'), { fetchers: f });
    badge.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(f.getWriteBackHome).toHaveBeenCalledTimes(1);
    window.dispatchEvent(new Event('focus'));
    await vi.advanceTimersByTimeAsync(0);
    expect(f.getWriteBackHome).toHaveBeenCalledTimes(2);
    badge.stop();
  });

  it('vault 切换（运行中）按新库重取', async () => {
    const f = makeFetchers();
    const vault = ref('col-1');
    const badge = useReviewBadge(vault, { fetchers: f });
    badge.start();
    await vi.advanceTimersByTimeAsync(0);
    vault.value = 'col-2';
    await nextTick();
    await vi.advanceTimersByTimeAsync(0);
    expect(f.listGovernancePending).toHaveBeenLastCalledWith('col-2');
    badge.stop();
  });

  it('stop 后不再请求（计时器与聚焦均失效）', async () => {
    const f = makeFetchers();
    const badge = useReviewBadge(ref('col-1'), { fetchers: f });
    badge.start();
    await vi.advanceTimersByTimeAsync(0);
    badge.stop();
    await vi.advanceTimersByTimeAsync(REVIEW_BADGE_POLL_INTERVAL_MS * 3);
    window.dispatchEvent(new Event('focus'));
    await vi.advanceTimersByTimeAsync(0);
    expect(f.getWriteBackHome).toHaveBeenCalledTimes(1);
  });

  it('stop 丢弃在途响应（代际守卫）', async () => {
    let resolveGov: ((r: { items: unknown[]; total: number }) => void) | undefined;
    const f = makeFetchers({
      listGovernancePending: vi.fn(
        () =>
          new Promise<{ items: unknown[]; total: number }>((res) => {
            resolveGov = res;
          }),
      ),
    });
    const badge = useReviewBadge(ref('col-1'), { fetchers: f });
    badge.start();
    await vi.advanceTimersByTimeAsync(0); // home 已结算，治理在途
    badge.stop();
    resolveGov?.({ items: [{ id: 9 }], total: 1 });
    await vi.advanceTimersByTimeAsync(0);
    expect(badge.counts.value).toEqual({ governancePending: null, writebackPending: null });
    expect(badge.lastRefreshAt.value).toBeNull();
  });

  it('全部拉取失败：计数全 null、total 0，静默降级不抛错', async () => {
    const f = makeFetchers({
      getWriteBackHome: vi.fn().mockRejectedValue(new Error('down')),
      listGovernancePending: vi.fn().mockRejectedValue(new Error('down')),
      listWriteBackPending: vi.fn().mockRejectedValue(new Error('down')),
    });
    const badge = useReviewBadge(ref('col-1'), { fetchers: f });
    await badge.refresh();
    expect(badge.counts.value).toEqual({ governancePending: null, writebackPending: null });
    expect(badge.total.value).toBe(0);
    expect(badge.lastRefreshAt.value).not.toBeNull();
  });

  it('重复 start 幂等（单计时器单监听）', async () => {
    const f = makeFetchers();
    const badge = useReviewBadge(ref('col-1'), { fetchers: f });
    badge.start();
    badge.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(f.getWriteBackHome).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(REVIEW_BADGE_POLL_INTERVAL_MS);
    expect(f.getWriteBackHome).toHaveBeenCalledTimes(2);
    badge.stop();
  });
});
