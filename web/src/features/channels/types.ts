import type { PlatformResource } from "../platform/api";

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
  catalog_group?: string;
  catalog_source?: string;
  external_id?: string;
  last_error_code?: string;
  last_error_message?: string;
  connected_at?: string;
  schema_version?: number;
};

export type ChannelTestResult = {
  ok: boolean;
  status: string;
  message: string;
  details?: Record<string, unknown>;
};

export type ChannelRow = PlatformResource;
