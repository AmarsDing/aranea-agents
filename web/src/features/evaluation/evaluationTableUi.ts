import type { QTableColumn } from 'quasar';
import type { EvalCaseResult, EvalRun, EvalRunComparison, EvalTrendPoint } from './types';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../ui/registryTableColumns';

/** EvaluationPage — Run 列表 */
export const EVAL_RUN_TABLE_COLUMNS: QTableColumn<EvalRun>[] = [
  registryCol<EvalRun>('id', 'Run ID', 'id', 'left', REGISTRY_COL_W.agent),
  registryCol<EvalRun>('agent_id', 'Agent', 'agent_id', 'left', REGISTRY_COL_W.agent),
  registryCol<EvalRun>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol<EvalRun>(
    'completed_cases',
    '进度',
    (r) => `${r.completed_cases}/${r.total_cases}`,
    'right',
    REGISTRY_COL_W.metric,
  ),
  registryCol<EvalRun>('exact_match_score', 'Exact', 'exact_match_score', 'right', REGISTRY_COL_W.narrow),
  registryColActions<EvalRun>(REGISTRY_COL_W.actions, ''),
];

/** EvaluationPage — Result 列表 */
export const EVAL_RESULT_TABLE_COLUMNS: QTableColumn<EvalCaseResult>[] = [
  registryCol<EvalCaseResult>('case_id', 'Case', 'case_id', 'left', REGISTRY_COL_W.agent),
  registryCol<EvalCaseResult>('exact_match', 'Exact', 'exact_match', 'center', REGISTRY_COL_W.narrow),
  registryCol<EvalCaseResult>('contains_match', 'Contains', 'contains_match', 'center', REGISTRY_COL_W.narrow),
  registryCol<EvalCaseResult>('human_pass', '人工', 'human_pass', 'center', REGISTRY_COL_W.narrow),
  registryCol<EvalCaseResult>('human_score', '分数', 'human_score', 'center', REGISTRY_COL_W.narrow),
  registryCol<EvalCaseResult>('human_comment', '评语', 'human_comment', 'left', '12%'),
  registryCol<EvalCaseResult>('annotate', '', 'id', 'right', REGISTRY_COL_W.actions),
];

/** EvaluationAnalyticsPanel — 趋势点 */
export const EVAL_TREND_TABLE_COLUMNS: QTableColumn<EvalTrendPoint>[] = [
  registryCol<EvalTrendPoint>('created_at', '时间', 'created_at', 'left', REGISTRY_COL_W.time),
  registryCol<EvalTrendPoint>('trigger_source', '触发', 'trigger_source', 'left', REGISTRY_COL_W.metric),
  registryCol<EvalTrendPoint>('exact_match_score', 'Exact', 'exact_match_score', 'right', REGISTRY_COL_W.narrow),
  registryCol<EvalTrendPoint>(
    'contains_match_score',
    'Contains',
    'contains_match_score',
    'right',
    REGISTRY_COL_W.narrow,
  ),
  registryCol<EvalTrendPoint>('llm_judge_score', 'LLM', 'llm_judge_score', 'right', REGISTRY_COL_W.narrow),
  registryCol<EvalTrendPoint>('pass_at_k', 'pass@k', 'pass_at_k', 'right', REGISTRY_COL_W.narrow),
];

/** EvaluationAnalyticsPanel — 最近 Run */
export const EVAL_RECENT_RUN_TABLE_COLUMNS: QTableColumn<EvalRun>[] = [
  registryCol<EvalRun>('id', 'Run', 'id', 'left', REGISTRY_COL_W.agent),
  registryCol<EvalRun>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol<EvalRun>('exact_match_score', 'Exact', 'exact_match_score', 'right', REGISTRY_COL_W.narrow),
  registryCol<EvalRun>('created_at', '时间', 'created_at', 'left', REGISTRY_COL_W.time),
];

/** EvaluationAnalyticsPanel — Run 对比 */
export const EVAL_COMPARE_TABLE_COLUMNS: QTableColumn<EvalRunComparison>[] = [
  registryCol<EvalRunComparison>('run_id', 'Run', 'run_id', 'left', REGISTRY_COL_W.agent),
  registryCol<EvalRunComparison>('exact_match_score', 'Exact', 'exact_match_score', 'right', REGISTRY_COL_W.narrow),
  registryCol<EvalRunComparison>('delta_exact_match', 'Δ Exact', 'delta_exact_match', 'right', REGISTRY_COL_W.narrow),
  registryCol<EvalRunComparison>('delta_llm_judge', 'Δ LLM', 'delta_llm_judge', 'right', REGISTRY_COL_W.narrow),
  registryCol<EvalRunComparison>(
    'delta_tool_call_accuracy',
    'Δ Tool',
    'delta_tool_call_accuracy',
    'right',
    REGISTRY_COL_W.narrow,
  ),
];
