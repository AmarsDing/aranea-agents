import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { HookCallbackPoint } from "../hooks/types";

export const CALLBACK_POINT_VALUES: HookCallbackPoint[] = [
  "before_agent",
  "after_agent",
  "before_model",
  "after_model",
  "before_tool",
  "after_tool",
  "on_event"
];

export const CALLBACK_POINT_OPTIONS: { label: string; value: HookCallbackPoint }[] = [
  { label: "Before Agent", value: "before_agent" },
  { label: "After Agent", value: "after_agent" },
  { label: "Before Model", value: "before_model" },
  { label: "After Model", value: "after_model" },
  { label: "Before Tool", value: "before_tool" },
  { label: "After Tool", value: "after_tool" },
  { label: "On Event", value: "on_event" }
];

const CALLBACK_POINT_I18N_KEYS: Record<HookCallbackPoint, string> = {
  before_agent: "hooksPage.callbackPoints.beforeAgent",
  after_agent: "hooksPage.callbackPoints.afterAgent",
  before_model: "hooksPage.callbackPoints.beforeModel",
  after_model: "hooksPage.callbackPoints.afterModel",
  before_tool: "hooksPage.callbackPoints.beforeTool",
  after_tool: "hooksPage.callbackPoints.afterTool",
  on_event: "hooksPage.callbackPoints.onEvent"
};

export function useCallbackPointOptions() {
  const { t } = useI18n();
  return computed(() =>
    CALLBACK_POINT_VALUES.map((v) => ({
      label: t(CALLBACK_POINT_I18N_KEYS[v]),
      value: v
    }))
  );
}

export const PLUGIN_RUN_STATUS_OPTIONS = [
  { label: "成功", value: "success" },
  { label: "阻断", value: "blocked" },
  { label: "错误", value: "error" }
] as const;

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
    status: str("status"),
    from: str("from"),
    to: str("to")
  };
}
