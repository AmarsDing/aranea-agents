// office 套件页内模型（E 阶段 W1c；字段 snake_case 与页内既有行模型同约，
// wire 映射集中在 api.ts）。

export type OfficeEventSource = 'manual' | 'task_schedule' | 'agent_draft' | '';

export type OfficeEventRow = {
  id: string;
  task_ref: string;
  task_title: string;
  title: string;
  note: string;
  /** RFC3339（ proto Timestamp 线格式）。 */
  start_at: string;
  end_at: string;
  timezone: string;
  reminder_minutes: number;
  source: OfficeEventSource;
};

export type OfficeNotificationRow = {
  id: string;
  kind: string;
  title: string;
  body: string;
  link: string;
  read: boolean;
  created_at: string;
};

export type ICalTokenRow = {
  id: string;
  name: string;
  /** 仅创建当次响应携带（相对路径，前端拼 origin）；列表恒空。 */
  feed_url: string;
  created_at: string;
  revoked_at: string;
};

export type OfficeAppRow = {
  id: string;
  name: string;
  url: string;
  icon: string;
  group_name: string;
  sort_order: number;
};
