import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  createWebhook,
  deleteWebhook,
  listWebhooks,
  listWebhooksPaged,
  updateWebhook,
  type WebhookListQuery,
} from '../../features/webhooks/api';
import type { WebhookRow } from '../../features/webhooks/types';

export const useWebhooksStore = defineStore('webhooks', () => {
  const webhooks = ref<WebhookRow[]>([]);
  const total = ref(0);
  const loading = ref(false);

  async function loadWebhooks(query?: WebhookListQuery): Promise<WebhookRow[]> {
    loading.value = true;
    try {
      if (query) {
        const result = await listWebhooksPaged(query);
        webhooks.value = result.items;
        total.value = result.total;
        return webhooks.value;
      }
      webhooks.value = await listWebhooks();
      total.value = webhooks.value.length;
      return webhooks.value;
    } finally {
      loading.value = false;
    }
  }

  async function addWebhook(input: {
    name: string;
    url: string;
    event_types_json?: string;
    secret?: string;
    headers?: Record<string, string>;
    enabled?: boolean;
  }): Promise<WebhookRow> {
    const created = await createWebhook(input);
    webhooks.value = [created, ...webhooks.value.filter((row) => row.id !== created.id)];
    total.value += 1;
    return created;
  }

  async function saveWebhook(
    id: string,
    patch: {
      name?: string;
      url?: string;
      event_types_json?: string;
      secret?: string;
      headers?: Record<string, string>;
      enabled?: boolean;
    },
  ): Promise<WebhookRow> {
    const updated = await updateWebhook(id, patch);
    webhooks.value = webhooks.value.map((row) => (row.id === updated.id ? updated : row));
    return updated;
  }

  async function removeWebhook(id: string): Promise<void> {
    await deleteWebhook(id);
    webhooks.value = webhooks.value.filter((row) => row.id !== id);
    total.value = Math.max(0, total.value - 1);
  }

  return { webhooks, total, loading, loadWebhooks, addWebhook, saveWebhook, removeWebhook };
});
