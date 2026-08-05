// web/src/features/chat/__tests__/memoryRecall.spec.ts
import { describe, it, expect } from 'vitest';
import { parseMemoryRecallHits, MEMORY_RECALLED_NOTICE_TYPE } from '../memoryRecall';

describe('memoryRecall', () => {
  it('exposes the backend notice type constant', () => {
    // Mirrors internal/agent/memory_inject.go memoryRecalledNoticeType.
    expect(MEMORY_RECALLED_NOTICE_TYPE).toBe('memory_recalled');
  });

  it('parses a valid payload with all fields', () => {
    const content = JSON.stringify({
      hits: [
        { layer: 'L3', line: '用户偏好 XX 餐厅', score: 0.91, fact_id: 'f-1', confidence: 0.88, version: 3 },
        { layer: 'L2', line: '上次聚餐点了日料', score: 0.72 },
      ],
    });
    const hits = parseMemoryRecallHits(content);
    expect(hits).toHaveLength(2);
    expect(hits[0]).toEqual({
      layer: 'L3',
      line: '用户偏好 XX 餐厅',
      score: 0.91,
      fact_id: 'f-1',
      confidence: 0.88,
      version: 3,
    });
    expect(hits[1]).toEqual({ layer: 'L2', line: '上次聚餐点了日料', score: 0.72 });
  });

  it('returns [] for empty content', () => {
    expect(parseMemoryRecallHits('')).toEqual([]);
    expect(parseMemoryRecallHits('   ')).toEqual([]);
  });

  it('returns [] for invalid JSON', () => {
    expect(parseMemoryRecallHits('{not json')).toEqual([]);
    expect(parseMemoryRecallHits('plain text notice')).toEqual([]);
  });

  it('returns [] when hits is missing or not an array', () => {
    expect(parseMemoryRecallHits('{}')).toEqual([]);
    expect(parseMemoryRecallHits('{"hits": "nope"}')).toEqual([]);
    expect(parseMemoryRecallHits('{"hits": null}')).toEqual([]);
  });

  it('filters malformed entries and keeps valid ones', () => {
    const content = JSON.stringify({
      hits: [
        null,
        'string-entry',
        { layer: 'L3', line: '' }, // empty line
        { line: 'missing layer' }, // missing layer
        { layer: 'L3', line: '有效条目', score: 0.5 },
      ],
    });
    const hits = parseMemoryRecallHits(content);
    expect(hits).toHaveLength(1);
    expect(hits[0].line).toBe('有效条目');
  });

  it('defaults non-number score to 0 and drops non-number optionals', () => {
    const content = JSON.stringify({
      hits: [{ layer: 'L2', line: 'x', score: 'high', confidence: 'n/a', version: 'v' }],
    });
    const hits = parseMemoryRecallHits(content);
    expect(hits).toHaveLength(1);
    expect(hits[0].score).toBe(0);
    expect(hits[0].confidence).toBeUndefined();
    expect(hits[0].version).toBeUndefined();
  });
});
