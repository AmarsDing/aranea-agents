/**
 * knowledge api.ts 薄封装测试（SP1-I 修复：listBlockBacklinks 走 doc_id
 * additional_binding——生成客户端只实现主绑定 block_id 路径，doc 级调用必须
 * 经 kratosApi.get 直连 /v1/knowledge/documents/{doc_id}/block-backlinks）。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest';

const mockGet = vi.fn();
const mockListGov = vi.fn();
const mockResolveGov = vi.fn();
vi.mock('../../../services/axiosHandler', () => ({
  kratosApi: { get: (...args: unknown[]) => mockGet(...args) },
}));
vi.mock('../../../services', () => ({
  createKnowledgeService: () => ({
    ListGovernanceProposals: (...args: unknown[]) => mockListGov(...args),
    ResolveGovernanceProposal: (...args: unknown[]) => mockResolveGov(...args),
  }),
}));

import { listBlockBacklinks, listGovernanceProposals, listUnlinkedMentions, resolveGovernanceProposal } from '../api';

describe('listBlockBacklinks（SP1-I：doc_id 附加绑定直连）', () => {
  beforeEach(() => {
    mockGet.mockReset();
  });

  it('经 documents/{doc_id}/block-backlinks 路由请求并映射 snake_case 字段', async () => {
    // kratosApi.get 裸调返回 AxiosResponse（载荷在 .data）——mock 必须保持真实形状，
    // 否则映射层漏 .data 时测试假绿（2026-08-09 运行时事故：UI 恒空）。
    mockGet.mockResolvedValue({
      data: {
        items: [
          {
            src_block_id: 'b1',
            src_doc_id: 'd1',
            src_collection_id: 'c1',
            src_doc_name: '指南/快速上手.md',
            raw_target: '笔记/设计原则#目标',
            edge_type: 'ref',
            context: '上下文片段',
            ambiguous: false,
          },
        ],
      },
    });
    const out = await listBlockBacklinks('doc/特殊 id');
    expect(mockGet).toHaveBeenCalledTimes(1);
    const url = String(mockGet.mock.calls[0][0]);
    expect(url).toBe('/v1/knowledge/documents/doc%2F%E7%89%B9%E6%AE%8A%20id/block-backlinks');
    expect(out).toHaveLength(1);
    expect(out[0]).toMatchObject({
      src_block_id: 'b1',
      src_doc_id: 'd1',
      src_doc_name: '指南/快速上手.md',
      raw_target: '笔记/设计原则#目标',
      edge_type: 'ref',
      ambiguous: false,
    });
  });

  it('响应缺 items 时返回空数组', async () => {
    mockGet.mockResolvedValue({ data: {} });
    await expect(listBlockBacklinks('d1')).resolves.toEqual([]);
  });
});

describe('listUnlinkedMentions（P2-7：doc_id 直连）', () => {
  beforeEach(() => {
    mockGet.mockReset();
  });

  it('经 documents/{doc_id}/unlinked-mentions 路由请求并映射 snake_case 字段', async () => {
    // kratosApi.get 裸调返回 AxiosResponse（载荷在 .data）——mock 保持真实形状。
    mockGet.mockResolvedValue({
      data: {
        items: [{ src_doc_id: 'd2', src_doc_name: 'notes/a.md', count: 2, snippet: '…上下文…' }],
      },
    });
    const out = await listUnlinkedMentions('doc/特殊 id');
    expect(mockGet).toHaveBeenCalledTimes(1);
    expect(String(mockGet.mock.calls[0][0])).toBe(
      '/v1/knowledge/documents/doc%2F%E7%89%B9%E6%AE%8A%20id/unlinked-mentions',
    );
    expect(out).toEqual([{ src_doc_id: 'd2', src_doc_name: 'notes/a.md', count: 2, snippet: '…上下文…' }]);
  });

  it('响应缺 items 时返回空数组', async () => {
    mockGet.mockResolvedValue({ data: {} });
    await expect(listUnlinkedMentions('d1')).resolves.toEqual([]);
  });
});

describe('governance proposals（US-52）', () => {
  beforeEach(() => {
    mockListGov.mockReset();
    mockResolveGov.mockReset();
  });

  it('listGovernanceProposals 映射 snake/camel 字段', async () => {
    mockListGov.mockResolvedValue({
      items: [
        {
          id: '12',
          collectionId: 'c1',
          kind: 'conflict',
          risk: 'high',
          status: 'pending',
          payloadJson: '{"target_fact_id":"fid-old","new_fact_id":"fid-new"}',
          createdAt: '2026-08-17T12:00:00Z',
        },
      ],
    });
    const out = await listGovernanceProposals('c1');
    expect(mockListGov).toHaveBeenCalledWith({ collectionId: 'c1', status: 'pending', limit: 50 });
    expect(out).toEqual([
      {
        id: 12,
        collection_id: 'c1',
        kind: 'conflict',
        risk: 'high',
        status: 'pending',
        payload_json: '{"target_fact_id":"fid-old","new_fact_id":"fid-new"}',
        created_at: '2026-08-17T12:00:00Z',
      },
    ]);
  });

  it('resolveGovernanceProposal 传 keep_new', async () => {
    mockResolveGov.mockResolvedValue({ id: 12, status: 'applied' });
    await expect(resolveGovernanceProposal(12, 'keep_new')).resolves.toEqual({ id: 12, status: 'applied' });
    expect(mockResolveGov).toHaveBeenCalledWith({ id: 12, decision: 'keep_new' });
  });
});
