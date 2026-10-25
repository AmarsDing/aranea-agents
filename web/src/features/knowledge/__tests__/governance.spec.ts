import { describe, expect, it } from 'vitest';
import { decisionsForProposal, isFactConflict, mocMembers, parseProposalPayload, proposalSummary } from '../governance';

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
    expect(decisionsForProposal('conflict', { doc_id: 'a', target_doc_id: 'b' })).toEqual(['applied', 'rejected']);
    expect(decisionsForProposal('orphan', { doc_id: 'd1', rel_path: 'entries/x.md' })).toEqual(['applied', 'rejected']);
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
    expect(proposalSummary('orphan', { rel_path: 'entries/stale.md', doc_id: 'd1' })).toBe('entries/stale.md');
  });
});

describe('moc_emerge（G-34）', () => {
  it('summary 取 suggested_title，缺失回退 hub_rel_path', () => {
    expect(proposalSummary('moc_emerge', { suggested_title: '发布体系', hub_rel_path: 'entries/发布体系.md' })).toBe(
      '发布体系',
    );
    expect(proposalSummary('moc_emerge', { hub_rel_path: 'entries/发布体系.md' })).toBe('entries/发布体系.md');
  });

  it('决策走默认 applied / rejected', () => {
    expect(decisionsForProposal('moc_emerge', { hub_doc_id: 'd-hub' })).toEqual(['applied', 'rejected']);
  });

  it('mocMembers 反解逗号串接的成员清单', () => {
    expect(mocMembers({ members: 'entries/a.md,entries/b.md' })).toEqual(['entries/a.md', 'entries/b.md']);
    expect(mocMembers({ members: ' entries/a.md , entries/b.md ' })).toEqual(['entries/a.md', 'entries/b.md']);
    expect(mocMembers({})).toEqual([]);
    expect(mocMembers({ members: '' })).toEqual([]);
  });

  it('F14：成员路径含逗号不被切碎——数组 JSON 保形经 JSON.parse 反解', () => {
    // 新格式：parseProposalPayload 把数组值 JSON.stringify 保形
    const payload = parseProposalPayload(
      JSON.stringify({ members: ['entries/a,b.md', 'entries/c.md'], doc_id: 'd1' }),
    );
    expect(payload.members).toBe('["entries/a,b.md","entries/c.md"]');
    expect(payload.doc_id).toBe('d1');
    // 成员反解：逗号元素不被切碎成幻影成员
    expect(mocMembers(payload)).toEqual(['entries/a,b.md', 'entries/c.md']);
    // 旧格式（String() 逗号串接的落库数据）回退逗号反解仍兼容
    expect(mocMembers({ members: 'entries/a.md,entries/b.md' })).toEqual(['entries/a.md', 'entries/b.md']);
    // 畸形 JSON 串回退逗号反解
    expect(mocMembers({ members: '["broken' })).toEqual(['["broken']);
  });
});

describe('entity_merge（G-37）', () => {
  it('summary 直陈合并方向「keeper ← mergee」，缺失回退 dedup_key', () => {
    expect(proposalSummary('entity_merge', { keeper_name: 'RAG', mergee_name: '检索增强生成' })).toBe(
      'RAG ← 检索增强生成',
    );
    // 缺一名回退 dedup_key（ADR-D 载荷纪律：dedup_key 恒在）
    expect(
      proposalSummary('entity_merge', {
        keeper_name: '',
        mergee_name: '',
        dedup_key: 'entity_merge:1:2:abcd1234ef',
      }),
    ).toBe('entity_merge:1:2:abcd1234ef');
  });

  it('决策走默认 applied / rejected（keep_* 守卫不松绑——ADR-D）', () => {
    expect(decisionsForProposal('entity_merge', { keeper_id: '1', mergee_id: '2' })).toEqual(['applied', 'rejected']);
  });

  it('parseProposalPayload 数值 id 与 sources 数组保形', () => {
    const payload = parseProposalPayload(
      JSON.stringify({ keeper_id: 1, mergee_id: 2, similarity: '0.86', sources: ['ann', 'norm'] }),
    );
    expect(payload.keeper_id).toBe('1');
    expect(payload.mergee_id).toBe('2');
    expect(payload.similarity).toBe('0.86');
    expect(payload.sources).toBe('["ann","norm"]');
  });
});
