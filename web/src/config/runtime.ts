/** Global monitor WS session: receives all sessions' monitor/team/graph events (server limit: 3 conns). */
export const GLOBAL_WS_SESSION_ID = "*";

type RuntimeConfig = {
  backendUrl?: string;
  wsOrigin?: string;
};

let runtimeConfig: RuntimeConfig = {};

export async function loadRuntimeConfig(): Promise<void> {
  try {
    const resp = await fetch("/assets/config/runtime-config.json", { cache: "no-store" });
    if (!resp.ok) return;
    runtimeConfig = await resp.json();
  } catch {
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
    return runtimeConfig.backendUrl.replace(/\/$/, "");
  }
  if (import.meta.env.DEV) {
    return "";
  }
  return "http://localhost:8080";
}

/** @deprecated 使用 {@link getBackendOrigin} */
export const getAdminServiceBaseURL = getBackendOrigin;

/**
 * **`{getBackendOrigin()}/api/v1`**（或开发环境下 **`/api/v1`**）。前端业务代码已统一走 **`kratosApi`** + **`/v1/...`**；保留此 helper 供运维脚本、DeepLink、或极少数仍需直连旧前缀的工具使用。
 */
export function getBackendBaseURL(): string {
  const origin = getBackendOrigin();
  if (origin === "") {
    return "/api/v1";
  }
  return `${origin}/api/v1`;
}

export function getWsOrigin(): string {
  if (runtimeConfig.wsOrigin && runtimeConfig.wsOrigin.trim() !== "") {
    return runtimeConfig.wsOrigin.replace(/\/$/, "");
  }
  return getBackendOrigin();
}

export function buildWsUrl(params: {
  sessionId: string;
  lastEventId?: string;
  token?: string;
  logEnabled?: boolean;
}): string {
  const origin = getWsOrigin();
  const protocol = origin.startsWith("https") ? "wss" : "ws";
  const wsOrigin = origin.replace(/^https?/, protocol);
  const q = new URLSearchParams({ session_id: params.sessionId });
  if (params.lastEventId) {
    q.set("last_event_id", params.lastEventId);
  }
  if (params.token) {
    q.set("token", params.token);
  }
  if (params.logEnabled) {
    q.set("log_enabled", "1");
  }
  return `${wsOrigin}/v1/ws?${q.toString()}`;
}

export function buildHealthWsUrl(): string {
  const origin = getWsOrigin();
  const protocol = origin.startsWith("https") ? "wss" : "ws";
  const wsOrigin = origin.replace(/^https?/, protocol);
  const q = new URLSearchParams({ session_id: GLOBAL_WS_SESSION_ID });
  const token = readAccessTokenCookie();
  if (token) {
    q.set("token", token);
  }
  return `${wsOrigin}/v1/ws?${q.toString()}`;
}

export function readAccessTokenCookie(): string | undefined {
  if (typeof document === "undefined") return undefined;
  const match = document.cookie.match(/(?:^|;\s*)access_token=([^;]*)/);
  return match ? decodeURIComponent(match[1]) : undefined;
}
