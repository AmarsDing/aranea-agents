import type { HookCallbackPoint } from "../hooks/types";

/** Canonical lifecycle points shared by Hooks UI and Plugin run audit filters. */
export const CALLBACK_POINT_OPTIONS: { label: string; value: HookCallbackPoint }[] = [
  { label: "Before Agent", value: "before_agent" },
  { label: "After Agent", value: "after_agent" },
  { label: "Before Model", value: "before_model" },
  { label: "After Model", value: "after_model" },
  { label: "Before Tool", value: "before_tool" },
  { label: "After Tool", value: "after_tool" },
  { label: "On Event", value: "on_event" }
];

export const PLUGIN_RUN_STATUS_OPTIONS = [
  { label: "成功", value: "success" },
  { label: "阻断", value: "blocked" },
  { label: "错误", value: "error" }
] as const;

/** Common plugin_key values for audit filters (includes hook: prefix rows). */
export const PLUGIN_RUN_KEY_PRESETS = [
  { label: "audit_log", value: "audit_log" },
  { label: "permission_guard", value: "permission_guard" },
  { label: "cost_guard", value: "cost_guard" },
  { label: "Hook 规则 (hook:*)", value: "hook:" }
] as const;

export function pluginRunsQueryFromRoute(query: Record<string, unknown>) {
  const str = (k: string) => {
    const v = query[k];
    return typeof v === "string" ? v : "";
  };
  return {
    plugin_key: str("plugin_key"),
    agent_id: str("agent_id"),
    callback_point: str("callback_point"),
    status: str("status")
  };
}
