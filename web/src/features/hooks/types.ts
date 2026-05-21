export type HookCallbackPoint =
  | "before_agent"
  | "after_agent"
  | "before_model"
  | "after_model"
  | "before_tool"
  | "after_tool"
  | "on_event";

export type HookActionType = "log" | "notify" | "block" | "modify";

export type HookRuleConfig = {
  callback_point: HookCallbackPoint;
  condition: {
    agent_id?: string;
    tool_name?: string;
    event_type?: string;
  };
  action: {
    type: HookActionType;
    webhook_url?: string;
    modify_patch?: Record<string, unknown>;
    log_level?: string;
    message?: string;
    notify_max_retries?: number;
    notify_timeout_sec?: number;
  };
};

export type HookRow = {
  id: string;
  key: string;
  name: string;
  description: string;
  status: string;
  enabled: boolean;
  sort_order: number;
  config_json: string;
  metadata_json: string;
  created_at: string;
  updated_at: string;
};

export { CALLBACK_POINT_OPTIONS } from "../callback/constants";

export const ACTION_TYPE_OPTIONS: { label: string; value: HookActionType }[] = [
  { label: "Log", value: "log" },
  { label: "Notify (Webhook)", value: "notify" },
  { label: "Block", value: "block" },
  { label: "Modify", value: "modify" }
];

export function defaultHookRuleConfig(agentId = "", agentKey = ""): HookRuleConfig {
  return {
    callback_point: "before_tool",
    condition: {
      agent_id: agentId || agentKey || ""
    },
    action: {
      type: "log",
      log_level: "info"
    }
  };
}

export function parseHookConfig(raw: string): HookRuleConfig {
  if (!raw?.trim()) {
    return defaultHookRuleConfig();
  }
  try {
    return JSON.parse(raw) as HookRuleConfig;
  } catch {
    return defaultHookRuleConfig();
  }
}

export function serializeHookConfig(cfg: HookRuleConfig): string {
  return JSON.stringify(cfg, null, 2);
}

/** Plain-object clone; safe for reactive Proxy values from v-model. */
export function cloneHookRuleConfig(cfg: HookRuleConfig): HookRuleConfig {
  return parseHookConfig(serializeHookConfig(cfg));
}
