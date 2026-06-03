/** WS run_status values aligned with backend event.SessionRunStatus* (M55). */
export const SESSION_RUN_STATUS = {
  SYNC: 'sync',
  RUNNING: 'running',
  COMPLETED: 'completed',
  FAILED: 'failed',
  CANCELLED: 'cancelled',
  IDLE: 'idle',
  INTERACTIVE: 'interactive',
  ESCALATING: 'escalating',
  DURABLE: 'durable',
} as const;

export type SessionRunStatus = (typeof SESSION_RUN_STATUS)[keyof typeof SESSION_RUN_STATUS];

export const ACTIVE_RUN_STATUSES: ReadonlySet<string> = new Set([
  SESSION_RUN_STATUS.RUNNING,
  SESSION_RUN_STATUS.INTERACTIVE,
  SESSION_RUN_STATUS.ESCALATING,
  SESSION_RUN_STATUS.DURABLE,
  'accepted',
  'async_queued',
  'queued',
]);
