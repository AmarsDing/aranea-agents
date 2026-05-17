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
  error_message: string;
  started_at: string;
  finished_at: string;
  created_at: string;
};

export type EvalCaseResult = {
  id: string;
  run_id: string;
  case_id: string;
  actual_output: string;
  exact_match: boolean;
  contains_match: boolean;
  llm_judge_score: number;
  tool_call_accuracy: number;
  error_message: string;
  created_at: string;
};

export type CreateDatasetInput = {
  name: string;
  description?: string;
};

export type RunEvaluationInput = {
  dataset_id: string;
  agent_id: string;
  /** comma-separated list of metrics; empty = all */
  metrics?: string;
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
