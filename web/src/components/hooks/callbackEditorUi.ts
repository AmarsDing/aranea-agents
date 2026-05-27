import type { HookActionType, HookCallbackPoint, HookRuleConfig } from "../../features/hooks/types";

export const LOG_LEVEL_OPTIONS = [
  { label: "debug", value: "debug" },
  { label: "info", value: "info" },
  { label: "warn", value: "warn" },
  { label: "error", value: "error" }
] as const;

export const MODIFY_PATCH_HINT =
  "before_model：generation_config / append_system / append_user。" +
  "before_tool：arguments 整包替换；merge_arguments 深度合并（嵌套对象递归，标量/数组以 patch 为准）。";

export function isToolCallbackPoint(point: HookCallbackPoint) {
  return point === "before_tool" || point === "after_tool";
}

export function isOnEventPoint(point: HookCallbackPoint) {
  return point === "on_event";
}

export function actionTypeLabel(type: HookActionType) {
  if (type === "log") return "Log";
  if (type === "notify") return "Notify";
  if (type === "block") return "Block";
  return "Modify";
}

export function actionTagClass(type: HookActionType) {
  if (type === "block") return "hook-tag hook-tag--block";
  if (type === "notify") return "hook-tag hook-tag--notify";
  if (type === "modify") return "hook-tag hook-tag--modify";
  return "hook-tag hook-tag--log";
}

export function stringifyModifyPatch(patch: Record<string, unknown> | undefined) {
  return JSON.stringify(patch ?? {}, null, 2);
}

export function parseModifyPatchText(raw: string | number | null): {
  patch: Record<string, unknown>;
  error: string;
} {
  const text = String(raw ?? "").trim();
  if (!text) {
    return { patch: {}, error: "" };
  }
  try {
    const parsed = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return { patch: {}, error: "modify_patch 须为 JSON 对象" };
    }
    return { patch: parsed as Record<string, unknown>, error: "" };
  } catch {
    return { patch: {}, error: "JSON 格式错误" };
  }
}

export function callbackPointSummary(rule: HookRuleConfig) {
  const parts: string[] = [rule.callback_point, rule.action.type];
  if (rule.condition.agent_id?.trim()) parts.push(`agent:${rule.condition.agent_id.trim()}`);
  if (rule.condition.tool_name?.trim()) parts.push(`tool:${rule.condition.tool_name.trim()}`);
  return parts.join(" · ");
}
