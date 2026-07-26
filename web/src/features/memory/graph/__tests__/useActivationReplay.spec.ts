// web/src/features/memory/graph/__tests__/useActivationReplay.spec.ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { SpreadingActivationResponse } from '../../types';

vi.mock('../../api', () => ({
  getSpreadingActivation: vi.fn(),
}));

import { getSpreadingActivation } from '../../api';
import { useActivationReplay, HOP_INTERVAL_MS } from '../composables/useActivationReplay';

const mockApi = vi.mocked(getSpreadingActivation);

function sampleResponse(): SpreadingActivationResponse {
  return {
    center_id: 'e1',
    hops: 3,
    top_k: 20,
    items: [
      { node_id: 'e1', activation: 1.0, hop_count: 0, activation_path: [] },
      {
        node_id: 'f1',
        activation: 0.8,
        hop_count: 1,
        activation_path: [{ from_node_id: 'e1', to_node_id: 'f1', edge_weight: 0.9, relation_type: 'entity_fact' }],
      },
      {
        node_id: 'f2',
        activation: 0.6,
        hop_count: 1,
        activation_path: [{ from_node_id: 'e1', to_node_id: 'f2', edge_weight: 0.7, relation_type: 'entity_fact' }],
      },
      {
        node_id: 'ep1',
        activation: 0.4,
        hop_count: 2,
        activation_path: [
          { from_node_id: 'e1', to_node_id: 'f1', edge_weight: 0.9, relation_type: 'entity_fact' },
          { from_node_id: 'f1', to_node_id: 'ep1', edge_weight: 0.5, relation_type: 'fact_source' },
        ],
      },
    ],
  };
}

describe('useActivationReplay', () => {
  beforeEach(() => {
    mockApi.mockReset();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('replay(centerId)：调 API 并按 hop_count 分组', async () => {
    mockApi.mockResolvedValue(sampleResponse());
    const replay = useActivationReplay();

    await replay.replay('e1');

    expect(mockApi).toHaveBeenCalledWith('e1', { hops: 3, top_k: 20 });
    expect(replay.playing.value).toBe(true);
    expect(replay.error.value).toBe('');
    // hop 0 立即激活
    expect(replay.activeHops.value.has(0)).toBe(true);
    expect(replay.activationOf('e1')).toBeCloseTo(1.0);
    expect(replay.activationOf('f1')).toBe(0); // 尚未点亮
  });

  it('定时器逐跳推进：每 600ms 点亮下一跳', async () => {
    mockApi.mockResolvedValue(sampleResponse());
    const replay = useActivationReplay();

    await replay.replay('e1');
    expect(replay.activeHops.value.has(0)).toBe(true);
    expect(replay.activeHops.value.has(1)).toBe(false);

    vi.advanceTimersByTime(HOP_INTERVAL_MS);
    expect(replay.activeHops.value.has(1)).toBe(true);
    expect(replay.activationOf('f1')).toBeCloseTo(0.8);
    expect(replay.activationOf('f2')).toBeCloseTo(0.6);
    expect(replay.activeHops.value.has(2)).toBe(false);

    vi.advanceTimersByTime(HOP_INTERVAL_MS);
    expect(replay.activeHops.value.has(2)).toBe(true);
    expect(replay.activationOf('ep1')).toBeCloseTo(0.4);
    // 全部点完后自动停止
    expect(replay.playing.value).toBe(false);
  });

  it('stop()：清除定时器并复位状态', async () => {
    mockApi.mockResolvedValue(sampleResponse());
    const replay = useActivationReplay();

    await replay.replay('e1');
    expect(replay.playing.value).toBe(true);

    replay.stop();
    expect(replay.playing.value).toBe(false);
    expect(replay.activeHops.value.size).toBe(0);
    expect(replay.activationOf('e1')).toBe(0);
  });

  it('replay 期间再次 replay：先清除旧定时器', async () => {
    mockApi.mockResolvedValue(sampleResponse());
    const replay = useActivationReplay();

    await replay.replay('e1');
    vi.advanceTimersByTime(HOP_INTERVAL_MS);
    expect(replay.activeHops.value.has(1)).toBe(true);

    await replay.replay('e1');
    // 重新开始，只有 hop 0 激活
    expect(replay.activeHops.value.has(0)).toBe(true);
    expect(replay.activeHops.value.has(1)).toBe(false);
  });

  it('API 失败：error 填充、playing=false', async () => {
    mockApi.mockRejectedValue(new Error('boom'));
    const replay = useActivationReplay();

    await replay.replay('e1');
    expect(replay.error.value).toBe('boom');
    expect(replay.playing.value).toBe(false);
    expect(replay.activeHops.value.size).toBe(0);
  });

  it('topK 排行：按 activation 降序返回', async () => {
    mockApi.mockResolvedValue(sampleResponse());
    const replay = useActivationReplay();

    await replay.replay('e1');
    const ranking = replay.topKRanking.value;
    expect(ranking.length).toBe(4);
    expect(ranking[0].node_id).toBe('e1');
    expect(ranking[0].activation).toBeCloseTo(1.0);
    expect(ranking[1].node_id).toBe('f1');
    expect(ranking[2].node_id).toBe('f2');
    expect(ranking[3].node_id).toBe('ep1');
  });

  it('highlightEdges：返回 activation_path 中所有边 key', async () => {
    mockApi.mockResolvedValue(sampleResponse());
    const replay = useActivationReplay();

    await replay.replay('e1');
    vi.advanceTimersByTime(HOP_INTERVAL_MS * 2); // 全部点亮

    const edges = replay.highlightEdges.value;
    expect(edges.has('e1->f1:entity_fact')).toBe(true);
    expect(edges.has('e1->f2:entity_fact')).toBe(true);
    expect(edges.has('f1->ep1:fact_source')).toBe(true);
  });

  it('组件 unmount 时清理定时器', async () => {
    mockApi.mockResolvedValue(sampleResponse());
    const replay = useActivationReplay();

    await replay.replay('e1');
    expect(replay.playing.value).toBe(true);

    replay.dispose();
    expect(replay.playing.value).toBe(false);
    expect(replay.activeHops.value.size).toBe(0);
  });
});
