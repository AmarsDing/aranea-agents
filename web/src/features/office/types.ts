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

// ---------- W1d 邮件聚合 ----------

export type MailTlsMode = 'TLS_MODE_TLS' | 'TLS_MODE_STARTTLS' | 'TLS_MODE_NONE';

export type MailAccountRow = {
  id: string;
  name: string;
  host: string;
  port: number;
  tls_mode: MailTlsMode;
  username: string;
  last_uid: number;
  /** RFC3339；空串 = 从未同步。 */
  last_sync_at: string;
};

export type MailMessageRow = {
  id: string;
  account_id: string;
  subject: string;
  sender: string;
  snippet: string;
  /** RFC3339；空串 = 无 Date 头（按 created_at 兜底排序）。 */
  received_at: string;
  task_ref: string;
  has_attachments: boolean;
};

export type MailAttachmentRow = {
  id: string;
  message_id: string;
  filename: string;
  content_type: string;
  size: number;
  knowledge_doc_id: string;
};

/** 个人任务（负责人是当前用户；邮件转任务与手工新建同表）。 */
export type OfficeTaskRow = {
  id: string;
  title: string;
  body: string;
  source: 'manual' | 'mail' | string;
  source_ref: string;
  status: 'open' | 'done' | string;
  created_at: string;
  updated_at: string;
};
