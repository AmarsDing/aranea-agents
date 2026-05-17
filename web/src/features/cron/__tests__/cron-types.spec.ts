import { describe, it, expect } from "vitest";
import type { CronTaskMetadata, CronTaskConfig } from "../types";

// Pure utility function tests — no API calls needed.

describe("CronTaskMetadata shape", () => {
  it("defaults to empty values when not set", () => {
    const meta: CronTaskMetadata = {};
    expect(meta.run_count).toBeUndefined();
    expect(meta.failure_count).toBeUndefined();
    expect(meta.last_error).toBeUndefined();
  });

  it("accepts partial failure tracking fields", () => {
    const meta: CronTaskMetadata = {
      run_count: 10,
      failure_count: 3,
      last_error: "timeout after 30s",
      recent_failures: [
        { started_at: "2026-01-01T00:00:00Z", error_message: "timeout" }
      ]
    };
    expect(meta.failure_count).toBe(3);
    expect(meta.recent_failures).toHaveLength(1);
  });
});

describe("CronTaskConfig shape", () => {
  it("accepts interval schedule config", () => {
    const cfg: CronTaskConfig = {
      schedule_type: "interval",
      interval_seconds: 3600,
      message: "ping"
    };
    expect(cfg.schedule_type).toBe("interval");
    expect(cfg.interval_seconds).toBe(3600);
  });

  it("accepts cron expression schedule config", () => {
    const cfg: CronTaskConfig = {
      schedule_type: "cron",
      cron_expression: "0 9 * * 1-5",
      timezone: "Asia/Shanghai"
    };
    expect(cfg.cron_expression).toBe("0 9 * * 1-5");
  });

  it("accepts once/run_at schedule config", () => {
    const cfg: CronTaskConfig = {
      schedule_type: "once",
      run_at: "2026-06-01T09:00:00Z"
    };
    expect(cfg.run_at).toBe("2026-06-01T09:00:00Z");
  });
});

describe("CronTaskMetadata failure reset logic", () => {
  it("failure_count increments on failure and resets on success", () => {
    let meta: CronTaskMetadata = { failure_count: 0, run_count: 0 };

    // Simulate failure
    meta = { ...meta, run_count: (meta.run_count ?? 0) + 1, failure_count: (meta.failure_count ?? 0) + 1, last_error: "error" };
    expect(meta.failure_count).toBe(1);

    // Simulate success → reset
    meta = { ...meta, run_count: (meta.run_count ?? 0) + 1, failure_count: 0, last_error: undefined };
    expect(meta.failure_count).toBe(0);
    expect(meta.last_error).toBeUndefined();
  });

  it("identifies dead state after 3 consecutive failures", () => {
    const MAX_DEAD_FAILURES = 3;
    const meta: CronTaskMetadata = { failure_count: 3 };
    const isDead = (meta.failure_count ?? 0) >= MAX_DEAD_FAILURES;
    expect(isDead).toBe(true);
  });
});
