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
};

/** 按状态原文（大小写不敏感）查元数据；未收录返回 null */
export function appStatusMeta(status?: string): AppStatusMeta | null {
  const key = (status ?? '').trim().toLowerCase();
  return key ? (STATUS_META[key] ?? null) : null;
}
