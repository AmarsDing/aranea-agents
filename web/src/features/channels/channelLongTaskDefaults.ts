/**
 * Long-task form field keys, defaults, and select presets (docs/需求/17 channel.md §8.4–8.6).
 */
export const LONG_TASK_FORM_KEYS = [
  'turn_timeout_sec',
  'first_byte_timeout_sec',
  'execution_mode',
  'progress_mode',
  'progress_quiet_sec',
  'ack_message',
  'heartbeat_message',
  'async_graph_id',
  'async_team_id',
  'async_cron_task_id',
  'streaming_enabled',
] as const;

export type LongTaskFormKey = (typeof LONG_TASK_FORM_KEYS)[number];

export const LONG_TASK_NUMERIC_KEYS = new Set<LongTaskFormKey>([
  'turn_timeout_sec',
  'first_byte_timeout_sec',
  'progress_quiet_sec',
]);

/** Recommended defaults for the LONG TASK section (飞书长任务 §8.6). */
export const CHANNEL_LONG_TASK_DEFAULTS: Record<string, string | number | boolean> = {
  streaming_enabled: true,
  ack_message: '收到，正在处理…',
  turn_timeout_sec: 600,
  first_byte_timeout_sec: 120,
  progress_mode: 'text',
  progress_quiet_sec: 20,
  heartbeat_message: '仍在处理中… {{elapsed}}',
  execution_mode: 'sync',
};

export const TURN_TIMEOUT_OPTIONS = [
  { label: 'channelEditor.timeoutOptions.300', value: '300' },
  { label: 'channelEditor.timeoutOptions.600', value: '600' },
  { label: 'channelEditor.timeoutOptions.900', value: '900' },
  { label: 'channelEditor.timeoutOptions.1800', value: '1800' },
];

export const FIRST_BYTE_TIMEOUT_OPTIONS = [
  { label: 'channelEditor.firstByteOptions.30', value: '30' },
  { label: 'channelEditor.firstByteOptions.45', value: '45' },
  { label: 'channelEditor.firstByteOptions.60', value: '60' },
  { label: 'channelEditor.firstByteOptions.120', value: '120' },
];

export const PROGRESS_QUIET_OPTIONS = [
  { label: 'channelEditor.quietOptions.0', value: '0' },
  { label: 'channelEditor.quietOptions.15', value: '15' },
  { label: 'channelEditor.quietOptions.20', value: '20' },
  { label: 'channelEditor.quietOptions.30', value: '30' },
  { label: 'channelEditor.quietOptions.45', value: '45' },
  { label: 'channelEditor.quietOptions.60', value: '60' },
];

export function applyLongTaskFormDefaults(boolDraft: Record<string, boolean>, textDraft: Record<string, string>) {
  for (const [key, value] of Object.entries(CHANNEL_LONG_TASK_DEFAULTS)) {
    if (typeof value === 'boolean') boolDraft[key] = value;
    else textDraft[key] = String(value);
  }
}

export function isLongTaskFormKey(key: string): key is LongTaskFormKey {
  return (LONG_TASK_FORM_KEYS as readonly string[]).includes(key);
}
