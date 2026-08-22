// web/src/features/memory/browse/__tests__/useMemoryEpisodes.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { ref } from 'vue';
import type { MemoryEpisodeListResult } from '../../types';

vi.mock('../../api', () => ({
  getMemoryEpisodes: vi.fn(),
}));

import { getMemoryEpisodes } from '../../api';
import { useMemoryEpisodes } from '../composables/useMemoryEpisodes';

const mockApi = vi.mocked(getMemoryEpisodes);

function sampleResult(offset = 0): MemoryEpisodeListResult {
  return {
    items: [
      {
        id: `ep${offset + 1}`,
        session_id: 's1',
        agent_id: 'agent-1',
        episode_kind: 'task',
        title: '季度复盘讨论',
        outcome_summary: '完成复盘并产出结论',
        importance: 0.8,
        consolidation_status: 'consolidated',
        consolidated_l3_count: 3,
        ended_at: '2026-07-20T01:00:00Z',
        created_at: '2026-07-20T00:00:00Z',
      },
    ],
    total: 2,
  };
}

describe('useMemoryEpisodes', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    mockApi.mockReset();
  });

  it('agentId 为空时不请求后端', async () => {
    const e = useMemoryEpisodes(ref(null), ref(null));
    await Promise.resolve();
    expect(mockApi).not.toHaveBeenCalled();
    expect(e.items.value).toEqual([]);
  });

  it('加载成功：items/total 填充，hasMore 依据 total 判定', async () => {
    mockApi.mockResolvedValue(sampleResult());
    const e = useMemoryEpisodes(ref('agent-1'), ref(null));
    await vi.waitFor(() => expect(e.items.value).toHaveLength(1));

    expect(mockApi).toHaveBeenCalledWith('agent-1', '', 20, 0);
    expect(e.total.value).toBe(2);
    expect(e.hasMore.value).toBe(true);
    expect(e.error.value).toBe('');
  });

  it('加载失败：error 填充、items 清空', async () => {
    mockApi.mockRejectedValue(new Error('boom'));
    const e = useMemoryEpisodes(ref('agent-1'), ref(null));
    await vi.waitFor(() => expect(e.error.value).toBe('boom'));
    expect(e.items.value).toEqual([]);
    expect(e.total.value).toBe(0);
  });

  it('loadMore：按已加载数量作 offset 追加，到达 total 后 hasMore=false', async () => {
    mockApi.mockResolvedValueOnce(sampleResult(0)).mockResolvedValueOnce(sampleResult(1));
    const e = useMemoryEpisodes(ref('agent-1'), ref(null));
    await vi.waitFor(() => expect(e.items.value).toHaveLength(1));

    await e.loadMore();
    expect(mockApi).toHaveBeenLastCalledWith('agent-1', '', 20, 1);
    expect(e.items.value).toHaveLength(2);
    expect(e.items.value.map((i) => i.id)).toEqual(['ep1', 'ep2']);
    expect(e.hasMore.value).toBe(false);

    await e.loadMore();
    expect(mockApi).toHaveBeenCalledTimes(2);
  });

  it('agentId 变化时重置并重新加载', async () => {
    mockApi.mockResolvedValue(sampleResult());
    const agentId = ref<string | null>('agent-1');
    const e = useMemoryEpisodes(agentId, ref(null));
    await vi.waitFor(() => expect(e.items.value).toHaveLength(1));

    mockApi.mockResolvedValue({ items: [], total: 0 });
    agentId.value = 'agent-2';
    await vi.waitFor(() => expect(mockApi).toHaveBeenLastCalledWith('agent-2', '', 20, 0));
    await vi.waitFor(() => expect(e.items.value).toEqual([]));
    expect(e.total.value).toBe(0);
  });
});
