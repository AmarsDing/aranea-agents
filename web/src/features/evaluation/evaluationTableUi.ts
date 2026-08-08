import type { QTableColumn } from 'quasar';
import type { EvalCaseResult, EvalRun, EvalRunComparison, EvalTrendPoint, JudgeDivergenceCase } from './types';
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
  registryCol<EvalCaseResult>('human_comment', '评语', 'human_comment', 'left', REGISTRY_COL_W.name),
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

/** 最小翻译器签名：兼容 vue-i18n Composer 的 t()（仅简单 key 场景）。 */
export type EvalTranslator = (key: string) => string;

/** EvaluationAnalyticsPanel — judge 分歧用例（工厂函数，label 走 i18n；human_pass / divergence_kind 由面板 slot 渲染 chip） */
export function buildEvalDivergenceColumns(t: EvalTranslator): QTableColumn<JudgeDivergenceCase>[] {
  return [
    registryCol<JudgeDivergenceCase>('input', t('evaluationPage.divergenceColInput'), 'input', 'left', REGISTRY_COL_W.content),
    registryCol<JudgeDivergenceCase>(
      'llm_judge_score',
      t('evaluationPage.divergenceColLlmScore'),
      (r) => r.llm_judge_score.toFixed(2),
      'right',
      REGISTRY_COL_W.narrow,
    ),
    registryCol<JudgeDivergenceCase>(
      'human_pass',
      t('evaluationPage.divergenceColHuman'),
      'human_pass',
      'center',
      REGISTRY_COL_W.narrow,
    ),
    registryCol<JudgeDivergenceCase>(
      'divergence_kind',
      t('evaluationPage.divergenceColKind'),
      'divergence_kind',
      'center',
      REGISTRY_COL_W.status,
    ),
    registryCol<JudgeDivergenceCase>(
      'human_comment',
      t('evaluationPage.divergenceColComment'),
      'human_comment',
      'left',
      REGISTRY_COL_W.name,
    ),
    registryCol<JudgeDivergenceCase>(
      'created_at',
      t('evaluationPage.divergenceColTime'),
      'created_at',
      'left',
      REGISTRY_COL_W.time,
    ),
  ];
}
