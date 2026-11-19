import { beforeEach, describe, expect, it, vi } from 'vitest';

// W1c schedule_draft 决议出口测试：批准/拒绝走 office 域 RPC（聚合端点只读，
// 不另开写旁路）；列表映射钉死 APPROVAL_KIND_SCHEDULE_DRAFT ↔ schedule_draft
// 与 context_json 解析。

const approveMock = vi.fn();
const rejectMock = vi.fn();
const listMock = vi.fn();
const reviewTaskMock = vi.fn();

vi.mock('../../../services', () => ({
  createAgentBridgeService: () => ({ ReviewTask: reviewTaskMock }),
  createAgentService: () => ({}),
  createApprovalCenterService: () => ({ ListApprovals: listMock }),
  createOfficeScheduleService: () => ({
    ApproveScheduleDraft: approveMock,
    RejectScheduleDraft: rejectMock,
  }),
  createSkillEvolutionSuggestionService: () => ({}),
}));

import { decideApproval, listApprovals } from '../api';
import type { ApprovalItemRow } from '../types';

function scheduleItem(overrides: Partial<ApprovalItemRow> = {}): ApprovalItemRow {
  return {
    kind: 'schedule_draft',
    id: 'sd_abc',
    title: '排期A 等 2 项',
    summary: '来自 ag-1 的排期建议，共 2 项',
    context: { agent_id: 'ag-1', item_count: 2 },
    context_raw: '',
    risk_level: 'medium',
    requester: 'ag-1',
    agent_key: 'ag-1',
    created_at: '2026-11-09T10:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  approveMock.mockReset();
  rejectMock.mockReset();
  listMock.mockReset();
  reviewTaskMock.mockReset();
});

describe('decideApproval schedule_draft', () => {
  it('approve → ApproveScheduleDraft({id})（批准=服务端单事务逐项 409 复检+落库）', async () => {
    approveMock.mockResolvedValue({});
    await decideApproval({ item: scheduleItem(), action: 'approve', note: '', actor: 'u-1' });
    expect(approveMock).toHaveBeenCalledTimes(1);
    expect(approveMock).toHaveBeenCalledWith({ id: 'sd_abc' });
    expect(rejectMock).not.toHaveBeenCalled();
    expect(reviewTaskMock).not.toHaveBeenCalled();
  });

  it('reject → RejectScheduleDraft({id, reason})（理由随请求，服务端空理由 400）', async () => {
    rejectMock.mockResolvedValue({});
    await decideApproval({ item: scheduleItem(), action: 'reject', note: '时段撞车', actor: 'u-1' });
    expect(rejectMock).toHaveBeenCalledTimes(1);
    expect(rejectMock).toHaveBeenCalledWith({ id: 'sd_abc', reason: '时段撞车' });
    expect(approveMock).not.toHaveBeenCalled();
  });

  it('无决议路径的 kind（memory_fact 预留）抛错而非误路由', async () => {
    await expect(
      decideApproval({ item: scheduleItem({ kind: 'memory_fact' }), action: 'approve', note: '', actor: 'u-1' }),
    ).rejects.toThrow('no decide path');
    expect(approveMock).not.toHaveBeenCalled();
  });
});

describe('listApprovals schedule_draft 映射', () => {
  it('APPROVAL_KIND_SCHEDULE_DRAFT → schedule_draft，context_json 解析为对象', async () => {
    listMock.mockResolvedValue({
      items: [
        {
          kind: 'APPROVAL_KIND_SCHEDULE_DRAFT',
          id: 'sd_1',
          title: '排期A 等 2 项',
          summary: '来自 ag-1 的排期建议，共 2 项',
          contextJson: JSON.stringify({ agent_id: 'ag-1', item_count: 2, items: [{ title: '排期A' }] }),
          riskLevel: 'RISK_LEVEL_MEDIUM',
          requester: 'ag-1',
          createdAt: '2026-11-09T10:00:00Z',
        },
      ],
      total: 1,
    });
    const res = await listApprovals({ kind: 'schedule_draft', page: 1, pageSize: 20 });
    expect(listMock).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'APPROVAL_KIND_SCHEDULE_DRAFT', page: 1, pageSize: 20 }),
    );
    expect(res.total).toBe(1);
    const row = res.items[0]!;
    expect(row.kind).toBe('schedule_draft');
    expect(row.risk_level).toBe('medium');
    expect(row.context?.agent_id).toBe('ag-1');
    expect(row.context_raw).toBe('');
  });

  it('context_json 非法 JSON → context=null 且保留原文（诚实失败回退）', async () => {
    listMock.mockResolvedValue({
      items: [
        {
          kind: 'APPROVAL_KIND_SCHEDULE_DRAFT',
          id: 'sd_2',
          title: 't',
          summary: 's',
          contextJson: '{not-json',
          riskLevel: 'RISK_LEVEL_MEDIUM',
          requester: 'ag-1',
          createdAt: '2026-11-09T10:00:00Z',
        },
      ],
      total: 1,
    });
    const res = await listApprovals({});
    expect(res.items[0]!.context).toBeNull();
    expect(res.items[0]!.context_raw).toBe('{not-json');
  });
});
