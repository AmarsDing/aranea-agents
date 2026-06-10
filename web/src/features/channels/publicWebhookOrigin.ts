// Re-export from domain layer for backward compatibility.
// New code should import from domain/channel directly.
export {
  normalizePublicOrigin,
  resolvePublicWebhookOrigin,
  isLocalhostOrigin,
  buildChannelWebhookURL,
} from '../../domain/channel/publicWebhookOrigin';
