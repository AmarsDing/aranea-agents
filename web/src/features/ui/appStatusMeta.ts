/**
 * 通用状态标签元数据：AppStatusChip 与各筛选下拉共用，避免状态文案/色调四处硬编码。
 * 文案走 i18n `common.status.*`；未知状态由调用方兜底显示原文。
 */
export type AppStatusTone = 'success' | 'danger' | 'warning' | 'info' | 'neutral';

export type AppStatusMeta = {
  tone: AppStatusTone;
  icon: string;
  /** i18n key（common.status.*） */
  labelKey: string;
};

const STATUS_META: Record<string, AppStatusMeta> = {
  success: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.success' },
  completed: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.completed' },
  error: { tone: 'danger', icon: 'error_outline', labelKey: 'common.status.error' },
  failed: { tone: 'danger', icon: 'cancel', labelKey: 'common.status.failed' },
  timeout: { tone: 'warning', icon: 'schedule', labelKey: 'common.status.timeout' },
  cancelled: { tone: 'neutral', icon: 'block', labelKey: 'common.status.cancelled' },
  running: { tone: 'info', icon: 'play_circle', labelKey: 'common.status.running' },
  pending: { tone: 'neutral', icon: 'hourglass_empty', labelKey: 'common.status.pending' },
  queued: { tone: 'neutral', icon: 'hourglass_empty', labelKey: 'common.status.queued' },
  idle: { tone: 'neutral', icon: 'radio_button_unchecked', labelKey: 'common.status.idle' },
  interrupted: { tone: 'warning', icon: 'pause_circle', labelKey: 'common.status.interrupted' },
  enabled: { tone: 'success', icon: 'toggle_on', labelKey: 'common.status.enabled' },
  disabled: { tone: 'neutral', icon: 'toggle_off', labelKey: 'common.status.disabled' },
  healthy: { tone: 'success', icon: 'favorite', labelKey: 'common.status.healthy' },
  unhealthy: { tone: 'danger', icon: 'heart_broken', labelKey: 'common.status.unhealthy' },
  active: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.active' },
  inactive: { tone: 'neutral', icon: 'radio_button_unchecked', labelKey: 'common.status.inactive' },
  replayed: { tone: 'success', icon: 'replay', labelKey: 'common.status.replayed' },
  abandoned: { tone: 'neutral', icon: 'delete_outline', labelKey: 'common.status.abandoned' },
  ok: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.ok' },
  draft: { tone: 'neutral', icon: 'edit_note', labelKey: 'common.status.draft' },
  published: { tone: 'success', icon: 'public', labelKey: 'common.status.published' },
  archived: { tone: 'neutral', icon: 'archive', labelKey: 'common.status.archived' },
  valid: { tone: 'success', icon: 'verified', labelKey: 'common.status.valid' },
  invalid: { tone: 'danger', icon: 'gpp_bad', labelKey: 'common.status.invalid' },
  awaiting_confirmation: { tone: 'warning', icon: 'approval', labelKey: 'common.status.awaitingConfirmation' },
  passed: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.passed' },
  warning: { tone: 'warning', icon: 'warning_amber', labelKey: 'common.status.warning' },
  normal: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.normal' },
  critical: { tone: 'danger', icon: 'emergency', labelKey: 'common.status.critical' },
  degraded: { tone: 'warning', icon: 'trending_down', labelKey: 'common.status.degraded' },
  online: { tone: 'success', icon: 'wifi', labelKey: 'common.status.online' },
  offline: { tone: 'neutral', icon: 'wifi_off', labelKey: 'common.status.offline' },
  connected: { tone: 'success', icon: 'link', labelKey: 'common.status.connected' },
  disconnected: { tone: 'neutral', icon: 'link_off', labelKey: 'common.status.disconnected' },
  syncing: { tone: 'info', icon: 'sync', labelKey: 'common.status.syncing' },
  synced: { tone: 'success', icon: 'sync_alt', labelKey: 'common.status.synced' },
  partial: { tone: 'warning', icon: 'pie_chart', labelKey: 'common.status.partial' },
  configured: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.configured' },
  unconfigured: { tone: 'warning', icon: 'build_circle', labelKey: 'common.status.unconfigured' },
  stopped: { tone: 'neutral', icon: 'stop_circle', labelKey: 'common.status.stopped' },
  starting: { tone: 'info', icon: 'rocket_launch', labelKey: 'common.status.starting' },
  skipped: { tone: 'neutral', icon: 'skip_next', labelKey: 'common.status.skipped' },
  retrying: { tone: 'info', icon: 'refresh', labelKey: 'common.status.retrying' },
  unknown: { tone: 'neutral', icon: 'help_outline', labelKey: 'common.status.unknown' },
  // 渠道
  pending_auth: { tone: 'warning', icon: 'vpn_key', labelKey: 'common.status.pendingAuth' },
  // 诊断（pass/warn/fail 短词）
  pass: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.pass' },
  warn: { tone: 'warning', icon: 'warning_amber', labelKey: 'common.status.warn' },
  fail: { tone: 'danger', icon: 'cancel', labelKey: 'common.status.fail' },
  failure: { tone: 'danger', icon: 'cancel', labelKey: 'common.status.failed' },
  // 队列/任务
  accepted: { tone: 'info', icon: 'check', labelKey: 'common.status.accepted' },
  async_queued: { tone: 'neutral', icon: 'hourglass_empty', labelKey: 'common.status.asyncQueued' },
  task_pending: { tone: 'neutral', icon: 'hourglass_empty', labelKey: 'common.status.taskPending' },
  task_claimed: { tone: 'info', icon: 'assignment_ind', labelKey: 'common.status.taskClaimed' },
  task_complete: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.taskComplete' },
  task_blocked: { tone: 'danger', icon: 'block', labelKey: 'common.status.taskBlocked' },
  task_review_required: { tone: 'warning', icon: 'rate_review', labelKey: 'common.status.taskReviewRequired' },
  task_failed: { tone: 'danger', icon: 'cancel', labelKey: 'common.status.taskFailed' },
  task_timed_out: { tone: 'warning', icon: 'schedule', labelKey: 'common.status.taskTimedOut' },
  task_cancelled: { tone: 'neutral', icon: 'block', labelKey: 'common.status.taskCancelled' },
  task_crashed: { tone: 'danger', icon: 'report', labelKey: 'common.status.taskCrashed' },
  task_pending_assignment: { tone: 'neutral', icon: 'hourglass_empty', labelKey: 'common.status.taskPendingAssignment' },
  // 运行/计划
  loading: { tone: 'info', icon: 'sync', labelKey: 'common.status.loading' },
  waiting: { tone: 'neutral', icon: 'hourglass_empty', labelKey: 'common.status.waiting' },
  waiting_human: { tone: 'warning', icon: 'person', labelKey: 'common.status.waitingHuman' },
  planning: { tone: 'info', icon: 'edit_note', labelKey: 'common.status.planning' },
  executing: { tone: 'info', icon: 'play_circle', labelKey: 'common.status.executing' },
  partial_failure: { tone: 'warning', icon: 'warning_amber', labelKey: 'common.status.partialFailure' },
  paused: { tone: 'warning', icon: 'pause_circle', labelKey: 'common.status.paused' },
  awaiting_user: { tone: 'warning', icon: 'person', labelKey: 'common.status.awaitingUser' },
  sync: { tone: 'info', icon: 'sync', labelKey: 'common.status.sync' },
  durable: { tone: 'success', icon: 'save', labelKey: 'common.status.durable' },
  // 学习/知识提议
  validated: { tone: 'success', icon: 'verified', labelKey: 'common.status.validated' },
  approved: { tone: 'success', icon: 'approval', labelKey: 'common.status.approved' },
  applied: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.applied' },
  rejected: { tone: 'danger', icon: 'block', labelKey: 'common.status.rejected' },
  conflict: { tone: 'warning', icon: 'compare_arrows', labelKey: 'common.status.conflict' },
  expired: { tone: 'neutral', icon: 'event_busy', labelKey: 'common.status.expired' },
  detected: { tone: 'info', icon: 'search', labelKey: 'common.status.detected' },
  confirmed: { tone: 'success', icon: 'check_circle', labelKey: 'common.status.confirmed' },
  dismissed: { tone: 'neutral', icon: 'block', labelKey: 'common.status.dismissed' },
  // 美式拼写别名
  canceled: { tone: 'neutral', icon: 'block', labelKey: 'common.status.cancelled' },
};

/** 按状态原文（大小写不敏感）查元数据；未收录返回 null */
export function appStatusMeta(status?: string): AppStatusMeta | null {
  const key = (status ?? '').trim().toLowerCase();
  return key ? (STATUS_META[key] ?? null) : null;
}
