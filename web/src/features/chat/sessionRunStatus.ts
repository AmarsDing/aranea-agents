/** WS run_status values aligned with backend event.SessionRunStatus* (M55). */
export const SESSION_RUN_STATUS = {
  SYNC: "sync",
  RUNNING: "running",
  COMPLETED: "completed",
  FAILED: "failed",
  CANCELLED: "cancelled",
  IDLE: "idle",
} as const;

export type SessionRunStatus = (typeof SESSION_RUN_STATUS)[keyof typeof SESSION_RUN_STATUS];
