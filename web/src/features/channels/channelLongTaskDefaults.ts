/**
 * Long-task form field keys, defaults, and select presets (docs/需求/17 channel.md §8.4–8.6).
 */
export const LONG_TASK_FORM_KEYS = [
  "turn_timeout_sec",
  "first_byte_timeout_sec",
  "execution_mode",
  "progress_mode",
  "progress_quiet_sec",
  "ack_message",
  "heartbeat_message",
  "async_graph_id",
  "async_team_id",
  "async_cron_task_id",
  "streaming_enabled"
] as const;

export type LongTaskFormKey = (typeof LONG_TASK_FORM_KEYS)[number];

export const LONG_TASK_NUMERIC_KEYS = new Set<LongTaskFormKey>([
  "turn_timeout_sec",
  "first_byte_timeout_sec",
  "progress_quiet_sec"
]);

/** Recommended defaults for the LONG TASK section (飞书长任务 §8.6). */
export const CHANNEL_LONG_TASK_DEFAULTS: Record<string, string | number | boolean> = {
  streaming_enabled: true,
  ack_message: "收到，正在处理…",
  turn_timeout_sec: 600,
  first_byte_timeout_sec: 120,
  progress_mode: "text",
  progress_quiet_sec: 20,
  heartbeat_message: "仍在处理中… {{elapsed}}",
  execution_mode: "sync"
};

export const TURN_TIMEOUT_OPTIONS = [
  { label: "300 秒（5 分钟）", value: "300" },
  { label: "600 秒（10 分钟，推荐）", value: "600" },
  { label: "900 秒（15 分钟）", value: "900" },
  { label: "1800 秒（30 分钟）", value: "1800" }
];

export const FIRST_BYTE_TIMEOUT_OPTIONS = [
  { label: "30 秒（默认）", value: "30" },
  { label: "45 秒", value: "45" },
  { label: "60 秒", value: "60" },
  { label: "120 秒（重工具推荐）", value: "120" }
];

export const PROGRESS_QUIET_OPTIONS = [
  { label: "0（关闭心跳）", value: "0" },
  { label: "15 秒", value: "15" },
  { label: "20 秒（默认）", value: "20" },
  { label: "30 秒", value: "30" },
  { label: "45 秒", value: "45" },
  { label: "60 秒", value: "60" }
];

export function applyLongTaskFormDefaults(
  boolDraft: Record<string, boolean>,
  textDraft: Record<string, string>
) {
  for (const [key, value] of Object.entries(CHANNEL_LONG_TASK_DEFAULTS)) {
    if (typeof value === "boolean") boolDraft[key] = value;
    else textDraft[key] = String(value);
  }
}

export function isLongTaskFormKey(key: string): key is LongTaskFormKey {
  return (LONG_TASK_FORM_KEYS as readonly string[]).includes(key);
}
