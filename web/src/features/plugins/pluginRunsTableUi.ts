import type { QTableColumn } from "quasar";
import type { PluginRun } from "./types";
import { REGISTRY_COL_W, registryCol } from "../ui/registryTableColumns";

/** PluginRunsPage 列定义 */
export const PLUGIN_RUN_TABLE_COLUMNS: QTableColumn<PluginRun>[] = [
  registryCol<PluginRun>("created_at", "时间", "created_at", "left", REGISTRY_COL_W.time),
  registryCol<PluginRun>("plugin_key", "Plugin / Hook", "plugin_key", "left", "12%"),
  registryCol<PluginRun>("agent_id", "Agent", "agent_id", "left", REGISTRY_COL_W.agent),
  registryCol<PluginRun>("callback_point", "生命周期点", "callback_point", "left", REGISTRY_COL_W.status),
  registryCol<PluginRun>("status", "结果", "status", "left", REGISTRY_COL_W.status),
  registryCol<PluginRun>("duration_ms", "耗时(ms)", "duration_ms", "right", REGISTRY_COL_W.metric)
];
