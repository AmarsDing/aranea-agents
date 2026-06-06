import type { Session } from '../../features/session/types';
import { REGISTRY_COL_W, registryCol } from '../../features/ui/registryTableColumns';

/** 列表摘要卡片（纯展示文案来源） */
export type SessionsSummaryCard = {
  label: string;
  value: string | number;
  hint: string;
};

export const ownerFilterOptions = [
  { label: 'Agent', value: 'agent' },
  { label: 'Team', value: 'team' },
];

export const statusFilterOptions = [
  'idle',
  'running',
  'completed',
  'interrupted',
  'awaiting_confirmation',
  'archived',
].map((value) => ({
  label: value,
  value,
}));

export const contextFilterOptions = ['normal', 'warning', 'critical', 'exceeded'].map((value) => ({
  label: value,
  value,
}));

export const pageSizeSelectOptions = [10, 20, 50].map((value) => ({
  label: `${value} / 页`,
  value,
}));

export const sessionsTableSelectionColumn = registryCol('select', '', 'id', 'left', REGISTRY_COL_W.select, {
  sortable: false,
});

export const sessionsTableColumns = [
  registryCol('session', '会话', 'title', 'left', '16%; max-width: 168px', { sortable: false }),
  registryCol('owner', '类型 / 归属', 'owner_type', 'left', '128px', { sortable: false }),
  registryCol('context', '上下文', 'context_used_ratio', 'left', '108px', { sortable: false }),
  registryCol('usage', '消耗', 'total_tokens', 'left', '108px', { sortable: false }),
  registryCol('time', '时间', 'last_message_at', 'left', '128px', { sortable: false }),
  registryCol('status', '状态', 'status', 'left', '80px', { sortable: false }),
  registryCol('actions', '操作', 'id', 'right', '168px', {
    sortable: false,
    classes: 'app-registry-col-actions',
    headerClasses: 'app-registry-col-actions',
  }),
];

export function buildSessionsSummaryCards(rows: Session[], total: number): SessionsSummaryCard[] {
  const active = rows.filter((item) => !item.archived_at && !item.deleted_at).length;
  const pinned = rows.filter((item) => isSessionPinned(item)).length;
  const avgContext = rows.length
    ? rows.reduce((sum, item) => sum + (item.context_used_ratio || 0), 0) / rows.length
    : 0;
  const tokens = rows.reduce((sum, item) => sum + (item.total_tokens || 0), 0);
  return [
    { label: '当前页会话', value: rows.length, hint: `总计 ${total}` },
    { label: '活跃 / 运行', value: active, hint: '当前页统计' },
    { label: '置顶', value: pinned, hint: '当前页已置顶' },
    { label: '平均上下文', value: formatPercent(avgContext), hint: '当前页平均值' },
    { label: 'Token', value: formatNumber(tokens), hint: '当前页累计' },
  ];
}

export function isSessionPinned(session: Session) {
  return Boolean(session.pinned_at?.trim());
}

export function ownerLabel(value: string) {
  return value === 'team' ? 'Team' : 'Agent';
}

/** Quasar chip：日间 primary / team 用 secondary 语义；与全局主题兼容 */
export function ownerChipColor(value: string) {
  return value === 'team' ? 'teal' : 'primary';
}

export function contextProgressColor(value: string) {
  return value === 'exceeded'
    ? 'purple'
    : value === 'critical'
      ? 'negative'
      : value === 'warning'
        ? 'warning'
        : 'positive';
}

export function ratioValue(value: number) {
  return Math.max(0, Math.min(1, value || 0));
}

export function formatPercent(value: number) {
  return `${Math.round(ratioValue(value) * 100)}%`;
}

export function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value || 0);
}

export function formatCostMicroUsd(value: number) {
  return `$${((value || 0) / 1_000_000).toFixed(4)}`;
}

export function formatSessionDate(value: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}
