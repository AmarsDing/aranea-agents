import type { QTableColumn } from "quasar";
import { parseHookConfig, type HookRow, type HookRuleConfig } from "../../features/hooks/types";
import { REGISTRY_COL_W, registryCol, registryColActions } from "../../features/ui/registryTableColumns";

export function hookRuleOf(row: HookRow): HookRuleConfig {
  return parseHookConfig(row.config_json);
}

/** Hooks 管理页表格列（修改列宽改此处） */
export const HOOKS_TABLE_COLUMNS: QTableColumn<HookRow>[] = [
  registryCol<HookRow>("name", "名称", "name", "left", REGISTRY_COL_W.nameWide),
  registryCol<HookRow>("rule", "规则", "config_json", "left", REGISTRY_COL_W.desc),
  registryCol<HookRow>("sort", "排序", "sort_order", "center", REGISTRY_COL_W.narrow),
  registryCol<HookRow>("enabled", "启用", "enabled", "center", REGISTRY_COL_W.enabled, { sortable: false }),
  registryColActions<HookRow>(REGISTRY_COL_W.actionsWide)
];

/** Agent 设置内嵌 Hook 列表 */
export const HOOKS_AGENT_TABLE_COLUMNS: QTableColumn<HookRow>[] = [
  registryCol<HookRow>("name", "名称", "name", "left", REGISTRY_COL_W.nameWide),
  registryCol<HookRow>("rule", "规则", "config_json", "left", REGISTRY_COL_W.desc),
  registryColActions<HookRow>()
];

/** HookDeliveriesPage 列定义 */
export const HOOK_DELIVERY_TABLE_COLUMNS = [
  registryCol("created_at", "时间", "created_at", "left", REGISTRY_COL_W.time),
  registryCol("hook_key", "Hook", "hook_key", "left", REGISTRY_COL_W.desc),
  registryCol("status", "状态", "status", "left", REGISTRY_COL_W.status),
  registryCol("attempt_count", "尝试", "attempt_count", "right", REGISTRY_COL_W.metric),
  registryCol("max_attempts", "上限", "max_attempts", "right", REGISTRY_COL_W.metric)
];

export function hookRunsTo(row: HookRow) {
  const rule = hookRuleOf(row);
  return {
    path: "/plugins/runs",
    query: {
      plugin_key: `hook:${row.key}`,
      callback_point: rule.callback_point
    }
  };
}

export function hookConditionHint(row: HookRow) {
  const rule = hookRuleOf(row);
  const parts: string[] = [];
  if (rule.condition.agent_id?.trim()) parts.push(`Agent: ${rule.condition.agent_id.trim()}`);
  if (rule.condition.tool_name?.trim()) parts.push(`Tool: ${rule.condition.tool_name.trim()}`);
  if (rule.condition.event_type?.trim()) parts.push(`Event: ${rule.condition.event_type.trim()}`);
  return parts.length ? parts.join("\n") : "";
}
