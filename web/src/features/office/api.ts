import {
  createOfficeAppsService,
  createOfficeMailService,
  createOfficeNotifyService,
  createOfficeScheduleService,
} from '../../services';
import { kratosApi } from '../../services/axiosHandler';
import type {
  ICalTokenRow,
  MailAccountRow,
  MailAttachmentRow,
  MailMessageRow,
  MailTlsMode,
  OfficeAppRow,
  OfficeEventRow,
  OfficeEventSource,
  OfficeNotificationRow,
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
  };
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
  });
  return wireToEvent(res as Record<string, unknown>);
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
    unread_count: Number(res.unreadCount ?? res.unread_count ?? 0),
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
};

export async function createMailAccount(input: MailAccountInput): Promise<MailAccountRow> {
  const res = await createOfficeMailService().CreateAccount({
    name: input.name,
    host: input.host,
    port: input.port,
    tlsMode: input.tls_mode,
    username: input.username,
    password: input.password,
  });
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
  });
  return wireToMailAccount(res as Record<string, unknown>);
}

export async function deleteMailAccount(id: string): Promise<void> {
  await createOfficeMailService().DeleteAccount({ id });
}

export async function syncMailAccount(id: string): Promise<{ imported: number; skipped: number }> {
  const res = await createOfficeMailService().SyncAccount({ id });
  return {
    imported: Number(res.importedCount ?? res.imported_count ?? 0),
    skipped: Number(res.skippedCount ?? res.skipped_count ?? 0),
  };
}

export type MailMessageListResult = {
  items: MailMessageRow[];
  total: number;
};

export async function listMailMessages(query: {
  accountId?: string;
  page?: number;
  pageSize?: number;
}): Promise<MailMessageListResult> {
  const res = await createOfficeMailService().ListMessages({
    accountId: query.accountId ?? '',
    quadrant: '',
    page: query.page ?? 1,
    pageSize: query.pageSize ?? 20,
  });
  return {
    items: (res.items ?? []).map((row) => wireToMailMessage(row as Record<string, unknown>)),
    total: Number(res.total ?? 0),
  };
}

export async function getMailMessage(id: string): Promise<{ message: MailMessageRow; body: string }> {
  const res = await createOfficeMailService().GetMessage({ id });
  return {
    message: wireToMailMessage((res.message ?? {}) as Record<string, unknown>),
    body: String(res.body ?? ''),
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
export async function downloadMailAttachment(att: MailAttachmentRow): Promise<void> {
  const res = await kratosApi.get(`/v1/office/mail/attachments/${encodeURIComponent(att.id)}/download`, {
    responseType: 'blob',
  });
  const blob = res.data as Blob;
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = att.filename || 'attachment';
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export async function attachmentToKnowledge(id: string, classification: number): Promise<string> {
  const res = await createOfficeMailService().AttachmentToKnowledge({ id, classification });
  return String(res.knowledgeDocId ?? res.knowledge_doc_id ?? '');
}

export async function importMailEml(eml: string): Promise<MailMessageRow> {
  const res = await createOfficeMailService().ImportEml({ eml });
  return wireToMailMessage(res as Record<string, unknown>);
}

/** 导出 .eml 原文（HttpBody 原始字节 → Blob 保存）。 */
export async function exportMailEml(msg: MailMessageRow): Promise<void> {
  const res = await kratosApi.get(`/v1/office/mail/messages/${encodeURIComponent(msg.id)}:exportEml`, {
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
  return String(res.taskRef ?? res.task_ref ?? '');
}
