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
