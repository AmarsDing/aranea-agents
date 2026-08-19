import { i18n } from '../../i18n';

export function formatDate(iso: string): string {
  if (!iso) return '';
  try {
    return new Date(iso).toLocaleDateString();
  } catch {
    return iso;
  }
}

/** 含时分秒：用于观察记录等需要区分同日时序的场景。 */
export function formatDateTime(iso: string): string {
  if (!iso) return '';
  try {
    const d = new Date(iso);
    const date = d.toLocaleDateString();
    const time = d.toLocaleTimeString('zh-CN', { hour12: false });
    return `${date} ${time}`;
  } catch {
    return iso;
  }
}

/** 审批人格式化：后端存 "user:{id}" / "system" / "auto"，展示为可读文案。 */
export function formatApprovedBy(approvedBy: string): string {
  if (!approvedBy) return '';
  const t = i18n.global.t;
  if (approvedBy === 'system' || approvedBy === 'auto') return t('agents.learning_loop.approved_by_system');
  const m = /^user:(\d+)$/.exec(approvedBy);
  if (m) return t('agents.learning_loop.approved_by_user', { id: m[1] });
  return approvedBy;
}
