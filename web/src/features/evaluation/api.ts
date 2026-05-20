/**
 * Evaluation 评估：**`createEvaluationService()`** → **`/v1/evaluation/...`**。
 *
 * 注意：后端 Evaluation Runtime 尚未完整接入（EP-DATA-01 / EP-RT-08）。
 * 启动评估运行（RunEvaluation）在 runtime nil 的情况下会返回错误。
 */
import { requestHandler } from "../../services/axiosHandler";
import { createEvaluationService } from "../../services";
import { asRecord, pickBool, pickI32, pickNum, pickStr } from "../../shared/wireJson";
import type {
  AnnotateCaseResultInput,
  CreateDatasetInput,
  EvalCaseResult,
  EvalDataset,
  EvalRun,
  GetRunResultsResult,
  ListDatasetsParams,
  ListDatasetsResult,
  ListRunsParams,
  ListRunsResult,
  RunEvaluationInput
} from "./types";

const svc = createEvaluationService();

function mapDataset(raw: unknown): EvalDataset {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    name: pickStr(r, "name", "name"),
    description: pickStr(r, "description", "description"),
    case_count: pickI32(r, "case_count", "caseCount"),
    workspace: pickStr(r, "workspace", "workspace"),
    created_at: pickStr(r, "created_at", "createdAt"),
    updated_at: pickStr(r, "updated_at", "updatedAt")
  };
}

function mapRun(raw: unknown): EvalRun {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    dataset_id: pickStr(r, "dataset_id", "datasetId"),
    agent_id: pickStr(r, "agent_id", "agentId"),
    status: pickStr(r, "status", "status"),
    total_cases: pickI32(r, "total_cases", "totalCases"),
    completed_cases: pickI32(r, "completed_cases", "completedCases"),
    exact_match_score: pickNum(r, "exact_match_score", "exactMatchScore"),
    contains_match_score: pickNum(r, "contains_match_score", "containsMatchScore"),
    llm_judge_score: pickNum(r, "llm_judge_score", "llmJudgeScore"),
    tool_call_accuracy: pickNum(r, "tool_call_accuracy", "toolCallAccuracy"),
    error_message: pickStr(r, "error_message", "errorMessage"),
    started_at: pickStr(r, "started_at", "startedAt"),
    finished_at: pickStr(r, "finished_at", "finishedAt"),
    created_at: pickStr(r, "created_at", "createdAt")
  };
}

function mapCaseResult(raw: unknown): EvalCaseResult {
  const r = asRecord(raw);
  const out: EvalCaseResult = {
    id: pickStr(r, "id", "id"),
    run_id: pickStr(r, "run_id", "runId"),
    case_id: pickStr(r, "case_id", "caseId"),
    actual_output: pickStr(r, "actual_output", "actualOutput"),
    exact_match: pickBool(r, "exact_match", "exactMatch"),
    contains_match: pickBool(r, "contains_match", "containsMatch"),
    llm_judge_score: pickNum(r, "llm_judge_score", "llmJudgeScore"),
    tool_call_accuracy: pickNum(r, "tool_call_accuracy", "toolCallAccuracy"),
    error_message: pickStr(r, "error_message", "errorMessage"),
    created_at: pickStr(r, "created_at", "createdAt"),
    human_comment: pickStr(r, "human_comment", "humanComment"),
    annotated_at: pickStr(r, "annotated_at", "annotatedAt"),
    annotated_by: pickStr(r, "annotated_by", "annotatedBy")
  };
  if ("humanPass" in r || "human_pass" in r) {
    out.human_pass = pickBool(r, "human_pass", "humanPass");
  }
  if ("humanScore" in r || "human_score" in r) {
    out.human_score = pickNum(r, "human_score", "humanScore");
  }
  return out;
}

export async function annotateCaseResult(input: AnnotateCaseResultInput): Promise<EvalCaseResult> {
  const body: Record<string, unknown> = {};
  if (input.human_pass !== undefined && input.human_pass !== null) body.human_pass = input.human_pass;
  if (input.human_score !== undefined && input.human_score !== null) body.human_score = input.human_score;
  if (input.human_comment !== undefined) body.human_comment = input.human_comment;
  const raw = await requestHandler({
    path: `v1/evaluation/runs/${encodeURIComponent(input.run_id)}/results/${encodeURIComponent(input.result_id)}/annotation`,
    method: "PATCH",
    body
  });
  return mapCaseResult(raw);
}

// ---------- Datasets ----------

export async function listDatasets(params: ListDatasetsParams = {}): Promise<ListDatasetsResult> {
  const res = asRecord(await svc.ListDatasets({ limit: params.limit ?? 0, offset: params.offset ?? 0 }));
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapDataset) : [];
  return { items, total: pickI32(res, "total", "total") || items.length };
}

export async function getDataset(id: string): Promise<EvalDataset> {
  const raw = await svc.GetDataset({ id });
  return mapDataset(raw);
}

export async function createDataset(input: CreateDatasetInput): Promise<EvalDataset> {
  const raw = await svc.CreateDataset({ name: input.name, description: input.description ?? "" });
  return mapDataset(raw);
}

export async function deleteDataset(id: string): Promise<void> {
  await svc.DeleteDataset({ id });
}

/** cases_json 为 JSON 数组，元素格式：`{input, expected_output, metadata_json}` */
export async function uploadCases(datasetId: string, casesJson: string): Promise<number> {
  const res = asRecord(await svc.UploadCases({ datasetId, casesJson }));
  return pickI32(res, "imported", "imported");
}

// ---------- Runs ----------

export async function runEvaluation(input: RunEvaluationInput): Promise<EvalRun> {
  const raw = await svc.RunEvaluation({
    datasetId: input.dataset_id,
    agentId: input.agent_id,
    metrics: input.metrics ?? "",
    numRuns: input.num_runs ?? 1
  });
  return mapRun(raw);
}

export async function listRuns(params: ListRunsParams = {}): Promise<ListRunsResult> {
  const res = asRecord(
    await svc.ListRuns({
      datasetId: params.dataset_id ?? "",
      agentId: params.agent_id ?? "",
      limit: params.limit ?? 0,
      offset: params.offset ?? 0
    })
  );
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapRun) : [];
  return { items, total: pickI32(res, "total", "total") || items.length };
}

export async function getRun(id: string): Promise<EvalRun> {
  const raw = await svc.GetRun({ id });
  return mapRun(raw);
}

export async function getRunResults(
  runId: string,
  params: { limit?: number; offset?: number } = {}
): Promise<GetRunResultsResult> {
  const res = asRecord(
    await svc.GetRunResults({ runId, limit: params.limit ?? 0, offset: params.offset ?? 0 })
  );
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapCaseResult) : [];
  return { items, total: pickI32(res, "total", "total") || items.length };
}
