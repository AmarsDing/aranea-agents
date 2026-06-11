/** Minimal metadata shape for webhook origin resolution (decoupled from feature types). */
type WebhookOriginMeta = {
  public_webhook_origin?: string;
};

/** 去掉末尾斜杠 */
export function normalizePublicOrigin(value: string): string {
  return String(value || '')
    .trim()
    .replace(/\/+$/, '');
}

/**
 * Webhook 对外 URL 的 origin 优先级：
 * metadata.public_webhook_origin > VITE_PUBLIC_SITE_URL > window.location.origin
 */
export function resolvePublicWebhookOrigin(metadata?: Pick<WebhookOriginMeta, 'public_webhook_origin'>): string {
  const override = normalizePublicOrigin(metadata?.public_webhook_origin ?? '');
  if (override) return override;
  const env = normalizePublicOrigin(String(import.meta.env.VITE_PUBLIC_SITE_URL ?? ''));
  if (env) return env;
  if (typeof window !== 'undefined') return window.location.origin;
  return '';
}

export function isLocalhostOrigin(origin: string): boolean {
  return /^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])(:\d+)?$/i.test(normalizePublicOrigin(origin));
}

export function buildChannelWebhookURL(
  path: string,
  metadata?: Pick<WebhookOriginMeta, 'public_webhook_origin'>,
): string {
  const normalized = path.trim() ? (path.startsWith('/') ? path : `/${path}`) : '';
  if (!normalized) return '';
  const origin = resolvePublicWebhookOrigin(metadata);
  return origin ? `${origin}${normalized}` : normalized;
}
