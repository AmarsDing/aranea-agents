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
  /** 日程 P1：plan 计划（打算做的）| log 工作内容（已经做了的） */
  kind: OfficeEventKind;
  /** todo 未开始 | doing 进行中 | done 已完成 | cancelled 已取消；逾期不存，按结束时间算 */
  status: OfficeEventStatus;
  all_day: boolean;
  /** 完成情况 / 结果 */
  outcome: string;
  location: string;
  url: string;
  /** 分类色：'' 默认 | work | meeting | personal | focus | travel */
  color: string;
  /** 重复规则（RRULE 子集）；空 = 不重复 */
  rrule: string;
  /** 跳过的日期 YYYY-MM-DD，逗号分隔 */
  exdates: string;
  /** 范围查询展开出来的一次：是哪天的那次（id 是系列的）；空 = 不是重复日程的展开项 */
  recurrence_day: string;
};

export type OfficeEventKind = 'plan' | 'log';
export type OfficeEventStatus = 'todo' | 'doing' | 'done' | 'cancelled';

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
  /** 发件地址 */
  email: string;
  /** 服务商预设键（qq / exmail / 163 …）；手动填为空 */
  provider: string;
  smtp_host: string;
  smtp_port: number;
  smtp_tls: MailTlsMode | 'TLS_MODE_UNSPECIFIED';
  /** '' 未检查 | ok | auth_failed | unreachable */
  status: '' | 'ok' | 'auth_failed' | 'unreachable' | string;
  /** dns | unreachable | tls | auth | protocol */
  last_error: string;
  /** 签名（纯文本），写信时自动带上 */
  signature: string;
  /** 发出的信同时存一份到服务器「已发送」 */
  save_sent: boolean;
};

/** 邮件页视图（左栏）。 */
export type MailView =
  | 'inbox'
  | 'unread'
  | 'flagged'
  | 'task'
  | 'attachments'
  | 'sent'
  | 'drafts'
  | 'archived'
  | 'spam'
  | 'trash';

export type MailboxSummaryRow = {
  inbox_unread: number;
  flagged: number;
  task: number;
  attachments: number;
  sent: number;
  drafts: number;
  archived: number;
  /** 垃圾邮件里没读的 */
  spam_unread: number;
  /** 已删除里的总数 */
  trash: number;
  /** 各标签的邮件数 */
  labels: Array<{ name: string; count: number }>;
  /** 账号 id → 收件箱未读 */
  accounts: Record<string, number>;
};

export type MailProviderRow = {
  found: boolean;
  key: string;
  name: string;
  imap_host: string;
  imap_port: number;
  imap_tls: MailTlsMode;
  smtp_host: string;
  smtp_port: number;
  smtp_tls: MailTlsMode;
  /** false = 要 OAuth 登录，暂不支持 */
  supported: boolean;
  detected_by: '' | 'domain' | 'mx' | string;
};

export type MailTestResult = {
  imap_ok: boolean;
  imap_error: string;
  inbox_count: number;
  smtp_ok: boolean;
  smtp_error: string;
  smtp_skipped: boolean;
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
  /** 收件人 / 抄送（解码后原文） */
  to: string;
  cc: string;
  seen: boolean;
  flagged: boolean;
  archived: boolean;
  /** inbox | sent | drafts | junk（服务器垃圾邮件文件夹同步来的） */
  folder: string;
  /** 在「已删除」里 */
  trashed: boolean;
  spam: boolean;
  labels: string[];
  /** 定时发送的草稿：RFC3339；空 = 普通草稿 */
  send_at: string;
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
  source: 'manual' | 'mail' | 'delegate' | 'note' | string;
  /** 邮件转任务：邮件 id；指令栏交办：承载会话 id */
  source_ref: string;
  /** open 待办 | doing 我在做 | done 已完成（派给 AI 的保持 open，进度看会话） */
  status: OfficeTaskStatus | string;
  /** 空 = 负责人是本人；agent / team = 派给了 AI */
  assignee_kind: string;
  assignee_id: string;
  assignee_name: string;
  /** AI 执行的承载会话；没派给 AI 为空 */
  session_id: string;
  /** RFC3339；空串 = 没设到期 */
  due_at: string;
  file_count: number;
  created_at: string;
  updated_at: string;
};

export type OfficeTaskStatus = 'open' | 'doing' | 'done';

/** 任务附件元数据。 */
export type OfficeTaskFileRow = {
  id: string;
  task_id: string;
  filename: string;
  content_type: string;
  size: number;
  created_at: string;
};
