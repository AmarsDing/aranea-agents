import { describe, it, expect } from 'vitest';
import {
  parseKnowledgeRecallChunks,
  mergeKnowledgeRecallChunks,
  KNOWLEDGE_RECALLED_NOTICE_TYPE,
} from '../knowledgeRecall';

describe('knowledgeRecall', () => {
  it('exposes the backend notice type constant', () => {
    expect(KNOWLEDGE_RECALLED_NOTICE_TYPE).toBe('knowledge_recalled');
  });

  it('parses a valid payload', () => {
    const content = JSON.stringify({
      chunks: [
        { chunk_id: 'k1', doc_id: 'd1', score: 0.91, line: 'SLA 承诺 99.9%' },
        { chunk_id: 'k2', score: 0.4, line: '值班电话' },
      ],
    });
    const chunks = parseKnowledgeRecallChunks(content);
    expect(chunks).toHaveLength(2);
    expect(chunks[0]).toEqual({ chunk_id: 'k1', doc_id: 'd1', score: 0.91, line: 'SLA 承诺 99.9%' });
    expect(chunks[1]).toEqual({ chunk_id: 'k2', score: 0.4, line: '值班电话' });
  });

  it('returns [] for empty or invalid content', () => {
    expect(parseKnowledgeRecallChunks('')).toEqual([]);
    expect(parseKnowledgeRecallChunks('{not json')).toEqual([]);
    expect(parseKnowledgeRecallChunks('{}')).toEqual([]);
    expect(parseKnowledgeRecallChunks('{"chunks":null}')).toEqual([]);
  });

  it('drops entries without chunk_id', () => {
    const content = JSON.stringify({
      chunks: [null, { line: 'x' }, { chunk_id: 'k1', line: 'ok' }],
    });
    expect(parseKnowledgeRecallChunks(content)).toEqual([{ chunk_id: 'k1', line: 'ok', score: 0 }]);
  });

  it('merges by chunk_id without duplicates', () => {
    const a = [{ chunk_id: 'k1', line: 'a', score: 1 }];
    const b = [
      { chunk_id: 'k1', line: 'a-dup', score: 0.2 },
      { chunk_id: 'k2', line: 'b', score: 0.5 },
    ];
    expect(mergeKnowledgeRecallChunks(a, b)).toEqual([
      { chunk_id: 'k1', line: 'a', score: 1 },
      { chunk_id: 'k2', line: 'b', score: 0.5 },
    ]);
  });
});
