import type { PlatformResource } from "../platform/types";

export type CronScheduleType = "interval" | "cron" | "once";
export type CronTaskStatus = "active" | "paused" | "dead" | string;
export type CronRunStatus = "pending" | "success" | "failure" | "skipped" | string;

export type CronTaskConfig = {
  target_type?: "agent" | "team";
  team_id?: string;
  schedule_type?: CronScheduleType;
  cron_expression?: string;
  interval_seconds?: number;
  run_at?: string;
  timezone?: string;
  message?: string;
  retry_max_attempts?: number; // retry count after first attempt; default 3; 0 = disable
};

export type CronFailureSummary = {
  started_at?: string;
  error_message?: string;
};

export type CronTaskMetadata = {
  run_count?: number;
  success_count?: number;
  failure_count?: number;
  last_run_at?: string;
  last_run_status?: CronRunStatus;
  last_error?: string;
  next_run_at?: string;
  recent_failures?: CronFailureSummary[];
  [key: string]: unknown;
};

export type CronTaskFormValue = {
  name: string;
  display_name: string;
  description: string;
  target_type: "agent" | "team";
  agent_id: string;
  team_id: string;
  schedule_type: CronScheduleType;
  interval_minutes: number;
  cron_expression: string;
  run_at_date: string;
  run_at_time: string;
  timezone: string;
  message: string;
  retry_max_attempts: number;
  enabled: boolean;
};

export type CronTaskRow = PlatformResource;

export type CronTaskRun = {
  id: string;
  task_id: string;
  task_name: string;
  status: CronRunStatus;
  started_at: string;
  finished_at: string;
  trigger: string;
  run_id: string;
  output_json: string;
  error_message: string;
  created_at: string;
};

export type CronTaskRunQuery = {
  cron_task_id?: string;
  status?: string;
  limit?: number;
};
