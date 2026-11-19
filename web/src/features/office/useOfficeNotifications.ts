import { onBeforeUnmount, onMounted, ref } from 'vue';
import { dismissNotification, listNotifications, markAllNotificationsRead } from './api';
import type { OfficeNotificationRow } from './types';

// 通知中心轮询节奏（F1：无用户级 WS 推送原语，30s 轮询 + 窗口聚焦即刷；
// WS 化留待后续拍板）。
const POLL_INTERVAL_MS = 30_000;

/** useOfficeNotifications 头部铃铛数据源（MainLayout 单例承载）。 */
export function useOfficeNotifications() {
  const items = ref<OfficeNotificationRow[]>([]);
  const unreadCount = ref(0);
  let timer: ReturnType<typeof setInterval> | null = null;
  let inflight: Promise<void> | null = null;

  async function refresh(): Promise<void> {
    if (inflight) return inflight; // 轮询与手动刷新去重
    inflight = (async () => {
      try {
        const res = await listNotifications({ page: 1, pageSize: 20 });
        items.value = res.items;
        unreadCount.value = res.unread_count;
      } catch {
        // 轮询失败静默（下轮自愈）；手动操作路径的错误由调用方提示。
      } finally {
        inflight = null;
      }
    })();
    return inflight;
  }

  async function dismiss(id: string): Promise<void> {
    await dismissNotification(id);
    await refresh();
  }

  async function markAll(): Promise<void> {
    await markAllNotificationsRead();
    await refresh();
  }

  function onFocus() {
    void refresh();
  }

  onMounted(() => {
    void refresh();
    timer = setInterval(() => void refresh(), POLL_INTERVAL_MS);
    window.addEventListener('focus', onFocus);
  });

  onBeforeUnmount(() => {
    if (timer) clearInterval(timer);
    window.removeEventListener('focus', onFocus);
  });

  return { items, unreadCount, refresh, dismiss, markAll };
}
