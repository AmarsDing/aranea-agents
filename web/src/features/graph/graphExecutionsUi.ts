export const STATUS_FILTER_OPTIONS = [
  { label: "全部", value: "" },
  { label: "已完成", value: "completed" },
  { label: "运行中", value: "running" },
  { label: "失败", value: "failed" },
  { label: "已中断", value: "interrupted" },
] as const;

export const TIME_RANGE_OPTIONS = [
  { label: "全部时间", value: "" },
  { label: "今天", value: "today" },
  { label: "最近7天", value: "7d" },
  { label: "最近30天", value: "30d" },
] as const;

export function statusColor(status: string): string {
  switch (status) {
    case "completed": return "positive";
    case "running": return "info";
    case "failed": return "negative";
    case "interrupted": return "warning";
    default: return "grey";
  }
}

export function statusLabel(status: string): string {
  switch (status) {
    case "completed": return "已完成";
    case "running": return "运行中";
    case "failed": return "失败";
    case "interrupted": return "已中断";
    default: return status;
  }
}

export function timeRangeStart(range: string): Date | null {
  const now = new Date();
  switch (range) {
    case "today":
      return new Date(now.getFullYear(), now.getMonth(), now.getDate());
    case "7d":
      return new Date(now.getTime() - 7 * 86400000);
    case "30d":
      return new Date(now.getTime() - 30 * 86400000);
    default:
      return null;
  }
}
