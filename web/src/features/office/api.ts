import {
  createOfficeAppsService,
  createOfficeMailService,
  createOfficeNotifyService,
  createOfficeScheduleService,
} from '../../services';
import { KRATOS_API_LONG_TIMEOUT_MS, kratosApi } from '../../services/axiosHandler';
import type {
  ICalTokenRow,
  MailAccountRow,
  MailAttachmentRow,
  MailboxSummaryRow,
  MailMessageRow,
  MailProviderRow,
  MailTestResult,
  MailTlsMode,
  MailView,
  OfficeAppRow,
  OfficeEventKind,
  OfficeEventRow,
  OfficeEventSource,
  OfficeEventStatus,
  OfficeNotificationRow,
  OfficeTaskFileRow,
  OfficeTaskRow,
  OfficeTaskStatus,
} from './types';

// office 套件 API 适配（E 阶段 W1c）：proto 生成客户端 → 页内行模型。
// 时间一律 RFC3339 字符串过线（proto Timestamp），页内再转本地呈现。

function wireToEvent(raw: Record<string, unknown>): OfficeEventRow {
  return {
    id: String(raw.id ?? ''),
    task_ref: String(raw.taskRef ?? raw.task_ref ?? ''),
    task_title: String(raw.taskTitle ?? raw.task_title ?? ''),
    title: String(raw.title ?? ''),
    note: String(raw.note ?? ''),
    start_at: String(raw.startAt ?? raw.start_at ?? ''),
    end_at: String(raw.endAt ?? raw.end_at ?? ''),
    timezone: String(raw.timezone ?? ''),
    reminder_minutes: Number(raw.reminderMinutes ?? raw.reminder_minutes ?? 0),
    source: String(raw.source ?? '') as OfficeEventSource,
    kind: (String(raw.kind ?? '') || 'plan') as OfficeEventKind,
    status: (String(raw.status ?? '') || 'todo') as OfficeEventStatus,
    all_day: Boolean(raw.allDay ?? raw.all_day ?? false),
    outcome: String(raw.outcome ?? ''),
    location: String(raw.location ?? ''),
    url: String(raw.url ?? ''),
    color: String(raw.color ?? ''),
    rrule: String(raw.rrule ?? ''),
    exdates: String(raw.exdates ?? ''),
    recurrence_day: String(raw.recurrenceDay ?? raw.recurrence_day ?? ''),
  };
}

/** 按关键字找日程（全部时间；重复日程按系列返回一条）。 */
export async function searchEvents(keyword: string, limit = 50): Promise<OfficeEventRow[]> {
  const res = await createOfficeScheduleService().SearchEvents({ keyword, limit });
  return (res.items ?? []).map((row) => wireToEvent(row as Record<string, unknown>));
}

// ---------- 日程事件 ----------

export async function listEventsRange(startISO: string, endISO: string): Promise<OfficeEventRow[]> {
  const res = await createOfficeScheduleService().ListEventsRange({ start: startISO, end: endISO });
  return (res.items ?? []).map((row) => wireToEvent(row as Record<string, unknown>));
}

export type EventInput = {
  title: string;
  note: string;
  start_at: string;
  end_at: string;
  timezone: string;
  reminder_minutes: number;
  task_ref?: string;
  kind?: OfficeEventKind;
  status?: OfficeEventStatus;
  all_day?: boolean;
  outcome?: string;
  location?: string;
  url?: string;
  color?: string;
  rrule?: string;
  /** 只在更新时用：整串替换 */
  exdates?: string;
};

export async function createEvent(input: EventInput): Promise<OfficeEventRow> {
  const res = await createOfficeScheduleService().CreateEvent({
    title: input.title,
    note: input.note,
    startAt: input.start_at,
    endAt: input.end_at,
    timezone: input.timezone,
    reminderMinutes: input.reminder_minutes,
    taskRef: input.task_ref ?? '',
    kind: input.kind ?? 'plan',
    status: input.status ?? 'todo',
    allDay: input.all_day ?? false,
    outcome: input.outcome ?? '',
    location: input.location ?? '',
    url: input.url ?? '',
    color: input.color ?? '',
    rrule: input.rrule ?? '',
  });
  return wireToEvent(res as Record<string, unknown>);
}

export async function getEvent(id: string): Promise<OfficeEventRow> {
  return wireToEvent((await createOfficeScheduleService().GetEvent({ id })) as Record<string, unknown>);
}

export async function updateEvent(id: string, input: EventInput): Promise<OfficeEventRow> {
  const res = await createOfficeScheduleService().UpdateEvent({
    id,
    title: input.title,
    note: input.note,
    startAt: input.start_at,
    endAt: input.end_at,
    timezone: input.timezone,
    reminderMinutes: input.reminder_minutes,
    // 有 presence 的字段：给了就改（含清空），不给不改
    kind: input.kind,
    status: input.status,
    allDay: input.all_day,
    outcome: input.outcome,
    location: input.location,
    url: input.url,
    color: input.color,
    rrule: input.rrule,
    exdates: input.exdates,
  });
  return wireToEvent(res as Record<string, unknown>);
}

/** 只改状态（如点一下「进行中 → 已完成」）。后端每次都会应用提醒分钟数（0 = 关提醒），所以把原值带上。 */
export async function setEventStatus(ev: OfficeEventRow, status: OfficeEventStatus): Promise<OfficeEventRow> {
  const res = await createOfficeScheduleService().UpdateEvent({
    id: ev.id,
    title: ev.title,
    // note 不传 = 不改（传空串会清空说明）
    startAt: undefined,
    endAt: undefined,
    timezone: '',
    reminderMinutes: ev.reminder_minutes,
    status,
  });
  return wireToEvent(res as Record<string, unknown>);
}

export async function deleteEvent(id: string): Promise<void> {
  await createOfficeScheduleService().DeleteEvent({ id });
}

// ---------- 通知中心 ----------

export type NotificationListResult = {
  items: OfficeNotificationRow[];
  total: number;
  unread_count: number;
};

export async function listNotifications(query: {
  unreadOnly?: boolean;
  page?: number;
  pageSize?: number;
}): Promise<NotificationListResult> {
  const res = await createOfficeNotifyService().ListNotifications({
    unreadOnly: query.unreadOnly ?? false,
    page: query.page ?? 1,
    pageSize: query.pageSize ?? 20,
  });
  return {
    items: (res.items ?? []).map((raw) => {
      const r = raw as Record<string, unknown>;
      return {
        id: String(r.id ?? ''),
        kind: String(r.kind ?? ''),
        title: String(r.title ?? ''),
        body: String(r.body ?? ''),
        link: String(r.link ?? ''),
        read: Boolean(r.read ?? false),
        created_at: String(r.createdAt ?? r.created_at ?? ''),
      };
    }),
    total: Number(res.total ?? 0),
    unread_count: Number(res.unreadCount ?? (res as unknown as Record<string, unknown>).unread_count ?? 0),
  };
}

export async function dismissNotification(id: string): Promise<void> {
  await createOfficeNotifyService().DismissNotification({ id });
}

export async function markAllNotificationsRead(): Promise<void> {
  await createOfficeNotifyService().MarkAllNotificationsRead({});
}

// ---------- ics 订阅令牌 ----------

function wireToToken(raw: Record<string, unknown>): ICalTokenRow {
  return {
    id: String(raw.id ?? ''),
    name: String(raw.name ?? ''),
    feed_url: String(raw.feedUrl ?? raw.feed_url ?? ''),
    created_at: String(raw.createdAt ?? raw.created_at ?? ''),
    revoked_at: String(raw.revokedAt ?? raw.revoked_at ?? ''),
  };
}

/** 创建令牌；明文 feed_url 仅此返回值出现一次（列表/刷新不可恢复）。 */
export async function createICalToken(name: string): Promise<ICalTokenRow> {
  const res = await createOfficeScheduleService().CreateICalToken({ name });
  return wireToToken(res as Record<string, unknown>);
}

export async function listICalTokens(): Promise<ICalTokenRow[]> {
  const res = await createOfficeScheduleService().ListICalTokens({});
  return (res.items ?? []).map((row) => wireToToken(row as Record<string, unknown>));
}

export async function revokeICalToken(id: string): Promise<void> {
  await createOfficeScheduleService().RevokeICalToken({ id });
}

// ---------- 应用启动器 ----------

function wireToApp(raw: Record<string, unknown>): OfficeAppRow {
  return {
    id: String(raw.id ?? ''),
    name: String(raw.name ?? ''),
    url: String(raw.url ?? ''),
    icon: String(raw.icon ?? ''),
    group_name: String(raw.groupName ?? raw.group_name ?? ''),
    sort_order: Number(raw.sortOrder ?? raw.sort_order ?? 0),
  };
}

export async function listApps(): Promise<OfficeAppRow[]> {
  const res = await createOfficeAppsService().ListApps({});
  return (res.items ?? []).map((row) => wireToApp(row as Record<string, unknown>));
}

export type AppInput = {
  name: string;
  url: string;
  icon: string;
  group_name: string;
  sort_order: number;
};

export async function upsertApp(id: string, input: AppInput): Promise<OfficeAppRow> {
  const res = await createOfficeAppsService().UpsertApp({
    id,
    name: input.name,
    url: input.url,
    icon: input.icon,
    groupName: input.group_name,
    sortOrder: input.sort_order,
  });
  return wireToApp(res as Record<string, unknown>);
}

export async function deleteApp(id: string): Promise<void> {
  await createOfficeAppsService().DeleteApp({ id });
}

// ---------- 邮件聚合（W1d） ----------

function wireToMailAccount(raw: Record<string, unknown>): MailAccountRow {
  return {
    id: String(raw.id ?? ''),
    name: String(raw.name ?? ''),
    host: String(raw.host ?? ''),
    port: Number(raw.port ?? 0),
    tls_mode: String(raw.tlsMode ?? raw.tls_mode ?? 'TLS_MODE_TLS') as MailTlsMode,
    username: String(raw.username ?? ''),
    last_uid: Number(raw.lastUid ?? raw.last_uid ?? 0),
    last_sync_at: String(raw.lastSyncAt ?? raw.last_sync_at ?? ''),
    email: String(raw.email ?? ''),
    provider: String(raw.provider ?? ''),
    smtp_host: String(raw.smtpHost ?? raw.smtp_host ?? ''),
    smtp_port: Number(raw.smtpPort ?? raw.smtp_port ?? 0),
    smtp_tls: String(raw.smtpTls ?? raw.smtp_tls ?? 'TLS_MODE_UNSPECIFIED') as MailAccountRow['smtp_tls'],
    status: String(raw.status ?? ''),
    last_error: String(raw.lastError ?? raw.last_error ?? ''),
    signature: String(raw.signature ?? ''),
    save_sent: Boolean(raw.saveSent ?? raw.save_sent ?? false),
  };
}

function wireToMailMessage(raw: Record<string, unknown>): MailMessageRow {
  return {
    id: String(raw.id ?? ''),
    account_id: String(raw.accountId ?? raw.account_id ?? ''),
    subject: String(raw.subject ?? ''),
    sender: String(raw.sender ?? ''),
    snippet: String(raw.snippet ?? ''),
    received_at: String(raw.receivedAt ?? raw.received_at ?? ''),
    task_ref: String(raw.taskRef ?? raw.task_ref ?? ''),
    has_attachments: Boolean(raw.hasAttachments ?? raw.has_attachments ?? false),
    to: String(raw.to ?? ''),
    cc: String(raw.cc ?? ''),
    seen: Boolean(raw.seen ?? false),
    flagged: Boolean(raw.flagged ?? false),
    archived: Boolean(raw.archived ?? false),
    folder: String(raw.folder ?? 'inbox') || 'inbox',
    trashed: Boolean(raw.trashed ?? false),
    spam: Boolean(raw.spam ?? false),
    labels: Array.isArray(raw.labels) ? raw.labels.map(String) : [],
    send_at: String(raw.sendAt ?? raw.send_at ?? ''),
  };
}

function wireToMailAttachment(raw: Record<string, unknown>): MailAttachmentRow {
  return {
    id: String(raw.id ?? ''),
    message_id: String(raw.messageId ?? raw.message_id ?? ''),
    filename: String(raw.filename ?? ''),
    content_type: String(raw.contentType ?? raw.content_type ?? ''),
    size: Number(raw.size ?? 0),
    knowledge_doc_id: String(raw.knowledgeDocId ?? raw.knowledge_doc_id ?? ''),
  };
}

export async function listMailAccounts(): Promise<MailAccountRow[]> {
  const res = await createOfficeMailService().ListAccounts({});
  return (res.items ?? []).map((row) => wireToMailAccount(row as Record<string, unknown>));
}

export type MailAccountInput = {
  name: string;
  host: string;
  port: number;
  tls_mode: MailTlsMode;
  username: string;
  /** 创建必填；编辑留空 = 不轮换密码。 */
  password: string;
  email?: string;
  provider?: string;
  smtp_host?: string;
  smtp_port?: number;
  smtp_tls?: MailTlsMode;
  /** 首次只同步这天之后的（RFC3339）；空 = 全部（只在创建时有效） */
  sync_since?: string;
};

export async function createMailAccount(input: MailAccountInput): Promise<MailAccountRow> {
  const res = await createOfficeMailService().CreateAccount({
    name: input.name,
    host: input.host,
    port: input.port,
    tlsMode: input.tls_mode,
    username: input.username,
    password: input.password,
    email: input.email ?? '',
    provider: input.provider ?? '',
    smtpHost: input.smtp_host ?? '',
    smtpPort: input.smtp_port ?? 0,
    smtpTls: input.smtp_tls,
    syncSince: input.sync_since || undefined,
  } as never);
  return wireToMailAccount(res as Record<string, unknown>);
}

export async function updateMailAccount(id: string, input: MailAccountInput): Promise<MailAccountRow> {
  const res = await createOfficeMailService().UpdateAccount({
    id,
    name: input.name,
    host: input.host,
    port: input.port,
    tlsMode: input.tls_mode,
    username: input.username,
    password: input.password,
    email: input.email ?? '',
    provider: input.provider ?? '',
    smtpHost: input.smtp_host ?? '',
    smtpPort: input.smtp_port ?? 0,
    smtpTls: input.smtp_tls,
  } as never);
  return wireToMailAccount(res as Record<string, unknown>);
}

function wireToProvider(raw: Record<string, unknown>): MailProviderRow {
  return {
    found: Boolean(raw.found ?? false),
    key: String(raw.key ?? ''),
    name: String(raw.name ?? ''),
    imap_host: String(raw.imapHost ?? ''),
    imap_port: Number(raw.imapPort ?? 0),
    imap_tls: String(raw.imapTls ?? 'TLS_MODE_TLS') as MailTlsMode,
    smtp_host: String(raw.smtpHost ?? ''),
    smtp_port: Number(raw.smtpPort ?? 0),
    smtp_tls: String(raw.smtpTls ?? 'TLS_MODE_TLS') as MailTlsMode,
    supported: Boolean(raw.supported ?? false),
    detected_by: String(raw.detectedBy ?? ''),
  };
}

/** 向导第 1 步：按邮箱地址认服务商（域名，认不出再查 MX）。 */
export async function detectMailProvider(email: string): Promise<MailProviderRow> {
  const res = await createOfficeMailService().DetectProvider({ email });
  return wireToProvider(res as Record<string, unknown>);
}

export async function listMailProviders(): Promise<MailProviderRow[]> {
  const res = await createOfficeMailService().ListProviders({});
  return (res.items ?? []).map((row) => wireToProvider(row as Record<string, unknown>));
}

/** 测连接：传 id 测已保存的账号；否则测向导里还没保存的配置。 */
export async function testMailAccount(input: { id: string } | MailAccountInput): Promise<MailTestResult> {
  const svc = createOfficeMailService();
  const res = (
    'id' in input
      ? await svc.TestAccount({ id: input.id } as never)
      : await svc.TestAccount({
          id: '',
          host: input.host,
          port: input.port,
          tlsMode: input.tls_mode,
          username: input.username,
          password: input.password,
          smtpHost: input.smtp_host ?? '',
          smtpPort: input.smtp_port ?? 0,
          smtpTls: input.smtp_tls,
        } as never)
  ) as Record<string, unknown>;
  return {
    imap_ok: Boolean(res.imapOk ?? false),
    imap_error: String(res.imapError ?? ''),
    inbox_count: Number(res.inboxCount ?? 0),
    smtp_ok: Boolean(res.smtpOk ?? false),
    smtp_error: String(res.smtpError ?? ''),
    smtp_skipped: Boolean(res.smtpSkipped ?? false),
  };
}

export async function getMailboxSummary(): Promise<MailboxSummaryRow> {
  const res = (await createOfficeMailService().GetMailboxSummary({})) as Record<string, unknown>;
  const accounts: Record<string, number> = {};
  for (const a of Array.isArray(res.accounts) ? res.accounts : []) {
    const rec = a as Record<string, unknown>;
    accounts[String(rec.accountId ?? '')] = Number(rec.unread ?? 0);
  }
  return {
    inbox_unread: Number(res.inboxUnread ?? 0),
    flagged: Number(res.flagged ?? 0),
    task: Number(res.task ?? 0),
    attachments: Number(res.attachments ?? 0),
    sent: Number(res.sent ?? 0),
    drafts: Number(res.drafts ?? 0),
    archived: Number(res.archived ?? 0),
    spam_unread: Number(res.spamUnread ?? 0),
    trash: Number(res.trash ?? 0),
    labels: (Array.isArray(res.labels) ? res.labels : []).map((l) => {
      const r = l as Record<string, unknown>;
      return { name: String(r.name ?? ''), count: Number(r.count ?? 0) };
    }),
    accounts,
  };
}

/** 批量标已读 / 星标 / 归档；不传 = 不改。 */
export async function markMailMessages(
  ids: string[],
  mark: {
    seen?: boolean;
    flagged?: boolean;
    archived?: boolean;
    trashed?: boolean;
    spam?: boolean;
    addLabel?: string;
    removeLabel?: string;
  },
): Promise<number> {
  const res = await createOfficeMailService().MarkMessages({
    ids,
    ...mark,
    addLabel: mark.addLabel ?? '',
    removeLabel: mark.removeLabel ?? '',
  });
  return Number(res.updated ?? 0);
}

/** 视图（可限定账户）里的未读全部标已读。 */
export async function markAllMailRead(view: MailView, accountId = ''): Promise<number> {
  const res = await createOfficeMailService().MarkAllRead({ view, accountId });
  return Number(res.updated ?? 0);
}

/** 同一会话（主题去掉 Re: / 回复: 后相同）的往来，按时间正序。 */
export async function listMailThread(id: string): Promise<MailMessageRow[]> {
  const res = await createOfficeMailService().ListThread({ id });
  return (res.items ?? []).map((row) => wireToMailMessage(row as Record<string, unknown>));
}

export type MailContactRow = { name: string; address: string; count: number };

/** 收件人联想：从来往邮件里统计。 */
export async function suggestMailContacts(q: string, limit = 8): Promise<MailContactRow[]> {
  const res = await createOfficeMailService().SuggestContacts({ q, limit });
  return (res.items ?? []).map((r) => ({
    name: String(r.name ?? ''),
    address: String(r.address ?? ''),
    count: Number(r.count ?? 0),
  }));
}

export async function listBlockedSenders(): Promise<string[]> {
  const res = await createOfficeMailService().ListBlockedSenders({});
  return (res.addresses ?? []).map(String);
}

/** 拒收：返回顺手标成垃圾邮件的封数。 */
export async function blockSender(address: string): Promise<number> {
  const res = await createOfficeMailService().BlockSender({ address });
  return Number(res.marked ?? 0);
}

export async function unblockSender(address: string): Promise<void> {
  await createOfficeMailService().UnblockSender({ address });
}

/** 只改签名 / 是否存已发送（其余设置不动）。 */
export async function updateMailAccountSettings(
  id: string,
  input: { signature: string; save_sent: boolean },
): Promise<MailAccountRow> {
  const res = await createOfficeMailService().UpdateAccount({
    id,
    signature: input.signature,
    saveSent: input.save_sent,
  } as never);
  return wireToMailAccount(res as Record<string, unknown>);
}

/** 彻底删除「已删除」/「垃圾邮件」里的信；ids 空 = 清空整个视图。 */
export async function purgeMailMessages(view: 'trash' | 'spam', ids: string[] = []): Promise<number> {
  const res = await createOfficeMailService().PurgeMessages({ view, ids });
  return Number(res.deleted ?? 0);
}

/** File / Blob → base64（proto bytes 字段的线格式）。 */
export async function blobToBase64(blob: Blob): Promise<string> {
  const buf = new Uint8Array(await blob.arrayBuffer());
  let bin = '';
  const step = 0x8000;
  for (let i = 0; i < buf.length; i += step) {
    bin += String.fromCharCode(...buf.subarray(i, i + step));
  }
  return btoa(bin);
}

export type MailOutgoingInput = {
  account_id: string;
  to: string;
  cc: string;
  subject: string;
  body: string;
  reply_to_id?: string;
  forward_of_id?: string;
  draft_id?: string;
  /** 新加的附件（已有草稿 / 转发的附件由服务端带上） */
  files: File[];
  /** 带格式的正文（HTML）；空 = 纯文本 */
  body_html?: string;
  /** 定时发送：ISO 时间，晚于现在 = 存成定时草稿 */
  send_at?: string;
};

async function outgoingWire(input: MailOutgoingInput) {
  const attachments = await Promise.all(
    input.files.map(async (f) => ({
      filename: f.name,
      contentType: f.type || 'application/octet-stream',
      content: await blobToBase64(f),
    })),
  );
  return {
    accountId: input.account_id,
    to: input.to,
    cc: input.cc,
    subject: input.subject,
    body: input.body,
    replyToId: input.reply_to_id ?? '',
    forwardOfId: input.forward_of_id ?? '',
    draftId: input.draft_id ?? '',
    attachments,
    bodyHtml: input.body_html ?? '',
    sendAt: input.send_at || undefined,
  };
}

/** 发信：发出后存进「已发送」；从草稿发出会删掉草稿。返回已发送那封的 id。 */
export async function sendMail(input: MailOutgoingInput): Promise<string> {
  const res = await createOfficeMailService().SendMail(await outgoingWire(input));
  return String(res.sendId ?? '');
}

/** 存草稿（draft_id 有 = 替换旧草稿）。返回新草稿 id。 */
export async function saveMailDraft(input: MailOutgoingInput): Promise<string> {
  const res = await createOfficeMailService().SaveDraft(await outgoingWire(input));
  return String(res.draftId ?? '');
}

export async function deleteMailAccount(id: string): Promise<void> {
  await createOfficeMailService().DeleteAccount({ id });
}

/** 首次同步动辄几十秒：用长超时；真超时了服务端也会同步完（不随请求取消）。 */
export async function syncMailAccount(id: string): Promise<{ imported: number; skipped: number }> {
  const res = await kratosApi.post(
    `/v1/office/mail/accounts/${encodeURIComponent(id)}/sync`,
    {},
    { timeout: KRATOS_API_LONG_TIMEOUT_MS },
  );
  const d = (res.data ?? {}) as Record<string, unknown>;
  return {
    imported: Number(d.importedCount ?? d.imported_count ?? 0),
    skipped: Number(d.skippedCount ?? d.skipped_count ?? 0),
  };
}

export type MailMessageListResult = {
  items: MailMessageRow[];
  total: number;
};

export async function listMailMessages(query: {
  accountId?: string;
  view?: MailView;
  keyword?: string;
  label?: string;
  page?: number;
  pageSize?: number;
}): Promise<MailMessageListResult> {
  const res = await createOfficeMailService().ListMessages({
    accountId: query.accountId ?? '',
    quadrant: '',
    page: query.page ?? 1,
    pageSize: query.pageSize ?? 20,
    view: query.view ?? 'inbox',
    label: query.label ?? '',
    keyword: query.keyword ?? '',
  });
  return {
    items: (res.items ?? []).map((row) => wireToMailMessage(row as Record<string, unknown>)),
    total: Number(res.total ?? 0),
  };
}

export async function getMailMessage(
  id: string,
): Promise<{ message: MailMessageRow; body: string; body_html: string }> {
  const res = await createOfficeMailService().GetMessage({ id });
  return {
    message: wireToMailMessage((res.message ?? {}) as Record<string, unknown>),
    body: String(res.body ?? ''),
    body_html: String((res as Record<string, unknown>).bodyHtml ?? ''),
  };
}

export async function deleteMailMessage(id: string): Promise<void> {
  await createOfficeMailService().DeleteMessage({ id });
}

export async function listMailAttachments(messageId: string): Promise<MailAttachmentRow[]> {
  const res = await createOfficeMailService().ListAttachments({ messageId });
  return (res.items ?? []).map((row) => wireToMailAttachment(row as Record<string, unknown>));
}

/**
 * 附件下载：服务端 HttpBody 以原始字节流出（非 JSON base64），走 blob
 * （与 knowledge fetchDocumentAsset 同模式）；仅一次性 objectURL。
 */
export async function fetchMailAttachment(att: MailAttachmentRow): Promise<Blob> {
  const res = await kratosApi.get(`/v1/office/mail/attachments/${encodeURIComponent(att.id)}/download`, {
    responseType: 'blob',
    timeout: KRATOS_API_LONG_TIMEOUT_MS,
  });
  return res.data as Blob;
}

export async function downloadMailAttachment(att: MailAttachmentRow): Promise<void> {
  const url = URL.createObjectURL(await fetchMailAttachment(att));
  const a = document.createElement('a');
  a.href = url;
  a.download = att.filename || 'attachment';
  document.body.appendChild(a);
  a.click();
  a.remove();
  // 点完立刻 revoke，Firefox / Safari 会把下载取消掉
  setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

export async function attachmentToKnowledge(id: string, classification: number): Promise<string> {
  const res = await createOfficeMailService().AttachmentToKnowledge({ id, classification });
  return String(res.knowledgeDocId ?? (res as unknown as Record<string, unknown>).knowledge_doc_id ?? '');
}

/** .eml 导入：proto bytes 字段要 base64（原来直接传文本，服务端解不开）。 */
export async function importMailEml(file: Blob): Promise<MailMessageRow> {
  const res = await createOfficeMailService().ImportEml({ eml: await blobToBase64(file) });
  return wireToMailMessage(res as Record<string, unknown>);
}

/** 导出 .eml 原文（HttpBody 原始字节 → Blob 保存）。 */
export async function exportMailEml(msg: MailMessageRow): Promise<void> {
  const res = await kratosApi.get(`/v1/office/mail/messages/${encodeURIComponent(msg.id)}/eml`, {
    responseType: 'blob',
  });
  const blob = res.data as Blob;
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${msg.id}.eml`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export async function mailToTask(id: string, titleOverride: string): Promise<string> {
  const res = await createOfficeMailService().MailToTask({ id, titleOverride });
  return String(res.taskRef ?? (res as unknown as Record<string, unknown>).task_ref ?? '');
}

// ---------- 个人任务（工作台「我的任务」） ----------

function wireToTask(raw: Record<string, unknown>): OfficeTaskRow {
  return {
    id: String(raw.id ?? ''),
    title: String(raw.title ?? ''),
    body: String(raw.body ?? ''),
    source: String(raw.source ?? ''),
    source_ref: String(raw.sourceRef ?? raw.source_ref ?? ''),
    status: String(raw.status ?? ''),
    assignee_kind: String(raw.assigneeKind ?? raw.assignee_kind ?? ''),
    assignee_id: String(raw.assigneeId ?? raw.assignee_id ?? ''),
    assignee_name: String(raw.assigneeName ?? raw.assignee_name ?? ''),
    session_id: String(raw.sessionId ?? raw.session_id ?? ''),
    due_at: String(raw.dueAt ?? raw.due_at ?? ''),
    file_count: Number(raw.fileCount ?? raw.file_count ?? 0),
    created_at: String(raw.createdAt ?? raw.created_at ?? ''),
    updated_at: String(raw.updatedAt ?? raw.updated_at ?? ''),
  };
}

/**
 * source 缺省 = 我的任务（手工 + 邮件转，且没派给 AI）；delegate = 派给了 AI 的；note = 随手记录。
 * status：active = 待办 + 我在做。
 */
export async function listOfficeTasks(
  status: OfficeTaskStatus | 'active' = 'open',
  limit = 50,
  source: '' | 'delegate' | 'note' = '',
): Promise<OfficeTaskRow[]> {
  const res = await createOfficeMailService().ListOfficeTasks({ status, limit, source: source || undefined });
  return (res.items ?? []).map((row) => wireToTask(row as Record<string, unknown>));
}

/**
 * 交办登记：记下这件事派给了谁、在哪个会话里跟进。
 * agent / team 必须有承载会话；tool / run / flow（运行台的编码工具、几个工具一起做、流程）没有会话，
 * sessionId 传空串，进度看任务的执行（listOfficeTaskExecutions）。
 */
export type OfficeTaskDelegation = {
  sessionId: string;
  assigneeKind: 'agent' | 'team' | 'tool' | 'run' | 'flow';
  assigneeId: string;
  assigneeName: string;
};

export async function createOfficeTask(
  title: string,
  body = '',
  kind: 'manual' | 'note' | OfficeTaskDelegation = 'manual',
  dueAt = '',
): Promise<OfficeTaskRow> {
  const delegation = typeof kind === 'object' ? kind : null;
  const res = await createOfficeMailService().CreateOfficeTask({
    title,
    body: body || undefined,
    source: delegation ? 'delegate' : kind === 'note' ? 'note' : undefined,
    sourceRef: delegation?.sessionId || undefined,
    assigneeKind: delegation?.assigneeKind,
    assigneeId: delegation?.assigneeId,
    assigneeName: delegation?.assigneeName,
    dueAt: dueAt || undefined,
  });
  return wireToTask(res as Record<string, unknown>);
}

/** 任务的一次执行：指向别处的一条记录（会话、流程运行、编码任务、运行台终端…），状态由各自的接口读 */
export type OfficeTaskExecutionKind =
  | 'session'
  | 'team_run'
  | 'graph_execution'
  | 'coding_task'
  | 'fleet_terminal'
  | 'fleet_run'
  | 'cron_run';

export interface OfficeTaskExecutionRow {
  id: string;
  task_id: string;
  kind: OfficeTaskExecutionKind;
  ref_id: string;
  /** agent:<id> / team:<id> / flow:<graphId> / 工具键 */
  executor: string;
  executor_name: string;
  /** 派出时发给执行者的交接单原文 */
  hand: string;
  created_at: string;
}

function wireToExecution(raw: Record<string, unknown>): OfficeTaskExecutionRow {
  return {
    id: String(raw.id ?? ''),
    task_id: String(raw.taskId ?? raw.task_id ?? ''),
    kind: String(raw.kind ?? '') as OfficeTaskExecutionKind,
    ref_id: String(raw.refId ?? raw.ref_id ?? ''),
    executor: String(raw.executor ?? ''),
    executor_name: String(raw.executorName ?? raw.executor_name ?? ''),
    hand: String(raw.hand ?? ''),
    created_at: String(raw.createdAt ?? raw.created_at ?? ''),
  };
}

/** 不带 taskId = 本人全部任务的执行（按先后） */
export async function listOfficeTaskExecutions(taskId = ''): Promise<OfficeTaskExecutionRow[]> {
  const res = await createOfficeMailService().ListOfficeTaskExecutions({
    taskId: taskId || undefined,
    limit: undefined,
  });
  return (res.items ?? []).map((x) => wireToExecution(x as Record<string, unknown>));
}

/** 把一条执行记录挂到任务下；同一条重复挂是幂等的 */
export async function addOfficeTaskExecution(
  taskId: string,
  e: Pick<OfficeTaskExecutionRow, 'kind' | 'ref_id'> &
    Partial<Pick<OfficeTaskExecutionRow, 'executor' | 'executor_name' | 'hand'>>,
): Promise<OfficeTaskExecutionRow> {
  const res = await createOfficeMailService().AddOfficeTaskExecution({
    taskId,
    kind: e.kind,
    refId: e.ref_id,
    executor: e.executor || undefined,
    executorName: e.executor_name || undefined,
    hand: e.hand || undefined,
  });
  return wireToExecution(res as Record<string, unknown>);
}

export async function setOfficeTaskStatus(id: string, status: OfficeTaskStatus): Promise<OfficeTaskRow> {
  const res = await createOfficeMailService().SetOfficeTaskStatus({ id, status });
  return wireToTask(res as Record<string, unknown>);
}

/** 改标题 / 说明 / 到期；dueAt 传 null = 清掉到期，undefined = 不动。 */
export async function updateOfficeTask(
  id: string,
  patch: { title?: string; body?: string; dueAt?: string | null },
): Promise<OfficeTaskRow> {
  const res = await createOfficeMailService().UpdateOfficeTask({
    id,
    title: patch.title,
    body: patch.body,
    dueAt: patch.dueAt || undefined,
    clearDue: patch.dueAt === null ? true : undefined,
  });
  return wireToTask(res as Record<string, unknown>);
}

/** 派给 AI（会话已建好）；delegation 传 null = 收回自己做。 */
export async function assignOfficeTask(id: string, delegation: OfficeTaskDelegation | null): Promise<OfficeTaskRow> {
  const res = await createOfficeMailService().AssignOfficeTask({
    id,
    assigneeKind: delegation?.assigneeKind,
    assigneeId: delegation?.assigneeId,
    assigneeName: delegation?.assigneeName,
    sessionId: delegation?.sessionId,
  });
  return wireToTask(res as Record<string, unknown>);
}

export async function deleteOfficeTask(id: string): Promise<void> {
  await createOfficeMailService().DeleteOfficeTask({ id });
}

// ---------- 任务附件 ----------

/** 与后端 OfficeTaskFileMaxBytes 一致。 */
export const TASK_FILE_MAX_BYTES = 50 * 1024 * 1024;
export const TASK_FILES_MAX = 20;

function wireToTaskFile(raw: Record<string, unknown>): OfficeTaskFileRow {
  return {
    id: String(raw.id ?? ''),
    task_id: String(raw.taskId ?? raw.task_id ?? ''),
    filename: String(raw.filename ?? ''),
    content_type: String(raw.contentType ?? raw.content_type ?? ''),
    size: Number(raw.size ?? 0),
    created_at: String(raw.createdAt ?? raw.created_at ?? ''),
  };
}

export async function listTaskFiles(taskId: string): Promise<OfficeTaskFileRow[]> {
  const res = await createOfficeMailService().ListOfficeTaskFiles({ taskId });
  return (res.items ?? []).map((row) => wireToTaskFile(row as Record<string, unknown>));
}

/** multipart 直传（不走 base64 JSON）；onProgress 给 0–1 的进度。 */
export async function uploadTaskFile(
  taskId: string,
  file: File,
  onProgress?: (ratio: number) => void,
): Promise<OfficeTaskFileRow> {
  const form = new FormData();
  form.append('file', file, file.name);
  form.append('filename', file.name);
  const res = await kratosApi.post(`/v1/office/tasks/${encodeURIComponent(taskId)}/files`, form, {
    timeout: KRATOS_API_LONG_TIMEOUT_MS,
    onUploadProgress: (event) => {
      if (onProgress && event.total) onProgress(event.loaded / event.total);
    },
  });
  return wireToTaskFile(res.data as Record<string, unknown>);
}

/** 附件原始字节（图片预览、转交给 AI、下载都用它）。 */
export async function fetchTaskFile(id: string): Promise<Blob> {
  const res = await kratosApi.get(`/v1/office/task-files/${encodeURIComponent(id)}`, { responseType: 'blob' });
  return res.data as Blob;
}

export async function downloadTaskFile(file: OfficeTaskFileRow): Promise<void> {
  const url = URL.createObjectURL(await fetchTaskFile(file.id));
  const a = document.createElement('a');
  a.href = url;
  a.download = file.filename || 'file';
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export async function deleteTaskFile(id: string): Promise<void> {
  await createOfficeMailService().DeleteOfficeTaskFile({ id });
}

// ---------- 任务 → 日程 ----------

/** 把任务排进日程（生成一条带 task_ref 回链的日程；时间冲突时后端回 409）。 */
export async function scheduleTask(
  task: Pick<OfficeTaskRow, 'id' | 'title'>,
  startISO: string,
  endISO: string,
): Promise<OfficeEventRow> {
  const res = await createOfficeScheduleService().ScheduleTask({
    taskRef: task.id,
    taskTitle: task.title,
    startAt: startISO,
    endAt: endISO,
    reminderMinutes: undefined,
  });
  return wireToEvent(res as Record<string, unknown>);
}

export async function listTaskEvents(taskId: string): Promise<OfficeEventRow[]> {
  const res = await createOfficeScheduleService().ListTaskEvents({ taskRef: taskId });
  return (res.items ?? []).map((row) => wireToEvent(row as Record<string, unknown>));
}
