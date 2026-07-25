import type { NodeDef, ValidationError, ValidationIssue, ValidationWarning } from './types';

/**
 * 合并本地/服务端校验结果为统一的 ValidationIssue 列表：
 * - level 标记 error/warning
 * - 按 level+code+nodeId+field 去重
 * - 错误排在警告前（稳定排序）
 * - nodeLabel 取 agentName，缺省回退 nodeId
 */
export function buildValidationIssues(
  errors: ValidationError[],
  warnings: ValidationWarning[],
  nodes: NodeDef[],
): ValidationIssue[] {
  const labelById = new Map<string, string>();
  for (const n of nodes) {
    labelById.set(n.id, n.agentName || n.id);
  }

  const seen = new Set<string>();
  const issues: ValidationIssue[] = [];

  const push = (item: ValidationError | ValidationWarning, level: ValidationIssue['level']) => {
    const key = `${level}:${item.code}:${item.nodeId}:${item.field}`;
    if (seen.has(key)) return;
    seen.add(key);
    issues.push({
      nodeId: item.nodeId,
      nodeLabel: item.nodeId ? (labelById.get(item.nodeId) ?? item.nodeId) : '',
      level,
      code: item.code,
      field: item.field,
      message: item.message,
    });
  };

  for (const e of errors) push(e, 'error');
  for (const w of warnings) push(w, 'warning');
  return issues;
}

/** nodeId → 该节点最严重的一条 issue（error 优先于 warning）。无 nodeId 的图级问题不进入映射。 */
export function pickNodeIssueMap(issues: ValidationIssue[]): Record<string, ValidationIssue> {
  const map: Record<string, ValidationIssue> = {};
  for (const issue of issues) {
    if (!issue.nodeId) continue;
    const existing = map[issue.nodeId];
    if (!existing || (existing.level === 'warning' && issue.level === 'error')) {
      map[issue.nodeId] = issue;
    }
  }
  return map;
}

const SUGGESTION_KEYS: Record<string, string> = {
  no_entry_point: 'graphs.suggestionNoEntryPoint',
  duplicate_node: 'graphs.suggestionDuplicateNode',
  edge_source_missing: 'graphs.suggestionEdgeMissingNode',
  edge_target_missing: 'graphs.suggestionEdgeMissingNode',
  unreachable_node: 'graphs.suggestionUnreachable',
  loop_no_exit: 'graphs.suggestionLoopNoExit',
  conditional_loop: 'graphs.suggestionConditionalLoop',
  orphan_node: 'graphs.suggestionOrphanNode',
};

/** 校验错误码 → 修复建议 i18n key；未知码返回空串（不显示建议行）。 */
export function validationSuggestionKey(code: string): string {
  return SUGGESTION_KEYS[code] ?? '';
}
