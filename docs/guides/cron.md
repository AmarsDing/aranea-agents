# Cron Job Retry Policy & Metrics

> Sprint 5 · T35 · Added 2026-05-17

Aranea-Agents cron jobs support **automatic retry**, **Prometheus metrics**, and a **dead-letter** mechanism to prevent runaway failures.

---

## Retry Policy

Each job that returns an error is automatically retried with an **exponential backoff** schedule before being counted as a failure.

| Attempt | Delay |
|---------|-------|
| 1st retry | 30 s |
| 2nd retry | 2 m |
| 3rd retry | 10 m |

After all retry attempts are exhausted the job is marked `failed` for that run.

**Panic recovery** is applied at each attempt via `pkg/safego`. A panicking job handler is treated as a hard failure and proceeds through the retry schedule.

### Configuration

The retry schedule is defined in `internal/cronrunner/runner.go`:

```go
var defaultRetryBackoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}
const maxDeadFailures = 3
```

---

## Dead-Letter State

When a job accumulates **3 consecutive failures** across separate scheduled runs it transitions to the `dead` state:

- `cron_tasks.status` is set to `"dead"`
- `cron_tasks.enabled` is set to `false`
- An admin alert event `cron.dead_letter` is emitted on the internal event bus with metadata:
  ```json
  { "job_id": "…", "task_key": "…", "name": "…" }
  ```

A dead job is **never re-scheduled** until manually reset (change `enabled = true`, `status = "active"`, `failure_count = 0`).

---

## Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `aranea_cron_job_runs_total` | Counter | `job_id`, `status` | Total executions by outcome (`success`/`failure`) |
| `aranea_cron_job_duration_seconds` | Histogram | `job_id` | Wall-clock time per execution |
| `aranea_cron_job_dead_total` | Counter | `job_id` | Times a job entered the dead state |

Buckets for `duration_seconds`: `0.5s, 1s, 5s, 15s, 30s, 60s, 120s, 300s, 600s`

---

## Failure Fields Persisted to DB

| DB Column | Updated on |
|-----------|-----------|
| `cron_tasks.failure_count` | Each failed run |
| `cron_tasks.last_error` | Latest error message |
| `cron_task_runs.status` | `"success"` or `"failure"` per run |
| `cron_task_runs.error_message` | Error text |
| `cron_task_runs.finished_at` | Completion timestamp |

---

## Frontend (Admin UI)

The Cron admin page shows:

- **Retry count / failure count** from `metadata_json.failure_count`
- **Last error** from `metadata_json.last_error`
- **Recent failures list** from `metadata_json.recent_failures`
- A **"Reset Failure Count"** button that clears `failure_count`, `last_error`, and sets `status = active`

---

## Rollback

To disable retry for a job temporarily set `RetryPolicy.MaxAttempts = 0` in the job's `config_json`:

```json
{ "retry_max_attempts": 0 }
```

A global kill-switch is not yet implemented; disable the feature by removing `dispatchWithRetry` from `executeTask` in `internal/cronrunner/runner.go`.
