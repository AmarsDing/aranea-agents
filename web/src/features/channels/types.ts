import type { PlatformResource } from "../platform/types";

export type ChannelTurnJobRow = {
  id: string;
  channel_id: string;
  session_id: string;
  peer_id: string;
  peer_key: string;
  idempotency_key: string;
  status: string;
  preview_message_id: string;
  content_preview: string;
  async_target_type: string;
  async_target_id: string;
  error_message: string;
  started_at: string;
  finished_at: string;
  created_at: string;
  updated_at: string;
};

export type ChannelCatalogItem = {
  type: string;
  label: string;
  description: string;
  group: string;
  receive_modes: string[];
  icon: string;
  bundled: boolean;
  supports_test: boolean;
  supports_webhook: boolean;
  config_schema: Record<string, unknown>;
  credential_schema: {
    required?: string[];
    [key: string]: unknown;
  };
  ui_hints: Record<string, unknown>;
  sort_order: number;
};

export type ChannelCredential = {
  id: string;
  channel_id: string;
  credential_key: string;
  status: string;
  metadata_json: string;
  configured: boolean;
  masked_preview?: string;
  created_at: string;
  updated_at: string;
};

export type ChannelCredentialInput = {
  credential_key: string;
  secret?: string;
  secret_ref?: string;
  status?: string;
  metadata_json?: string;
};

export type ChannelResourceInput = Partial<PlatformResource> & {
  key: string;
  name: string;
  credentials?: ChannelCredentialInput[];
};

export type ChannelConfig = {
  type?: string;
  variant?: string;
  receive_mode?: string;
  webhook?: Record<string, unknown>;
  routing?: Record<string, unknown>;
  config?: Record<string, unknown>;
  accounts?: unknown[];
};

export type ChannelMetadata = {
  icon_url?: string;
  icon_asset_id?: string;
  public_webhook_origin?: string;
  catalog_group?: string;
  catalog_source?: string;
  external_id?: string;
  last_error_code?: string;
  last_error_message?: string;
  connected_at?: string;
  runtime_connected?: boolean;
  runtime_connected_since?: string;
  runtime_last_disconnect?: string;
  schema_version?: number;
  /** Last applied long-task preset id (UI hint; config fields are authoritative). */
  long_task_preset?: string;
};

export type ChannelTestResult = {
  ok: boolean;
  status: string;
  message: string;
  details?: Record<string, unknown>;
};

export type ChannelRow = PlatformResource;
