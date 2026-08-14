// buildImportDecisions — 导入向导 decisions 组装纯函数契约：
// 普通候选 import_passed / 风险放行 approve_risky_import / 风险拒绝 reject_risky_upload /
// keep_separate / 炼化合并 merge_group_with_ai（group_id 经 candidate_ids 匹配，无匹配时为空串）。
import { describe, it, expect } from 'vitest';
import { buildImportDecisions } from '../importDecisions';
import type {
  SkillConflictGroup,
  SkillImportCandidate,
  SkillImportJob,
  SkillRefineResult,
  SkillSimilarityMetrics,
} from '../types';

function makeMetrics(): SkillSimilarityMetrics {
  return {
    similarity_score: 0.9,
    name_similarity: 0.9,
    description_similarity: 0.8,
    body_similarity: 0.85,
    trigger_similarity: 0.7,
    tool_similarity: 0.6,
    conflict_risk: 'low',
    recommendation: 'keep_separate',
    confidence: 0.95,
  };
}

function makeCandidate(id: string, validationStatus: string): SkillImportCandidate {
  return {
    candidate_id: id,
    name: id,
    slug: id,
    description: '',
    body_preview: '',
    target_dir: `/tmp/${id}`,
    validation_status: validationStatus,
    status_icon: '',
    warnings: [],
    blocks: [],
  };
}

function makeGroup(groupId: string, candidateIds: string[]): SkillConflictGroup {
  return {
    group_id: groupId,
    highest_similarity_score: 0.9,
    metrics: makeMetrics(),
    reason: '',
    evidence: [],
    candidate_ids: candidateIds,
    existing_skills: [],
    can_refine: true,
  };
}

function makeJob(overrides: Partial<SkillImportJob> = {}): SkillImportJob {
  return {
    job_id: 'job-1',
    status: 'completed',
    validation_status: 'warn',
    storage_root: '/tmp',
    candidates: [],
    conflict_groups: [],
    ...overrides,
  };
}

const refineResult: SkillRefineResult = {
  merged_name: 'Merged',
  merged_description: 'merged desc',
  merged_body: 'merged body',
  merged_tags: [{ name: 'domain:x', source: 'user' }],
  source_candidate_ids: ['c2'],
  source_existing_skill_ids: ['e1'],
};

describe('buildImportDecisions', () => {
  it('普通候选：仅 validation_status=pass 的候选生成 import_passed', () => {
    const job = makeJob({
      candidates: [makeCandidate('c1', 'pass'), makeCandidate('c2', 'warn'), makeCandidate('c3', 'block')],
    });
    const decisions = buildImportDecisions(job, null, [], [], []);
    expect(decisions).toEqual([{ candidate_id: 'c1', action: 'import_passed' }]);
  });

  it('风险放行：approved 候选生成 approve_risky_import', () => {
    const job = makeJob({ candidates: [makeCandidate('c1', 'block')] });
    const decisions = buildImportDecisions(job, null, ['c1'], [], []);
    expect(decisions).toEqual([{ candidate_id: 'c1', action: 'approve_risky_import' }]);
  });

  it('风险拒绝：rejected 候选生成 reject_risky_upload', () => {
    const job = makeJob({ candidates: [makeCandidate('c1', 'block')] });
    const decisions = buildImportDecisions(job, null, [], ['c1'], []);
    expect(decisions).toEqual([{ candidate_id: 'c1', action: 'reject_risky_upload' }]);
  });

  it('keep_separate：kept 候选生成 keep_separate', () => {
    const job = makeJob({ candidates: [makeCandidate('c1', 'pass'), makeCandidate('c2', 'pass')] });
    const decisions = buildImportDecisions(job, null, [], [], ['c1', 'c2']);
    expect(decisions).toEqual([
      { candidate_id: 'c1', action: 'import_passed' },
      { candidate_id: 'c2', action: 'import_passed' },
      { candidate_id: 'c1', action: 'keep_separate' },
      { candidate_id: 'c2', action: 'keep_separate' },
    ]);
  });

  it('炼化合并：refineResult 生成 merge_group_with_ai，group_id 按 source_candidate_ids 匹配', () => {
    const job = makeJob({
      candidates: [makeCandidate('c1', 'pass')],
      conflict_groups: [makeGroup('g-other', ['c9']), makeGroup('g-hit', ['c2', 'c3'])],
    });
    const decisions = buildImportDecisions(job, refineResult, [], [], []);
    expect(decisions).toEqual([
      { candidate_id: 'c1', action: 'import_passed' },
      {
        group_id: 'g-hit',
        action: 'merge_group_with_ai',
        merged_name: 'Merged',
        merged_description: 'merged desc',
        merged_body: 'merged body',
        merged_tags: [{ name: 'domain:x', source: 'user' }],
      },
    ]);
  });

  it('炼化合并：无匹配冲突组时 group_id 为空串', () => {
    const job = makeJob({ conflict_groups: [makeGroup('g-other', ['c9'])] });
    const decisions = buildImportDecisions(job, refineResult, [], [], []);
    expect(decisions).toHaveLength(1);
    expect(decisions[0]?.action).toBe('merge_group_with_ai');
    expect(decisions[0]?.group_id).toBe('');
  });
});
