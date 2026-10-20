import { describe, expect, it } from 'vitest';
import { classifyResults, WEAK_THRESHOLD } from '../relevanceHint';

describe('relevanceHint U1-6 classifyResults', () => {
  it('WEAK_THRESHOLD 导出为 0.3（调参口）', () => {
    expect(WEAK_THRESHOLD).toBe(0.3);
  });

  it('空数组 → empty，topScore=0，引导 tips 含 expand-scope 与 open-hit-test', () => {
    const r = classifyResults([]);
    expect(r.level).toBe('empty');
    expect(r.topScore).toBe(0);
    expect(r.tips).toEqual(['expand-scope', 'open-hit-test']);
  });

  it('全部命中且最高有效分远高于阈值 → good（不打扰，tips 为空）', () => {
    const r = classifyResults([{ score: 0.42 }, { score: 0.81 }, { score: 0.55 }]);
    expect(r.level).toBe('good');
    expect(r.topScore).toBeCloseTo(0.81);
    expect(r.tips).toEqual([]);
  });

  it('阈值边界：最高有效分恰好等于 WEAK_THRESHOLD → good（严格小于才 weak）', () => {
    const at = classifyResults([{ score: WEAK_THRESHOLD }]);
    expect(at.level).toBe('good');
    const below = classifyResults([{ score: 0.29 }, { score: 0.1 }]);
    expect(below.level).toBe('weak');
  });

  it('最高有效分低于阈值 → weak，tips 含 refine-keywords / try-rerank / narrow-scope', () => {
    const r = classifyResults([{ score: 0.12 }, { score: 0.05 }]);
    expect(r.level).toBe('weak');
    expect(r.topScore).toBeCloseTo(0.12);
    expect(r.tips).toEqual(['refine-keywords', 'try-rerank', 'narrow-scope']);
  });

  it('score 缺失时回退 rrf_score', () => {
    const r = classifyResults([{ rrf_score: 0.5 }]);
    expect(r.level).toBe('good');
    expect(r.topScore).toBe(0.5);
  });

  it('score 优先于 rrf_score（两者皆有限时以 score 为准，不因更大 rrf 误判 good）', () => {
    const r = classifyResults([{ score: 0.1, rrf_score: 0.9 }]);
    expect(r.level).toBe('weak');
    expect(r.topScore).toBeCloseTo(0.1);
  });

  it('缺字段鲁棒性：{} / NaN / Infinity 均被忽略，全部无有效分按 0 → weak', () => {
    expect(classifyResults([{ score: NaN }]).level).toBe('weak');
    expect(classifyResults([{ score: Infinity }]).topScore).toBe(0);
    expect(classifyResults([{}]).level).toBe('weak');
    expect(classifyResults([{ score: undefined, rrf_score: undefined }]).topScore).toBe(0);
    // 混合：无效分不影响其他结果取有效分
    const mixed = classifyResults([{ score: NaN }, { score: 0.6 }]);
    expect(mixed.level).toBe('good');
    expect(mixed.topScore).toBe(0.6);
  });

  it('topScore 取所有结果有效分最大值，与输入顺序无关', () => {
    const a = classifyResults([{ score: 0.9 }, { score: 0.4 }]);
    const b = classifyResults([{ score: 0.4 }, { score: 0.9 }]);
    expect(a.topScore).toBeCloseTo(0.9);
    expect(b.topScore).toBeCloseTo(0.9);
  });

  it('返回的 tips 为新数组：调用方修改不污染后续调用结果', () => {
    const first = classifyResults([]);
    first.tips.push('refine-keywords');
    expect(classifyResults([]).tips).toEqual(['expand-scope', 'open-hit-test']);
    const weak = classifyResults([{ score: 0.1 }]);
    weak.tips.pop();
    expect(classifyResults([{ score: 0.1 }]).tips).toEqual(['refine-keywords', 'try-rerank', 'narrow-scope']);
  });
});
