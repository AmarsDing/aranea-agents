/**
 * Loopback-only backend config exposed by the Tauri embedded proxy
 * (`/__local/backend-config`). Absent outside the Tauri shell (dev server,
 * plain browser), where callers must treat the endpoint as unavailable.
 */

export type BackendConfig = {
  url: string | null;
  platform: 'android' | 'desktop';
  requiresSetup: boolean;
};

export const MOBILE_SETUP_PATH = '/mobile/server-setup';

let cached: BackendConfig | null | undefined;

/** Returns null when not running inside the Tauri embedded proxy. */
export async function fetchBackendConfig(force = false): Promise<BackendConfig | null> {
  if (!force && cached !== undefined) return cached;
  try {
    const resp = await fetch('/__local/backend-config', { cache: 'no-store' });
    if (!resp.ok) {
      cached = null;
      return null;
    }
    cached = (await resp.json()) as BackendConfig;
    return cached;
  } catch {
    cached = null;
    return null;
  }
}

export async function saveBackendConfig(url: string): Promise<{ ok: boolean; error?: string }> {
  const resp = await fetch('/__local/backend-config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
  });
  if (!resp.ok) {
    let error = `HTTP ${resp.status}`;
    try {
      const body = (await resp.json()) as { error?: string };
      if (body.error) error = body.error;
    } catch {
      // keep HTTP status fallback
    }
    return { ok: false, error };
  }
  await fetchBackendConfig(true);
  return { ok: true };
}

/** Probe the upstream through the proxy; any JSON reply means reachable. */
export async function probeBackend(): Promise<boolean> {
  try {
    const resp = await fetch('/healthz', { cache: 'no-store' });
    return resp.ok;
  } catch {
    return false;
  }
}

/** Loose client-side pre-check; the Rust side is the authoritative validator. */
export function isPlausibleServerUrl(raw: string): boolean {
  const trimmed = raw.trim();
  if (!trimmed) return false;
  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    return false;
  }
  if (!['http:', 'https:', 'ws:', 'wss:'].includes(parsed.protocol)) return false;
  if (!parsed.hostname) return false;
  if (parsed.pathname !== '' && parsed.pathname !== '/') return false;
  if (parsed.search || parsed.hash) return false;
  return true;
}
