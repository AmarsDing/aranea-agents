import type { HookActionType, HookRuleConfig } from "../../features/hooks/types";

export const LOG_LEVEL_OPTIONS = [
  { label: "debug", value: "debug" },
  { label: "info", value: "info" },
  { label: "warn", value: "warn" },
  { label: "error", value: "error" }
] as const;

export function isToolCallbackPoint(point: string) {
  return point === "before_tool" || point === "after_tool";
}

export function isOnEventPoint(point: string) {
  return point === "on_event";
}

const ACTION_TYPE_I18N_KEYS: Record<HookActionType, string> = {
  log: "hooksPage.actionTypes.log",
  notify: "hooksPage.actionTypes.notify",
  block: "hooksPage.actionTypes.block",
  modify: "hooksPage.actionTypes.modify"
};

export function actionTypeLabel(type: HookActionType, t?: (key: string) => string) {
  if (t) return t(ACTION_TYPE_I18N_KEYS[type]);
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

export function parseModifyPatchText(
  raw: string | number | null,
  t?: (key: string) => string
): { patch: Record<string, unknown>; error: string } {
  const text = String(raw ?? "").trim();
  if (!text) {
    return { patch: {}, error: "" };
  }
  try {
    const parsed = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      const msg = t ? t("hooksPage.callbackEditor.modifyPatchInvalidObject") : "modify_patch must be a JSON object";
      return { patch: {}, error: msg };
    }
    return { patch: parsed as Record<string, unknown>, error: "" };
  } catch {
    const msg = t ? t("hooksPage.callbackEditor.modifyPatchInvalidJson") : "Invalid JSON format";
    return { patch: {}, error: msg };
  }
}

export function callbackPointSummary(rule: HookRuleConfig) {
  const parts: string[] = [rule.callback_point, rule.action.type];
  if (rule.condition.agent_id?.trim()) parts.push(`agent:${rule.condition.agent_id.trim()}`);
  if (rule.condition.tool_name?.trim()) parts.push(`tool:${rule.condition.tool_name.trim()}`);
  return parts.join(" · ");
}
