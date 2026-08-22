export type EvalDataset = {
  id: string;
  name: string;
  description: string;
  case_count: number;
  workspace: string;
  created_at: string;
  updated_at: string;
};

export type EvalRun = {
  id: string;
  dataset_id: string;
  agent_id: string;
  /** pending | running | completed | failed */
  status: string;
  total_cases: number;
  completed_cases: number;
  exact_match_score: number;
  contains_match_score: number;
  llm_judge_score: number;
  tool_call_accuracy: number;
  pass_at_k?: number;
  pass_hat_k?: number;
  trigger_source?: string;
  num_runs?: number;
  scores_json?: string;
  error_message: string;
  started_at: string;
  finished_at: string;
  created_at: string;
  dataset_hash?: string;
  dataset_version_id?: string;
  dataset_version?: number;
  experiment_id?: string;
  variant_label?: string;
  model?: string;
  prompt?: string;
  tools?: string;
  judge_calls?: number;
  judge_tokens?: number;
};

export type EvalCaseResult = {
  id: string;
  run_id: string;
  case_id: string;
  /** 用例输入（后端 JOIN eval_cases 读取；用例行已删时为空串） */
  input: string;
  actual_output: string;
  exact_match: boolean;
  contains_match: boolean;
  llm_judge_score: number;
  tool_call_accuracy: number;
  scores_json?: string;
  error_message: string;
  created_at: string;
  human_pass?: boolean | null;
  human_score?: number | null;
  human_comment?: string;
  annotated_at?: string;
  annotated_by?: string;
  session_id?: string;
  trace_run_id?: string;
};

export type AnnotateCaseResultInput = {
  run_id: string;
  result_id: string;
  human_pass?: boolean | null;
  human_score?: number | null;
  human_comment?: string;
  /** 显式清除标注位：proto3 optional + JSON null 无法表达"清除"，须用标志位（优先于值字段） */
  clear_human_pass?: boolean;
  clear_human_score?: boolean;
};

export type EvalCase = {
  id: string;
  dataset_id: string;
  input: string;
  expected_output: string;
  metadata_json: string;
};

export type CreateDatasetInput = {
  name: string;
  description?: string;
};

export type UpdateDatasetInput = {
  id: string;
  name: string;
  description?: string;
};

export type UpdateCaseInput = {
  dataset_id: string;
  id: string;
  input: string;
  expected_output?: string;
  metadata_json?: string;
};

export type RunEvaluationInput = {
  dataset_id: string;
  agent_id: string;
  /** comma-separated list of metrics; empty = all core four; extended: json_match,xml_match,rouge_l,tool_trajectory */
  metrics?: string;
  /** AgentEvaluator MultiRun repeat count (default 1) */
  num_runs?: number;
  use_user_simulation?: boolean;
  model?: string;
  prompt?: string;
  tools?: string;
  dataset_version_id?: string;
};

export type ListDatasetsParams = {
  limit?: number;
  offset?: number;
};

export type ListRunsParams = {
  dataset_id?: string;
  agent_id?: string;
  limit?: number;
  offset?: number;
};

export type ListDatasetsResult = {
  items: EvalDataset[];
  total: number;
};

export type ListRunsResult = {
  items: EvalRun[];
  total: number;
};

export type GetRunResultsResult = {
  items: EvalCaseResult[];
  total: number;
};

export type EvalTrendPoint = {
  run_id: string;
  created_at: string;
  trigger_source: string;
  exact_match_score: number;
  contains_match_score: number;
  llm_judge_score: number;
  tool_call_accuracy: number;
  pass_at_k: number;
  pass_hat_k: number;
};

export type EvalRunComparison = {
  run_id: string;
  agent_id: string;
  dataset_id: string;
  created_at: string;
  exact_match_score: number;
  contains_match_score: number;
  llm_judge_score: number;
  tool_call_accuracy: number;
  pass_at_k: number;
  pass_hat_k: number;
  delta_exact_match: number;
  delta_contains_match: number;
  delta_llm_judge: number;
  delta_tool_call_accuracy: number;
  /** P3-5: dataset content hash at run start; differs across runs → not comparable */
  dataset_hash: string;
  dataset_version_id?: string;
  dataset_version?: number;
  experiment_id?: string;
  variant_label?: string;
  model?: string;
  prompt?: string;
};

export type EvalDatasetVersion = {
  id: string;
  dataset_id: string;
  version: number;
  hash: string;
  case_count: number;
  created_at: string;
};

export type ExperimentVariant = {
  agent_id: string;
  label?: string;
  model?: string;
  prompt?: string;
  tools?: string;
};

export type RunExperimentInput = {
  dataset_id: string;
  metrics?: string;
  num_runs?: number;
  use_user_simulation?: boolean;
  dataset_version_id?: string;
  variants: ExperimentVariant[];
};

/** P1-3 judge 校准：judge 与人工标注分歧统计 */
export type JudgeDivergenceCase = {
  result_id: string;
  run_id: string;
  case_id: string;
  input: string;
  expected_output: string;
  actual_output: string;
  llm_judge_score: number;
  human_pass: boolean;
  human_comment: string;
  /** false_pass（judge 过松）| false_fail（judge 过严） */
  divergence_kind: string;
  created_at: string;
};

export type JudgeDivergence = {
  threshold: number;
  /** 参与统计的样本数（已人工标注且 judge 实际评分） */
  annotated_total: number;
  agree_count: number;
  diverge_count: number;
  agreement_rate: number;
  false_pass_count: number;
  false_fail_count: number;
  divergent_cases: JudgeDivergenceCase[];
};

/** P2-3 失败归组：按 error_message 聚合的失败用例统计 */
export type EvalFailureGroup = {
  error_message: string;
  count: number;
  run_count: number;
  latest_at: string;
};

export type GetFailureGroupsResult = {
  total_failed: number;
  groups: EvalFailureGroup[];
};

/** P3-3 pairwise：人工裁决某次运行优于另一次 */
export type EvalRunPreference = {
  id: string;
  dataset_id: string;
  run_id_a: string;
  run_id_b: string;
  winner_run_id: string;
  comment: string;
  created_by: string;
  created_at: string;
};

export type SubmitRunPreferenceInput = {
  dataset_id: string;
  run_id_a: string;
  run_id_b: string;
  winner_run_id: string;
  comment?: string;
};

/** P2-1 发布门禁配置（单例） */
export type EvalGateConfig = {
  enabled: boolean;
  agent_id: string;
  dataset_id: string;
  metric: string;
  min_score: number;
  max_drop: number;
  mode: string;
  updated_at: string;
};

export type UpdateEvalGateInput = {
  enabled: boolean;
  agent_id: string;
  dataset_id: string;
  metric: string;
  min_score: number;
  max_drop: number;
  mode: string;
};
