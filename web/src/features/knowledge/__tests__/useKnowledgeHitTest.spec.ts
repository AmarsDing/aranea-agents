import { describe, expect, it } from 'vitest';
import { useKnowledgeHitTest, HIT_TEST_STRATEGIES, type HitTestFetcher } from '../useKnowledgeHitTest';
import type { KnowledgeHitTestResult } from '../types';

const mkResult = (id: string, extra?: Partial<KnowledgeHitTestResult>): KnowledgeHitTestResult => ({
  chunks: [
    {
      id: `${id}-1`,
      doc_id: 'd1',
      collection_id: 'c1',
      content: 'a',
      embedding: [],
      metadata_json: '{}',
      chunk_index: 0,
      score: 0.9,
      rrf_score: 0.03,
    },
    {
      id: `${id}-2`,
      doc_id: 'd2',
      collection_id: 'c1',
      content: 'b',
      embedding: [],
      metadata_json: '{}',
      chunk_index: 1,
      score: 0.8,
      rrf_score: 0.02,
    },
  ],
  mode_used: 'rrf',
  reranked: false,
  ...extra,
});

describe('useKnowledgeHitTest', () => {
  it('run 并行跑 dense/sparse/rrf 三个策略并分别结算', async () => {
    const seen: string[] = [];
    const fetcher: HitTestFetcher = async (q) => {
      seen.push(q.hybrid_search ?? '');
      return mkResult(q.hybrid_search ?? 'x');
    };
    const ht = useKnowledgeHitTest({ fetcher });
    ht.query.value = '  refund policy  ';
    await ht.run('col-1');
    expect(seen.sort()).toEqual([...HIT_TEST_STRATEGIES].sort());
    expect(ht.runs.value).toHaveLength(3);
    for (const row of ht.runs.value) {
      expect(row.loading).toBe(false);
      expect(row.error).toBe('');
      expect(row.result?.chunks.length).toBe(2);
    }
    expect(ht.running.value).toBe(false);
  });

  it('空 query / 空 collection 不发起请求', async () => {
    let calls = 0;
    const ht = useKnowledgeHitTest({
      fetcher: async () => {
        calls += 1;
        return mkResult('a');
      },
    });
    await ht.run('');
    ht.query.value = 'q';
    await ht.run('');
    expect(calls).toBe(0);
    expect(ht.runs.value).toHaveLength(0);
  });

  it('单策略失败不拖垮其他列（独立出错）', async () => {
    const fetcher: HitTestFetcher = async (q) => {
      if (q.hybrid_search === 'sparse') throw new Error('bm25 unavailable');
      return mkResult(q.hybrid_search ?? 'x');
    };
    const ht = useKnowledgeHitTest({ fetcher });
    ht.query.value = 'q';
    await ht.run('col-1');
    const sparse = ht.runs.value.find((r) => r.strategy === 'sparse');
    const dense = ht.runs.value.find((r) => r.strategy === 'dense');
    expect(sparse?.error).toBe('bm25 unavailable');
    expect(sparse?.result).toBeNull();
    expect(dense?.error).toBe('');
    expect(dense?.result?.chunks.length).toBe(2);
  });

  it('top_k <= 0 回退 10；rerank 开关与 path_prefix 透传', async () => {
    const got: Array<{ top_k?: number; use_rerank?: boolean; path_prefix?: string }> = [];
    const fetcher: HitTestFetcher = async (q) => {
      got.push({ top_k: q.top_k, use_rerank: q.use_rerank, path_prefix: q.path_prefix });
      return mkResult('x');
    };
    const ht = useKnowledgeHitTest({ fetcher });
    ht.query.value = 'q';
    ht.topK.value = 0;
    ht.useRerank.value = false;
    ht.queryPrefix.value = ' notes/ ';
    await ht.run('col-1');
    for (const g of got) {
      expect(g.top_k).toBe(10);
      expect(g.use_rerank).toBe(false);
      expect(g.path_prefix).toBe('notes/');
    }
  });

  it('preRerankRank：reranked 时按融合分还原前序（1-based），否则为 null', () => {
    const ht = useKnowledgeHitTest();
    // reranked=true 且 score 顺序与 rrf 顺序相反：score 把 c2 排前，rrf 把 c1 排前。
    const row = {
      strategy: 'rrf' as const,
      loading: false,
      error: '',
      elapsed_ms: 1,
      result: {
        chunks: [
          {
            id: 'c2',
            doc_id: 'd2',
            collection_id: 'c',
            content: '',
            embedding: [],
            metadata_json: '{}',
            chunk_index: 1,
            score: 0.95,
            rrf_score: 0.01,
            rerank_score: 0.95,
          },
          {
            id: 'c1',
            doc_id: 'd1',
            collection_id: 'c',
            content: '',
            embedding: [],
            metadata_json: '{}',
            chunk_index: 0,
            score: 0.9,
            rrf_score: 0.03,
            rerank_score: 0.9,
          },
        ],
        mode_used: 'rrf',
        reranked: true,
      },
    };
    const c1 = row.result.chunks[1];
    const c2 = row.result.chunks[0];
    expect(ht.preRerankRank(row, c1)).toBe(1); // 融合分更高 → 前序第 1
    expect(ht.preRerankRank(row, c2)).toBe(2); // 重排把它提到了第 1
    expect(ht.preRerankRank({ ...row, result: { ...row.result, reranked: false } }, c1)).toBeNull();
  });

  it('clear 清空状态', async () => {
    const ht = useKnowledgeHitTest({ fetcher: async () => mkResult('a') });
    ht.query.value = 'q';
    await ht.run('col-1');
    expect(ht.runs.value.length).toBe(3);
    ht.clear();
    expect(ht.runs.value).toHaveLength(0);
    expect(ht.query.value).toBe('');
  });
});
