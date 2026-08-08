/**
 * Evaluation 评估：**`createEvaluationService()`** → **`/v1/evaluation/...`**。
 *
 * Runtime：FrameworkBridge + ChatService.RunNativeTurnUnary；LLMJudge 需 Provider 目录或 env。
 */
import { requestHandler } from '../../services/axiosHandler';
import { createEvaluationService } from '../../services';
import { asRecord, pickBool, pickI32, pickNum, pickStr } from '../../shared/wireJson';
import type {
  AnnotateCaseResultInput,
  CreateDatasetInput,
  EvalCaseResult,
  EvalDataset,
  EvalFailureGroup,
  EvalGateConfig,
  EvalRun,
  EvalRunPreference,
  GetFailureGroupsResult,
  GetRunResultsResult,
  JudgeDivergence,
  JudgeDivergenceCase,
  ListDatasetsParams,
  ListDatasetsResult,
  ListRunsParams,
  ListRunsResult,
  RunEvaluationInput,
  SubmitRunPreferenceInput,
  UpdateEvalGateInput,
  EvalTrendPoint,
  EvalRunComparison,
} from './types';

const svc = createEvaluationService();

function mapDataset(raw: unknown): EvalDataset {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    name: pickStr(r, 'name', 'name'),
    description: pickStr(r, 'description', 'description'),
    case_count: pickI32(r, 'case_count', 'caseCount'),
    workspace: pickStr(r, 'workspace', 'workspace'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
  };
}

function mapRun(raw: unknown): EvalRun {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    dataset_id: pickStr(r, 'dataset_id', 'datasetId'),
    agent_id: pickStr(r, 'agent_id', 'agentId'),
    status: pickStr(r, 'status', 'status'),
    total_cases: pickI32(r, 'total_cases', 'totalCases'),
    completed_cases: pickI32(r, 'completed_cases', 'completedCases'),
    exact_match_score: pickNum(r, 'exact_match_score', 'exactMatchScore'),
    contains_match_score: pickNum(r, 'contains_match_score', 'containsMatchScore'),
    llm_judge_score: pickNum(r, 'llm_judge_score', 'llmJudgeScore'),
    tool_call_accuracy: pickNum(r, 'tool_call_accuracy', 'toolCallAccuracy'),
    pass_at_k: pickNum(r, 'pass_at_k', 'passAtK'),
    pass_hat_k: pickNum(r, 'pass_hat_k', 'passHatK'),
    trigger_source: pickStr(r, 'trigger_source', 'triggerSource'),
    num_runs: pickI32(r, 'num_runs', 'numRuns'),
    scores_json: pickStr(r, 'scores_json', 'scoresJson'),
    error_message: pickStr(r, 'error_message', 'errorMessage'),
    started_at: pickStr(r, 'started_at', 'startedAt'),
    finished_at: pickStr(r, 'finished_at', 'finishedAt'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
  };
}

function mapCaseResult(raw: unknown): EvalCaseResult {
  const r = asRecord(raw);
  const out: EvalCaseResult = {
    id: pickStr(r, 'id', 'id'),
    run_id: pickStr(r, 'run_id', 'runId'),
    case_id: pickStr(r, 'case_id', 'caseId'),
    actual_output: pickStr(r, 'actual_output', 'actualOutput'),
    exact_match: pickBool(r, 'exact_match', 'exactMatch'),
    contains_match: pickBool(r, 'contains_match', 'containsMatch'),
    llm_judge_score: pickNum(r, 'llm_judge_score', 'llmJudgeScore'),
    tool_call_accuracy: pickNum(r, 'tool_call_accuracy', 'toolCallAccuracy'),
    scores_json: pickStr(r, 'scores_json', 'scoresJson'),
    error_message: pickStr(r, 'error_message', 'errorMessage'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
    human_comment: pickStr(r, 'human_comment', 'humanComment'),
    annotated_at: pickStr(r, 'annotated_at', 'annotatedAt'),
    annotated_by: pickStr(r, 'annotated_by', 'annotatedBy'),
  };
  if ('humanPass' in r || 'human_pass' in r) {
    out.human_pass = pickBool(r, 'human_pass', 'humanPass');
  }
  if ('humanScore' in r || 'human_score' in r) {
    out.human_score = pickNum(r, 'human_score', 'humanScore');
  }
  return out;
}

export async function annotateCaseResult(input: AnnotateCaseResultInput): Promise<EvalCaseResult> {
  const body: Record<string, unknown> = {};
  if (input.human_pass !== undefined) body.human_pass = input.human_pass ?? null;
  if (input.human_score !== undefined) body.human_score = input.human_score ?? null;
  if (input.human_comment !== undefined) body.human_comment = input.human_comment;
  const raw = await requestHandler({
    path: `v1/evaluation/runs/${encodeURIComponent(input.run_id)}/results/${encodeURIComponent(input.result_id)}/annotation`,
    method: 'PATCH',
    body: JSON.stringify(body),
  });
  return mapCaseResult(raw);
}

// ---------- Datasets ----------

export async function listDatasets(params: ListDatasetsParams = {}): Promise<ListDatasetsResult> {
  const res = asRecord(await svc.ListDatasets({ limit: params.limit ?? 0, offset: params.offset ?? 0 }));
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapDataset) : [];
  return { items, total: pickI32(res, 'total', 'total') || items.length };
}

export async function getDataset(id: string): Promise<EvalDataset> {
  const raw = await svc.GetDataset({ id });
  return mapDataset(raw);
}

export async function createDataset(input: CreateDatasetInput): Promise<EvalDataset> {
  const raw = await svc.CreateDataset({ name: input.name, description: input.description ?? '' });
  return mapDataset(raw);
}

export async function deleteDataset(id: string): Promise<void> {
  await svc.DeleteDataset({ id });
}

/** cases_json 为 JSON 数组，元素格式：`{input, expected_output, metadata_json}` */
export async function uploadCases(datasetId: string, casesJson: string): Promise<number> {
  const res = asRecord(await svc.UploadCases({ datasetId, casesJson }));
  return pickI32(res, 'imported', 'imported');
}

// ---------- Runs ----------

export async function runEvaluation(input: RunEvaluationInput): Promise<EvalRun> {
  const raw = await svc.RunEvaluation({
    datasetId: input.dataset_id,
    agentId: input.agent_id,
    metrics: input.metrics ?? '',
    numRuns: input.num_runs ?? 1,
    useUserSimulation: input.use_user_simulation ?? false,
  });
  return mapRun(raw);
}

export async function listRuns(params: ListRunsParams = {}): Promise<ListRunsResult> {
  const res = asRecord(
    await svc.ListRuns({
      datasetId: params.dataset_id ?? '',
      agentId: params.agent_id ?? '',
      limit: params.limit ?? 0,
      offset: params.offset ?? 0,
    }),
  );
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapRun) : [];
  return { items, total: pickI32(res, 'total', 'total') || items.length };
}

export async function getRun(id: string): Promise<EvalRun> {
  const raw = await svc.GetRun({ id });
  return mapRun(raw);
}

export async function getRunResults(
  runId: string,
  params: { limit?: number; offset?: number } = {},
): Promise<GetRunResultsResult> {
  const res = asRecord(await svc.GetRunResults({ runId, limit: params.limit ?? 0, offset: params.offset ?? 0 }));
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapCaseResult) : [];
  return { items, total: pickI32(res, 'total', 'total') || items.length };
}

function mapTrendPoint(raw: unknown): EvalTrendPoint {
  const r = asRecord(raw);
  return {
    run_id: pickStr(r, 'run_id', 'runId'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
    trigger_source: pickStr(r, 'trigger_source', 'triggerSource'),
    exact_match_score: pickNum(r, 'exact_match_score', 'exactMatchScore'),
    contains_match_score: pickNum(r, 'contains_match_score', 'containsMatchScore'),
    llm_judge_score: pickNum(r, 'llm_judge_score', 'llmJudgeScore'),
    tool_call_accuracy: pickNum(r, 'tool_call_accuracy', 'toolCallAccuracy'),
    pass_at_k: pickNum(r, 'pass_at_k', 'passAtK'),
    pass_hat_k: pickNum(r, 'pass_hat_k', 'passHatK'),
  };
}

function mapRunComparison(raw: unknown): EvalRunComparison {
  const r = asRecord(raw);
  return {
    run_id: pickStr(r, 'run_id', 'runId'),
    agent_id: pickStr(r, 'agent_id', 'agentId'),
    dataset_id: pickStr(r, 'dataset_id', 'datasetId'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
    exact_match_score: pickNum(r, 'exact_match_score', 'exactMatchScore'),
    contains_match_score: pickNum(r, 'contains_match_score', 'containsMatchScore'),
    llm_judge_score: pickNum(r, 'llm_judge_score', 'llmJudgeScore'),
    tool_call_accuracy: pickNum(r, 'tool_call_accuracy', 'toolCallAccuracy'),
    pass_at_k: pickNum(r, 'pass_at_k', 'passAtK'),
    pass_hat_k: pickNum(r, 'pass_hat_k', 'passHatK'),
    delta_exact_match: pickNum(r, 'delta_exact_match', 'deltaExactMatch'),
    delta_contains_match: pickNum(r, 'delta_contains_match', 'deltaContainsMatch'),
    delta_llm_judge: pickNum(r, 'delta_llm_judge', 'deltaLlmJudge'),
    delta_tool_call_accuracy: pickNum(r, 'delta_tool_call_accuracy', 'deltaToolCallAccuracy'),
    dataset_hash: pickStr(r, 'dataset_hash', 'datasetHash'),
  };
}

export async function getAgentEvalTrend(params: {
  agent_id: string;
  dataset_id?: string;
  limit?: number;
}): Promise<EvalTrendPoint[]> {
  const res = asRecord(
    await svc.GetAgentEvalTrend({
      agentId: params.agent_id,
      datasetId: params.dataset_id ?? '',
      limit: params.limit ?? 20,
    }),
  );
  const pointsRaw = res.points ?? res.Points;
  return Array.isArray(pointsRaw) ? pointsRaw.map(mapTrendPoint) : [];
}

export async function compareEvalRuns(runIds: string[]): Promise<EvalRunComparison[]> {
  const res = asRecord(await svc.CompareEvalRuns({ runIds }));
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapRunComparison) : [];
}

// ---------- P1-3 Judge 校准 ----------

function mapDivergenceCase(raw: unknown): JudgeDivergenceCase {
  const r = asRecord(raw);
  return {
    result_id: pickStr(r, 'result_id', 'resultId'),
    run_id: pickStr(r, 'run_id', 'runId'),
    case_id: pickStr(r, 'case_id', 'caseId'),
    input: pickStr(r, 'input', 'input'),
    expected_output: pickStr(r, 'expected_output', 'expectedOutput'),
    actual_output: pickStr(r, 'actual_output', 'actualOutput'),
    llm_judge_score: pickNum(r, 'llm_judge_score', 'llmJudgeScore'),
    human_pass: pickBool(r, 'human_pass', 'humanPass'),
    human_comment: pickStr(r, 'human_comment', 'humanComment'),
    divergence_kind: pickStr(r, 'divergence_kind', 'divergenceKind'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
  };
}

export async function getJudgeDivergence(
  datasetId: string,
  params: { agent_id?: string; threshold?: number; limit?: number } = {},
): Promise<JudgeDivergence> {
  const res = asRecord(
    await svc.GetJudgeDivergence({
      datasetId,
      agentId: params.agent_id ?? '',
      threshold: params.threshold ?? 0,
      limit: params.limit ?? 0,
    }),
  );
  const casesRaw = res.divergent_cases ?? res.divergentCases;
  return {
    threshold: pickNum(res, 'threshold', 'threshold'),
    annotated_total: pickI32(res, 'annotated_total', 'annotatedTotal'),
    agree_count: pickI32(res, 'agree_count', 'agreeCount'),
    diverge_count: pickI32(res, 'diverge_count', 'divergeCount'),
    agreement_rate: pickNum(res, 'agreement_rate', 'agreementRate'),
    false_pass_count: pickI32(res, 'false_pass_count', 'falsePassCount'),
    false_fail_count: pickI32(res, 'false_fail_count', 'falseFailCount'),
    divergent_cases: Array.isArray(casesRaw) ? casesRaw.map(mapDivergenceCase) : [],
  };
}

// ---------- P2-3 失败归组 ----------

function mapFailureGroup(raw: unknown): EvalFailureGroup {
  const r = asRecord(raw);
  return {
    error_message: pickStr(r, 'error_message', 'errorMessage'),
    count: pickI32(r, 'count', 'count'),
    run_count: pickI32(r, 'run_count', 'runCount'),
    latest_at: pickStr(r, 'latest_at', 'latestAt'),
  };
}

export async function getFailureGroups(
  datasetId: string,
  params: { agent_id?: string; limit?: number } = {},
): Promise<GetFailureGroupsResult> {
  const res = asRecord(
    await svc.GetFailureGroups({
      datasetId,
      agentId: params.agent_id ?? '',
      limit: params.limit ?? 0,
    }),
  );
  const groupsRaw = res.groups ?? res.Groups;
  return {
    total_failed: pickI32(res, 'total_failed', 'totalFailed'),
    groups: Array.isArray(groupsRaw) ? groupsRaw.map(mapFailureGroup) : [],
  };
}

// ---------- P3-3 Pairwise 偏好 ----------

function mapRunPreference(raw: unknown): EvalRunPreference {
  const r = asRecord(raw);
  return {
    id: pickStr(r, 'id', 'id'),
    dataset_id: pickStr(r, 'dataset_id', 'datasetId'),
    run_id_a: pickStr(r, 'run_id_a', 'runIdA'),
    run_id_b: pickStr(r, 'run_id_b', 'runIdB'),
    winner_run_id: pickStr(r, 'winner_run_id', 'winnerRunId'),
    comment: pickStr(r, 'comment', 'comment'),
    created_by: pickStr(r, 'created_by', 'createdBy'),
    created_at: pickStr(r, 'created_at', 'createdAt'),
  };
}

export async function submitRunPreference(input: SubmitRunPreferenceInput): Promise<EvalRunPreference> {
  const raw = await svc.SubmitRunPreference({
    datasetId: input.dataset_id,
    runIdA: input.run_id_a,
    runIdB: input.run_id_b,
    winnerRunId: input.winner_run_id,
    comment: input.comment ?? '',
  });
  return mapRunPreference(raw);
}

export async function listRunPreferences(datasetId: string, limit = 50): Promise<EvalRunPreference[]> {
  const res = asRecord(await svc.ListRunPreferences({ datasetId, limit }));
  const itemsRaw = res.items ?? res.Items;
  return Array.isArray(itemsRaw) ? itemsRaw.map(mapRunPreference) : [];
}

// ---------- P2-1 发布门禁配置 ----------

function mapGateConfig(raw: unknown): EvalGateConfig {
  const r = asRecord(raw);
  return {
    enabled: pickBool(r, 'enabled', 'enabled'),
    agent_id: pickStr(r, 'agent_id', 'agentId'),
    dataset_id: pickStr(r, 'dataset_id', 'datasetId'),
    metric: pickStr(r, 'metric', 'metric'),
    min_score: pickNum(r, 'min_score', 'minScore'),
    max_drop: pickNum(r, 'max_drop', 'maxDrop'),
    updated_at: pickStr(r, 'updated_at', 'updatedAt'),
  };
}

export async function getEvalGate(): Promise<EvalGateConfig> {
  return mapGateConfig(await svc.GetEvalGate({}));
}

export async function updateEvalGate(input: UpdateEvalGateInput): Promise<EvalGateConfig> {
  const raw = await svc.UpdateEvalGate({
    enabled: input.enabled,
    agentId: input.agent_id,
    datasetId: input.dataset_id,
    metric: input.metric,
    minScore: input.min_score,
    maxDrop: input.max_drop,
  });
  return mapGateConfig(raw);
}
