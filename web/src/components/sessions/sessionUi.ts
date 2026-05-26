import type { Session } from "../../features/session/types";
import { registryColWidth } from "../../features/ui/registryTableColumns";

/** 列表摘要卡片（纯展示文案来源） */
export type SessionsSummaryCard = {
  label: string;
  value: string | number;
  hint: string;
};

export const ownerFilterOptions = [
  { label: "Agent", value: "agent" },
  { label: "Team", value: "team" }
];

export const statusFilterOptions = ["active", "running", "completed", "failed", "archived"].map((value) => ({
  label: value,
  value
}));

export const contextFilterOptions = ["normal", "warning", "critical", "exceeded"].map((value) => ({
  label: value,
  value
}));

export const pageSizeSelectOptions = [10, 20, 50].map((value) => ({
  label: `${value} / 页`,
  value
}));

export const sessionsTableSelectionColumn = {
  name: "select",
  label: "",
  field: "id",
  align: "left" as const,
  sortable: false,
  ...registryColWidth("48px")
};

export const sessionsTableColumns = [
  { name: "session", label: "会话", field: "title", align: "left" as const, sortable: false, ...registryColWidth("16%; max-width: 168px") },
  { name: "owner", label: "类型 / 归属", field: "owner_type", align: "left" as const, sortable: false, ...registryColWidth("128px") },
  { name: "context", label: "上下文", field: "context_used_ratio", align: "left" as const, sortable: false, ...registryColWidth("108px") },
  { name: "usage", label: "消耗", field: "total_tokens", align: "left" as const, sortable: false, ...registryColWidth("108px") },
  { name: "time", label: "时间", field: "last_message_at", align: "left" as const, sortable: false, ...registryColWidth("128px") },
  { name: "status", label: "状态", field: "status", align: "left" as const, sortable: false, ...registryColWidth("80px") },
  {
    name: "actions",
    label: "操作",
    field: "id",
    align: "right" as const,
    sortable: false,
    ...registryColWidth("168px"),
    classes: "app-registry-col-actions",
    headerClasses: "app-registry-col-actions"
  }
];

export function buildSessionsSummaryCards(rows: Session[], total: number): SessionsSummaryCard[] {
  const active = rows.filter((item) => item.status === "active" || item.status === "running").length;
  const pinned = rows.filter((item) => isSessionPinned(item)).length;
  const avgContext = rows.length
    ? rows.reduce((sum, item) => sum + (item.context_used_ratio || 0), 0) / rows.length
    : 0;
  const tokens = rows.reduce((sum, item) => sum + (item.total_tokens || 0), 0);
  return [
    { label: "当前页会话", value: rows.length, hint: `总计 ${total}` },
    { label: "活跃 / 运行", value: active, hint: "当前页统计" },
    { label: "置顶", value: pinned, hint: "当前页已置顶" },
    { label: "平均上下文", value: formatPercent(avgContext), hint: "当前页平均值" },
    { label: "Token", value: formatNumber(tokens), hint: "当前页累计" }
  ];
}

export function isSessionPinned(session: Session) {
  return Boolean(session.pinned_at?.trim());
}

export function ownerLabel(value: string) {
  return value === "team" ? "Team" : "Agent";
}

/** Quasar chip：日间 primary / team 用 secondary 语义；与全局主题兼容 */
export function ownerChipColor(value: string) {
  return value === "team" ? "deep-purple" : "primary";
}

export function statusBadgeColor(value: string) {
  return value === "failed" ? "negative" : value === "archived" ? "grey" : value === "running" ? "primary" : "positive";
}

export function contextProgressColor(value: string) {
  return value === "exceeded" ? "purple" : value === "critical" ? "negative" : value === "warning" ? "warning" : "positive";
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
  if (!value) return "-";
  return new Date(value).toLocaleString();
}
