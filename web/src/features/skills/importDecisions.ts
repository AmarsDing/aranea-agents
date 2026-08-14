import type { SkillConflictGroup, SkillImportDecision, SkillImportJob, SkillRefineResult } from './types';

/**
 * 导入向导 decisions 组装（纯函数）：
 * 校验通过候选 + 风险放行/拒绝 + keep_separate + 炼化结果合并。
 * 从 SkillUploadPlaceholder.applyImportResult 抽离，便于单测。
 */
export function buildImportDecisions(
  job: SkillImportJob,
  refineResult: SkillRefineResult | null,
  approvedRiskyCandidateIds: string[],
  rejectedRiskyCandidateIds: string[],
  keptSeparateCandidateIds: string[],
): SkillImportDecision[] {
  const decisions: SkillImportDecision[] = job.candidates
    .filter((candidate) => candidate.validation_status === 'pass')
    .map((candidate) => ({ candidate_id: candidate.candidate_id, action: 'import_passed' }));
  decisions.push(
    ...approvedRiskyCandidateIds.map((candidateId) => ({
      candidate_id: candidateId,
      action: 'approve_risky_import' as const,
    })),
  );
  decisions.push(
    ...rejectedRiskyCandidateIds.map((candidateId) => ({
      candidate_id: candidateId,
      action: 'reject_risky_upload' as const,
    })),
  );
  decisions.push(
    ...keptSeparateCandidateIds.map((candidateId) => ({
      candidate_id: candidateId,
      action: 'keep_separate' as const,
    })),
  );
  if (refineResult) {
    decisions.push({
      group_id: firstRefinedGroup(job.conflict_groups, refineResult.source_candidate_ids),
      action: 'merge_group_with_ai',
      merged_name: refineResult.merged_name,
      merged_description: refineResult.merged_description,
      merged_body: refineResult.merged_body,
      merged_tags: refineResult.merged_tags,
    });
  }
  return decisions;
}

function firstRefinedGroup(groups: SkillConflictGroup[], candidateIds: string[], refineSourceGroupID?: string) {
  // Prefer explicit group_id from refine result if available.
  if (refineSourceGroupID) {
    const found = groups.find((g) => g.group_id === refineSourceGroupID);
    if (found) return found.group_id;
  }
  // Fallback: match by candidate IDs.
  return groups.find((group) => group.candidate_ids.some((id) => candidateIds.includes(id)))?.group_id ?? '';
}
