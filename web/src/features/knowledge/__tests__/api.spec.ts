/**
 * knowledge api.ts 薄封装测试（SP1-I 修复：listBlockBacklinks 走 doc_id
 * additional_binding——生成客户端只实现主绑定 block_id 路径，doc 级调用必须
 * 经 kratosApi.get 直连 /v1/knowledge/documents/{doc_id}/block-backlinks）。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest';

const mockGet = vi.fn();
const mockPost = vi.fn();
const mockListGov = vi.fn();
const mockResolveGov = vi.fn();
const mockCurateNow = vi.fn();
vi.mock('../../../services/axiosHandler', () => ({
  kratosApi: {
    get: (...args: unknown[]) => mockGet(...args),
    post: (...args: unknown[]) => mockPost(...args),
  },
}));
vi.mock('../../../services', () => ({
  createKnowledgeService: () => ({
    ListGovernanceProposals: (...args: unknown[]) => mockListGov(...args),
    ResolveGovernanceProposal: (...args: unknown[]) => mockResolveGov(...args),
    CurateKnowledgeNow: (...args: unknown[]) => mockCurateNow(...args),
  }),
}));

import {
  curateKnowledge,
  listBlockBacklinks,
  listGovernanceProposals,
  listSourceConnectors,
  listSourceSyncItems,
  listSourceSyncRuns,
  listUnlinkedMentions,
  resolveGovernanceProposal,
  updateDocumentVisibility,
} from '../api';

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

  it('listGovernanceProposals 映射 snake/camel 字段（G-30：分页入参 + total 透传）', async () => {
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
      total: 42,
    });
    const out = await listGovernanceProposals('c1', 'pending', { limit: 20, offset: 20 });
    expect(mockListGov).toHaveBeenCalledWith({ collectionId: 'c1', status: 'pending', limit: 20, offset: 20 });
    expect(out).toEqual({
      items: [
        {
          id: 12,
          collection_id: 'c1',
          kind: 'conflict',
          risk: 'high',
          status: 'pending',
          payload_json: '{"target_fact_id":"fid-old","new_fact_id":"fid-new"}',
          created_at: '2026-08-17T12:00:00Z',
        },
      ],
      total: 42,
    });
  });

  it('listGovernanceProposals 缺 total 时回退 items 长度', async () => {
    mockListGov.mockResolvedValue({ items: [] });
    await expect(listGovernanceProposals('c1')).resolves.toEqual({ items: [], total: 0 });
    expect(mockListGov).toHaveBeenCalledWith({ collectionId: 'c1', status: 'pending', limit: 20, offset: 0 });
  });

  it('curateKnowledge 手动触发治理（G-30）：dry_run 透传', async () => {
    mockCurateNow.mockResolvedValue({ status: 'curating' });
    await expect(curateKnowledge('c1', true)).resolves.toEqual({ status: 'curating' });
    expect(mockCurateNow).toHaveBeenCalledWith({ collectionId: 'c1', dryRun: true });
  });

  it('resolveGovernanceProposal 传 keep_new', async () => {
    mockResolveGov.mockResolvedValue({ id: 12, status: 'applied' });
    await expect(resolveGovernanceProposal(12, 'keep_new')).resolves.toEqual({ id: 12, status: 'applied' });
    expect(mockResolveGov).toHaveBeenCalledWith({ id: 12, decision: 'keep_new' });
  });
});

describe('updateDocumentVisibility', () => {
  beforeEach(() => {
    mockPost.mockReset();
  });

  it('POST documents/{id}/visibility 并映射字段', async () => {
    mockPost.mockResolvedValue({
      data: { id: 'd1', visibility: 'private', owner_user_id: '7' },
    });
    const out = await updateDocumentVisibility('doc/特殊 id', 'private');
    expect(mockPost).toHaveBeenCalledWith('/v1/knowledge/documents/doc%2F%E7%89%B9%E6%AE%8A%20id/visibility', {
      visibility: 'private',
    });
    expect(out).toEqual({ id: 'd1', visibility: 'private', owner_user_id: '7', shared_with: [] });
  });
});

it('sends scoped Agent sharing targets without changing them to human membership', async () => {
  mockPost.mockResolvedValueOnce({
    data: { id: 'd', visibility: 'shared', owner_user_id: '7', shared_with: ['department-1', 'position-2'] },
  });
  const result = await updateDocumentVisibility('d', 'shared', ['department-1', 'position-2']);
  expect(mockPost).toHaveBeenLastCalledWith('/v1/knowledge/documents/d/visibility', {
    visibility: 'shared',
    shared_with: ['department-1', 'position-2'],
  });
  expect(result.shared_with).toEqual(['department-1', 'position-2']);
});

// ───────────────────────── Source Sync（G-38 W8） ─────────────────────────

describe('sourceSync api', () => {
  beforeEach(() => {
    mockGet.mockReset();
  });

  it('listSourceConnectors maps snake_case view rows', async () => {
    mockGet.mockResolvedValueOnce({
      data: {
        items: [
          {
            id: 'c1',
            kind: 'notion',
            name: 'n1',
            collection_id: 'col1',
            config_json: '{"credential_key":"default"}',
            interval_seconds: 900,
            enabled: true,
            sync_state: 'auth_paused',
            next_run_at: '2026-10-28T01:00:00Z',
            last_run_at: '',
            last_success_at: '2026-10-27T10:00:00Z',
            consecutive_failures: 0,
            error_code: 'credential_missing',
            error_message: '',
            max_items_per_run: 500,
            max_deletes_per_run: 200,
            max_fetch_bytes_per_run: 536870912,
            created_at: '2026-10-27T09:00:00Z',
            updated_at: '2026-10-27T09:00:00Z',
          },
        ],
      },
    });
    const rows = await listSourceConnectors();
    expect(mockGet).toHaveBeenCalledWith('/v1/knowledge/connectors');
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      id: 'c1',
      kind: 'notion',
      sync_state: 'auth_paused',
      error_code: 'credential_missing',
      max_fetch_bytes_per_run: 536870912,
    });
  });

  it('listSourceSyncItems passes status filter', async () => {
    mockGet.mockResolvedValueOnce({ data: { items: [] } });
    await listSourceSyncItems('c/特殊', 'missing', 50);
    expect(mockGet).toHaveBeenCalledWith('/v1/knowledge/connectors/c%2F%E7%89%B9%E6%AE%8A/items', {
      params: { limit: 50, status: 'missing' },
    });
  });

  it('listSourceSyncRuns omits status param', async () => {
    mockGet.mockResolvedValueOnce({ data: { items: [] } });
    await listSourceSyncRuns('c1', 10);
    expect(mockGet).toHaveBeenCalledWith('/v1/knowledge/connectors/c1/runs', { params: { limit: 10 } });
  });
});
