/**
 * Shared A2A auth utilities for building auth_config_json payloads.
 * Used by A2ARemoteAgentPanel and useAgentA2AProxyTab.
 */

export interface MtlsFields {
  cert_file: string;
  key_file: string;
  ca_file: string;
}

/**
 * Builds the auth_config_json string for A2A auth types.
 * Returns undefined when no auth is needed.
 */
export function buildA2AAuthJSON(
  authType: string,
  secret: string,
  mtls: MtlsFields,
): string | undefined {
  const type = authType?.trim();
  if (!type || type === 'none') return undefined;
  if (type === 'mtls') {
    if (!mtls.cert_file.trim() || !mtls.key_file.trim()) return undefined;
    return JSON.stringify({
      cert_file: mtls.cert_file.trim(),
      key_file: mtls.key_file.trim(),
      ca_file: mtls.ca_file.trim(),
    });
  }
  const s = secret.trim();
  if (!s) return undefined;
  if (type === 'bearer') return JSON.stringify({ token: s });
  if (type === 'api_key') return JSON.stringify({ api_key: s });
  return undefined;
}

/** Standard auth type options for q-select. */
export const A2A_AUTH_TYPE_OPTIONS = [
  { label: '无', value: 'none' },
  { label: 'API Key', value: 'api_key' },
  { label: 'Bearer', value: 'bearer' },
  { label: 'mTLS', value: 'mtls' },
];
