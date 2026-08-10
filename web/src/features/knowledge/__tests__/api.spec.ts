/**
 * knowledge api.ts 薄封装测试（SP1-I 修复：listBlockBacklinks 走 doc_id
 * additional_binding——生成客户端只实现主绑定 block_id 路径，doc 级调用必须
 * 经 kratosApi.get 直连 /v1/knowledge/documents/{doc_id}/block-backlinks）。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest';

const mockGet = vi.fn();
vi.mock('../../../services/axiosHandler', () => ({
  kratosApi: { get: (...args: unknown[]) => mockGet(...args) },
}));
vi.mock('../../../services', () => ({
  createKnowledgeService: () => ({}),
}));

import { listBlockBacklinks } from '../api';

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
