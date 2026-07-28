import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearAuthToken, setAuthToken } from '../../services/authToken';

/**
 * P2 (mobile): buildWsUrl falls back to the persisted auth token when the
 * caller does not pass one explicitly, but only emits `?token=` for
 * cross-origin WS — same-origin deployments rely on the HttpOnly cookie and
 * must not leak the token into URLs.
 */

function mockRuntimeConfig(config: { backendUrl?: string; wsOrigin?: string }) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      new Response(JSON.stringify(config), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  );
}

async function loadFreshRuntime() {
  const mod = await import('../runtime');
  await mod.loadRuntimeConfig();
  return mod;
}

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});

describe('buildWsUrl token fallback', () => {
  it('appends stored token for cross-origin WS when no explicit token given', async () => {
    mockRuntimeConfig({ backendUrl: 'https://aranea.example.com', wsOrigin: 'https://aranea.example.com' });
    const { buildWsUrl, isWsSameOriginAsPage } = await loadFreshRuntime();
    expect(isWsSameOriginAsPage()).toBe(false);

    setAuthToken('jwt.stored.token');
    const url = buildWsUrl({ sessionId: 's1' });
    expect(url).toContain('token=jwt.stored.token');
    expect(url).toContain('session_id=s1');
  });

  it('explicit token param wins over stored token', async () => {
    mockRuntimeConfig({ backendUrl: 'https://aranea.example.com', wsOrigin: 'https://aranea.example.com' });
    const { buildWsUrl } = await loadFreshRuntime();

    setAuthToken('jwt.stored.token');
    const url = buildWsUrl({ sessionId: 's1', token: 'jwt.explicit' });
    expect(url).toContain('token=jwt.explicit');
    expect(url).not.toContain('jwt.stored.token');
  });

  it('does not append token for cross-origin WS when nothing stored', async () => {
    mockRuntimeConfig({ backendUrl: 'https://aranea.example.com', wsOrigin: 'https://aranea.example.com' });
    const { buildWsUrl } = await loadFreshRuntime();

    const url = buildWsUrl({ sessionId: 's1' });
    expect(url).not.toContain('token=');
  });

  it('does not append token for same-origin WS even when stored', async () => {
    // Empty runtime config → getWsOrigin() === '' → same-origin gate.
    mockRuntimeConfig({});
    const { buildWsUrl, isWsSameOriginAsPage } = await loadFreshRuntime();
    expect(isWsSameOriginAsPage()).toBe(true);

    setAuthToken('jwt.stored.token');
    const url = buildWsUrl({ sessionId: 's1' });
    expect(url).not.toContain('token=');
    expect(url).toContain('session_id=s1');
  });
});
