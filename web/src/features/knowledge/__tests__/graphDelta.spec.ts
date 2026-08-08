// SP1-I（I-4）：knowledge.graph.delta WS 增量——Meta 解析 + 受影响面提取。
import { describe, expect, it } from 'vitest';
import { graphDeltaAffected, parseGraphDeltaMeta } from '../graphDelta';

const edge = {
  collection_id: 'c1',
  src_block_id: 'sb1',
  src_doc_id: 'd1',
  dst_collection_id: 'c2',
  dst_doc_id: 'd2',
  dst_block_id: 'db2',
  raw_target: '目标',
  edge_type: 'ref',
  context: '见 [[目标]]',
  ambiguous: false,
};

describe('parseGraphDeltaMeta（I-4 delta Meta 解析）', () => {
  it('parses added/removed edges and version', () => {
    const d = parseGraphDeltaMeta({ version: 7, added: [edge], removed: [edge] });
    expect(d?.version).toBe(7);
    expect(d?.added).toHaveLength(1);
    expect(d?.removed).toHaveLength(1);
    expect(d?.added[0]).toMatchObject({ src_doc_id: 'd1', dst_doc_id: 'd2', raw_target: '目标' });
  });

  it('returns null for missing/non-object meta', () => {
    expect(parseGraphDeltaMeta(null)).toBeNull();
    expect(parseGraphDeltaMeta(undefined)).toBeNull();
    expect(parseGraphDeltaMeta('x')).toBeNull();
  });

  it('tolerates missing arrays and non-number version', () => {
    const d = parseGraphDeltaMeta({ version: '9' });
    expect(d).toEqual({ version: 9, added: [], removed: [] });
  });

  it('drops malformed edge entries', () => {
    const d = parseGraphDeltaMeta({ version: 1, added: [null, 'x', { src_doc_id: 'd1' }] });
    expect(d?.added).toHaveLength(1);
    expect(d?.added[0].src_doc_id).toBe('d1');
  });
});

describe('graphDeltaAffected（I-4 受影响 doc/collection 提取）', () => {
  it('collects src+dst doc ids and both collection ids, deduped', () => {
    const a = graphDeltaAffected({ version: 1, added: [edge], removed: [] });
    expect(a.docIds.sort()).toEqual(['d1', 'd2']);
    expect(a.collectionIds.sort()).toEqual(['c1', 'c2']);
  });

  it('dedupes across added and removed, skipping empty ids', () => {
    const dangling = { ...edge, dst_collection_id: '', dst_doc_id: '' };
    const a = graphDeltaAffected({ version: 2, added: [edge], removed: [dangling] });
    expect(a.docIds).toEqual(['d1', 'd2']);
    expect(a.collectionIds).toEqual(['c1', 'c2']);
  });

  it('empty delta yields empty affected sets', () => {
    expect(graphDeltaAffected({ version: 0, added: [], removed: [] })).toEqual({ docIds: [], collectionIds: [] });
  });
});
