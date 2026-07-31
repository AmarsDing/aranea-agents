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
};

/** 按状态原文（大小写不敏感）查元数据；未收录返回 null */
export function appStatusMeta(status?: string): AppStatusMeta | null {
  const key = (status ?? '').trim().toLowerCase();
  return key ? (STATUS_META[key] ?? null) : null;
}
