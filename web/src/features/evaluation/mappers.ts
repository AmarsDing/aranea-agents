import { asRecord, pickI32, pickNum, pickStr } from "../../shared/wireJson";

export type EvaluationDatasetRow = {
  id: string;
  name: string;
  case_count: number;
  created_at: string;
};

export type EvaluationRunRow = {
  id: string;
  dataset_id: string;
  agent_id: string;
  status: string;
  total_cases: number;
  completed_cases: number;
  exact_match_score: number;
};

export function mapDataset(raw: unknown): EvaluationDatasetRow {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    name: pickStr(r, "name", "name"),
    case_count: pickI32(r, "case_count", "caseCount"),
    created_at: pickStr(r, "created_at", "createdAt")
  };
}

export function mapRun(raw: unknown): EvaluationRunRow {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    dataset_id: pickStr(r, "dataset_id", "datasetId"),
    agent_id: pickStr(r, "agent_id", "agentId"),
    status: pickStr(r, "status", "status"),
    total_cases: pickI32(r, "total_cases", "totalCases"),
    completed_cases: pickI32(r, "completed_cases", "completedCases"),
    exact_match_score: pickNum(r, "exact_match_score", "exactMatchScore")
  };
}
