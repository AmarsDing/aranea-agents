export const WEBHOOK_SECRET_MASK = '••••••••';

export const WEBHOOK_EVENT_TYPES = [
  { value: 'run.completed', labelKey: 'webhooksPage.eventRunCompleted' },
  { value: 'run.failed', labelKey: 'webhooksPage.eventRunFailed' },
  { value: 'run.cancelled', labelKey: 'webhooksPage.eventRunCancelled' },
  { value: 'graph.task.status', labelKey: 'webhooksPage.eventGraphTaskStatus' },
] as const;

export type WebhookEventType = (typeof WEBHOOK_EVENT_TYPES)[number]['value'];

export type WebhookRow = {
  id: string;
  name: string;
  url: string;
  event_types_json: string;
  secret: string;
  headers: Record<string, string>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};
