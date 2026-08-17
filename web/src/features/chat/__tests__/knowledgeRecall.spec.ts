import { describe, it, expect } from 'vitest';
import {
  parseKnowledgeRecallChunks,
  mergeKnowledgeRecallChunks,
  assignCitationNumbers,
  linkKnowledgeCitations,
  KNOWLEDGE_RECALLED_NOTICE_TYPE,
} from '../knowledgeRecall';

describe('knowledgeRecall', () => {
  it('exposes the backend notice type constant', () => {
    expect(KNOWLEDGE_RECALLED_NOTICE_TYPE).toBe('knowledge_recalled');
  });

  it('parses a valid payload', () => {
    const content = JSON.stringify({
      chunks: [
        { n: 1, chunk_id: 'k1', doc_id: 'd1', score: 0.91, line: 'SLA 承诺 99.9%' },
        { chunk_id: 'k2', score: 0.4, line: '值班电话' },
      ],
    });
    const chunks = parseKnowledgeRecallChunks(content);
    expect(chunks).toHaveLength(2);
    expect(chunks[0]).toEqual({ n: 1, chunk_id: 'k1', doc_id: 'd1', score: 0.91, line: 'SLA 承诺 99.9%' });
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

  it('merges by chunk_id without duplicates and assigns [n]', () => {
    const a = [{ chunk_id: 'k1', line: 'a', score: 1, n: 1 }];
    const b = [
      { chunk_id: 'k1', line: 'a-dup', score: 0.2, n: 1 },
      { chunk_id: 'k2', line: 'b', score: 0.5 },
    ];
    expect(mergeKnowledgeRecallChunks(a, b)).toEqual([
      { chunk_id: 'k1', line: 'a', score: 1, n: 1 },
      { chunk_id: 'k2', line: 'b', score: 0.5, n: 2 },
    ]);
  });

  it('keeps payload n and fills gaps', () => {
    expect(
      assignCitationNumbers([
        { chunk_id: 'k2', line: 'b', score: 0, n: 2 },
        { chunk_id: 'k1', line: 'a', score: 0 },
      ]),
    ).toEqual([
      { chunk_id: 'k2', line: 'b', score: 0, n: 2 },
      { chunk_id: 'k1', line: 'a', score: 0, n: 1 },
    ]);
  });

  it('rewrites reply [n] into footnote buttons and skips code', () => {
    const chunks = assignCitationNumbers([
      { chunk_id: 'k1', doc_id: 'd1', line: 'SLA', score: 1, n: 1 },
    ]);
    const html = '<p>值班 [1] 见 <code>[1]</code></p>';
    expect(linkKnowledgeCitations(html, chunks)).toBe(
      '<p>值班 <button type="button" class="kb-cite" data-n="1" data-doc-id="d1">[1]</button> 见 <code>[1]</code></p>',
    );
    expect(linkKnowledgeCitations('<p>无引用</p>', chunks)).toBe('<p>无引用</p>');
  });
});
