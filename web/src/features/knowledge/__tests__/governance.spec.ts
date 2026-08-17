import { describe, expect, it } from 'vitest';
import {
  decisionsForProposal,
  isFactConflict,
  parseProposalPayload,
  proposalSummary,
} from '../governance';

describe('parseProposalPayload', () => {
  it('解析事实冲突字段', () => {
    const payload = parseProposalPayload(
      JSON.stringify({
        doc_id: 'd1',
        target_fact_id: 'fid-old',
        new_fact_id: 'fid-new',
        new_statement: '夜班改为 45 分钟',
        confidence: 0.91,
      }),
    );
    expect(payload.target_fact_id).toBe('fid-old');
    expect(payload.new_statement).toBe('夜班改为 45 分钟');
    expect(payload.confidence).toBe('0.91');
  });

  it('非法 JSON 返回空对象', () => {
    expect(parseProposalPayload('{')).toEqual({});
    expect(parseProposalPayload('')).toEqual({});
    expect(parseProposalPayload('[]')).toEqual({});
  });
});

describe('decisionsForProposal', () => {
  it('事实段 conflict 只给 keep_old / keep_new / rejected', () => {
    const payload = { target_fact_id: 'fid-old', new_fact_id: 'fid-new', doc_id: 'd1' };
    expect(isFactConflict('conflict', payload)).toBe(true);
    expect(decisionsForProposal('conflict', payload)).toEqual(['keep_old', 'keep_new', 'rejected']);
  });

  it('文档级 conflict 与 orphan 走 applied / rejected', () => {
    expect(decisionsForProposal('conflict', { doc_id: 'a', target_doc_id: 'b' })).toEqual([
      'applied',
      'rejected',
    ]);
    expect(decisionsForProposal('orphan', { doc_id: 'd1', rel_path: 'entries/x.md' })).toEqual([
      'applied',
      'rejected',
    ]);
  });
});

describe('proposalSummary', () => {
  it('事实冲突优先展示新陈述', () => {
    expect(
      proposalSummary('conflict', {
        target_fact_id: 'fid-old',
        new_statement: '夜班改为 45 分钟',
      }),
    ).toBe('夜班改为 45 分钟');
  });

  it('孤儿词条展示路径', () => {
    expect(proposalSummary('orphan', { rel_path: 'entries/stale.md', doc_id: 'd1' })).toBe(
      'entries/stale.md',
    );
  });
});
