import { describe, expect, it } from 'vitest';
import {
  useHitTestCases,
  normalizeParams,
  validateCaseDefinition,
  HIT_TEST_CASES_STORAGE_KEY,
  HIT_TEST_MAX_RUNS,
  type HitTestRunRecord,
  type StorageLike,
} from '../useHitTestCases';

/** 内存 mock storage（可记录写入内容）。 */
function createMockStorage(behavior: { throwOnGet?: boolean; throwOnSet?: boolean } = {}) {
  const map = new Map<string, string>();
  const storage: StorageLike & { map: Map<string, string> } = {
    map,
    getItem(key: string) {
      if (behavior.throwOnGet) throw new Error('denied');
      return map.has(key) ? (map.get(key) as string) : null;
    },
    setItem(key: string, value: string) {
      if (behavior.throwOnSet) throw new Error('quota');
      map.set(key, value);
    },
  };
  return storage;
}

function seqIds(prefix = 'id') {
  let n = 0;
  return () => `${prefix}-${++n}`;
}

function mkRun(at: string, scores: number[] = [0.9]): HitTestRunRecord {
  return { at, modeUsed: 'rrf', reranked: false, durationMs: 5, topScores: scores };
}

describe('useHitTestCases', () => {
  describe('addCase / listCases / getCase / deleteCase', () => {
    it('按 collectionId 分组，互不串库；id/name/query/params 正确保存', () => {
      const s = createMockStorage();
      const tc = useHitTestCases({ storage: s, idGen: seqIds() });
      const a = tc.addCase('col-1', {
        name: ' 退款政策 ',
        query: 'refund?',
        params: { topK: 5, useRerank: true, pathPrefix: 'notes/' },
      });
      const b = tc.addCase('col-2', { name: 'B', query: 'q2' });
      expect(a.id).toBe('id-1');
      expect(a.name).toBe('退款政策');
      expect(a.runs).toEqual([]);
      expect(tc.listCases('col-1').map((c) => c.id)).toEqual(['id-1']);
      expect(tc.listCases('col-2').map((c) => c.id)).toEqual(['id-2']);
      expect(tc.listCases('nope')).toEqual([]);
      expect(b.params).toEqual({ topK: 10, useRerank: false, pathPrefix: '' });
      expect(tc.getCase('col-1', a.id)?.query).toBe('refund?');
      // 跨 collection 取不到
      expect(tc.getCase('col-2', a.id)).toBeNull();
    });

    it('空名 / 空 query 抛错', () => {
      const tc = useHitTestCases({ storage: createMockStorage() });
      expect(() => tc.addCase('c', { name: '   ', query: 'q' })).toThrow(/name/);
      expect(() => tc.addCase('c', { name: 'n', query: '  ' })).toThrow(/query/);
    });

    it('deleteCase 删除并返回 true；未知 id 返回 false', () => {
      const tc = useHitTestCases({ storage: createMockStorage(), idGen: seqIds() });
      const a = tc.addCase('c', { name: 'a', query: 'q' });
      expect(tc.deleteCase('c', 'missing')).toBe(false);
      expect(tc.deleteCase('c', a.id)).toBe(true);
      expect(tc.listCases('c')).toEqual([]);
    });
  });

  describe('持久化', () => {
    it('写入 storage，新实例（同 storage）可复现', () => {
      const s = createMockStorage();
      const tc1 = useHitTestCases({ storage: s, idGen: seqIds() });
      tc1.addCase('col-1', { name: 'keep', query: 'persist me', params: { topK: 3 } });
      const raw = s.map.get(HIT_TEST_CASES_STORAGE_KEY);
      expect(raw).toBeTruthy();
      expect(JSON.parse(raw as string).collections['col-1']).toHaveLength(1);
      const tc2 = useHitTestCases({ storage: s });
      expect(tc2.listCases('col-1').map((c) => c.name)).toEqual(['keep']);
      expect(tc2.listCases('col-1')[0].params.topK).toBe(3);
    });

    it('真实 localStorage 链路可用（happy-dom）', () => {
      localStorage.removeItem(HIT_TEST_CASES_STORAGE_KEY);
      const tc1 = useHitTestCases({ idGen: seqIds() });
      tc1.addCase('col-x', { name: 'ls', query: 'via real localStorage' });
      expect(localStorage.getItem(HIT_TEST_CASES_STORAGE_KEY)).toContain('col-x');
      const tc2 = useHitTestCases();
      expect(tc2.listCases('col-x')[0]?.name).toBe('ls');
      tc2.deleteCase('col-x', tc2.listCases('col-x')[0].id);
    });

    it('localStorage 读写抛错时降级内存态：用例在会话内仍可用', () => {
      const throwing = createMockStorage({ throwOnGet: true, throwOnSet: true });
      const tc = useHitTestCases({ storage: throwing, idGen: seqIds() });
      // 构造时 load 走 getItem → 抛错被吞 → 空库起步
      const a = tc.addCase('c', { name: 'mem', query: 'q' });
      tc.addRun('c', a.id, mkRun('2026-10-20T00:00:00.000Z'));
      expect(tc.listCases('c')).toHaveLength(1);
      expect(tc.getCase('c', a.id)?.runs).toHaveLength(1);
      expect(throwing.map.size).toBe(0);
    });

    it('落库内容损坏（非法 JSON / 缺 collections）时安静回退空库', () => {
      const s = createMockStorage();
      s.setItem(HIT_TEST_CASES_STORAGE_KEY, '{not-json');
      expect(useHitTestCases({ storage: s }).listCases('c')).toEqual([]);
      s.setItem(HIT_TEST_CASES_STORAGE_KEY, JSON.stringify({ hello: 1 }));
      expect(useHitTestCases({ storage: s }).listCases('c')).toEqual([]);
      // 存在非法 case 条目时被过滤
      s.setItem(
        HIT_TEST_CASES_STORAGE_KEY,
        JSON.stringify({
          collections: {
            c: [
              { id: 1 },
              { id: 'ok', name: 'n', query: 'q', params: { topK: 10, useRerank: false, pathPrefix: '' }, runs: [] },
            ],
          },
        }),
      );
      const tc = useHitTestCases({ storage: s });
      expect(tc.listCases('c').map((x) => x.id)).toEqual(['ok']);
    });
  });

  describe('addRun 回归历史', () => {
    it('最新在前，超过上限截断到 HIT_TEST_MAX_RUNS', () => {
      const tc = useHitTestCases({ storage: createMockStorage(), idGen: seqIds() });
      const a = tc.addCase('c', { name: 'case', query: 'q' });
      const total = HIT_TEST_MAX_RUNS + 5;
      for (let i = 1; i <= total; i += 1) {
        expect(tc.addRun('c', a.id, mkRun(`2026-10-20T00:00:${String(i).padStart(2, '0')}.000Z`, [i / 100]))).toBe(
          true,
        );
      }
      const runs = tc.getCase('c', a.id)?.runs ?? [];
      expect(runs).toHaveLength(HIT_TEST_MAX_RUNS);
      expect(runs[0].at).toBe(`2026-10-20T00:00:${String(total).padStart(2, '0')}.000Z`);
      expect(runs[0].topScores).toEqual([total / 100]);
    });

    it('未知用例返回 false 且不改库', () => {
      const s = createMockStorage();
      const tc = useHitTestCases({ storage: s, idGen: seqIds() });
      expect(tc.addRun('c', 'ghost', mkRun('2026-10-20T00:00:00Z'))).toBe(false);
      const a = tc.addCase('c', { name: 'case', query: 'q' });
      expect(tc.addRun('c', 'ghost', mkRun('2026-10-20T00:00:00Z'))).toBe(false);
      expect(tc.getCase('c', a.id)?.runs).toEqual([]);
    });
  });

  describe('exportCase / importCase', () => {
    it('导出→导入往返：新 id、空历史、name/query/params 一致', () => {
      const tc = useHitTestCases({ storage: createMockStorage(), idGen: seqIds() });
      const a = tc.addCase('c', {
        name: '回归用例',
        query: 'refund flow',
        params: { topK: 7, useRerank: true, pathPrefix: 'p/', filterJson: '{"a":1}', minScore: 0.2 },
      });
      tc.addRun('c', a.id, mkRun('2026-10-20T00:00:00Z', [0.9, 0.8]));
      const json = tc.exportCase('c', a.id);
      expect(json).toBeTruthy();
      const res = tc.importCase('c', json as string);
      expect(res.ok).toBe(true);
      if (!res.ok) return;
      expect(res.case.id).not.toBe(a.id);
      expect(res.case.name).toBe('回归用例');
      expect(res.case.query).toBe('refund flow');
      expect(res.case.params).toEqual({
        topK: 7,
        useRerank: true,
        pathPrefix: 'p/',
        filterJson: '{"a":1}',
        minScore: 0.2,
      });
      expect(res.case.runs).toEqual([]);
      expect(tc.listCases('c')).toHaveLength(2);
    });

    it('未导出过的用例导出为 null', () => {
      const tc = useHitTestCases({ storage: createMockStorage() });
      expect(tc.exportCase('c', 'missing')).toBeNull();
    });

    it('坏 JSON 报 invalid json', () => {
      const tc = useHitTestCases({ storage: createMockStorage() });
      const res = tc.importCase('c', '{oops');
      expect(res.ok).toBe(false);
      if (res.ok) return;
      expect(res.error).toBe('invalid json');
    });

    it('缺字段校验：缺 name / 缺 query / 非法 params 字段', () => {
      const tc = useHitTestCases({ storage: createMockStorage() });
      const bad1 = tc.importCase('c', JSON.stringify({ query: 'q' }));
      expect(bad1.ok).toBe(false);
      if (!bad1.ok) expect(bad1.error).toBe('missing name');
      const bad2 = tc.importCase('c', JSON.stringify({ name: 'n' }));
      expect(bad2.ok).toBe(false);
      if (!bad2.ok) expect(bad2.error).toBe('missing query');
      const bad3 = tc.importCase('c', JSON.stringify({ name: 'n', query: 'q', params: { topK: -1 } }));
      expect(bad3.ok).toBe(false);
      if (!bad3.ok) expect(bad3.error).toBe('invalid params.topK');
      const bad4 = tc.importCase('c', JSON.stringify({ name: 'n', query: 'q', params: { useRerank: 'yes' } }));
      expect(bad4.ok).toBe(false);
      if (!bad4.ok) expect(bad4.error).toBe('invalid params.useRerank');
      expect(tc.listCases('c')).toEqual([]);
    });

    it('裸 case 定义（无 {version,case} 包裹）也可导入，参数按 normalize 补缺省', () => {
      const tc = useHitTestCases({ storage: createMockStorage(), idGen: seqIds() });
      const res = tc.importCase('c', JSON.stringify({ name: 'bare', query: 'q', params: { topK: 4 } }));
      expect(res.ok).toBe(true);
      if (!res.ok) return;
      expect(res.case.params).toEqual({ topK: 4, useRerank: false, pathPrefix: '' });
    });
  });

  describe('纯函数', () => {
    it('normalizeParams：topK<=0/NaN 回退 10；可选字段仅在有效时保留', () => {
      expect(normalizeParams(undefined)).toEqual({ topK: 10, useRerank: false, pathPrefix: '' });
      expect(normalizeParams({ topK: 0 })).toEqual({ topK: 10, useRerank: false, pathPrefix: '' });
      expect(
        normalizeParams({ topK: 7.9, useRerank: true, pathPrefix: 'x/', filterJson: '', minScore: Number.NaN }),
      ).toEqual({ topK: 7, useRerank: true, pathPrefix: 'x/' });
    });

    it('validateCaseDefinition：合法定义返回 null', () => {
      expect(validateCaseDefinition(null)).toBe('not an object');
      expect(validateCaseDefinition({ name: 'n', query: 'q' })).toBeNull();
      expect(validateCaseDefinition({ name: 'n', query: 'q', params: { minScore: 'x' } })).toBe(
        'invalid params.minScore',
      );
      expect(validateCaseDefinition({ name: 'n', query: 'q', params: { filterJson: 1 } })).toBe(
        'invalid params.filterJson',
      );
      expect(validateCaseDefinition({ name: 'n', query: 'q', params: { pathPrefix: 1 } })).toBe(
        'invalid params.pathPrefix',
      );
    });
  });
});
