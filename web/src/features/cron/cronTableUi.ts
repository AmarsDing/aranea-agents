import type { QTableColumn } from "quasar";
import type { CronTaskRow, CronTaskRun } from "./types";
import {
  REGISTRY_COL_W,
  registryCol,
  registryColActions,
  registryColEnabled
} from "../ui/registryTableColumns";

/** CronTasksPage 列定义 */
export const CRON_TASK_TABLE_COLUMNS: QTableColumn<CronTaskRow>[] = [
  registryCol<CronTaskRow>("name", "任务", "name", "left", REGISTRY_COL_W.name, { sortable: true }),
  registryCol<CronTaskRow>("schedule", "调度", "config_json", "left", REGISTRY_COL_W.category),
  registryCol<CronTaskRow>("agent", "目标", "agent_id", "left", REGISTRY_COL_W.agent),
  registryCol<CronTaskRow>("counts", "统计", "metadata_json", "left", REGISTRY_COL_W.status),
  registryCol<CronTaskRow>("status", "状态", "status", "left", REGISTRY_COL_W.status),
  registryCol<CronTaskRow>("timing", "执行时间", "metadata_json", "left", REGISTRY_COL_W.time),
  registryColEnabled<CronTaskRow>(),
  registryColActions<CronTaskRow>()
];

/** CronRunsPage 列定义 */
export const CRON_RUN_TABLE_COLUMNS: QTableColumn<CronTaskRun>[] = [
  registryCol<CronTaskRun>("task", "任务名称", "task_name", "left", REGISTRY_COL_W.name),
  registryCol<CronTaskRun>("time", "时间", "started_at", "left", REGISTRY_COL_W.time),
  registryCol<CronTaskRun>("status", "结果", "status", "left", REGISTRY_COL_W.status),
  registryCol<CronTaskRun>("trigger", "触发", "trigger", "left", REGISTRY_COL_W.metric),
  registryCol<CronTaskRun>("run", "Agent 运行", "run_id", "right", REGISTRY_COL_W.actions)
];
