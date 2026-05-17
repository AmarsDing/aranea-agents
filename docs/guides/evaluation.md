# Evaluation Guide

> Sprint 6 — T39 | M-11

The Evaluation module provides a structured way to benchmark agent quality using datasets of input/expected-output pairs. Runs are executed asynchronously with four built-in metrics, and results are stored for inspection and comparison.

---

## Architecture

```
EvaluationService (Kratos HTTP/gRPC)
       │
EvalUsecase (biz)
       │
   EvalRepo (data) ─── SQLite / PostgreSQL tables
       │
evaluation.Runner (async goroutine)
   ├── exact_match
   ├── contains_match
   ├── llm_as_judge   (optional LLMJudge hook)
   └── tool_call_accuracy
```

---

## Components

| Component | Path | Purpose |
|-----------|------|---------|
| Proto | `api/kratos/evaluation/v1/evaluation.proto` | HTTP + gRPC API |
| Biz | `internal/biz/evaluation.go` | Domain types + `EvalRepo` interface |
| Data | `internal/data/evaluation.go` | Raw SQL persistence (SQLite/Postgres) |
| Runner | `internal/evaluation/runner.go` | Async execution + metric computation |
| Service | `internal/service/evaluation.go` | Kratos service adapter |

---

## Database Schema

Tables are created by `data.EnsureEvalSchema(ctx, db)`:

```sql
eval_datasets      (id, name, description, case_count, workspace, ...)
eval_cases         (id, dataset_id, input, expected_output, metadata_json)
eval_runs          (id, dataset_id, agent_id, status, scores..., ...)
eval_case_results  (id, run_id, case_id, actual_output, exact_match, ...)
```

---

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/evaluation/datasets` | Create dataset |
| GET | `/v1/evaluation/datasets` | List datasets |
| GET | `/v1/evaluation/datasets/{id}` | Get dataset |
| DELETE | `/v1/evaluation/datasets/{id}` | Delete dataset |
| POST | `/v1/evaluation/datasets/{dataset_id}/cases` | Upload cases (JSON array) |
| POST | `/v1/evaluation/runs` | Start async eval run |
| GET | `/v1/evaluation/runs` | List runs |
| GET | `/v1/evaluation/runs/{id}` | Get run + scores |
| GET | `/v1/evaluation/runs/{run_id}/results` | Per-case results |

---

## Evaluation Metrics

| Metric | Key | Description |
|--------|-----|-------------|
| Exact match | `exact_match` | Case-insensitive string equality |
| Contains match | `contains_match` | Expected string appears in output |
| LLM-as-judge | `llm_as_judge` | Score [0,1] from a judge model |
| Tool call accuracy | `tool_call_accuracy` | Fraction of expected tools mentioned in output |

Use the `metrics` field on `RunEvaluationRequest` to select a subset:

```json
{ "dataset_id": "...", "agent_id": "...", "metrics": "exact_match,contains_match" }
```

An empty `metrics` runs all four.

---

## Uploading Cases

`cases_json` must be a JSON array:

```json
[
  { "input": "What is 2+2?", "expected_output": "4" },
  {
    "input": "Call the weather tool",
    "expected_output": "The weather is sunny",
    "metadata_json": "{\"expected_tools\":[\"get_weather\"]}"
  }
]
```

---

## Async Runner

`RunEvaluation` creates an `EvalRun` record with `status=pending` and immediately dispatches an async goroutine.

Run lifecycle: `pending` → `running` → `completed` | `failed`

Progress is observable by polling `GET /v1/evaluation/runs/{id}` (`completed_cases` increments with each case).

---

## LLM-as-Judge Wiring

To enable `llm_as_judge`, provide an `evaluation.LLMJudge` function when constructing the runner:

```go
runner := evaluation.NewRunner(evalUsecase, agentRunner, func(ctx context.Context, input, expected, actual string) (float32, error) {
    // call your judge model
    return 0.85, nil
})
```

A `nil` judge silently skips the metric.

---

## Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `aranea_eval_runs_total{status}` | Counter | Runs by status (started / completed / error) |
| `aranea_eval_case_duration_seconds` | Histogram | Per-case execution time |

---

## Acceptance Criteria

- 100-case dataset completes in < 5 minutes.
- Report contains all 4 metric scores.
- Individual case results available via `GetRunResults`.
