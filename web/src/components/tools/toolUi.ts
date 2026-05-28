import type { QTableColumn } from "quasar";
import type { Tool, ToolInvocation, ToolInvocationAudit, ToolSummary } from "../../features/tools/types";
import {
  REGISTRY_COL_W,
  registryCol,
  registryColActions,
  registryColEnabled
} from "../../features/ui/registryTableColumns";

/** ToolsTable 列定义 */
export const TOOL_TABLE_COLUMNS: QTableColumn<Tool>[] = [
  registryCol<Tool>("name", "Tool", "display_name", "left", REGISTRY_COL_W.name),
  registryCol<Tool>("category", "分类 / 来源", "category", "left", REGISTRY_COL_W.category),
  registryCol<Tool>("runtime", "运行时", "runtime_status", "left", REGISTRY_COL_W.category),
  registryColEnabled<Tool>(),
  registryCol<Tool>("stats", "调用", "invoke_count", "left", REGISTRY_COL_W.status),
  registryCol<Tool>("risk", "风险", "risk_level", "left", REGISTRY_COL_W.status),
  registryColActions<Tool>()
];

/** ToolRunsTable 列定义 */
export const TOOL_RUNS_TABLE_COLUMNS: QTableColumn<ToolInvocation>[] = [
  registryCol<ToolInvocation>("tool", "Tool", "tool_key", "left", REGISTRY_COL_W.nameWide),
  registryCol<ToolInvocation>("agent", "Agent", "agent_id", "left", REGISTRY_COL_W.agent),
  registryCol<ToolInvocation>("status", "状态", "status", "left", REGISTRY_COL_W.status),
  registryCol<ToolInvocation>("session_id", "Session", "session_id", "left", REGISTRY_COL_W.session),
  registryCol<ToolInvocation>("time", "时间 / 耗时", "started_at", "left", REGISTRY_COL_W.time)
];

/** ToolAuditsTable 列定义 */
export const TOOL_AUDITS_TABLE_COLUMNS: QTableColumn<ToolInvocationAudit>[] = [
  registryCol<ToolInvocationAudit>("tool", "Tool / Action", "tool_key", "left", REGISTRY_COL_W.name),
  registryCol<ToolInvocationAudit>("actor", "Agent / User", "agent_id", "left", REGISTRY_COL_W.agent),
  registryCol<ToolInvocationAudit>("status", "状态", "status", "left", REGISTRY_COL_W.status),
  registryCol<ToolInvocationAudit>("time", "时间", "created_at", "left", REGISTRY_COL_W.timeWide)
];

/** 列表筛选：分类（与常见 builtin/schema 对齐） */
export const categoryFilterOptions = [
  "system",
  "web",
  "filesystem",
  "skill",
  "memory",
  "media",
  "runtime",
  "integration",
  "custom"
].map((value) => ({ label: value, value }));

/** 新建 Tool 时的来源提示（非枚举：仍为自由文本） */
export const sourceSuggestions = [
  { value: "builtin", label: "builtin", caption: "平台内置" },
  { value: "mcp", label: "mcp", caption: "MCP 暴露" },
  { value: "system", label: "system", caption: "系统级" },
  { value: "external", label: "external", caption: "外部注册" }
];

export const riskLevelOptions = [
  { label: "低", value: "low" },
  { label: "中", value: "medium" },
  { label: "高", value: "high" },
  { label: "严重", value: "critical" }
];

/** 列表「启用」三态筛选 */
export const enabledTriStateOptions = [
  { label: "已启用", value: true },
  { label: "已禁用", value: false }
];

/** 调用记录状态（与后端 `tool_invocations.status` 对齐） */
export const toolInvocationStatusOptions = [
  { label: "成功", value: "success" },
  { label: "错误", value: "error" },
  { label: "阻断", value: "blocked" },
  { label: "取消", value: "cancelled" }
];

const RISK_LABELS: Record<string, string> = {
  low: "低",
  medium: "中",
  high: "高",
  critical: "严重"
};

/** Quasar 色名；语义对齐 UX §2 success / warning / danger，避免昼间霓虹 */
export function riskQuasarColor(value: string): string {
  const m: Record<string, string> = {
    low: "positive",
    medium: "warning",
    high: "orange",
    critical: "negative"
  };
  return m[value] ?? "grey";
}

export function riskLabel(value: string): string {
  return RISK_LABELS[value] ?? value;
}

export function runtimeStatusLabel(value?: string): string {
  if (!value || value === "available") return "可用";
  if (value === "catalog_only") return "仅目录";
  if (value === "disabled") return "禁用";
  return value;
}

export function runtimeKindHint(tool: Pick<Tool, "supports_streaming">): string {
  return tool.supports_streaming ? "streaming" : "function";
}

const STATUS_LABELS: Record<string, string> = {
  success: "成功",
  error: "错误",
  blocked: "阻断",
  cancelled: "取消",
  failed: "失败"
};

export function toolInvocationStatusLabel(value: string): string {
  return STATUS_LABELS[value] ?? value;
}

export function toolInvocationStatusColor(value: string): string {
  const m: Record<string, string> = {
    success: "positive",
    error: "negative",
    failed: "negative",
    blocked: "warning",
    cancelled: "grey"
  };
  return m[value] ?? "grey";
}

/** 汇总卡片数据（Tools 列表 API summary） */
export function buildToolSummaryCards(summary: ToolSummary) {
  return [
    { label: "总工具", value: String(summary.total_tools), hint: "已注册 Tool" },
    { label: "已启用", value: String(summary.enabled_tools), hint: "全局启用" },
    { label: "高风险启用", value: String(summary.high_risk_enabled), hint: "high / critical" },
    {
      label: "24h 调用",
      value: String(summary.calls_24h),
      hint: `失败率 ${formatFailureRatePercent(summary.failure_rate_24h)}`
    }
  ];
}

export function formatFailureRatePercent(rate: number): string {
  if (!Number.isFinite(rate)) return "—";
  return `${(rate * 100).toFixed(1)}%`;
}

export function prettyJSON(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw || "{}"), null, 2);
  } catch {
    return raw || "{}";
  }
}

/** 粗略 Schema 体量提示（字符长度 → 估算行复杂度） */
export function schemaSizeHint(json: string): string {
  const len = (json || "").trim().length;
  if (len === 0) return "空";
  if (len < 120) return "轻量";
  if (len < 2000) return "中等";
  return "较大";
}

export function formatInvocationDuration(ms?: number): string {
  if (ms == null || Number.isNaN(ms)) return "—";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

export function formatInvocationWhen(iso?: string): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

/** 预览列截断，避免表格撑爆 */
export function clipPreview(text: string | undefined | null, max = 120): string {
  const t = (text ?? "").trim();
  if (t.length <= max) return t;
  return `${t.slice(0, max)}…`;
}

export type ToolMetricCard = { label: string; value: string; hint: string };

/** 从 Tool 行生成一行副标题（供详情/卡片） */
export function toolSubtitleLine(tool: Pick<Tool, "key" | "category" | "source">): string {
  return `${tool.key} · ${tool.category || "custom"} · ${tool.source || "—"}`;
}

/** 编辑器 JSON 字段校验（与页面保存前一致） */
export function validateToolJsonFields(
  fields: Record<string, string>,
  keys: readonly string[]
): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const key of keys) {
    try {
      JSON.parse(fields[key] || "{}");
    } catch (err) {
      errors[key] = err instanceof Error ? err.message : "JSON 格式错误";
    }
  }
  return errors;
}

export const toolEditorJsonKeys = [
  "parameters_schema_json",
  "result_schema_json",
  "config_schema_json",
  "config_json",
  "default_config_json",
  "metadata_json"
] as const;

export type ToolEditorJsonKey = (typeof toolEditorJsonKeys)[number];

/** Map JSON field → editor tab for validation focus. */
export function editorTabForJsonKey(key: string): "schema" | "advanced" {
  if (key === "parameters_schema_json" || key === "result_schema_json" || key === "config_schema_json" || key === "config_json") {
    return "schema";
  }
  return "advanced";
}

/** First invalid JSON key in stable field order. */
export function firstInvalidToolJsonKey(errors: Record<string, string>): ToolEditorJsonKey | null {
  for (const key of toolEditorJsonKeys) {
    if (errors[key]) return key;
  }
  return null;
}

export function invocationAgentLine(row: ToolInvocation): string {
  return row.agent_display_name || row.agent_key || row.agent_id || "—";
}
