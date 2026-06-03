import { computed, ref } from 'vue';
import { defineStore } from 'pinia';

export type InboundNotificationKind = 'running' | 'completed';

export type InboundNotification = {
  id: string;
  sessionId: string;
  agentId: string;
  title: string;
  preview: string;
  source: string;
  kind: InboundNotificationKind;
  ts: number;
  read: boolean;
};

const MAX_ITEMS = 20;

export const useInboundNotificationStore = defineStore('inboundNotifications', () => {
  const items = ref<InboundNotification[]>([]);

  const unreadCount = computed(() => items.value.filter((n) => !n.read).length);

  function upsert(payload: Omit<InboundNotification, 'read'> & { read?: boolean }) {
    const idx = items.value.findIndex((n) => n.id === payload.id);
    const row: InboundNotification = {
      ...payload,
      read: payload.read ?? false,
    };
    if (idx >= 0) {
      items.value[idx] = { ...row, read: row.kind === 'completed' ? false : items.value[idx].read };
    } else {
      items.value.unshift(row);
    }
    if (items.value.length > MAX_ITEMS) {
      items.value = items.value.slice(0, MAX_ITEMS);
    }
  }

  function markRead(id: string) {
    const hit = items.value.find((n) => n.id === id);
    if (hit) hit.read = true;
  }

  function markAllRead() {
    for (const n of items.value) n.read = true;
  }

  function clearAll() {
    items.value = [];
  }

  return { items, unreadCount, upsert, markRead, markAllRead, clearAll };
});
