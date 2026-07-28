import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  fetchBackendConfig,
  isPlausibleServerUrl,
  probeBackend,
  saveBackendConfig,
} from '../backendConfig';

function mockFetch(impl: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) {
  vi.stubGlobal('fetch', vi.fn(impl));
}

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
  // Reset the module-level cache between tests.
  vi.resetModules();
});

describe('isPlausibleServerUrl', () => {
  it('accepts http/https origins with or without port and trailing slash', () => {
    expect(isPlausibleServerUrl('https://aranea.example.com')).toBe(true);
    expect(isPlausibleServerUrl('https://aranea.example.com/')).toBe(true);
    expect(isPlausibleServerUrl('http://192.168.1.10:8000')).toBe(true);
  });

  it('accepts ws/wss (normalized to http form by the Rust side)', () => {
    expect(isPlausibleServerUrl('wss://aranea.example.com')).toBe(true);
    expect(isPlausibleServerUrl('ws://192.168.1.10:8000')).toBe(true);
  });

  it('rejects empty, malformed, unsupported scheme, path, query, and fragment', () => {
    expect(isPlausibleServerUrl('')).toBe(false);
    expect(isPlausibleServerUrl('   ')).toBe(false);
    expect(isPlausibleServerUrl('not-a-url')).toBe(false);
    expect(isPlausibleServerUrl('ftp://example.com')).toBe(false);
    expect(isPlausibleServerUrl('https://example.com/api')).toBe(false);
    expect(isPlausibleServerUrl('https://example.com?x=1')).toBe(false);
    expect(isPlausibleServerUrl('https://example.com#frag')).toBe(false);
  });
});

describe('fetchBackendConfig', () => {
  it('returns null when the loopback endpoint is absent (plain browser/dev server)', async () => {
    mockFetch(() => Promise.reject(new TypeError('Failed to fetch')));
    // Re-import after stubbing so the module cache starts cold.
    const mod = await import('../backendConfig');
    expect(await mod.fetchBackendConfig(true)).toBeNull();
  });

  it('returns null on non-OK responses', async () => {
    mockFetch(() => Promise.resolve(new Response('not found', { status: 404 })));
    const mod = await import('../backendConfig');
    expect(await mod.fetchBackendConfig(true)).toBeNull();
  });

  it('parses the config payload from the embedded proxy', async () => {
    mockFetch(() =>
      Promise.resolve(
        jsonResponse(200, { url: 'https://aranea.example.com', platform: 'android', requiresSetup: false }),
      ),
    );
    const mod = await import('../backendConfig');
    const cfg = await mod.fetchBackendConfig(true);
    expect(cfg?.url).toBe('https://aranea.example.com');
    expect(cfg?.platform).toBe('android');
    expect(cfg?.requiresSetup).toBe(false);
  });

  it('serves subsequent calls from cache unless forced', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        jsonResponse(200, { url: null, platform: 'android', requiresSetup: true }),
      ),
    );
    vi.stubGlobal('fetch', fetchMock);
    const mod = await import('../backendConfig');
    await mod.fetchBackendConfig(true);
    await mod.fetchBackendConfig();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await mod.fetchBackendConfig(true);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe('saveBackendConfig', () => {
  it('returns ok on 200 and refreshes the cached config', async () => {
    const calls: Array<{ url: string; method?: string }> = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        calls.push({ url, method: init?.method });
        if (init?.method === 'PUT') return Promise.resolve(new Response(null, { status: 200 }));
        return Promise.resolve(
          jsonResponse(200, { url: 'https://aranea.example.com', platform: 'android', requiresSetup: false }),
        );
      }),
    );
    const mod = await import('../backendConfig');
    const result = await mod.saveBackendConfig('https://aranea.example.com');
    expect(result.ok).toBe(true);
    expect(calls.some((c) => c.method === 'PUT')).toBe(true);
  });

  it('surfaces the server-side validation message on failure', async () => {
    mockFetch(() =>
      Promise.resolve(jsonResponse(400, { error: 'unsupported scheme: ftp' })),
    );
    const mod = await import('../backendConfig');
    const result = await mod.saveBackendConfig('ftp://example.com');
    expect(result.ok).toBe(false);
    expect(result.error).toBe('unsupported scheme: ftp');
  });

  it('falls back to the HTTP status when the body is not JSON', async () => {
    mockFetch(() => Promise.resolve(new Response('bad request', { status: 400 })));
    const mod = await import('../backendConfig');
    const result = await mod.saveBackendConfig('x');
    expect(result.ok).toBe(false);
    expect(result.error).toBe('HTTP 400');
  });
});

describe('probeBackend', () => {
  it('returns true when /healthz answers OK through the proxy', async () => {
    mockFetch(() => Promise.resolve(new Response('{}', { status: 200 })));
    expect(await probeBackend()).toBe(true);
  });

  it('returns false on network errors and non-OK statuses', async () => {
    mockFetch(() => Promise.reject(new TypeError('Failed to fetch')));
    expect(await probeBackend()).toBe(false);
    mockFetch(() => Promise.resolve(new Response('down', { status: 502 })));
    expect(await probeBackend()).toBe(false);
  });
});
