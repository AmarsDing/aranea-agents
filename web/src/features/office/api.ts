import { createOfficeAppsService, createOfficeNotifyService, createOfficeScheduleService } from '../../services';
import type { ICalTokenRow, OfficeAppRow, OfficeEventRow, OfficeEventSource, OfficeNotificationRow } from './types';

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
