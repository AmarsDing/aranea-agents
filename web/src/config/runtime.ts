/** Global monitor WS session: receives all sessions' monitor/team/graph events (server limit: 3 conns). */
export const GLOBAL_WS_SESSION_ID = '*';

import { getAuthToken } from '../services/authToken';

type RuntimeConfig = {
  backendUrl?: string;
  wsOrigin?: string;
};

let runtimeConfig: RuntimeConfig = {};

export async function loadRuntimeConfig(): Promise<void> {
  // Standalone/file:// contexts: absolute path fetch fails —
  // detect and use relative path; fall back to local backend on error.
  const isFileProtocol = typeof window !== 'undefined' && window.location?.protocol === 'file:';
  const configPath = isFileProtocol ? './assets/config/runtime-config.json' : '/assets/config/runtime-config.json';
  try {
    const resp = await fetch(configPath, { cache: 'no-store' });
    if (resp.ok) {
      runtimeConfig = await resp.json();
      return;
    }
  } catch {
    // fetch failed (e.g. file:// without relative file access)
  }
  // Fallback for standalone: point to local backend
  if (isFileProtocol) {
    runtimeConfig = {
      backendUrl: 'http://127.0.0.1:8000',
      wsOrigin: 'http://127.0.0.1:8000',
    };
  } else {
    runtimeConfig = {};
  }
}

/**
 * 网关 **Origin**，与 Kratos Admin HTTP（`v1/...` 相对其根）一致。
 * 新能力请通过 **proto + `web/src/services`** 以 Kratos 方式接入。
 *
 * **开发环境**：`backendUrl` 留空则 `kratosApi` 走同源 `/v1`（`quasar.config` 代理到后端），登录 Cookie 与页面同站；
 * 开发时页面在 `http://localhost:9001`（Quasar），API 走同源 `/v1` 代理到 `:8000`。
 * 若写成 `http://127.0.0.1:8000` 而页面在 `http://localhost:9001`，浏览器视其为跨站，**默认 SameSite=Lax 的 Cookie 不会随 XHR 带上**，会出现大量 401。
 * **勿访问 `:9000`**：该端口为后端 gRPC，不是 Web UI。
 */
export function getBackendOrigin(): string {
  if (runtimeConfig.backendUrl && runtimeConfig.backendUrl.trim().length > 0) {
    return runtimeConfig.backendUrl.replace(/\/$/, '');
  }
  if (import.meta.env.DEV) {
    return '';
  }
  // Production build: same-origin (nginx / admin 反代) — admin HTTP 默认 :8000，勿写死 :8080。
  if (typeof window !== 'undefined' && window.location?.origin) {
    return window.location.origin;
  }
  return '';
}

/** @deprecated 使用 {@link getBackendOrigin} */
export const getAdminServiceBaseURL = getBackendOrigin;

/**
 * **`{getBackendOrigin()}/api/v1`**（或开发环境下 **`/api/v1`**）。前端业务代码已统一走 **`kratosApi`** + **`/v1/...`**；保留此 helper 供运维脚本、DeepLink、或极少数仍需直连旧前缀的工具使用。
 */
export function getBackendBaseURL(): string {
  const origin = getBackendOrigin();
  if (origin === '') {
    return '/api/v1';
  }
  return `${origin}/api/v1`;
}

export function getWsOrigin(): string {
  if (runtimeConfig.wsOrigin && runtimeConfig.wsOrigin.trim() !== '') {
    return runtimeConfig.wsOrigin.replace(/\/$/, '');
  }
  return getBackendOrigin();
}

/** True when WS uses the same origin as the SPA (Vite proxy); HttpOnly session cookies are sent automatically. */
export function isWsSameOriginAsPage(): boolean {
  return getWsOrigin() === '';
}

/**
 * True when the page is served from a local HTTP origin (127.0.0.1 / localhost / ::1).
 * In the Tauri desktop app, the Rust side starts an embedded loopback HTTP server
 * so the page origin is `http://127.0.0.1:PORT/` — same-site as the backend at
 * `http://127.0.0.1:8000`. This means SameSite=Lax session cookies are sent
 * with XHR/fetch/WS requests even though JS cannot read the HttpOnly cookie.
 */
export function isLocalHttpOrigin(): boolean {
  if (typeof window === 'undefined') return false;
  const { protocol, hostname } = window.location;
  return protocol === 'http:' && (hostname === '127.0.0.1' || hostname === 'localhost' || hostname === '[::1]');
}

export function buildWsUrl(params: {
  sessionId: string;
  lastEventId?: string;
  /** Only used for cross-origin WS when session cookie is not sent (legacy); omit for same-origin + HttpOnly cookie. */
  token?: string;
  logEnabled?: boolean;
  /** Lightweight health probe: system channel only; does not count toward global monitor conn limit. */
  probe?: boolean;
}): string {
  const origin = getWsOrigin();
  const protocol = origin.startsWith('https') ? 'wss' : 'ws';
  const wsOrigin = origin.replace(/^https?/, protocol);
  const q = new URLSearchParams({ session_id: params.sessionId });
  if (params.lastEventId) {
    q.set('last_event_id', params.lastEventId);
  }
  // P2 (mobile): explicit token wins; otherwise fall back to the persisted
  // login token. Only emitted cross-origin — same-origin uses the HttpOnly
  // cookie and must not leak the token into URLs.
  const token = params.token?.trim() || getAuthToken();
  if (token && !isWsSameOriginAsPage()) {
    q.set('token', token);
  }
  if (params.logEnabled) {
    q.set('log_enabled', '1');
  }
  if (params.probe) {
    q.set('probe', '1');
  }
  return `${wsOrigin}/v1/ws?${q.toString()}`;
}

export function buildHealthWsUrl(): string {
  return buildWsUrl({ sessionId: GLOBAL_WS_SESSION_ID, probe: true });
}

/**
 * 语音通道 `/v1/voice`（M74 设计 §2.1）：独立于 /v1/ws 事件总线。
 * 鉴权与 /v1/ws 一致（同源 HttpOnly Cookie；跨源回退 token query）。
 */
export function buildVoiceWsUrl(params: { sessionId: string; token?: string }): string {
  const origin = getWsOrigin();
  const protocol = origin.startsWith('https') ? 'wss' : 'ws';
  const wsOrigin = origin.replace(/^https?/, protocol);
  const q = new URLSearchParams({ session_id: params.sessionId });
  const token = params.token?.trim() || getAuthToken();
  if (token && !isWsSameOriginAsPage()) {
    q.set('token', token);
  }
  return `${wsOrigin}/v1/voice?${q.toString()}`;
}

/**
 * @deprecated Session cookie is HttpOnly; JS cannot read it on same-origin deployments.
 * Cross-origin integrations should use Bearer token or explicit `token` query on WS URL.
 */
export function readAccessTokenCookie(): string | undefined {
  if (typeof document === 'undefined') return undefined;
  const match = document.cookie.match(/(?:^|;\s*)access_token=([^;]*)/);
  return match ? decodeURIComponent(match[1]) : undefined;
}
